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

package hotkey

import (
	"strings"
	"testing"
)

func TestParseHotkeyRejectsInvalidModifiers(t *testing.T) {
	tests := []struct {
		name    string
		spec    string
		wantErr string
	}{
		{name: "unknown", spec: "textctrrl+q", wantErr: "unsupported modifier token"},
		{name: "empty", spec: "ctrl++q", wantErr: "empty modifier token"},
		{name: "duplicate alias", spec: "ctrl+control+q", wantErr: "duplicate modifier token"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := parseHotkey(tt.spec)
			if err == nil {
				t.Fatalf("parseHotkey(%q) succeeded, want error", tt.spec)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("parseHotkey(%q) error = %q, want substring %q", tt.spec, err, tt.wantErr)
			}
		})
	}
}

func TestValidateBindingsAcceptsDistinctAliases(t *testing.T) {
	if err := ValidateBindings("control+alt+Q", "shift+F12", "menu+escape"); err != nil {
		t.Fatalf("ValidateBindings failed: %v", err)
	}
}

func TestValidateBindingsRejectsEquivalentDuplicates(t *testing.T) {
	err := ValidateBindings("ctrl+alt+q", "ALT + CONTROL + Q", "alt+esc")
	if err == nil {
		t.Fatal("ValidateBindings succeeded, want duplicate error")
	}
	if !strings.Contains(err.Error(), "PAUSE_KEY") || !strings.Contains(err.Error(), "duplicates START_KEY") {
		t.Fatalf("ValidateBindings error = %q, want PAUSE_KEY/START_KEY duplicate details", err)
	}
}
