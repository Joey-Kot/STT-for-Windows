//go:build windows

package notify

import "github.com/gen2brain/beeep"

// Notify shows a Windows notification.
func Notify(title, message string) {
	_ = beeep.Notify(title, message, "")
}
