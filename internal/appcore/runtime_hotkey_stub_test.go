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

//go:build !windows

package appcore

import (
	"strings"
	"testing"

	"stt/internal/config"
)

func TestStartHotkeysStoresRegistrationFailure(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.CacheDir = t.TempDir()
	runtime, err := NewRuntime(cfg)
	if err != nil {
		t.Fatalf("NewRuntime failed: %v", err)
	}
	defer runtime.Stop()

	var events []Event
	runtime.SetEventHandler(func(event Event) {
		events = append(events, event)
	})

	err = runtime.StartHotkeys()
	if err == nil {
		t.Fatal("StartHotkeys succeeded on an unsupported platform")
	}

	want := runtime.Snapshot()
	if want.State != StateError {
		t.Fatalf("Snapshot state = %s, want %s", want.State, StateError)
	}
	if want.Message != "Hotkey registration failed" {
		t.Fatalf("Snapshot message = %q, want hotkey registration failure", want.Message)
	}
	if !strings.Contains(want.Error, "not supported") {
		t.Fatalf("Snapshot error = %q, want unsupported-platform details", want.Error)
	}
	if len(events) != 1 || events[0] != want {
		t.Fatalf("events = %#v, want one retained error event %#v", events, want)
	}
}
