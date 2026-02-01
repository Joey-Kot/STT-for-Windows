package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Config holds configurable parameters.
type Config struct {
	APIEndpoint               string  `json:"API_ENDPOINT"`
	Token                     string  `json:"TOKEN"`
	Model                     string  `json:"MODEL"`
	Language                  string  `json:"LANGUAGE"`
	Prompt                    string  `json:"PROMPT"`
	TEXTPath                  string  `json:"TEXT_PATH"`
	ExtraConfig               string  `json:"ExtraConfig"`
	Channels                  int     `json:"CHANNELS"`
	SAMPLING_RATE             int     `json:"SAMPLING_RATE"`
	SAMPLING_RATE_DEPTH       int     `json:"SAMPLING_RATE_DEPTH"`
	BIT_RATE                  int     `json:"BIT_RATE"`
	CODECS                    string  `json:"CODECS"`
	CONTAINER                 string  `json:"CONTAINER"`
	RequestTimeout            int     `json:"REQUEST_TIMEOUT"`
	MaxRetry                  int     `json:"MAX_RETRY"`
	RetryBaseDelay            float64 `json:"RETRY_BASE_DELAY"`
	EnableHTTP2               bool    `json:"ENABLE_HTTP2"`
	VerifySSL                 bool    `json:"VERIFY_SSL"`
	HotKeyHook                bool    `json:"HOTKEY_HOOK"`
	StartKey                  string  `json:"START_KEY"`
	PauseKey                  string  `json:"PAUSE_KEY"`
	CancelKey                 string  `json:"CANCEL_KEY"`
	CacheDir                  string  `json:"CACHE_DIR"`
	KeepCache                 bool    `json:"KEEP_CACHE"`
	Notification              bool    `json:"NOTIFICATION"`
	RequestFailedNotification bool    `json:"REQUEST_FAILED_NOTIFICATION"`
	FFMPEG_DEBUG              bool    `json:"FFMPEG_DEBUG"`
	RECORD_DEBUG              bool    `json:"RECORD_DEBUG"`
	HOTKEY_DEBUG              bool    `json:"HOTKEY_DEBUG"`
	UPLOAD_DEBUG              bool    `json:"UPLOAD_DEBUG"`
}

// DefaultConfig returns a Config with default values.
func DefaultConfig() Config {
	return Config{
		APIEndpoint:               "",
		Token:                     "",
		Model:                     "",
		Language:                  "",
		Prompt:                    "",
		TEXTPath:                  "text",
		ExtraConfig:               "",
		Channels:                  1,
		SAMPLING_RATE:             16000,
		SAMPLING_RATE_DEPTH:       16,
		BIT_RATE:                  128,
		CODECS:                    "opus",
		CONTAINER:                 "ogg",
		RequestTimeout:            30,
		MaxRetry:                  3,
		RetryBaseDelay:            0.5,
		EnableHTTP2:               true,
		VerifySSL:                 true,
		HotKeyHook:                false,
		StartKey:                  "alt+q",
		PauseKey:                  "alt+s",
		CancelKey:                 "esc",
		CacheDir:                  "",
		KeepCache:                 false,
		Notification:              false,
		RequestFailedNotification: false,
		FFMPEG_DEBUG:              false,
		RECORD_DEBUG:              false,
		HOTKEY_DEBUG:              true,
		UPLOAD_DEBUG:              false,
	}
}

// Load loads config from JSON file if provided.
func Load(path string) (Config, error) {
	cfg := DefaultConfig()
	if path == "" {
		return cfg, nil
	}
	f, err := os.Open(path)
	if err != nil {
		return cfg, err
	}
	defer f.Close()
	dec := json.NewDecoder(f)
	if err := dec.Decode(&cfg); err != nil {
		return cfg, err
	}
	return cfg, nil
}

// SaveDefault writes a default config JSON to the provided path.
func SaveDefault(path string) error {
	cfg := DefaultConfig()
	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0644)
}

