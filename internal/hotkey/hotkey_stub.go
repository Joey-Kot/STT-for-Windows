//go:build !windows

package hotkey

import "fmt"

// Register is not supported on non-Windows builds.
func Register(startKey, pauseKey, cancelKey string, hook bool, handler func(id int), debug bool) error {
	return fmt.Errorf("hotkey not supported on this platform")
}
