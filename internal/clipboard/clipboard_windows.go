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
	"fmt"
	"time"

	"github.com/atotto/clipboard"
	"github.com/micmonay/keybd_event"
)

// PasteText writes text to clipboard, sends Ctrl+V, and restores clipboard.
// The system clipboard is intentionally used as the text transport; Windows
// history and third-party clipboard observers may see the temporary contents.
func PasteText(text string) error {
	return pasteText(text, pasteOperations{
		readAll:   clipboard.ReadAll,
		writeAll:  clipboard.WriteAll,
		sendPaste: sendPasteShortcut,
		sleep:     time.Sleep,
	})
}

// keybd_event is intentionally kept for the Ctrl+V chord while the clipboard
// carries the text, instead of injecting text character by character with
// SendInput. No target window is bound or verified: the current foreground
// window decides whether to accept the paste command.
func sendPasteShortcut() error {
	kb, err := keybd_event.NewKeyBonding()
	if err != nil {
		return fmt.Errorf("create key binding: %w", err)
	}
	kb.HasCTRL(true)
	kb.SetKeys(keybd_event.VK_V)
	if err := kb.Launching(); err != nil {
		return fmt.Errorf("launch key binding: %w", err)
	}
	return nil
}
