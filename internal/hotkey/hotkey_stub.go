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

package hotkey

import "fmt"

// Registration represents a registered hotkey set.
type Registration struct{}

// Stop releases registered hotkeys.
func (r *Registration) Stop() {}

// Register is not supported on non-Windows builds.
func Register(startKey, pauseKey, cancelKey string, hook bool, handler func(id int), debug bool) error {
	return fmt.Errorf("hotkey not supported on this platform")
}

// RegisterWithStop is not supported on non-Windows builds.
func RegisterWithStop(startKey, pauseKey, cancelKey string, hook bool, handler func(id int), debug bool) (*Registration, error) {
	return nil, fmt.Errorf("hotkey not supported on this platform")
}
