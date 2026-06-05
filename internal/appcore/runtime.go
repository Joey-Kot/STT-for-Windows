// Copyright (C) 2026 Joey Kot <joey.kot.x@gmail.com>
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed WITHOUT ANY WARRANTY; without even the
// implied warranty of MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.
// See <https://www.gnu.org/licenses/> for more details.

package appcore

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"golang.org/x/net/http2"

	"stt/internal/asr"
	"stt/internal/audio/ffmpeg"
	"stt/internal/clipboard"
	"stt/internal/config"
	"stt/internal/hotkey"
	"stt/internal/notify"
	"stt/internal/record"
)

// State is the GUI/CLI-visible runtime state.
type State string

const (
	StateIdle      State = "Idle"
	StateRecording State = "Recording"
	StatePaused    State = "Paused"
	StateUploading State = "Uploading"
	StateError     State = "Error"
)

// Event describes a runtime state update.
type Event struct {
	State   State  `json:"state"`
	Message string `json:"message"`
	Error   string `json:"error,omitempty"`
}

// Runtime owns recorder, uploader, hotkeys, and shared state transitions.
type Runtime struct {
	mu          sync.Mutex
	actionMu    sync.Mutex
	cfg         config.Config
	tempDir     string
	recorder    *record.Recorder
	asrClient   *asr.Client
	stopHotkeys func()
	onEvent     func(Event)
	state       State
	lastMessage string
	lastError   string
}

// NewRuntime creates a reusable record-mode runtime.
func NewRuntime(cfg config.Config) (*Runtime, error) {
	if err := config.Validate(&cfg); err != nil {
		return nil, err
	}
	config.InitCacheDir(&cfg)
	tempDir := config.TempDir(&cfg)
	cleanupOldTempFiles(tempDir)

	asrClient, err := asr.New(cfg, newHTTPClient(cfg))
	if err != nil {
		return nil, err
	}

	r := &Runtime{
		cfg:       cfg,
		tempDir:   tempDir,
		recorder:  record.New(cfg, tempDir),
		asrClient: asrClient,
		state:     StateIdle,
	}
	return r, nil
}

// SetEventHandler registers a callback for state updates.
func (r *Runtime) SetEventHandler(handler func(Event)) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.onEvent = handler
}

// Snapshot returns the current runtime state.
func (r *Runtime) Snapshot() Event {
	r.mu.Lock()
	defer r.mu.Unlock()
	return Event{State: r.state, Message: r.lastMessage, Error: r.lastError}
}

// Config returns the active config.
func (r *Runtime) Config() config.Config {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.cfg
}

// CanReload reports whether config can be reloaded immediately.
func (r *Runtime) CanReload() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.state == StateIdle || r.state == StateError
}

// Reload replaces the runtime configuration while idle.
func (r *Runtime) Reload(cfg config.Config) error {
	r.actionMu.Lock()
	defer r.actionMu.Unlock()

	if err := config.Validate(&cfg); err != nil {
		return err
	}

	r.mu.Lock()
	state := r.state
	r.mu.Unlock()
	if state != StateIdle && state != StateError {
		return fmt.Errorf("cannot save settings while %s", state)
	}

	config.InitCacheDir(&cfg)
	asrClient, err := asr.New(cfg, newHTTPClient(cfg))
	if err != nil {
		return err
	}

	if r.stopHotkeys != nil {
		r.stopHotkeys()
		r.stopHotkeys = nil
	}

	r.mu.Lock()
	r.cfg = cfg
	r.tempDir = config.TempDir(&cfg)
	r.recorder = record.New(cfg, r.tempDir)
	r.asrClient = asrClient
	r.mu.Unlock()

	if err := r.StartHotkeys(); err != nil {
		r.setState(StateError, "Failed to register hotkeys", err)
		return err
	}
	r.setState(StateIdle, "Settings saved", nil)
	return nil
}

