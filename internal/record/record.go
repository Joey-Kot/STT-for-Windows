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

// Result is returned when a recording completes or is canceled.
type Result struct {
	WavPath  string
	Canceled bool
	Err      error
}

// Recorder manages PortAudio recording and streaming WAV writing.
type Recorder struct {
	mu         sync.Mutex
	state      State
	cfg        config.Config
	tempDir    string
	wavPath    string
	stopCtx    context.Context
	stopCancel context.CancelFunc
	done       chan Result
}

// New creates a recorder.
func New(cfg config.Config, tempDir string) *Recorder {
	return &Recorder{cfg: cfg, tempDir: tempDir, state: StateIdle}
}

// Start begins recording.
func (r *Recorder) Start(ctx context.Context) error {
	r.mu.Lock()
	if r.state != StateIdle {
		r.mu.Unlock()
		return fmt.Errorf("recorder not idle")
	}
	r.state = StateRecording
	r.done = make(chan Result, 1)
	r.stopCtx, r.stopCancel = context.WithCancel(ctx)
	r.mu.Unlock()

	go r.recordLoop()
	return nil
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

func (r *Recorder) recordLoop() {
	wavPath := r.generateTempWav()
	r.wavPath = wavPath

	if r.cfg.RECORD_DEBUG {
		fmt.Printf("[record] starting, writing to %s\n", wavPath)
	}

	if err := portaudio.Initialize(); err != nil {
		r.finish(Result{WavPath: wavPath, Err: fmt.Errorf("portaudio init failed: %w", err)})
		return
	}
	defer portaudio.Terminate()

	in := make([]int16, 1024)
	stream, err := portaudio.OpenDefaultStream(r.cfg.Channels, 0, float64(r.cfg.SAMPLING_RATE), len(in), in)
	if err != nil {
		r.finish(Result{WavPath: wavPath, Err: fmt.Errorf("open stream failed: %w", err)})
		return
	}
	if err := stream.Start(); err != nil {
		_ = stream.Close()
		r.finish(Result{WavPath: wavPath, Err: fmt.Errorf("start stream failed: %w", err)})
		return
	}

	file, err := os.Create(wavPath)
	if err != nil {
		_ = stream.Stop()
		_ = stream.Close()
		r.finish(Result{WavPath: wavPath, Err: fmt.Errorf("create wav failed: %w", err)})
		return
	}
	enc := wav.NewEncoder(file, r.cfg.SAMPLING_RATE, 16, r.cfg.Channels, 1)
	format := &audio.Format{NumChannels: r.cfg.Channels, SampleRate: r.cfg.SAMPLING_RATE}
	intBuf := make([]int, len(in))

	for {
		if r.isCanceled() {
			break
		}
		if r.isPaused() {
			time.Sleep(100 * time.Millisecond)
			continue
		}
		select {
		case <-r.stopCtx.Done():
			goto done
		default:
		}

		if err := stream.Read(); err != nil {
			if r.cfg.RECORD_DEBUG {
				fmt.Printf("[record] stream read error: %v\n", err)
			}
			continue
		}
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
			r.finish(Result{WavPath: wavPath, Err: fmt.Errorf("wav write failed: %w", err)})
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
		r.finish(Result{WavPath: "", Canceled: true})
		return
	}

	if err := enc.Close(); err != nil {
		_ = file.Close()
		_ = os.Remove(wavPath)
		r.finish(Result{WavPath: wavPath, Err: fmt.Errorf("wav close failed: %w", err)})
		return
	}
	_ = file.Close()

	r.finish(Result{WavPath: wavPath})
}

func (r *Recorder) finish(res Result) {
	r.mu.Lock()
	r.state = StateIdle
	r.mu.Unlock()
	r.done <- res
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
