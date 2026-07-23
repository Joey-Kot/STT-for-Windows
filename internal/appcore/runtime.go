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

const (
	shutdownGracePeriod  = 250 * time.Millisecond
	shutdownPollInterval = 5 * time.Millisecond
)

// Event describes a runtime state update.
type Event struct {
	State   State  `json:"state"`
	Message string `json:"message"`
	Error   string `json:"error,omitempty"`
}

// Runtime owns recorder, uploader, hotkeys, and shared state transitions.
// The application is designed for one process; actionMu intentionally provides
// only in-process serialization rather than cross-process coordination.
type Runtime struct {
	mu              sync.Mutex
	actionMu        sync.Mutex
	cfg             config.Config
	tempDir         string
	recorder        *record.Recorder
	asrClient       *asr.Client
	stopHotkeys     func()
	onEvent         func(Event)
	state           State
	lastMessage     string
	lastError       string
	lifecycleCtx    context.Context
	lifecycleCancel context.CancelFunc
	stopOnce        sync.Once

	nextRecordingSession   uint64
	activeRecordingSession uint64
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
	lifecycleCtx, lifecycleCancel := context.WithCancel(context.Background())

	r := &Runtime{
		cfg:             cfg,
		tempDir:         tempDir,
		recorder:        record.New(cfg, tempDir),
		asrClient:       asrClient,
		state:           StateIdle,
		lifecycleCtx:    lifecycleCtx,
		lifecycleCancel: lifecycleCancel,
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
	if r.isStopped() {
		return fmt.Errorf("runtime stopped")
	}

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
	tempDir := config.TempDir(&cfg)
	recorder := record.New(cfg, tempDir)

	r.mu.Lock()
	stopHotkeys := r.stopHotkeys
	r.stopHotkeys = nil
	r.mu.Unlock()
	if stopHotkeys != nil {
		stopHotkeys()
	}
	if r.isStopped() {
		return fmt.Errorf("runtime stopped")
	}

	r.mu.Lock()
	r.cfg = cfg
	r.tempDir = tempDir
	r.recorder = recorder
	r.asrClient = asrClient
	r.activeRecordingSession = 0
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
	if r.isStopped() {
		return fmt.Errorf("runtime stopped")
	}
	r.mu.Lock()
	if r.stopHotkeys != nil {
		r.mu.Unlock()
		return nil
	}
	cfg := r.cfg
	r.mu.Unlock()

	reg, err := hotkey.RegisterWithStop(cfg.StartKey, cfg.PauseKey, cfg.CancelKey, cfg.HotKeyHook, func(id int) {
		if !r.tryHandleAction(id) && cfg.HOTKEY_DEBUG {
			fmt.Printf("[hotkey-debug] dropped action id=%d while another action is in progress\n", id)
		}
	}, cfg.HOTKEY_DEBUG)
	if err != nil {
		return err
	}

	r.mu.Lock()
	if r.lifecycleCtx != nil && r.lifecycleCtx.Err() != nil {
		r.mu.Unlock()
		reg.Stop()
		return fmt.Errorf("runtime stopped")
	}
	r.stopHotkeys = reg.Stop
	r.mu.Unlock()
	return nil
}

// Stop cancels runtime work and releases resources without waiting
// indefinitely for an in-flight conversion, request, or paste operation.
func (r *Runtime) Stop() {
	r.stopOnce.Do(func() {
		if r.lifecycleCancel != nil {
			r.lifecycleCancel()
		}

		r.mu.Lock()
		stopHotkeys := r.stopHotkeys
		r.stopHotkeys = nil
		recorder := r.recorder
		r.mu.Unlock()

		if recorder != nil {
			recorder.RequestCancel()
		}
		if stopHotkeys != nil {
			stopHotkeys()
		}
		r.waitForCurrentAction()
	})
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

// TryToggleRecording asynchronously starts or stops recording when no other
// runtime action is in progress.
func (r *Runtime) TryToggleRecording() bool {
	return r.tryHandleAction(1)
}

// TryTogglePause asynchronously pauses or resumes recording when no other
// runtime action is in progress.
func (r *Runtime) TryTogglePause() bool {
	return r.tryHandleAction(2)
}

// TryCancel asynchronously cancels recording when no other runtime action is
// in progress.
func (r *Runtime) TryCancel() bool {
	return r.tryHandleAction(3)
}

// HandleAction maps hotkey IDs to runtime operations.
func (r *Runtime) HandleAction(id int) {
	if r.isStopped() {
		return
	}
	r.actionMu.Lock()
	defer r.actionMu.Unlock()
	if r.isStopped() {
		return
	}

	r.handleActionLocked(id)
}

// tryHandleAction admits an asynchronous action only when no runtime action is
// in progress. It is used by both hotkey and GUI inputs so busy actions are
// discarded at capture time instead of queued for a later runtime state.
// Acquiring the lock before dispatch prevents inputs captured during a long
// transcription from waiting and executing against a later runtime state.
func (r *Runtime) tryHandleAction(id int) bool {
	if r.isStopped() {
		return false
	}
	if !r.actionMu.TryLock() {
		return false
	}
	if r.isStopped() {
		r.actionMu.Unlock()
		return false
	}

	go func() {
		defer r.actionMu.Unlock()
		r.handleActionLocked(id)
	}()
	return true
}

func (r *Runtime) lifecycleContext() context.Context {
	if r.lifecycleCtx != nil {
		return r.lifecycleCtx
	}
	return context.Background()
}

func (r *Runtime) isStopped() bool {
	ctx := r.lifecycleCtx
	if ctx == nil {
		return false
	}
	return ctx.Err() != nil
}

func (r *Runtime) waitForCurrentAction() {
	if r.actionMu.TryLock() {
		r.actionMu.Unlock()
		return
	}

	timer := time.NewTimer(shutdownGracePeriod)
	ticker := time.NewTicker(shutdownPollInterval)
	defer timer.Stop()
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if r.actionMu.TryLock() {
				r.actionMu.Unlock()
				return
			}
		case <-timer.C:
			return
		}
	}
}

