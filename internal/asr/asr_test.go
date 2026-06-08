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

package asr

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"stt/internal/config"
)

func TestTranscribeRetryExhaustedError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("fail"))
	}))
	defer server.Close()

	cfg := config.DefaultConfig()
	cfg.APIEndpoint = server.URL
	cfg.TEXTPath = "text"
	cfg.MaxRetry = 2
	cfg.RetryBaseDelay = 0
	cfg.RequestTimeout = 2

	client, err := New(cfg, &http.Client{Timeout: time.Second})
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	audioPath := tempAudioFile(t, "test")

	_, _, err = client.Transcribe(context.Background(), audioPath)
	if err == nil {
		t.Fatalf("expected error")
	}

	var re *RetryExhaustedError
	if !errors.As(err, &re) {
		t.Fatalf("expected RetryExhaustedError, got %T: %v", err, err)
	}
	if re.Attempts != cfg.MaxRetry {
		t.Fatalf("expected attempts %d, got %d", cfg.MaxRetry, re.Attempts)
	}
	if re.MaxRetry != cfg.MaxRetry {
		t.Fatalf("expected MaxRetry %d, got %d", cfg.MaxRetry, re.MaxRetry)
	}
}

func TestExtraConfigNullDeletesBaseField(t *testing.T) {
	requestChecked := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			http.Error(w, fmt.Sprintf("parse multipart form: %v", err), http.StatusBadRequest)
			return
		}
		if _, ok := r.MultipartForm.Value["language"]; ok {
			http.Error(w, "language field should be deleted", http.StatusBadRequest)
			return
		}
		requestChecked = true
		_, _ = w.Write([]byte(`{"text":"ok"}`))
	}))
	defer server.Close()

	cfg := config.DefaultConfig()
	cfg.APIEndpoint = server.URL
	cfg.Language = "zh"
	cfg.ExtraConfig = `{"language":null}`
	cfg.TEXTPath = "text"
	cfg.MaxRetry = 1
	cfg.RequestTimeout = 2

	client, err := New(cfg, &http.Client{Timeout: time.Second})
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	audioPath := tempAudioFile(t, "test")

	text, _, err := client.Transcribe(context.Background(), audioPath)
	if err != nil {
		t.Fatalf("Transcribe failed: %v", err)
	}
	if text != "ok" {
		t.Fatalf("expected text ok, got %q", text)
	}
	if !requestChecked {
		t.Fatalf("server did not check request")
	}
}

func TestNewRejectsInvalidExtraConfig(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.ExtraConfig = `{"unterminated"`

	if _, err := New(cfg, nil); err == nil {
		t.Fatalf("New succeeded, want invalid extra-config error")
	}
}

func TestTranscribeRejectsEmptyEndpointBeforeOpeningFile(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.APIEndpoint = ""
	client, err := New(cfg, nil)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	_, _, err = client.Transcribe(context.Background(), "does-not-need-to-exist.wav")
	if err == nil || err.Error() != "API endpoint is empty" {
		t.Fatalf("Transcribe error = %v, want API endpoint is empty", err)
	}
}

func TestTranscribeSendsMultipartFieldsAuthAndExtractsConfiguredPath(t *testing.T) {
	requestChecked := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, fmt.Sprintf("method = %s, want POST", r.Method), http.StatusBadRequest)
			return
		}
		if got := r.Header.Get("Authorization"); got != "Bearer token-123" {
			http.Error(w, fmt.Sprintf("Authorization = %q", got), http.StatusBadRequest)
			return
		}
		if got := r.Header.Get("User-Agent"); got != "stt-go-client/1.0" {
			http.Error(w, fmt.Sprintf("User-Agent = %q", got), http.StatusBadRequest)
			return
		}
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			http.Error(w, fmt.Sprintf("parse multipart form: %v", err), http.StatusBadRequest)
			return
		}
		checks := map[string]string{
			"model":       "base",
			"language":    "en",
			"prompt":      "hello",
			"temperature": "0.25",
			"stream":      "false",
			"metadata":    `{"tier":"test"}`,
		}
		for key, want := range checks {
			if got := r.FormValue(key); got != want {
				http.Error(w, fmt.Sprintf("%s = %q, want %q", key, got, want), http.StatusBadRequest)
				return
			}
		}
		files := r.MultipartForm.File["file"]
		if len(files) != 1 || files[0].Filename == "" {
			http.Error(w, "missing uploaded file", http.StatusBadRequest)
			return
		}
		requestChecked = true
		_, _ = w.Write([]byte(`{"data":{"items":[{"text":"configured path"}]}}`))
	}))
	defer server.Close()

	cfg := config.DefaultConfig()
	cfg.APIEndpoint = server.URL
	cfg.Token = "token-123"
	cfg.Model = "base"
	cfg.Language = "en"
	cfg.Prompt = "hello"
	cfg.TEXTPath = "data.items[0].text"
	cfg.ExtraConfig = `{"temperature":0.25,"stream":false,"metadata":{"tier":"test"}}`
	cfg.MaxRetry = 1
	cfg.RequestTimeout = 2

	client, err := New(cfg, &http.Client{Timeout: time.Second})
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	text, raw, err := client.Transcribe(context.Background(), tempAudioFile(t, "audio"))
	if err != nil {
		t.Fatalf("Transcribe failed: %v", err)
	}
	if text != "configured path" {
		t.Fatalf("text = %q, want configured path", text)
	}
	if string(raw) != `{"data":{"items":[{"text":"configured path"}]}}` {
		t.Fatalf("raw = %s", raw)
	}
	if !requestChecked {
		t.Fatalf("server did not check request")
	}
}

func TestFormatResponse(t *testing.T) {
	if got := formatResponse(nil); got != "<empty>" {
		t.Fatalf("formatResponse(nil) = %q", got)
	}
	if got := formatResponse([]byte("hello")); got != "hello" {
		t.Fatalf("formatResponse(text) = %q", got)
	}
	if got := formatResponse([]byte{0xff, 0x00}); got != "<binary 2 bytes, hex: ff00>" {
		t.Fatalf("formatResponse(binary) = %q", got)
	}
}

func tempAudioFile(t *testing.T, contents string) string {
	t.Helper()
	tmp, err := os.CreateTemp(t.TempDir(), "asr-test-*.wav")
	if err != nil {
		t.Fatalf("CreateTemp failed: %v", err)
	}
	if _, err := tmp.Write([]byte(contents)); err != nil {
		_ = tmp.Close()
		t.Fatalf("write temp file failed: %v", err)
	}
	if err := tmp.Close(); err != nil {
		t.Fatalf("close temp file failed: %v", err)
	}
	return tmp.Name()
}