// StartHotkeys registers global hotkeys and wires them to runtime actions.
func (r *Runtime) StartHotkeys() error {
	r.mu.Lock()
	if r.stopHotkeys != nil {
		r.mu.Unlock()
		return nil
	}
	cfg := r.cfg
	r.mu.Unlock()

	reg, err := hotkey.RegisterWithStop(cfg.StartKey, cfg.PauseKey, cfg.CancelKey, cfg.HotKeyHook, func(id int) {
		r.HandleAction(id)
	}, cfg.HOTKEY_DEBUG)
	if err != nil {
		return err
	}

	r.mu.Lock()
	r.stopHotkeys = reg.Stop
	r.mu.Unlock()
	return nil
}

// Stop releases hotkeys and recording resources.
func (r *Runtime) Stop() {
	r.actionMu.Lock()
	defer r.actionMu.Unlock()

	r.mu.Lock()
	stopHotkeys := r.stopHotkeys
	r.stopHotkeys = nil
	state := r.state
	r.mu.Unlock()

	if stopHotkeys != nil {
		stopHotkeys()
	}
	if state == StateRecording || state == StatePaused {
		_, _ = r.cancelRecording()
	}
}

// ToggleRecording starts recording when idle, otherwise stops and uploads.
func (r *Runtime) ToggleRecording() Event {
	r.HandleAction(1)
	return r.Snapshot()
}

// TogglePause pauses or resumes an active recording.
func (r *Runtime) TogglePause() Event {
	r.HandleAction(2)
	return r.Snapshot()
}

// Cancel cancels an active recording.
func (r *Runtime) Cancel() Event {
	r.HandleAction(3)
	return r.Snapshot()
}

// HandleAction maps hotkey IDs to runtime operations.
func (r *Runtime) HandleAction(id int) {
	r.actionMu.Lock()
	defer r.actionMu.Unlock()

	switch id {
	case 1:
		r.toggleRecordingLocked()
	case 2:
		r.togglePauseLocked()
	case 3:
		_, _ = r.cancelRecording()
	}
}

func (r *Runtime) toggleRecordingLocked() {
	r.mu.Lock()
	state := r.state
	recorder := r.recorder
	cfg := r.cfg
	r.mu.Unlock()

	if state == StateIdle || state == StateError {
		if err := recorder.Start(context.Background()); err != nil {
			r.setState(StateError, "Recording start failed", err)
			return
		}
		if cfg.Notification {
			notify.Notify("STT", "Recording started")
		}
		r.setState(StateRecording, "Recording started", nil)
		return
	}

	if state != StateRecording && state != StatePaused {
		return
	}

	res, err := recorder.Stop()
	if err != nil {
		r.setState(StateError, "Recording stop failed", err)
		return
	}
	if res.Canceled {
		r.setState(StateIdle, "Recording canceled", nil)
		return
	}
	if res.Err != nil {
		r.setState(StateError, "Recording failed", res.Err)
		return
	}

	if cfg.Notification {
		notify.Notify("STT", "Recording finished")
	}
	r.setState(StateUploading, "Uploading ASR request", nil)
	r.transcribeResult(res)
}

func (r *Runtime) togglePauseLocked() {
	r.mu.Lock()
	recorder := r.recorder
	cfg := r.cfg
	r.mu.Unlock()

	if err := recorder.TogglePause(); err != nil {
		if cfg.HOTKEY_DEBUG {
			fmt.Println("[hotkey] not recording; cannot pause/resume")
		}
		return
	}

	if recorder.State() == record.StatePaused {
		r.setState(StatePaused, "Recording paused", nil)
	} else {
		r.setState(StateRecording, "Recording resumed", nil)
	}
}

func (r *Runtime) cancelRecording() (record.Result, error) {
	r.mu.Lock()
	state := r.state
	recorder := r.recorder
	cfg := r.cfg
	r.mu.Unlock()

	if state != StateRecording && state != StatePaused {
		if cfg.HOTKEY_DEBUG {
			fmt.Println("[hotkey] not recording; nothing to cancel")
		}
		return record.Result{}, nil
	}
	res, err := recorder.Cancel()
	if err != nil {
		r.setState(StateError, "Cancel failed", err)
		return res, err
	}
	r.setState(StateIdle, "Recording canceled", nil)
	return res, nil
}