func (r *Runtime) handleActionLocked(id int) {
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
	ctx := r.lifecycleContext()
	if ctx.Err() != nil {
		return
	}

	r.mu.Lock()
	state := r.state
	recorder := r.recorder
	cfg := r.cfg
	session := r.activeRecordingSession
	r.mu.Unlock()

	if state == StateIdle || state == StateError {
		session = r.beginRecordingSession(recorder)
		if err := recorder.Start(ctx); err != nil {
			r.clearRecordingSession(recorder, session)
			if ctx.Err() != nil {
				return
			}
			r.setState(StateError, "Recording start failed", err)
			return
		}
		if ctx.Err() != nil {
			recorder.RequestCancel()
			r.clearRecordingSession(recorder, session)
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
	if ctx.Err() != nil {
		r.clearRecordingSession(recorder, session)
		discardRecordingResult(res)
		return
	}
	if err != nil {
		if res.Err != nil {
			r.clearRecordingSession(recorder, session)
		}
		r.setState(StateError, "Recording stop failed", err)
		return
	}
	r.clearRecordingSession(recorder, session)
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
	r.transcribeResult(ctx, res)
}

func (r *Runtime) beginRecordingSession(recorder *record.Recorder) uint64 {
	r.mu.Lock()
	r.nextRecordingSession++
	session := r.nextRecordingSession
	r.activeRecordingSession = session
	r.mu.Unlock()

	recorder.SetErrorHandler(func(err error) {
		r.handleRecorderError(recorder, session, err)
	})
	return session
}

func (r *Runtime) clearRecordingSession(recorder *record.Recorder, session uint64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.recorder == recorder && r.activeRecordingSession == session {
		r.activeRecordingSession = 0
	}
}

func (r *Runtime) handleRecorderError(recorder *record.Recorder, session uint64, err error) {
	if err == nil || r.isStopped() {
		return
	}

	// finish writes the buffered recorder result before invoking this callback,
	// so waiting for actionMu cannot deadlock a concurrent Stop or Cancel. The
	// lock also keeps the error event ordered after an in-flight start action.
	r.actionMu.Lock()
	defer r.actionMu.Unlock()
	if r.isStopped() {
		return
	}

	r.mu.Lock()
	if r.recorder != recorder || r.activeRecordingSession != session {
		r.mu.Unlock()
		return
	}
	// A concurrent stop/cancel can report "recorder not running" after the
	// recorder has already failed. Preserve the original background error by
	// allowing it to replace that generic Error state.
	if r.state != StateRecording && r.state != StatePaused && r.state != StateError {
		r.mu.Unlock()
		return
	}
	r.activeRecordingSession = 0
	r.mu.Unlock()

	r.setState(StateError, "Recording failed", err)
}

func (r *Runtime) togglePauseLocked() {
	if r.isStopped() {
		return
	}
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
	ctx := r.lifecycleContext()
	if ctx.Err() != nil {
		return record.Result{}, ctx.Err()
	}

	r.mu.Lock()
	state := r.state
	recorder := r.recorder
	cfg := r.cfg
	session := r.activeRecordingSession
	r.mu.Unlock()

	if state != StateRecording && state != StatePaused {
		if cfg.HOTKEY_DEBUG {
			fmt.Println("[hotkey] not recording; nothing to cancel")
		}
		return record.Result{}, nil
	}
	res, err := recorder.Cancel()
	if ctx.Err() != nil {
		r.clearRecordingSession(recorder, session)
		discardRecordingResult(res)
		return res, ctx.Err()
	}
	if err != nil {
		if res.Err != nil {
			r.clearRecordingSession(recorder, session)
		}
		r.setState(StateError, "Cancel failed", err)
		return res, err
	}
	r.clearRecordingSession(recorder, session)
	r.setState(StateIdle, "Recording canceled", nil)
	return res, nil
}

func discardRecordingResult(res record.Result) {
	if res.WavPath != "" {
		_ = os.Remove(res.WavPath)
	}
}

func (r *Runtime) transcribeResult(ctx context.Context, res record.Result) {
	r.mu.Lock()
	cfg := r.cfg
	asrClient := r.asrClient
	r.mu.Unlock()
	if ctx.Err() != nil {
		discardRecordingResult(res)
		return
	}

	outPath := strings.TrimSuffix(res.WavPath, filepath.Ext(res.WavPath)) + "." + config.ContainerExt(cfg.CONTAINER)
	if err := ffmpeg.ConvertContext(ctx, cfg, res.WavPath, outPath, cfg.SAMPLING_RATE); err != nil {
		_ = os.Remove(res.WavPath)
		_ = os.Remove(outPath)
		if ctx.Err() != nil {
			return
		}
		r.setState(StateError, "FFmpeg conversion failed", err)
		return
	}
	if ctx.Err() != nil {
		handleCache(cfg, res.WavPath, outPath, false, nil)
		return
	}

	text, raw, err := asrClient.Transcribe(ctx, outPath)
	uploadOk := err == nil
	if ctx.Err() != nil {
		handleCache(cfg, res.WavPath, outPath, uploadOk, raw)
		return
	}
	if err != nil {
		if cfg.Notification {
			notify.Notify("STT", "Upload failed")
		}
		if cfg.RequestFailedNotification {
			var re *asr.RetryExhaustedError
			if errors.As(err, &re) {
				if pasteErr := clipboard.PasteTextContext(ctx, "[request failed]"); pasteErr != nil {
					if ctx.Err() != nil {
						handleCache(cfg, res.WavPath, outPath, uploadOk, raw)
						return
					}
					fmt.Printf("[paste] failed: %v\n", pasteErr)
				} else if cfg.Notification {
					notify.Notify("STT", "Request failed")
				}
			}
		}
		if ctx.Err() != nil {
			handleCache(cfg, res.WavPath, outPath, uploadOk, raw)
			return
		}
		handleCache(cfg, res.WavPath, outPath, uploadOk, raw)
		r.setState(StateError, "Upload failed", err)
		return
	}

	if ctx.Err() != nil {
		handleCache(cfg, res.WavPath, outPath, uploadOk, raw)
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

	if err := clipboard.PasteTextContext(ctx, text); err != nil {
		if ctx.Err() != nil {
			handleCache(cfg, res.WavPath, outPath, uploadOk, raw)
			return
		}
		message := "Paste failed"
		var restoreErr *clipboard.RestoreError
		if errors.As(err, &restoreErr) && restoreErr.PasteSent {
			message = "Paste sent; clipboard restore failed"
		}
		if cfg.Notification {
			notify.Notify("STT", message)
		}
		handleCache(cfg, res.WavPath, outPath, uploadOk, raw)
		r.setState(StateError, message, err)
		return
	}
	if ctx.Err() != nil {
		handleCache(cfg, res.WavPath, outPath, uploadOk, raw)
		return
	}

	// Success means the clipboard transaction and Ctrl+V dispatch completed.
	// Whether the current foreground control accepts the paste is intentionally
	// outside the runtime's responsibility and is not verified.
	if cfg.Notification {
		notify.Notify("STT", "Paste success")
	}
	handleCache(cfg, res.WavPath, outPath, uploadOk, raw)
	r.setState(StateIdle, "Transcription pasted", nil)
}

func (r *Runtime) setState(state State, message string, err error) {
	var event Event
	r.mu.Lock()
	if r.lifecycleCtx != nil && r.lifecycleCtx.Err() != nil {
		r.mu.Unlock()
		return
	}
	r.state = state
	r.lastMessage = message
	r.lastError = ""
	if err != nil {
		r.lastError = err.Error()
	}
	event = Event{State: r.state, Message: r.lastMessage, Error: r.lastError}
	handler := r.onEvent
	r.mu.Unlock()

	if handler != nil && !r.isStopped() {
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
