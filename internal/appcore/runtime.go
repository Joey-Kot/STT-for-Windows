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

// State is the shared application state exposed to CLI/GUI controllers.
type State string

const (
	StateIdle      State = "Idle"
	StateRecording State = "Recording"
	StatePaused    State = "Paused"
	StateUploading State = "Uploading"
	StateError     State = "Error"
)

// Snapshot describes the current runtime state for presentation layers.
type Snapshot struct {
	State   State  `json:"state"`
	Message string `json:"message"`
}

// Runtime coordinates recording, pause/cancel, upload, paste, hotkeys, and reloadable config.
type Runtime struct {
	mu       sync.Mutex
	cfg      config.Config
	recorder *record.Recorder
	asr      *asr.Client
	state    State
	message  string
	onChange func(Snapshot)
}

// NewRuntime creates a runtime from validated config.
func NewRuntime(cfg config.Config) (*Runtime, error) {
	config.InitCacheDir(&cfg)
	r := &Runtime{cfg: cfg, state: StateIdle, message: "Ready"}
	if err := r.rebuildLocked(); err != nil {
		return nil, err
	}
	cleanupOldTempFiles(config.TempDir(&cfg))
	return r, nil
}

// SetOnChange registers a callback invoked after state changes.
func (r *Runtime) SetOnChange(fn func(Snapshot)) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.onChange = fn
}

// Snapshot returns current state.
func (r *Runtime) Snapshot() Snapshot {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.snapshotLocked()
}

// RegisterHotkeys wires configured global hotkeys into runtime actions.
func (r *Runtime) RegisterHotkeys() error {
	r.mu.Lock()
	cfg := r.cfg
	r.mu.Unlock()
	return hotkey.Register(cfg.StartKey, cfg.PauseKey, cfg.CancelKey, cfg.HotKeyHook, func(id int) {
		switch id {
		case 1:
			_ = r.ToggleRecording(context.Background())
		case 2:
			_ = r.TogglePause()
		case 3:
			_ = r.Cancel()
		}
	}, cfg.HOTKEY_DEBUG)
}

// ToggleRecording starts recording from Idle, or stops and uploads from Recording/Paused.
func (r *Runtime) ToggleRecording(ctx context.Context) error {
	r.mu.Lock()
	state := r.state
	rec := r.recorder
	r.mu.Unlock()

	switch state {
	case StateIdle, StateError:
		if err := rec.Start(ctx); err != nil {
			r.set(StateError, fmt.Sprintf("Start failed: %v", err))
			return err
		}
		r.set(StateRecording, "Recording started")
		return nil
	case StateRecording, StatePaused:
		go r.stopAndUpload()
		return nil
	default:
		return fmt.Errorf("cannot toggle recording while %s", state)
	}
}

// TogglePause pauses or resumes the active recorder.
func (r *Runtime) TogglePause() error {
	r.mu.Lock()
	state := r.state
	rec := r.recorder
	r.mu.Unlock()
	if state != StateRecording && state != StatePaused {
		return fmt.Errorf("cannot pause while %s", state)
	}
	if err := rec.TogglePause(); err != nil {
		return err
	}
	if state == StatePaused {
		r.set(StateRecording, "Recording resumed")
	} else {
		r.set(StatePaused, "Recording paused")
	}
	return nil
}

// Cancel cancels active recording and returns to Idle.
func (r *Runtime) Cancel() error {
	r.mu.Lock()
	state := r.state
	rec := r.recorder
	r.mu.Unlock()
	if state != StateRecording && state != StatePaused {
		return fmt.Errorf("cannot cancel while %s", state)
	}
	if _, err := rec.Cancel(); err != nil {
		r.set(StateError, fmt.Sprintf("Cancel failed: %v", err))
		return err
	}
	r.set(StateIdle, "Recording cancelled")
	return nil
}

// ReloadConfig replaces runtime config when not busy.
func (r *Runtime) ReloadConfig(cfg config.Config) error {
	if err := config.Validate(&cfg); err != nil {
		return err
	}
	config.InitCacheDir(&cfg)
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.state != StateIdle && r.state != StateError {
		return fmt.Errorf("cannot save config while %s", r.state)
	}
	r.cfg = cfg
	if err := r.rebuildLocked(); err != nil {
		r.state = StateError
		r.message = err.Error()
		return err
	}
	r.state = StateIdle
	r.message = "Config reloaded"
	r.emitLocked()
	return nil
}

