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
