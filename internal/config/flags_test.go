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
	"flag"
	"testing"
)

func TestBindFlagsApplyFlagsAndAnySet(t *testing.T) {
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	fv := BindFlags(fs)
	if fv.AnySet() {
		t.Fatalf("AnySet before parsing = true, want false")
	}

	args := []string{
		"-api-endpoint", "https://example.test/asr",
		"-token", "secret",
		"-model", "whisper",
		"-language", "en",
		"-prompt", "say words",
		"-text-path", "data.text",
		"-extra-config", `{"temperature":0}`,
		"-codecs", "mp3",
		"-container", "mp3",
		"-channels", "2",
		"-sampling-rate", "48000",
		"-sampling-rate-depth", "24",
		"-bit-rate", "192",
		"-request-timeout", "9",
		"-max-retry", "5",
		"-retry-base-delay", "0.25",
		"-enable-http2", "no",
		"-verify-ssl", "0",
		"-start-key", "ctrl+a",
		"-pause-key", "ctrl+b",
		"-cancel-key", "ctrl+c",
		"-hotkeyhook", "false",
		"-cache-dir", "cache",
		"-keep-cache", "yes",
		"-notification", "true",
		"-request-failed-notification", "1",
		"-ffmpeg-debug", "y",
		"-record-debug", "true",
		"-hotkey-debug", "false",
		"-upload-debug", "true",
		"-output", "out.txt",
	}
	if err := fs.Parse(args); err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if !fv.AnySet() {
		t.Fatalf("AnySet after parsing = false, want true")
	}

	cfg := DefaultConfig()
	ApplyFlags(&cfg, fv)

	if cfg.APIEndpoint != "https://example.test/asr" || cfg.Token != "secret" || cfg.Model != "whisper" {
		t.Fatalf("string flags not applied: %#v", cfg)
	}
	if cfg.Language != "en" || cfg.Prompt != "say words" || cfg.TEXTPath != "data.text" || cfg.ExtraConfig != `{"temperature":0}` {
		t.Fatalf("request flags not applied: %#v", cfg)
	}
	if cfg.CODECS != "mp3" || cfg.CONTAINER != "mp3" || cfg.Channels != 2 || cfg.SAMPLING_RATE != 48000 || cfg.SAMPLING_RATE_DEPTH != 24 || cfg.BIT_RATE != 192 {
		t.Fatalf("audio flags not applied: %#v", cfg)
	}
	if cfg.RequestTimeout != 9 || cfg.MaxRetry != 5 || cfg.RetryBaseDelay != 0.25 || cfg.EnableHTTP2 || cfg.VerifySSL {
		t.Fatalf("HTTP flags not applied: %#v", cfg)
	}
	if cfg.StartKey != "ctrl+a" || cfg.PauseKey != "ctrl+b" || cfg.CancelKey != "ctrl+c" || cfg.HotKeyHook {
		t.Fatalf("hotkey flags not applied: %#v", cfg)
	}
	if cfg.CacheDir != "cache" || !cfg.KeepCache || !cfg.Notification || !cfg.RequestFailedNotification || !cfg.FFMPEG_DEBUG || !cfg.RECORD_DEBUG || cfg.HOTKEY_DEBUG || !cfg.UPLOAD_DEBUG {
		t.Fatalf("misc flags not applied: %#v", cfg)
	}
	if fv.OutputPath != "out.txt" || !fv.OutputPathSet {
		t.Fatalf("output flag = %q set=%v, want out.txt true", fv.OutputPath, fv.OutputPathSet)
	}
}

func TestBoolFlagRejectsInvalidValues(t *testing.T) {
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	BindFlags(fs)
	if err := fs.Parse([]string{"-verify-ssl", "maybe"}); err == nil {
		t.Fatalf("Parse succeeded, want invalid boolean error")
	}
}

func TestDeprecatedRateAliasSetsSamplingRate(t *testing.T) {
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	fv := BindFlags(fs)
	if err := fs.Parse([]string{"-rate", "22050"}); err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	cfg := DefaultConfig()
	ApplyFlags(&cfg, fv)
	if cfg.SAMPLING_RATE != 22050 {
		t.Fatalf("SAMPLING_RATE = %d, want 22050", cfg.SAMPLING_RATE)
	}
}
