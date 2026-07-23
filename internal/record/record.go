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
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/go-audio/audio"
	"github.com/go-audio/wav"
	"github.com/google/uuid"
	"github.com/gordonklaus/portaudio"

	"stt/internal/config"
)

// State represents recorder state.
type State int

const (
	StateIdle State = iota
	StateRecording
	StatePaused
	StateStopping
	StateCanceled
)

const (
	maxConsecutiveReadErrors = 10
	readErrorRetryDelay      = 10 * time.Millisecond
)

// Result is returned when a recording completes or is canceled.
type Result struct {
	WavPath  string
	Canceled bool
	Err      error
}

type audioStream interface {
	Start() error
	Stop() error
	Close() error
	Read() error
}

type audioBackend interface {
	Initialize() error
	Terminate() error
	OpenDefaultStream(inputChannels, outputChannels int, sampleRate float64, framesPerBuffer int, input []int16) (audioStream, error)
}

type portAudioBackend struct{}

func (portAudioBackend) Initialize() error {
	return portaudio.Initialize()
}

func (portAudioBackend) Terminate() error {
	return portaudio.Terminate()
}

func (portAudioBackend) OpenDefaultStream(inputChannels, outputChannels int, sampleRate float64, framesPerBuffer int, input []int16) (audioStream, error) {
	return portaudio.OpenDefaultStream(inputChannels, outputChannels, sampleRate, framesPerBuffer, input)
}

// Recorder manages PortAudio recording and streaming WAV writing.
type Recorder struct {
	mu                 sync.Mutex
	state              State
	cfg                config.Config
	tempDir            string
	wavPath            string
	stopCtx            context.Context
	stopCancel         context.CancelFunc
	done               chan Result
	backend            audioBackend
	ready              bool
	onError            func(error)
	activeErrorHandler func(error)
}

// New creates a recorder.
func New(cfg config.Config, tempDir string) *Recorder {
	return newRecorder(cfg, tempDir, portAudioBackend{})
}

func newRecorder(cfg config.Config, tempDir string, backend audioBackend) *Recorder {
	return &Recorder{cfg: cfg, tempDir: tempDir, state: StateIdle, backend: backend}
}

// SetErrorHandler registers a callback for errors that terminate an active
// recording without a matching Stop or Cancel request. Start captures the
// current handler for the lifetime of that recording session.
func (r *Recorder) SetErrorHandler(handler func(error)) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.onError = handler
}

// Start initializes recording resources and returns once audio capture is ready.
func (r *Recorder) Start(ctx context.Context) error {
	r.mu.Lock()
	if r.state != StateIdle {
		r.mu.Unlock()
		return fmt.Errorf("recorder not idle")
	}
	r.state = StateRecording
	r.ready = false
	r.activeErrorHandler = r.onError
	r.done = make(chan Result, 1)
	r.stopCtx, r.stopCancel = context.WithCancel(ctx)
	stopCtx := r.stopCtx
	done := r.done
	started := make(chan error, 1)
	r.mu.Unlock()

	go r.recordLoop(stopCtx, done, started)
	return <-started
}

// Stop requests a clean stop and waits for completion.
func (r *Recorder) Stop() (Result, error) {
	r.mu.Lock()
	if r.state != StateRecording && r.state != StatePaused {
		r.mu.Unlock()
		return Result{}, fmt.Errorf("recorder not running")
	}
	r.state = StateStopping
	cancel := r.stopCancel
	done := r.done
	r.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	res := <-done
	return res, res.Err
}

// Cancel requests immediate stop and cleanup, waits for completion.
func (r *Recorder) Cancel() (Result, error) {
	r.mu.Lock()
	if r.state != StateRecording && r.state != StatePaused {
		r.mu.Unlock()
		return Result{}, fmt.Errorf("recorder not running")
	}
	r.state = StateCanceled
	cancel := r.stopCancel
	done := r.done
	r.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	res := <-done
	return res, res.Err
}

// TogglePause toggles pause/resume.
func (r *Recorder) TogglePause() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.state != StateRecording && r.state != StatePaused {
		return fmt.Errorf("recorder not running")
	}
	if r.state == StatePaused {
		r.state = StateRecording
	} else {
		r.state = StatePaused
	}
	return nil
}

// State returns the current recorder state.
func (r *Recorder) State() State {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.state
}

