//go:build !windows

package notify

// Notify is a no-op on non-Windows builds.
func Notify(title, message string) {}