func (r *Runtime) stopAndUpload() {
	r.mu.Lock()
	rec := r.recorder
	r.mu.Unlock()

	res, err := rec.Stop()
	if err != nil {
		r.set(StateError, fmt.Sprintf("Stop failed: %v", err))
		return
	}
	if res.Canceled {
		r.set(StateIdle, "Recording cancelled")
		return
	}
	if res.Err != nil {
		r.set(StateError, fmt.Sprintf("Recording error: %v", res.Err))
		return
	}
	r.set(StateUploading, "Uploading ASR request")

	r.mu.Lock()
	cfg := r.cfg
	asrClient := r.asr
	r.mu.Unlock()

	outPath := strings.TrimSuffix(res.WavPath, filepath.Ext(res.WavPath)) + "." + config.ContainerExt(cfg.CONTAINER)
	if err := ffmpeg.Convert(cfg, res.WavPath, outPath, cfg.SAMPLING_RATE); err != nil {
		_ = os.Remove(res.WavPath)
		_ = os.Remove(outPath)
		r.set(StateError, fmt.Sprintf("FFmpeg failed: %v", err))
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
				_ = clipboard.PasteText("[request failed]")
			}
		}
		handleCache(cfg, res.WavPath, outPath, uploadOk, raw)
		r.set(StateError, fmt.Sprintf("Upload failed: %v", err))
		return
	}
	if text != "" {
		if err := clipboard.PasteText(text); err != nil {
			handleCache(cfg, res.WavPath, outPath, uploadOk, raw)
			r.set(StateError, fmt.Sprintf("Paste failed: %v", err))
			return
		}
	}
	handleCache(cfg, res.WavPath, outPath, uploadOk, raw)
	r.set(StateIdle, "Transcription pasted")
}

func (r *Runtime) rebuildLocked() error {
	httpClient := newHTTPClient(r.cfg)
	asrClient, err := asr.New(r.cfg, httpClient)
	if err != nil {
		return err
	}
	r.asr = asrClient
	r.recorder = record.New(r.cfg, config.TempDir(&r.cfg))
	return nil
}

func (r *Runtime) set(state State, message string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.state = state
	r.message = message
	r.emitLocked()
}

func (r *Runtime) snapshotLocked() Snapshot {
	return Snapshot{State: r.state, Message: r.message}
}

func (r *Runtime) emitLocked() {
	if r.onChange != nil {
		snap := r.snapshotLocked()
		go r.onChange(snap)
	}
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
	return &http.Client{Transport: tr, Timeout: time.Duration(cfg.RequestTimeout) * time.Second}
}

func cleanupOldTempFiles(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "RecordTemp_") {
			_ = os.Remove(filepath.Join(dir, e.Name()))
		}
	}
}

func handleCache(cfg config.Config, wavPath string, outPath string, uploadOk bool, resBody []byte) {
	if cfg.KeepCache && cfg.CacheDir != "" {
		timestamp := time.Now().Format("2006-01-02-15.04.05")
		base := fmt.Sprintf("audio-%s", timestamp)
		if wavPath != "" {
			newWav := filepath.Join(cfg.CacheDir, base+filepath.Ext(wavPath))
			if err := os.Rename(wavPath, newWav); err != nil {
				_ = os.Remove(wavPath)
			}
		}
		if outPath != "" {
			newOut := filepath.Join(cfg.CacheDir, base+filepath.Ext(outPath))
			if err := os.Rename(outPath, newOut); err != nil {
				_ = os.Remove(outPath)
			}
		}
		if uploadOk && len(resBody) > 0 {
			_ = os.WriteFile(filepath.Join(cfg.CacheDir, base+".json"), resBody, 0644)
		}
		return
	}
	if wavPath != "" {
		_ = os.Remove(wavPath)
	}
	if outPath != "" {
		_ = os.Remove(outPath)
	}
}

func TempOutputPath(dir, ext string) string {
	id := strings.ReplaceAll(uuid.New().String(), "-", "")[:16]
	base := fmt.Sprintf("RecordTemp_%s.%s", id, ext)
	if dir == "" {
		dir, _ = os.Getwd()
	}
	return filepath.Join(dir, base)
}
