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

package record

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"stt/internal/config"
)

type fakeAudioBackend struct {
	initializeErr     error
	openErr           error
	stream            *fakeAudioStream
	initializeEntered chan struct{}
	initializeRelease chan struct{}

	mu             sync.Mutex
	terminateCalls int
}

func (b *fakeAudioBackend) Initialize() error {
	if b.initializeEntered != nil {
		close(b.initializeEntered)
	}
	if b.initializeRelease != nil {
		<-b.initializeRelease
	}
	return b.initializeErr
}

func (b *fakeAudioBackend) Terminate() error {
	b.mu.Lock()
	b.terminateCalls++
	b.mu.Unlock()
	return nil
}

func (b *fakeAudioBackend) OpenDefaultStream(_, _ int, _ float64, _ int, _ []int16) (audioStream, error) {
	if b.openErr != nil {
		return nil, b.openErr
	}
	return b.stream, nil
}

func (b *fakeAudioBackend) terminateCount() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.terminateCalls
}

type fakeAudioStream struct {
	startErr     error
	readErrs     []error
	readDone     chan struct{}
	readEntered  chan struct{}
	readRelease  chan struct{}
	readGateOnce sync.Once

	mu         sync.Mutex
	startCalls int
	stopCalls  int
	closeCalls int
	readCalls  int
}

func (s *fakeAudioStream) Start() error {
	s.mu.Lock()
	s.startCalls++
	s.mu.Unlock()
	return s.startErr
}

func (s *fakeAudioStream) Stop() error {
	s.mu.Lock()
	s.stopCalls++
	s.mu.Unlock()
	return nil
}

func (s *fakeAudioStream) Close() error {
	s.mu.Lock()
	s.closeCalls++
	s.mu.Unlock()
	return nil
}

func (s *fakeAudioStream) Read() error {
	s.readGateOnce.Do(func() {
		if s.readEntered != nil {
			close(s.readEntered)
		}
		if s.readRelease != nil {
			<-s.readRelease
		}
	})

	s.mu.Lock()
	index := s.readCalls
	s.readCalls++
	var err error
	if len(s.readErrs) > 0 {
		if index < len(s.readErrs) {
			err = s.readErrs[index]
		} else {
			err = s.readErrs[len(s.readErrs)-1]
		}
	}
	if s.readDone != nil && index+1 == len(s.readErrs) {
		close(s.readDone)
	}
	s.mu.Unlock()
	return err
}

func (s *fakeAudioStream) counts() (start, stop, close, read int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.startCalls, s.stopCalls, s.closeCalls, s.readCalls
}

func testConfig() config.Config {
	cfg := config.DefaultConfig()
	cfg.Channels = 1
	cfg.SAMPLING_RATE = 16000
	cfg.RECORD_DEBUG = false
	return cfg
}

