// Copyright (C) 2026 Joey Kot <joey.kot.x@gmail.com>
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.
// See <https://www.gnu.org/licenses/> for more details.

package appcore

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"stt/internal/config"
	"stt/internal/record"
)

func TestRuntimeSnapshotAndEventHandler(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.CacheDir = t.TempDir()
	r, err := NewRuntime(cfg)
	if err != nil {
		t.Fatalf("NewRuntime failed: %v", err)
	}

	var got Event
	r.SetEventHandler(func(event Event) { got = event })
	r.setState(StateError, "boom", errors.New("bad"))

	want := Event{State: StateError, Message: "boom", Error: "bad"}
	if got != want {
		t.Fatalf("handler event = %#v, want %#v", got, want)
	}
	if snap := r.Snapshot(); snap != want {
		t.Fatalf("Snapshot = %#v, want %#v", snap, want)
	}
	if !r.CanReload() {
		t.Fatalf("CanReload = false for error state, want true")
	}
}

func TestStopCancelsBeforeWaitingForCurrentAction(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	r := &Runtime{
		state:           StateUploading,
		lastMessage:     "uploading",
		recorder:        &record.Recorder{},
		lifecycleCtx:    ctx,
		lifecycleCancel: cancel,
	}
	r.actionMu.Lock()
	stopped := make(chan struct{})
	go func() {
		r.Stop()
		close(stopped)
	}()

	select {
	case <-ctx.Done():
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Stop waited for actionMu before canceling the lifecycle context")
	}
	if r.TryToggleRecording() {
		t.Fatal("runtime accepted a new action after Stop")
	}
	r.actionMu.Unlock()

	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("Stop did not return after the current action completed")
	}

	r.setState(StateError, "late error", errors.New("late"))
	want := Event{State: StateUploading, Message: "uploading"}
	if got := r.Snapshot(); got != want {
		t.Fatalf("Snapshot after Stop = %#v, want %#v", got, want)
	}
}

func TestStopHasBoundedWaitForBlockedAction(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	r := &Runtime{
		recorder:        &record.Recorder{},
		lifecycleCtx:    ctx,
		lifecycleCancel: cancel,
	}
	r.actionMu.Lock()
	started := time.Now()
	r.Stop()
	elapsed := time.Since(started)
	r.actionMu.Unlock()

	if elapsed > shutdownGracePeriod+500*time.Millisecond {
		t.Fatalf("Stop took %v, want bounded wait near %v", elapsed, shutdownGracePeriod)
	}
}

func TestRecorderErrorUpdatesActiveRuntimeState(t *testing.T) {
	for _, state := range []State{StateRecording, StatePaused, StateError} {
		t.Run(string(state), func(t *testing.T) {
			recorder := &record.Recorder{}
			const session = 1
			r := &Runtime{
				state:                  state,
				lastMessage:            "generic error",
				lastError:              "recorder not running",
				recorder:               recorder,
				activeRecordingSession: session,
			}
			wantErr := errors.New("input device disconnected")

			r.handleRecorderError(recorder, session, wantErr)

			want := Event{State: StateError, Message: "Recording failed", Error: wantErr.Error()}
			if got := r.Snapshot(); got != want {
				t.Fatalf("Snapshot = %#v, want %#v", got, want)
			}
			if r.activeRecordingSession != 0 {
				t.Fatalf("active recording session = %d, want cleared", r.activeRecordingSession)
			}
		})
	}
}

func TestRecorderErrorDoesNotOverwriteInactiveRuntimeState(t *testing.T) {
	for _, state := range []State{StateIdle, StateUploading} {
		t.Run(string(state), func(t *testing.T) {
			recorder := &record.Recorder{}
			const session = 1
			r := &Runtime{
				state:                  state,
				lastMessage:            "current state",
				recorder:               recorder,
				activeRecordingSession: session,
			}

			r.handleRecorderError(recorder, session, errors.New("stale recorder error"))

			want := Event{State: state, Message: "current state"}
			if got := r.Snapshot(); got != want {
				t.Fatalf("Snapshot = %#v, want %#v", got, want)
			}
		})
	}
}

func TestRecorderErrorDoesNotOverwriteAnotherRecordingSession(t *testing.T) {
	currentRecorder := &record.Recorder{}
	oldRecorder := &record.Recorder{}
	const currentSession = 2

	tests := []struct {
		name     string
		recorder *record.Recorder
		session  uint64
	}{
		{name: "old recorder", recorder: oldRecorder, session: currentSession},
		{name: "old session", recorder: currentRecorder, session: currentSession - 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &Runtime{
				state:                  StateRecording,
				lastMessage:            "new recording active",
				recorder:               currentRecorder,
				activeRecordingSession: currentSession,
			}

			r.handleRecorderError(tt.recorder, tt.session, errors.New("old recording failed"))

			want := Event{State: StateRecording, Message: "new recording active"}
			if got := r.Snapshot(); got != want {
				t.Fatalf("Snapshot = %#v, want %#v", got, want)
			}
			if r.activeRecordingSession != currentSession {
				t.Fatalf("active recording session = %d, want %d", r.activeRecordingSession, currentSession)
			}
		})
	}
}

