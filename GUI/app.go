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

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"stt/internal/appcore"
	"stt/internal/config"
)

// ConfigPayload carries editable ASR JSON and GUI metadata to the frontend.
type ConfigPayload struct {
	Path string        `json:"path"`
	JSON string        `json:"json"`
	Data config.Config `json:"data"`
}

// QuitDecision tells the UI whether quit happened or confirmation is needed.
type QuitDecision struct {
	Quit    bool   `json:"quit"`
	Message string `json:"message"`
}

// WindowState tracks the floating window presentation mode.
type WindowState struct {
	Minimal bool `json:"minimal"`
}

// UIStyle carries platform-dependent presentation flags.
type UIStyle struct {
	Rounded bool `json:"rounded"`
}

// App is the Wails binding surface.
type App struct {
	ctx             context.Context
	mu              sync.Mutex
	runtime         *appcore.Runtime
	configPath      string
	minimal         bool
	rounded         bool
	trayMinimalSync func(bool)
}

func NewApp() *App {
	return &App{}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	a.rounded = detectRoundedCorners()

	path, err := defaultConfigPath()
	if err != nil {
		a.emitError("Config path failed", err)
		return
	}
	a.configPath = path

	cfg, err := ensureConfig(path)
	if err != nil {
		a.emitError("Config load failed", err)
		return
	}

	rt, err := appcore.NewRuntime(cfg)
	if err != nil {
		a.emitError("Runtime init failed", err)
		return
	}
	rt.SetEventHandler(func(event appcore.Event) {
		if a.ctx != nil {
			wailsruntime.EventsEmit(a.ctx, "runtime:state", event)
		}
	})
	a.runtime = rt

	if err := rt.StartHotkeys(); err != nil {
		a.emitError("Hotkey registration failed", err)
	}
	go startTray(a)
	wailsruntime.EventsEmit(ctx, "runtime:state", rt.Snapshot())
	wailsruntime.EventsEmit(ctx, "window:minimal", a.GetWindowState())
	wailsruntime.EventsEmit(ctx, "ui:style", a.GetUIStyle())
}

func (a *App) shutdown(ctx context.Context) {
	a.mu.Lock()
	rt := a.runtime
	a.runtime = nil
	a.mu.Unlock()
	if rt != nil {
		rt.Stop()
	}
	stopTray()
}

// GetState returns the current runtime state.
func (a *App) GetState() appcore.Event {
	rt := a.getRuntime()
	if rt == nil {
		return appcore.Event{State: appcore.StateError, Message: "Runtime is not ready"}
	}
	return rt.Snapshot()
}

// GetWindowState returns the current presentation mode.
func (a *App) GetWindowState() WindowState {
	a.mu.Lock()
	defer a.mu.Unlock()
	return WindowState{Minimal: a.minimal}
}

// GetUIStyle returns platform-dependent style switches.
func (a *App) GetUIStyle() UIStyle {
	a.mu.Lock()
	defer a.mu.Unlock()
	return UIStyle{Rounded: a.rounded}
}

// SetMinimal switches between compact and full floating window modes.
func (a *App) SetMinimal(minimal bool) WindowState {
	a.mu.Lock()
	a.minimal = minimal
	state := WindowState{Minimal: a.minimal}
	syncTray := a.trayMinimalSync
	ctx := a.ctx
	a.mu.Unlock()

	if syncTray != nil {
		syncTray(minimal)
	}
	if ctx != nil {
		a.showWindow()
		applyTaskbarVisibility(!minimal)
		wailsruntime.EventsEmit(ctx, "window:minimal", state)
	}
	return state
}

// ToggleRecording starts/stops recording.
func (a *App) ToggleRecording() appcore.Event {
	rt := a.getRuntime()
	if rt == nil {
		return appcore.Event{State: appcore.StateError, Message: "Runtime is not ready"}
	}
	go rt.ToggleRecording()
	return rt.Snapshot()
}

// TogglePause pauses/resumes recording.
func (a *App) TogglePause() appcore.Event {
	rt := a.getRuntime()
	if rt == nil {
		return appcore.Event{State: appcore.StateError, Message: "Runtime is not ready"}
	}
	go rt.TogglePause()
	return rt.Snapshot()
}

// Cancel cancels the current recording.
func (a *App) Cancel() appcore.Event {
	rt := a.getRuntime()
	if rt == nil {
		return appcore.Event{State: appcore.StateError, Message: "Runtime is not ready"}
	}
	go rt.Cancel()
	return rt.Snapshot()
}