func TestStartWaitsUntilRecordingResourcesAreReady(t *testing.T) {
	stream := &fakeAudioStream{}
	backend := &fakeAudioBackend{
		stream:            stream,
		initializeEntered: make(chan struct{}),
		initializeRelease: make(chan struct{}),
	}
	recorder := newRecorder(testConfig(), t.TempDir(), backend)
	startResult := make(chan error, 1)

	go func() {
		startResult <- recorder.Start(context.Background())
	}()

	<-backend.initializeEntered
	select {
	case err := <-startResult:
		t.Fatalf("Start returned before initialization completed: %v", err)
	default:
	}

	close(backend.initializeRelease)
	select {
	case err := <-startResult:
		if err != nil {
			t.Fatalf("Start failed: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Start did not return after initialization completed")
	}

	res, err := recorder.Cancel()
	if err != nil {
		t.Fatalf("Cancel failed: %v", err)
	}
	if !res.Canceled {
		t.Fatalf("Cancel result = %#v, want Canceled=true", res)
	}
}

func TestStartReturnsInitializationErrorsWithoutAsyncNotification(t *testing.T) {
	initErr := errors.New("initialize unavailable")
	openErr := errors.New("no input device")
	startErr := errors.New("stream start rejected")

	tests := []struct {
		name          string
		tempDir       func(*testing.T) string
		backend       func() *fakeAudioBackend
		wantErr       error
		wantMessage   string
		wantTerminate int
	}{
		{
			name:    "initialize",
			tempDir: func(t *testing.T) string { return t.TempDir() },
			backend: func() *fakeAudioBackend {
				return &fakeAudioBackend{initializeErr: initErr, stream: &fakeAudioStream{}}
			},
			wantErr:       initErr,
			wantMessage:   "portaudio init failed",
			wantTerminate: 0,
		},
		{
			name:    "open stream",
			tempDir: func(t *testing.T) string { return t.TempDir() },
			backend: func() *fakeAudioBackend {
				return &fakeAudioBackend{openErr: openErr, stream: &fakeAudioStream{}}
			},
			wantErr:       openErr,
			wantMessage:   "open stream failed",
			wantTerminate: 1,
		},
		{
			name:    "start stream",
			tempDir: func(t *testing.T) string { return t.TempDir() },
			backend: func() *fakeAudioBackend {
				return &fakeAudioBackend{stream: &fakeAudioStream{startErr: startErr}}
			},
			wantErr:       startErr,
			wantMessage:   "start stream failed",
			wantTerminate: 1,
		},
		{
			name: "create wav",
			tempDir: func(t *testing.T) string {
				return filepath.Join(t.TempDir(), "missing", "directory")
			},
			backend: func() *fakeAudioBackend {
				return &fakeAudioBackend{stream: &fakeAudioStream{}}
			},
			wantMessage:   "create wav failed",
			wantTerminate: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			backend := tt.backend()
			recorder := newRecorder(testConfig(), tt.tempDir(t), backend)
			notified := make(chan error, 1)
			recorder.SetErrorHandler(func(err error) { notified <- err })

			err := recorder.Start(context.Background())
			if err == nil || !strings.Contains(err.Error(), tt.wantMessage) {
				t.Fatalf("Start error = %v, want message containing %q", err, tt.wantMessage)
			}
			if tt.wantErr != nil && !errors.Is(err, tt.wantErr) {
				t.Fatalf("Start error = %v, want wrapped error %v", err, tt.wantErr)
			}
			if got := recorder.State(); got != StateIdle {
				t.Fatalf("State = %v, want StateIdle", got)
			}
			if got := backend.terminateCount(); got != tt.wantTerminate {
				t.Fatalf("Terminate calls = %d, want %d before Start returns", got, tt.wantTerminate)
			}
			select {
			case got := <-notified:
				t.Fatalf("startup error was also sent asynchronously: %v", got)
			default:
			}
		})
	}
}