func TestRecorderErrorWaitsForCurrentAction(t *testing.T) {
	recorder := &record.Recorder{}
	const session = 1
	r := &Runtime{state: StateRecording, recorder: recorder, activeRecordingSession: session}
	wantErr := errors.New("stream failed immediately after startup")
	done := make(chan struct{})
	r.actionMu.Lock()

	go func() {
		r.handleRecorderError(recorder, session, wantErr)
		close(done)
	}()

	select {
	case <-done:
		t.Fatal("recorder error bypassed the in-flight runtime action")
	case <-time.After(20 * time.Millisecond):
	}
	if got := r.Snapshot(); got.State != StateRecording {
		t.Fatalf("Snapshot changed before the action completed: %#v", got)
	}

	r.actionMu.Unlock()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("recorder error did not resume after the action completed")
	}

	want := Event{State: StateError, Message: "Recording failed", Error: wantErr.Error()}
	if got := r.Snapshot(); got != want {
		t.Fatalf("Snapshot = %#v, want %#v", got, want)
	}
}

func TestTryActionsDropWhileBusy(t *testing.T) {
	tests := map[string]func(*Runtime) bool{
		"hotkey action":    func(r *Runtime) bool { return r.tryHandleAction(0) },
		"toggle recording": (*Runtime).TryToggleRecording,
		"toggle pause":     (*Runtime).TryTogglePause,
		"cancel":           (*Runtime).TryCancel,
	}

	for name, action := range tests {
		t.Run(name, func(t *testing.T) {
			r := &Runtime{}
			r.actionMu.Lock()
			defer r.actionMu.Unlock()

			result := make(chan bool, 1)
			go func() {
				result <- action(r)
			}()

			select {
			case accepted := <-result:
				if accepted {
					t.Fatal("action was accepted while actionMu was locked")
				}
			case <-time.After(time.Second):
				t.Fatal("action blocked instead of being dropped")
			}
		})
	}
}

func TestNewHTTPClientHonorsConfig(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.RequestTimeout = 7
	cfg.VerifySSL = false
	cfg.EnableHTTP2 = false

	client := newHTTPClient(cfg)
	if client.Timeout != 7*time.Second {
		t.Fatalf("Timeout = %v, want 7s", client.Timeout)
	}
	transport, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("Transport = %T, want *http.Transport", client.Transport)
	}
	if transport.TLSClientConfig == nil || !transport.TLSClientConfig.InsecureSkipVerify {
		t.Fatalf("InsecureSkipVerify not enabled when VerifySSL=false")
	}
}

func TestCleanupOldTempFilesOnlyRemovesRecordTemps(t *testing.T) {
	dir := t.TempDir()
	remove := filepath.Join(dir, "RecordTemp_old.wav")
	keep := filepath.Join(dir, "keep.wav")
	if err := os.WriteFile(remove, []byte("remove"), 0644); err != nil {
		t.Fatalf("WriteFile remove failed: %v", err)
	}
	if err := os.WriteFile(keep, []byte("keep"), 0644); err != nil {
		t.Fatalf("WriteFile keep failed: %v", err)
	}

	cleanupOldTempFiles(dir)

	if _, err := os.Stat(remove); !os.IsNotExist(err) {
		t.Fatalf("RecordTemp file still exists or stat error was unexpected: %v", err)
	}
	if _, err := os.Stat(keep); err != nil {
		t.Fatalf("non-temp file was removed or inaccessible: %v", err)
	}
}

func TestHandleCacheRemovesFilesWhenKeepCacheDisabled(t *testing.T) {
	dir := t.TempDir()
	wav := filepath.Join(dir, "input.wav")
	out := filepath.Join(dir, "output.ogg")
	if err := os.WriteFile(wav, []byte("wav"), 0644); err != nil {
		t.Fatalf("WriteFile wav failed: %v", err)
	}
	if err := os.WriteFile(out, []byte("out"), 0644); err != nil {
		t.Fatalf("WriteFile out failed: %v", err)
	}

	cfg := config.DefaultConfig()
	cfg.KeepCache = false
	handleCache(cfg, wav, out, true, []byte(`{"text":"ok"}`))

	if _, err := os.Stat(wav); !os.IsNotExist(err) {
		t.Fatalf("wav still exists or stat error was unexpected: %v", err)
	}
	if _, err := os.Stat(out); !os.IsNotExist(err) {
		t.Fatalf("out still exists or stat error was unexpected: %v", err)
	}
}