func (r *Recorder) recordLoop(stopCtx context.Context, done chan<- Result, started chan<- error) {
	wavPath := r.generateTempWav()
	r.mu.Lock()
	r.wavPath = wavPath
	r.mu.Unlock()

	if r.cfg.RECORD_DEBUG {
		fmt.Printf("[record] starting, writing to %s\n", wavPath)
	}

	if err := r.backend.Initialize(); err != nil {
		r.finishStart(done, started, Result{WavPath: wavPath, Err: fmt.Errorf("portaudio init failed: %w", err)}, false)
		return
	}

	in := make([]int16, 1024)
	stream, err := r.backend.OpenDefaultStream(r.cfg.Channels, 0, float64(r.cfg.SAMPLING_RATE), len(in), in)
	if err != nil {
		r.finishStart(done, started, Result{WavPath: wavPath, Err: fmt.Errorf("open stream failed: %w", err)}, true)
		return
	}
	if err := stream.Start(); err != nil {
		_ = stream.Close()
		r.finishStart(done, started, Result{WavPath: wavPath, Err: fmt.Errorf("start stream failed: %w", err)}, true)
		return
	}

	file, err := os.Create(wavPath)
	if err != nil {
		_ = stream.Stop()
		_ = stream.Close()
		r.finishStart(done, started, Result{WavPath: wavPath, Err: fmt.Errorf("create wav failed: %w", err)}, true)
		return
	}
	enc := wav.NewEncoder(file, r.cfg.SAMPLING_RATE, 16, r.cfg.Channels, 1)
	format := &audio.Format{NumChannels: r.cfg.Channels, SampleRate: r.cfg.SAMPLING_RATE}
	intBuf := make([]int, len(in))

	r.mu.Lock()
	r.ready = true
	r.mu.Unlock()
	started <- nil

	consecutiveReadErrors := 0

	for {
		if r.isCanceled() {
			break
		}
		if r.isPaused() {
			time.Sleep(100 * time.Millisecond)
			continue
		}
		select {
		case <-stopCtx.Done():
			goto done
		default:
		}

		if err := stream.Read(); err != nil {
			consecutiveReadErrors++
			if r.cfg.RECORD_DEBUG {
				fmt.Printf("[record] stream read error (%d/%d): %v\n", consecutiveReadErrors, maxConsecutiveReadErrors, err)
			}
			if consecutiveReadErrors >= maxConsecutiveReadErrors {
				_ = enc.Close()
				_ = file.Close()
				_ = stream.Stop()
				_ = stream.Close()
				_ = os.Remove(wavPath)
				r.finish(done, Result{WavPath: wavPath, Err: fmt.Errorf("stream read failed after %d consecutive errors: %w", consecutiveReadErrors, err)}, true)
				return
			}
			select {
			case <-stopCtx.Done():
				goto done
			case <-time.After(readErrorRetryDelay):
			}
			continue
		}
		consecutiveReadErrors = 0
		for i, v := range in {
			intBuf[i] = int(v)
		}
		buf := &audio.IntBuffer{Format: format, Data: intBuf[:len(in)], SourceBitDepth: 16}
		if err := enc.Write(buf); err != nil {
			_ = enc.Close()
			_ = file.Close()
			_ = stream.Stop()
			_ = stream.Close()
			_ = os.Remove(wavPath)
			r.finish(done, Result{WavPath: wavPath, Err: fmt.Errorf("wav write failed: %w", err)}, true)
			return
		}
		time.Sleep(10 * time.Millisecond)
	}

done:
	_ = stream.Stop()
	_ = stream.Close()

	if r.isCanceled() {
		_ = enc.Close()
		_ = file.Close()
		_ = os.Remove(wavPath)
		r.finish(done, Result{WavPath: "", Canceled: true}, true)
		return
	}

	if err := enc.Close(); err != nil {
		_ = file.Close()
		_ = os.Remove(wavPath)
		r.finish(done, Result{WavPath: wavPath, Err: fmt.Errorf("wav close failed: %w", err)}, true)
		return
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(wavPath)
		r.finish(done, Result{WavPath: wavPath, Err: fmt.Errorf("wav file close failed: %w", err)}, true)
		return
	}

	r.finish(done, Result{WavPath: wavPath}, true)
}

func (r *Recorder) finishStart(done chan<- Result, started chan<- error, res Result, initialized bool) {
	r.finish(done, res, initialized)
	started <- res.Err
}

func (r *Recorder) finish(done chan<- Result, res Result, initialized bool) {
	if initialized {
		_ = r.backend.Terminate()
	}

	r.mu.Lock()
	previousState := r.state
	ready := r.ready
	handler := r.activeErrorHandler
	r.state = StateIdle
	r.ready = false
	r.activeErrorHandler = nil
	r.stopCtx = nil
	r.stopCancel = nil
	r.mu.Unlock()

	done <- res
	if res.Err != nil && ready && (previousState == StateRecording || previousState == StatePaused) && handler != nil {
		handler(res.Err)
	}
}

func (r *Recorder) isPaused() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.state == StatePaused
}

func (r *Recorder) isCanceled() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.state == StateCanceled
}

func (r *Recorder) generateTempWav() string {
	id := strings.ReplaceAll(uuid.New().String(), "-", "")[:16]
	base := fmt.Sprintf("RecordTemp_%s.wav", id)
	dir := r.tempDir
	if dir == "" {
		cwd, _ := os.Getwd()
		dir = cwd
	}
	return filepath.Join(dir, base)
}
