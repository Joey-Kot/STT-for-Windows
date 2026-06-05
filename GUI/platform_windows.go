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

package main

import (
	"strconv"

	"golang.org/x/sys/windows/registry"
)

func detectRoundedCorners() bool {
	key, err := registry.OpenKey(registry.LOCAL_MACHINE, `SOFTWARE\Microsoft\Windows NT\CurrentVersion`, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer key.Close()

	build, _, err := key.GetStringValue("CurrentBuildNumber")
	if err != nil {
		return false
	}

	n, err := strconv.Atoi(build)
	if err != nil {
		return false
	}

	return n >= 22000
}
