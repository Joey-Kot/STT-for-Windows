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
	"fmt"
	"strconv"
	"strings"
)

const (
	modifierAlt   uint32 = 0x0001
	modifierCtrl  uint32 = 0x0002
	modifierShift uint32 = 0x0004
	modifierWin   uint32 = 0x0008

	VK_NUMPAD0  = 0x60
	VK_NUMPAD1  = 0x61
	VK_NUMPAD2  = 0x62
	VK_NUMPAD3  = 0x63
	VK_NUMPAD4  = 0x64
	VK_NUMPAD5  = 0x65
	VK_NUMPAD6  = 0x66
	VK_NUMPAD7  = 0x67
	VK_NUMPAD8  = 0x68
	VK_NUMPAD9  = 0x69
	VK_ADD      = 0x6B
	VK_SUBTRACT = 0x6D
)

type parsedHotkey struct {
	mod uint32
	vk  uint32
}

// ValidateBindings validates all configured hotkeys and rejects equivalent
// bindings, including bindings written with different aliases or casing.
func ValidateBindings(startKey, pauseKey, cancelKey string) error {
	bindings := []struct {
		name string
		spec string
	}{
		{name: "START_KEY", spec: startKey},
		{name: "PAUSE_KEY", spec: pauseKey},
		{name: "CANCEL_KEY", spec: cancelKey},
	}

	seen := make(map[parsedHotkey]struct {
		name string
		spec string
	}, len(bindings))
	for _, binding := range bindings {
		mod, vk, err := parseHotkey(binding.spec)
		if err != nil {
			return fmt.Errorf("invalid %s %q: %w", binding.name, binding.spec, err)
		}

		parsed := parsedHotkey{mod: mod, vk: vk}
		if previous, ok := seen[parsed]; ok {
			return fmt.Errorf("%s %q duplicates %s %q", binding.name, binding.spec, previous.name, previous.spec)
		}
		seen[parsed] = struct {
			name string
			spec string
		}{name: binding.name, spec: binding.spec}
	}
	return nil
}

// parseHotkey accepts strings like "alt+q", "ctrl+shift+F1", and "esc",
// returning a Windows modifier mask and virtual-key code.
func parseHotkey(s string) (uint32, uint32, error) {
	if strings.TrimSpace(s) == "" {
		return 0, 0, fmt.Errorf("empty key")
	}
	parts := strings.Split(s, "+")
	for i := range parts {
		parts[i] = strings.TrimSpace(strings.ToLower(parts[i]))
	}

	var mod uint32
	keyToken := parts[len(parts)-1]
	if keyToken == "" {
		return 0, 0, fmt.Errorf("empty key token")
	}
	for _, token := range parts[:len(parts)-1] {
		modifier, ok := modifierForToken(token)
		if !ok {
			if token == "" {
				return 0, 0, fmt.Errorf("empty modifier token")
			}
			return 0, 0, fmt.Errorf("unsupported modifier token: %s", token)
		}
		if mod&modifier != 0 {
			return 0, 0, fmt.Errorf("duplicate modifier token: %s", token)
		}
		mod |= modifier
	}

	if len(keyToken) == 1 {
		ch := keyToken[0]
		if ch >= 'a' && ch <= 'z' {
			return mod, uint32(ch - 'a' + 'A'), nil
		}
		if ch >= '0' && ch <= '9' {
			return mod, uint32(ch), nil
		}
	}
	switch keyToken {
	case "esc", "escape":
		return mod, 0x1B, nil
	case "space":
		return mod, 0x20, nil
	case "enter", "return":
		return mod, 0x0D, nil
	}
	if strings.HasPrefix(keyToken, "f") {
		nStr := strings.TrimPrefix(keyToken, "f")
		if n, err := strconv.Atoi(nStr); err == nil && n >= 1 && n <= 24 {
			return mod, 0x70 + uint32(n-1), nil
		}
	}
	switch keyToken {
	case "numpad0", "num0", "kp0":
		return mod, VK_NUMPAD0, nil
	case "numpad1", "num1", "kp1":
		return mod, VK_NUMPAD1, nil
	case "numpad2", "num2", "kp2":
		return mod, VK_NUMPAD2, nil
	case "numpad3", "num3", "kp3":
		return mod, VK_NUMPAD3, nil
	case "numpad4", "num4", "kp4":
		return mod, VK_NUMPAD4, nil
	case "numpad5", "num5", "kp5":
		return mod, VK_NUMPAD5, nil
	case "numpad6", "num6", "kp6":
		return mod, VK_NUMPAD6, nil
	case "numpad7", "num7", "kp7":
		return mod, VK_NUMPAD7, nil
	case "numpad8", "num8", "kp8":
		return mod, VK_NUMPAD8, nil
	case "numpad9", "num9", "kp9":
		return mod, VK_NUMPAD9, nil
	case "add", "plus", "kpadd":
		return mod, VK_ADD, nil
	case "subtract", "minus", "kpsubtract":
		return mod, VK_SUBTRACT, nil
	}

	named := map[string]uint32{
		"tab":       0x09,
		"backspace": 0x08,
		"insert":    0x2D,
		"delete":    0x2E,
		"home":      0x24,
		"end":       0x23,
		"pageup":    0x21,
		"pagedown":  0x22,
		"left":      0x25,
		"up":        0x26,
		"right":     0x27,
		"down":      0x28,
	}
	if v, ok := named[keyToken]; ok {
		return mod, v, nil
	}
	if len(keyToken) == 1 {
		return mod, uint32(strings.ToUpper(keyToken)[0]), nil
	}
	return 0, 0, fmt.Errorf("unsupported key token: %s", keyToken)
}

func modifierForToken(token string) (uint32, bool) {
	switch token {
	case "alt", "menu":
		return modifierAlt, true
	case "ctrl", "control":
		return modifierCtrl, true
	case "shift":
		return modifierShift, true
	case "win", "meta", "super":
		return modifierWin, true
	default:
		return 0, false
	}
}
