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
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"stt/internal/config"
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
