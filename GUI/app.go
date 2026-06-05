// Copyright (C) 2026 Joey Kot <joey.kot.x@gmail.com>
// SPDX-License-Identifier: GPL-3.0-or-later

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

type App struct {
	ctx        context.Context
	mu         sync.Mutex
	configPath string
	runtime    *appcore.Runtime
}

func NewApp() *App { return &App{} }

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	cfgPath, cfg, err := loadGUIConfig()
	a.configPath = cfgPath
	if err != nil {
		a.emitStatus(appcore.Snapshot{State: appcore.StateError, Message: err.Error()})
		return
	}
	rt, err := appcore.NewRuntime(cfg)
	if err != nil {
		a.emitStatus(appcore.Snapshot{State: appcore.StateError, Message: err.Error()})
		return
	}
	rt.SetOnChange(a.emitStatus)
	a.mu.Lock()
	a.runtime = rt
	a.mu.Unlock()
	if err := rt.RegisterHotkeys(); err != nil {
		a.emitStatus(appcore.Snapshot{State: appcore.StateIdle, Message: fmt.Sprintf("Hotkeys unavailable: %v", err)})
	}
	a.emitStatus(rt.Snapshot())
}

func (a *App) Shutdown() { StopTray() }

func (a *App) GetStatus() appcore.Snapshot {
	rt := a.currentRuntime()
	if rt == nil {
		return appcore.Snapshot{State: appcore.StateError, Message: "Runtime is not initialized"}
	}
	return rt.Snapshot()
}

func (a *App) ToggleRecording() (appcore.Snapshot, error) {
	rt := a.currentRuntime()
	if rt == nil {
		return a.GetStatus(), fmt.Errorf("runtime is not initialized")
	}
	if err := rt.ToggleRecording(context.Background()); err != nil {
		return rt.Snapshot(), err
	}
	return rt.Snapshot(), nil
}

func (a *App) TogglePause() (appcore.Snapshot, error) {
	rt := a.currentRuntime()
	if rt == nil {
		return a.GetStatus(), fmt.Errorf("runtime is not initialized")
	}
	if err := rt.TogglePause(); err != nil {
		return rt.Snapshot(), err
	}
	return rt.Snapshot(), nil
}

func (a *App) CancelRecording() (appcore.Snapshot, error) {
	rt := a.currentRuntime()
	if rt == nil {
		return a.GetStatus(), fmt.Errorf("runtime is not initialized")
	}
	if err := rt.Cancel(); err != nil {
		return rt.Snapshot(), err
	}
	return rt.Snapshot(), nil
}

func (a *App) ConfigPath() string { return a.configPath }

func (a *App) LoadConfigJSON() (string, error) {
	if a.configPath == "" {
		return "", fmt.Errorf("config path is not initialized")
	}
	b, err := os.ReadFile(a.configPath)
	if err != nil {
		return "", err
	}
	var v any
	if err := json.Unmarshal(b, &v); err != nil {
		return "", err
	}
	pretty, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return "", err
	}
	return string(pretty), nil
}

func (a *App) SaveConfigJSON(raw string) (appcore.Snapshot, error) {
	var cfg config.Config
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		return a.GetStatus(), err
	}
	if err := config.Validate(&cfg); err != nil {
		return a.GetStatus(), err
	}
	pretty, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return a.GetStatus(), err
	}
	if err := os.WriteFile(a.configPath, append(pretty, '\n'), 0600); err != nil {
		return a.GetStatus(), err
	}
	rt := a.currentRuntime()
	if rt == nil {
		return a.GetStatus(), fmt.Errorf("runtime is not initialized")
	}
	if err := rt.ReloadConfig(cfg); err != nil {
		return rt.Snapshot(), err
	}
	return rt.Snapshot(), nil
}

func (a *App) ShowWindow() {
	if a.ctx != nil {
		wailsruntime.WindowShow(a.ctx)
		wailsruntime.WindowUnminimise(a.ctx)
	}
}

func (a *App) HideWindow() {
	if a.ctx != nil {
		wailsruntime.WindowHide(a.ctx)
	}
}

func (a *App) Quit() {
	if a.ctx != nil {
		wailsruntime.Quit(a.ctx)
	}
}

func (a *App) OpenSettings() {
	if a.ctx != nil {
		wailsruntime.WindowShow(a.ctx)
		wailsruntime.EventsEmit(a.ctx, "settings:open")
	}
}

func (a *App) currentRuntime() *appcore.Runtime {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.runtime
}

func (a *App) emitStatus(s appcore.Snapshot) {
	if a.ctx != nil {
		wailsruntime.EventsEmit(a.ctx, "runtime:status", s)
	}
}

func loadGUIConfig() (string, config.Config, error) {
	cfgPath, err := defaultConfigPath()
	if err != nil {
		return "", config.Config{}, err
	}
	if _, err := os.Stat(cfgPath); os.IsNotExist(err) {
		if err := os.MkdirAll(filepath.Dir(cfgPath), 0700); err != nil {
			return "", config.Config{}, err
		}
		cfg := config.DefaultConfig()
		cfg.StartKey = "f1"
		cfg.PauseKey = "ctrl+alt+s"
		cfg.CancelKey = "alt+esc"
		b, err := json.MarshalIndent(cfg, "", "  ")
		if err != nil {
			return "", config.Config{}, err
		}
		if err := os.WriteFile(cfgPath, append(b, '\n'), 0600); err != nil {
			return "", config.Config{}, err
		}
		return cfgPath, cfg, nil
	} else if err != nil {
		return "", config.Config{}, err
	}
	cfg, err := config.Load(cfgPath)
	return cfgPath, cfg, err
}

func defaultConfigPath() (string, error) {
	if appData := os.Getenv("APPDATA"); appData != "" {
		return filepath.Join(appData, "stt", "config.json"), nil
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "stt", "config.json"), nil
}