func (r *Runtime) transcribeResult(res record.Result) {
	r.mu.Lock()
	cfg := r.cfg
	asrClient := r.asrClient
	r.mu.Unlock()

	outPath := strings.TrimSuffix(res.WavPath, filepath.Ext(res.WavPath)) + "." + config.ContainerExt(cfg.CONTAINER)
	if err := ffmpeg.Convert(cfg, res.WavPath, outPath, cfg.SAMPLING_RATE); err != nil {
		_ = os.Remove(res.WavPath)
		_ = os.Remove(outPath)
		r.setState(StateError, "FFmpeg conversion failed", err)
		return
	}

	text, raw, err := asrClient.Transcribe(context.Background(), outPath)
	uploadOk := err == nil
	if err != nil {
		if cfg.Notification {
			notify.Notify("STT", "Upload failed")
		}
		if cfg.RequestFailedNotification {
			var re *asr.RetryExhaustedError
			if errors.As(err, &re) {
				if pasteErr := clipboard.PasteText("[request failed]"); pasteErr != nil {
					fmt.Printf("[paste] failed: %v\n", pasteErr)
				} else if cfg.Notification {
					notify.Notify("STT", "Request failed")
				}
			}
		}
		handleCache(cfg, res.WavPath, outPath, uploadOk, raw)
		r.setState(StateError, "Upload failed", err)
		return
	}

	if text == "" {
		if cfg.Notification {
			notify.Notify("STT", "Empty result from ASR")
		}
		handleCache(cfg, res.WavPath, outPath, uploadOk, raw)
		r.setState(StateIdle, "Empty result from ASR", nil)
		return
	}

	if err := clipboard.PasteText(text); err != nil {
		if cfg.Notification {
			notify.Notify("STT", "Paste failed")
		}
		handleCache(cfg, res.WavPath, outPath, uploadOk, raw)
		r.setState(StateError, "Paste failed", err)
		return
	}

	if cfg.Notification {
		notify.Notify("STT", "Paste success")
	}
	handleCache(cfg, res.WavPath, outPath, uploadOk, raw)
	r.setState(StateIdle, "Transcription pasted", nil)
}

func (r *Runtime) setState(state State, message string, err error) {
	var event Event
	r.mu.Lock()
	r.state = state
	r.lastMessage = message
	r.lastError = ""
	if err != nil {
		r.lastError = err.Error()
	}
	event = Event{State: r.state, Message: r.lastMessage, Error: r.lastError}
	handler := r.onEvent
	r.mu.Unlock()

	if handler != nil {
		handler(event)
	}
}

// RunRecordMode starts hotkeys and blocks forever for CLI compatibility.
func RunRecordMode(cfg config.Config) error {
	r, err := NewRuntime(cfg)
	if err != nil {
		return err
	}
	r.SetEventHandler(func(event Event) {
		if event.Error != "" {
			fmt.Printf("[state] %s: %s (%s)\n", event.State, event.Message, event.Error)
			return
		}
		fmt.Printf("[state] %s: %s\n", event.State, event.Message)
	})
	if err := r.StartHotkeys(); err != nil {
		return err
	}
	fmt.Println("[main] ready. Use hotkeys to start/stop/pause/cancel.")
	for {
		time.Sleep(time.Hour)
	}
}

