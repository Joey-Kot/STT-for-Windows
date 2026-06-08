// Copyright (C) 2026 Joey Kot <joey.kot.x@gmail.com>
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.
// See <https://www.gnu.org/licenses/> for more details.

package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadReturnsDefaultsWhenPathEmpty(t *testing.T) {
	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if cfg != DefaultConfig() {
		t.Fatalf("Load empty path = %#v, want defaults %#v", cfg, DefaultConfig())
	}
}

func TestSaveDefaultAndLoadRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := SaveDefault(path); err != nil {
		t.Fatalf("SaveDefault failed: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if cfg != DefaultConfig() {
		t.Fatalf("loaded config = %#v, want %#v", cfg, DefaultConfig())
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	var decoded map[string]interface{}
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("saved config is not valid JSON: %v", err)
	}
	if decoded["TEXT_PATH"] != "text" {
		t.Fatalf("TEXT_PATH = %v, want text", decoded["TEXT_PATH"])
	}
}

func TestValidateAcceptsCaseInsensitiveKnownCodecAndContainer(t *testing.T) {
	cfg := DefaultConfig()
	cfg.CODECS = "MP3"
	cfg.CONTAINER = "M4A"
	if err := Validate(&cfg); err != nil {
		t.Fatalf("Validate failed: %v", err)
	}
}

func TestValidateRejectsInvalidValues(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*Config)
		wantErr string
	}{
		{name: "channels too low", mutate: func(c *Config) { c.Channels = 0 }, wantErr: "invalid Channels"},
		{name: "channels too high", mutate: func(c *Config) { c.Channels = 9 }, wantErr: "invalid Channels"},
		{name: "sample rate", mutate: func(c *Config) { c.SAMPLING_RATE = 0 }, wantErr: "invalid SAMPLING_RATE"},
		{name: "depth", mutate: func(c *Config) { c.SAMPLING_RATE_DEPTH = 12 }, wantErr: "invalid SAMPLING_RATE_DEPTH"},
		{name: "bitrate", mutate: func(c *Config) { c.BIT_RATE = 0 }, wantErr: "invalid BIT_RATE"},
		{name: "codec", mutate: func(c *Config) { c.CODECS = "bad-codec" }, wantErr: "invalid CODECS"},
		{name: "container", mutate: func(c *Config) { c.CONTAINER = "bad-container" }, wantErr: "invalid CONTAINER"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultConfig()
			tt.mutate(&cfg)
			err := Validate(&cfg)
			if err == nil {
				t.Fatalf("Validate succeeded, want error containing %q", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("Validate error = %q, want substring %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestInitCacheDirCreatesAndAbsolutizesDirectory(t *testing.T) {
	root := t.TempDir()
	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd failed: %v", err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatalf("Chdir failed: %v", err)
	}
	defer func() {
		if err := os.Chdir(oldWd); err != nil {
			t.Fatalf("restore cwd failed: %v", err)
		}
	}()

	cfg := DefaultConfig()
	cfg.CacheDir = "cache/subdir"
	InitCacheDir(&cfg)

	want := filepath.Join(root, "cache", "subdir")
	if cfg.CacheDir != want {
		t.Fatalf("CacheDir = %q, want %q", cfg.CacheDir, want)
	}
	if info, err := os.Stat(want); err != nil || !info.IsDir() {
		t.Fatalf("cache directory not created: info=%v err=%v", info, err)
	}
}

func TestInitCacheDirClearsFilePath(t *testing.T) {
	file := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(file, []byte("x"), 0644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	cfg := DefaultConfig()
	cfg.CacheDir = file
	InitCacheDir(&cfg)
	if cfg.CacheDir != "" {
		t.Fatalf("CacheDir = %q, want empty fallback", cfg.CacheDir)
	}
}

func TestContainerExt(t *testing.T) {
	tests := map[string]string{
		"":      "ogg",
		"OGG":   "ogg",
		"S16LE": "s16le",
		"weird": "weird",
	}
	for input, want := range tests {
		if got := ContainerExt(input); got != want {
			t.Fatalf("ContainerExt(%q) = %q, want %q", input, got, want)
		}
	}
}
