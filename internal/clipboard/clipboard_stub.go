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

//go:build !windows

package clipboard

import (
	"context"
	"fmt"
)

// PasteText is not supported on non-Windows builds.
func PasteText(text string) error {
	return PasteTextContext(context.Background(), text)
}

// PasteTextContext is not supported on non-Windows builds.
func PasteTextContext(ctx context.Context, text string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return fmt.Errorf("clipboard paste not supported on this platform")
}