func TestConsecutiveReadErrorsEndRecordingAndNotify(t *testing.T) {
	readErr := errors.New("input device disconnected")
	readErrs := make([]error, maxConsecutiveReadErrors)
	for i := range readErrs {
		readErrs[i] = readErr
	}
	stream := &fakeAudioStream{readErrs: readErrs}
	recorder := newRecorder(testConfig(), t.TempDir(), &fakeAudioBackend{stream: stream})
	notified := make(chan error, 1)
	recorder.SetErrorHandler(func(err error) { notified <- err })

	if err := recorder.Start(context.Background()); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	select {
	case err := <-notified:
		if !errors.Is(err, readErr) {
			t.Fatalf("notification error = %v, want wrapped error %v", err, readErr)
		}
		if !strings.Contains(err.Error(), "10 consecutive errors") {
			t.Fatalf("notification error = %v, want consecutive error count", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("recording error was not reported")
	}

	if got := recorder.State(); got != StateIdle {
		t.Fatalf("State = %v, want StateIdle", got)
	}
	_, _, _, reads := stream.counts()
	if reads != maxConsecutiveReadErrors {
		t.Fatalf("Read calls = %d, want %d", reads, maxConsecutiveReadErrors)
	}
	matches, err := filepath.Glob(filepath.Join(recorder.tempDir, "RecordTemp_*.wav"))
	if err != nil {
		t.Fatalf("Glob failed: %v", err)
	}
	if len(matches) != 0 {
		t.Fatalf("partial WAV files were not removed: %v", matches)
	}
}

func TestRecordingKeepsErrorHandlerCapturedAtStart(t *testing.T) {
	readErr := errors.New("input device disconnected")
	readErrs := make([]error, maxConsecutiveReadErrors)
	for i := range readErrs {
		readErrs[i] = readErr
	}
	stream := &fakeAudioStream{
		readErrs:    readErrs,
		readEntered: make(chan struct{}),
		readRelease: make(chan struct{}),
	}
	recorder := newRecorder(testConfig(), t.TempDir(), &fakeAudioBackend{stream: stream})
	firstHandler := make(chan error, 1)
	secondHandler := make(chan error, 1)
	recorder.SetErrorHandler(func(err error) { firstHandler <- err })

	if err := recorder.Start(context.Background()); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	select {
	case <-stream.readEntered:
	case <-time.After(time.Second):
		t.Fatal("recording did not enter stream.Read")
	}

	recorder.SetErrorHandler(func(err error) { secondHandler <- err })
	close(stream.readRelease)

	select {
	case err := <-firstHandler:
		if !errors.Is(err, readErr) {
			t.Fatalf("first handler error = %v, want wrapped error %v", err, readErr)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("handler captured at Start did not receive the recording error")
	}
	select {
	case err := <-secondHandler:
		t.Fatalf("handler installed during recording received the old session error: %v", err)
	default:
	}
}

func TestSuccessfulReadResetsConsecutiveErrorCount(t *testing.T) {
	readErr := errors.New("temporary overflow")
	readErrs := make([]error, 0, 2*maxConsecutiveReadErrors)
	for range maxConsecutiveReadErrors - 1 {
		readErrs = append(readErrs, readErr)
	}
	readErrs = append(readErrs, nil)
	for range maxConsecutiveReadErrors - 1 {
		readErrs = append(readErrs, readErr)
	}
	readErrs = append(readErrs, nil)
	readDone := make(chan struct{})
	stream := &fakeAudioStream{readErrs: readErrs, readDone: readDone}
	recorder := newRecorder(testConfig(), t.TempDir(), &fakeAudioBackend{stream: stream})
	notified := make(chan error, 1)
	recorder.SetErrorHandler(func(err error) { notified <- err })

	if err := recorder.Start(context.Background()); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	select {
	case <-readDone:
	case <-time.After(2 * time.Second):
		t.Fatal("stream did not complete the transient read sequence")
	}

	res, err := recorder.Stop()
	if err != nil {
		t.Fatalf("Stop failed after transient errors: %v", err)
	}
	defer os.Remove(res.WavPath)
	select {
	case got := <-notified:
		t.Fatalf("transient read errors unexpectedly terminated recording: %v", got)
	default:
	}
}

func TestNormalStopAndCancelDoNotNotifyErrorHandler(t *testing.T) {
	tests := []struct {
		name string
		stop func(*Recorder) (Result, error)
	}{
		{name: "stop", stop: (*Recorder).Stop},
		{name: "cancel", stop: (*Recorder).Cancel},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := newRecorder(testConfig(), t.TempDir(), &fakeAudioBackend{stream: &fakeAudioStream{}})
			notified := make(chan error, 1)
			recorder.SetErrorHandler(func(err error) { notified <- err })

			if err := recorder.Start(context.Background()); err != nil {
				t.Fatalf("Start failed: %v", err)
			}
			res, err := tt.stop(recorder)
			if err != nil {
				t.Fatalf("%s failed: %v", tt.name, err)
			}
			if tt.name == "cancel" && !res.Canceled {
				t.Fatalf("Cancel result = %#v, want Canceled=true", res)
			}
			if res.WavPath != "" {
				defer os.Remove(res.WavPath)
			}
			select {
			case got := <-notified:
				t.Fatalf("normal %s unexpectedly notified an error: %v", tt.name, got)
			default:
			}
		})
	}
}
