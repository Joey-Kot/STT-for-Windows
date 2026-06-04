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

//go:build windows

package clipboard

import (
	"time"

	"github.com/atotto/clipboard"
	"github.com/micmonay/keybd_event"
)

// PasteText writes text to clipboard, sends Ctrl+V, and restores clipboard.
func PasteText(text string) error {
	orig, _ := clipboard.ReadAll()
	_ = clipboard.WriteAll(text)
	time.Sleep(80 * time.Millisecond)

	kb, err := keybd_event.NewKeyBonding()
	if err != nil {
		return err
	}
	kb.HasCTRL(true)
	kb.SetKeys(keybd_event.VK_V)
	if err := kb.Launching(); err != nil {
		return err
	}
	time.Sleep(120 * time.Millisecond)
	_ = clipboard.WriteAll(orig)
	return nil
}
