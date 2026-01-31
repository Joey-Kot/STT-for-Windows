//go:build !windows

package clipboard

import "fmt"

// PasteText is not supported on non-Windows builds.
func PasteText(text string) error {
	return fmt.Errorf("clipboard paste not supported on this platform")
}
