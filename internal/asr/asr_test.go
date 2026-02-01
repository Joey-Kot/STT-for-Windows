package asr

import (
	"context"
	"errors"
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

	tmp, err := os.CreateTemp("", "asr-test-*.wav")
	if err != nil {
		t.Fatalf("CreateTemp failed: %v", err)
	}
	if _, err := tmp.Write([]byte("test")); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
		t.Fatalf("write temp file failed: %v", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmp.Name())
		t.Fatalf("close temp file failed: %v", err)
	}
	defer os.Remove(tmp.Name())

	_, _, err = client.Transcribe(context.Background(), tmp.Name())
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