// Validate verifies config fields and returns an error if any value is invalid.
func Validate(cfg *Config) error {
	if cfg.Channels < 1 || cfg.Channels > 8 {
		return fmt.Errorf("invalid Channels: %d (allowed 1..8)", cfg.Channels)
	}
	if cfg.SAMPLING_RATE <= 0 {
		return fmt.Errorf("invalid SAMPLING_RATE: %d (must be > 0)", cfg.SAMPLING_RATE)
	}
	allowedDepth := map[int]bool{8: true, 16: true, 24: true, 32: true}
	if !allowedDepth[cfg.SAMPLING_RATE_DEPTH] {
		return fmt.Errorf("invalid SAMPLING_RATE_DEPTH: %d (allowed: 8,16,24,32)", cfg.SAMPLING_RATE_DEPTH)
	}
	if cfg.BIT_RATE <= 0 {
		return fmt.Errorf("invalid BIT_RATE: %d (must be > 0)", cfg.BIT_RATE)
	}

	allowedCodecs := map[string]bool{
		"opus":      true,
		"libopus":   true,
		"wavpack":   true,
		"aac":       true,
		"ac3":       true,
		"eac3":      true,
		"mp3":       true,
		"mp2":       true,
		"mp1":       true,
		"flac":      true,
		"alac":      true,
		"pcm":       true,
		"vorbis":    true,
		"libvorbis": true,
		"vorb":      true,
		"adpcm":     true,
		"amr":       true,
		"pcm_f32be": true,
		"pcm_f32le": true,
		"pcm_f64be": true,
		"pcm_f64le": true,
		"pcm_s16be": true,
		"pcm_s16le": true,
		"pcm_s24be": true,
		"pcm_s24le": true,
		"pcm_s32be": true,
		"pcm_s32le": true,
		"pcm_s64be": true,
		"pcm_s64le": true,
		"pcm_s8":    true,
	}
	if !allowedCodecs[strings.ToLower(cfg.CODECS)] {
		return fmt.Errorf("invalid CODECS: %s (allowed: OPUS, LIBOPUS, WAVPACK, AAC, AC3, EAC3, MP3, MP2, MP1, FLAC, ALAC, PCM, VORBIS, LIBVORBIS, VORB, ADPCM, AMR, PCM_F32BE, PCM_F32LE, PCM_F64BE, PCM_F64LE, PCM_S16BE, PCM_S16LE, PCM_S24BE, PCM_S24LE, PCM_S32BE, PCM_S32LE, PCM_S64BE, PCM_S64LE, PCM_S8)", cfg.CODECS)
	}

	allowedContainers := map[string]bool{
		"wav":   true,
		"ac3":   true,
		"ac4":   true,
		"ogg":   true,
		"oga":   true,
		"mp3":   true,
		"flac":  true,
		"eac3":  true,
		"aac":   true,
		"m4a":   true,
		"mp4":   true,
		"opus":  true,
		"webm":  true,
		"s8":    true,
		"s16be": true,
		"s16le": true,
		"s24be": true,
		"s24le": true,
		"s32be": true,
		"s32le": true,
		"f32be": true,
		"f32le": true,
		"f64be": true,
		"f64le": true,
	}
	if !allowedContainers[strings.ToLower(cfg.CONTAINER)] {
		return fmt.Errorf("invalid CONTAINER: %s (allowed: WAV, AC3, AC4, OGG, OGA, MP3, FLAC, EAC3, AAC, M4A, MP4, OPUS, WEBM, S8, S16BE, S16LE, S24BE, S24LE, S32BE, S32LE, F32BE, F32LE, F64BE, F64LE)", cfg.CONTAINER)
	}
	return nil
}

// InitCacheDir validates/creates the configured cache directory.
// It mutates cfg.CacheDir to an absolute path or clears it on failure.
func InitCacheDir(cfg *Config) {
	if cfg.CacheDir == "" {
		return
	}
	abs, err := filepath.Abs(cfg.CacheDir)
	if err != nil {
		fmt.Printf("[main] cache-dir path invalid '%s': %v. Falling back to cwd.\n", cfg.CacheDir, err)
		cfg.CacheDir = ""
		return
	}
	info, err := os.Stat(abs)
	if err == nil {
		if !info.IsDir() {
			fmt.Printf("[main] cache-dir '%s' exists but is not a directory. Falling back to cwd.\n", abs)
			cfg.CacheDir = ""
			return
		}
		cfg.CacheDir = abs
		fmt.Printf("[main] using existing cache-dir: %s\n", cfg.CacheDir)
		return
	}
	if os.IsNotExist(err) {
		if err := os.MkdirAll(abs, 0755); err != nil {
			fmt.Printf("[main] cannot create cache-dir '%s': %v. Falling back to cwd.\n", abs, err)
			cfg.CacheDir = ""
			return
		}
		cfg.CacheDir = abs
		fmt.Printf("[main] created and using cache-dir: %s\n", cfg.CacheDir)
		return
	}
	fmt.Printf("[main] cannot access cache-dir '%s': %v. Falling back to cwd.\n", abs, err)
	cfg.CacheDir = ""
}

// TempDir returns the directory to use for temporary files.
func TempDir(cfg *Config) string {
	if cfg.CacheDir != "" {
		return cfg.CacheDir
	}
	cwd, _ := os.Getwd()
	return cwd
}

// ContainerExt maps container names to file extensions (lowercase).
func ContainerExt(container string) string {
	c := strings.ToLower(container)
	switch c {
	case "wav":
		return "wav"
	case "ac3":
		return "ac3"
	case "ac4":
		return "ac4"
	case "ogg":
		return "ogg"
	case "oga":
		return "oga"
	case "mp3":
		return "mp3"
	case "flac":
		return "flac"
	case "eac3":
		return "eac3"
	case "aac":
		return "aac"
	case "m4a":
		return "m4a"
	case "mp4":
		return "mp4"
	case "opus":
		return "opus"
	case "webm":
		return "webm"
	case "s8", "s16be", "s16le", "s24be", "s24le", "s32be", "s32le",
		"f32be", "f32le", "f64be", "f64le":
		return c
	default:
		if c == "" {
			return "ogg"
		}
		return c
	}
}