func TestHandleCacheKeepsAudioAndResponseWhenEnabled(t *testing.T) {
	dir := t.TempDir()
	wav := filepath.Join(dir, "input.wav")
	out := filepath.Join(dir, "output.ogg")
	if err := os.WriteFile(wav, []byte("wav"), 0644); err != nil {
		t.Fatalf("WriteFile wav failed: %v", err)
	}
	if err := os.WriteFile(out, []byte("out"), 0644); err != nil {
		t.Fatalf("WriteFile out failed: %v", err)
	}

	cfg := config.DefaultConfig()
	cfg.CacheDir = dir
	cfg.KeepCache = true
	handleCache(cfg, wav, out, true, []byte(`{"text":"ok"}`))

	matches, err := filepath.Glob(filepath.Join(dir, "audio-*"))
	if err != nil {
		t.Fatalf("Glob failed: %v", err)
	}
	counts := map[string]int{}
	for _, match := range matches {
		counts[filepath.Ext(match)]++
	}
	if counts[".wav"] != 1 || counts[".ogg"] != 1 || counts[".json"] != 1 {
		t.Fatalf("cached file counts = %#v from matches %#v, want one .wav, .ogg, and .json", counts, matches)
	}
	if _, err := os.Stat(wav); !os.IsNotExist(err) {
		t.Fatalf("original wav still exists or stat error was unexpected: %v", err)
	}
	if _, err := os.Stat(out); !os.IsNotExist(err) {
		t.Fatalf("original out still exists or stat error was unexpected: %v", err)
	}
}

func TestHandleCacheKeepsDistinctWAVFiles(t *testing.T) {
	dir := t.TempDir()
	wav := filepath.Join(dir, "input.wav")
	out := filepath.Join(dir, "output_convert.wav")
	if err := os.WriteFile(wav, []byte("original"), 0644); err != nil {
		t.Fatalf("WriteFile wav failed: %v", err)
	}
	if err := os.WriteFile(out, []byte("converted"), 0644); err != nil {
		t.Fatalf("WriteFile out failed: %v", err)
	}

	cfg := config.DefaultConfig()
	cfg.CacheDir = dir
	cfg.KeepCache = true
	handleCache(cfg, wav, out, false, nil)

	matches, err := filepath.Glob(filepath.Join(dir, "audio-*.wav"))
	if err != nil {
		t.Fatalf("Glob failed: %v", err)
	}
	if len(matches) != 2 {
		t.Fatalf("cached WAV files = %#v, want original and converted files", matches)
	}
	contents := make(map[string]bool, len(matches))
	for _, match := range matches {
		data, err := os.ReadFile(match)
		if err != nil {
			t.Fatalf("ReadFile %s failed: %v", match, err)
		}
		contents[string(data)] = true
	}
	if !contents["original"] || !contents["converted"] {
		t.Fatalf("cached WAV contents = %#v, want original and converted", contents)
	}
}

func TestRecordingOutputPathAvoidsWAVInput(t *testing.T) {
	wav := filepath.Join("temp", "RecordTemp_1234567890123456.wav")
	want := filepath.Join("temp", "RecordTemp_1234567890123456_convert.wav")
	if got := recordingOutputPath(wav, "WAV"); got != want {
		t.Fatalf("recordingOutputPath = %q, want %q", got, want)
	}
	if pathsEqual(wav, recordingOutputPath(wav, "wav")) {
		t.Fatal("WAV conversion output path still equals its input path")
	}
}

func TestRecordingOutputPathKeepsOtherContainerName(t *testing.T) {
	wav := filepath.Join("temp", "RecordTemp_1234567890123456.wav")
	want := filepath.Join("temp", "RecordTemp_1234567890123456.ogg")
	if got := recordingOutputPath(wav, "ogg"); got != want {
		t.Fatalf("recordingOutputPath = %q, want %q", got, want)
	}
}

func TestTempOutputPathUsesDirectoryAndExtension(t *testing.T) {
	dir := t.TempDir()
	path := tempOutputPath(dir, "ogg")
	if filepath.Dir(path) != dir {
		t.Fatalf("tempOutputPath dir = %q, want %q", filepath.Dir(path), dir)
	}
	if filepath.Ext(path) != ".ogg" {
		t.Fatalf("tempOutputPath ext = %q, want .ogg", filepath.Ext(path))
	}
	if got := filepath.Base(path); len(got) != len("RecordTemp_1234567890123456.ogg") {
		t.Fatalf("tempOutputPath base = %q, unexpected length", got)
	}
}
