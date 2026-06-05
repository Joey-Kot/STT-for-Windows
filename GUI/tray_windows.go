//go:build windows

package main

import "github.com/getlantern/systray"

var trayApp *App

// StartTray starts the Windows system tray menu.
func (a *App) StartTray() {
	trayApp = a
	systray.Run(func() {
		systray.SetTitle("STT")
		systray.SetTooltip("STT ASR")
		showHide := systray.AddMenuItem("Show / Hide", "Show or hide floating panel")
		settings := systray.AddMenuItem("Settings", "Open settings")
		systray.AddSeparator()
		quit := systray.AddMenuItem("Quit", "Quit STT")

		go func() {
			visible := true
			for {
				select {
				case <-showHide.ClickedCh:
					if visible {
						a.HideWindow()
					} else {
						a.ShowWindow()
					}
					visible = !visible
				case <-settings.ClickedCh:
					a.OpenSettings()
				case <-quit.ClickedCh:
					a.Quit()
					return
				}
			}
		}()
	}, func() {})
}

func StopTray() { systray.Quit() }
