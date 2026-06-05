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
	_ "embed"
	"encoding/binary"
	"sync"

	"github.com/getlantern/systray"
)

var trayStopOnce sync.Once

//go:embed build/icons/icon64.png
var trayPNG []byte

func startTray(app *App) {
	go systray.Run(func() {
		systray.SetIcon(trayIcon())
		systray.SetTooltip("STT")
		minimal := systray.AddMenuItemCheckbox("Minimal", "Show compact microphone-only panel", false)
		settings := systray.AddMenuItem("Settings", "Open settings")
		quit := systray.AddMenuItem("Quit", "Quit STT")
		app.setTrayMinimalSync(func(enabled bool) {
			if enabled {
				minimal.Check()
				return
			}
			minimal.Uncheck()
		})

		go func() {
			for {
				select {
				case <-minimal.ClickedCh:
					app.SetMinimal(!minimal.Checked())
				case <-settings.ClickedCh:
					app.OpenSettings()
				case <-quit.ClickedCh:
					app.RequestQuit()
				}
			}
		}()
	}, func() {})
}

func stopTray() {
	trayStopOnce.Do(func() {
		systray.Quit()
	})
}

func trayIcon() []byte {
	if len(trayPNG) == 0 {
		return nil
	}

	ico := make([]byte, 22+len(trayPNG))
	binary.LittleEndian.PutUint16(ico[2:4], 1)
	binary.LittleEndian.PutUint16(ico[4:6], 1)
	ico[6] = 64
	ico[7] = 64
	binary.LittleEndian.PutUint16(ico[10:12], 1)
	binary.LittleEndian.PutUint16(ico[12:14], 32)
	binary.LittleEndian.PutUint32(ico[14:18], uint32(len(trayPNG)))
	binary.LittleEndian.PutUint32(ico[18:22], 22)
	copy(ico[22:], trayPNG)
	return ico
}
