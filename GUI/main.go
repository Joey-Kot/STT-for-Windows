// Copyright (C) 2026 Joey Kot <joey.kot.x@gmail.com>
// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"context"
	"embed"
	"log"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	app := NewApp()
	go app.StartTray()

	err := wails.Run(&options.App{
		Title:            "STT",
		Width:            260,
		Height:           132,
		Frameless:        true,
		DisableResize:    true,
		AlwaysOnTop:      true,
		BackgroundColour: &options.RGBA{R: 16, G: 18, B: 24, A: 235},
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		OnStartup: app.startup,
		OnShutdown: func(_ context.Context) {
			app.Shutdown()
		},
		Bind:            []interface{}{app},
		CSSDragProperty: "--wails-draggable",
		CSSDragValue:    "drag",
	})
	if err != nil {
		log.Fatal(err)
	}
}