// LoadConfig reads the editable ASR config JSON.
func (a *App) LoadConfig() (ConfigPayload, error) {
	if a.configPath == "" {
		return ConfigPayload{}, fmt.Errorf("config path is not ready")
	}
	cfg, err := ensureConfig(a.configPath)
	if err != nil {
		return ConfigPayload{}, err
	}
	raw, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return ConfigPayload{}, err
	}
	return ConfigPayload{Path: a.configPath, JSON: string(raw), Data: cfg}, nil
}

// SaveConfig validates, saves, and reloads the ASR config JSON.
func (a *App) SaveConfig(raw string) (appcore.Event, error) {
	rt := a.getRuntime()
	if rt == nil {
		return appcore.Event{State: appcore.StateError, Message: "Runtime is not ready"}, fmt.Errorf("runtime is not ready")
	}
	if !rt.CanReload() {
		return rt.Snapshot(), fmt.Errorf("settings can only be saved while idle")
	}

	var cfg config.Config
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		return rt.Snapshot(), err
	}
	if err := config.Validate(&cfg); err != nil {
		return rt.Snapshot(), err
	}
	out, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return rt.Snapshot(), err
	}
	if err := os.MkdirAll(filepath.Dir(a.configPath), 0755); err != nil {
		return rt.Snapshot(), err
	}
	if err := os.WriteFile(a.configPath, out, 0600); err != nil {
		return rt.Snapshot(), err
	}
	if err := rt.Reload(cfg); err != nil {
		return rt.Snapshot(), err
	}
	return rt.Snapshot(), nil
}

// OpenSettings asks the frontend to show the settings view.
func (a *App) OpenSettings() {
	a.SetMinimal(false)
	a.showWindow()
	if a.ctx != nil {
		wailsruntime.EventsEmit(a.ctx, "settings:open")
	}
}

// RequestQuit exits immediately when safe, otherwise asks the UI to confirm.
func (a *App) RequestQuit() QuitDecision {
	rt := a.getRuntime()
	if rt == nil {
		a.quit()
		return QuitDecision{Quit: true}
	}
	snapshot := rt.Snapshot()
	if snapshot.State == appcore.StateRecording || snapshot.State == appcore.StatePaused || snapshot.State == appcore.StateUploading {
		a.showWindow()
		if a.ctx != nil {
			wailsruntime.EventsEmit(a.ctx, "quit:confirm", snapshot)
		}
		return QuitDecision{Message: fmt.Sprintf("STT is currently %s", snapshot.State)}
	}
	a.quit()
	return QuitDecision{Quit: true}
}

// ConfirmQuit forces shutdown after the user confirms.
func (a *App) ConfirmQuit() {
	a.quit()
}

// ShowWindow displays the floating window.
func (a *App) ShowWindow() {
	a.showWindow()
}

func (a *App) setTrayMinimalSync(sync func(bool)) {
	a.mu.Lock()
	a.trayMinimalSync = sync
	current := a.minimal
	a.mu.Unlock()
	if sync != nil {
		sync(current)
	}
}

func (a *App) getRuntime() *appcore.Runtime {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.runtime
}

func (a *App) emitError(message string, err error) {
	event := appcore.Event{State: appcore.StateError, Message: message}
	if err != nil {
		event.Error = err.Error()
	}
	if a.ctx != nil {
		wailsruntime.EventsEmit(a.ctx, "runtime:state", event)
	}
}

func (a *App) showWindow() {
	if a.ctx == nil {
		return
	}
	wailsruntime.WindowShow(a.ctx)
	wailsruntime.WindowUnminimise(a.ctx)
	wailsruntime.WindowSetAlwaysOnTop(a.ctx, true)
}

func (a *App) quit() {
	if a.ctx == nil {
		return
	}
	wailsruntime.Quit(a.ctx)
}

func defaultConfigPath() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "stt", "config.json"), nil
}

func ensureConfig(path string) (config.Config, error) {
	if _, err := os.Stat(path); err == nil {
		return config.Load(path)
	} else if !os.IsNotExist(err) {
		return config.Config{}, err
	}

	cfg := guiDefaultConfig()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return config.Config{}, err
	}
	raw, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return config.Config{}, err
	}
	if err := os.WriteFile(path, raw, 0600); err != nil {
		return config.Config{}, err
	}
	return cfg, nil
}

func guiDefaultConfig() config.Config {
	return config.DefaultConfig()
}
