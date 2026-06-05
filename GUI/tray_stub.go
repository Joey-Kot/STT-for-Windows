//go:build !windows

package main

// StartTray is a no-op outside Windows; the GUI targets Windows releases.
func (a *App) StartTray() {}

func StopTray() {}