// RunFileMode uploads an existing file and writes the result to a .txt file.
func RunFileMode(cfg config.Config, inputPath string, outputPath string) error {
	if err := config.Validate(&cfg); err != nil {
		return err
	}
	config.InitCacheDir(&cfg)
	tempDir := config.TempDir(&cfg)
	cleanupOldTempFiles(tempDir)

	if _, err := os.Stat(inputPath); err != nil {
		return fmt.Errorf("file '%s' stat failed: %w", inputPath, err)
	}

	asrClient, err := asr.New(cfg, newHTTPClient(cfg))
	if err != nil {
		return err
	}

	tempOut := tempOutputPath(tempDir, config.ContainerExt(cfg.CONTAINER))
	if err := ffmpeg.Convert(cfg, inputPath, tempOut, cfg.SAMPLING_RATE); err != nil {
		_ = os.Remove(tempOut)
		return err
	}

	text, raw, err := asrClient.Transcribe(context.Background(), tempOut)
	uploadOk := err == nil
	if err != nil {
		if cfg.Notification {
			notify.Notify("STT", "Upload failed")
		}
		handleCache(cfg, "", tempOut, uploadOk, raw)
		return err
	}

	outPath := outputPath
	if outPath == "" {
		base := strings.TrimSuffix(filepath.Base(inputPath), filepath.Ext(inputPath))
		outPath = filepath.Join(".", base+".txt")
	}

	if err := os.WriteFile(outPath, []byte(text), 0644); err != nil {
		handleCache(cfg, "", tempOut, uploadOk, raw)
		return err
	}

	handleCache(cfg, "", tempOut, uploadOk, raw)
	return nil
}

func newHTTPClient(cfg config.Config) *http.Client {
	tr := &http.Transport{
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}
	if !cfg.VerifySSL {
		tr.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	}
	if cfg.EnableHTTP2 {
		_ = http2.ConfigureTransport(tr)
	}
	return &http.Client{
		Transport: tr,
		Timeout:   time.Duration(cfg.RequestTimeout) * time.Second,
	}
}

func cleanupOldTempFiles(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		fmt.Printf("[cleanup] read dir '%s' failed: %v\n", dir, err)
		return
	}
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, "RecordTemp_") {
			path := filepath.Join(dir, name)
			if err := os.Remove(path); err != nil {
				fmt.Printf("[cleanup] failed remove %s: %v\n", path, err)
			} else {
				fmt.Printf("[cleanup] removed %s\n", path)
			}
		}
	}
}

func handleCache(cfg config.Config, wavPath string, outPath string, uploadOk bool, resBody []byte) {
	if cfg.KeepCache && cfg.CacheDir != "" {
		timestamp := time.Now().Format("2006-01-02-15.04.05")
		base := fmt.Sprintf("audio-%s", timestamp)

		if wavPath != "" {
			wavExt := filepath.Ext(wavPath)
			newWav := filepath.Join(cfg.CacheDir, base+wavExt)
			if err := os.Rename(wavPath, newWav); err != nil {
				fmt.Printf("[cache] failed to rename wav to %s: %v\n", newWav, err)
				_ = os.Remove(wavPath)
			}
		}

		if outPath != "" {
			outExt := filepath.Ext(outPath)
			newOut := filepath.Join(cfg.CacheDir, base+outExt)
			if err := os.Rename(outPath, newOut); err != nil {
				fmt.Printf("[cache] failed to rename output to %s: %v\n", newOut, err)
				_ = os.Remove(outPath)
			}
		}

		if uploadOk && len(resBody) > 0 {
			jsonPath := filepath.Join(cfg.CacheDir, base+".json")
			if err := os.WriteFile(jsonPath, resBody, 0644); err != nil {
				fmt.Printf("[cache] failed to write json to %s: %v\n", jsonPath, err)
			}
		}
	} else {
		if wavPath != "" {
			_ = os.Remove(wavPath)
		}
		if outPath != "" {
			_ = os.Remove(outPath)
		}
	}
}

func tempOutputPath(dir, ext string) string {
	id := strings.ReplaceAll(uuid.New().String(), "-", "")[:16]
	base := fmt.Sprintf("RecordTemp_%s.%s", id, ext)
	if dir == "" {
		cwd, _ := os.Getwd()
		dir = cwd
	}
	return filepath.Join(dir, base)
}
