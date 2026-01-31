package ffmpeg

import (
	"bytes"
	"fmt"
	"os/exec"
	"strconv"
	"strings"

	"stt/internal/config"
)

// Convert converts input audio into the configured codec/container.
func Convert(cfg config.Config, inPath, outPath string, rate int) error {
	codecKey := strings.ToLower(cfg.CODECS)
	channels := cfg.Channels
	if channels <= 0 {
		channels = 1
	}
	sr := cfg.SAMPLING_RATE
	if sr <= 0 {
		sr = rate
	}
	bitrate := cfg.BIT_RATE
	if bitrate <= 0 {
		bitrate = 128
	}
	depth := cfg.SAMPLING_RATE_DEPTH
	if depth == 0 {
		depth = 16
	}

	ffCodec, codecHasBitrate := ffmpegCodecFor(codecKey)
	if ffCodec == "" {
		return fmt.Errorf("unsupported codec: %s", cfg.CODECS)
	}

	args := []string{"-y", "-i", inPath, "-ac", strconv.Itoa(channels), "-ar", strconv.Itoa(sr)}
	if strings.HasPrefix(ffCodec, "pcm_") {
		args = append(args, "-c:a", ffCodec)
	} else {
		args = append(args, "-c:a", ffCodec)
		if codecHasBitrate {
			args = append(args, "-b:a", fmt.Sprintf("%dk", bitrate))
		}
		switch depth {
		case 8:
			args = append(args, "-sample_fmt", "u8")
		case 16:
			args = append(args, "-sample_fmt", "s16")
		case 24:
			args = append(args, "-sample_fmt", "s24")
		case 32:
			args = append(args, "-sample_fmt", "s32")
		}
	}

	args = append(args, outPath)

	if cfg.FFMPEG_DEBUG {
		fmt.Printf("[ffmpeg] executing: ffmpeg %s\n", strings.Join(args, " "))
	}
	cmd := exec.Command("ffmpeg", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("ffmpeg failed: %v\n%s", err, stderr.String())
	}
	return nil
}

func ffmpegCodecFor(key string) (string, bool) {
	k := strings.ToLower(key)
	switch k {
	case "opus", "libopus":
		return "libopus", true
	case "wavpack":
		return "wavpack", false
	case "aac":
		return "aac", true
	case "ac3":
		return "ac3", true
	case "eac3":
		return "eac3", true
	case "mp3":
		return "libmp3lame", true
	case "mp2":
		return "mp2", true
	case "mp1":
		return "mp1", true
	case "flac":
		return "flac", false
	case "alac":
		return "alac", false
	case "pcm":
		return "pcm_s16le", false
	case "vorbis", "libvorbis", "vorb":
		return "libvorbis", true
	case "adpcm":
		return "adpcm_ms", false
	case "amr":
		return "libopencore_amrnb", true
	case "pcm_f32be", "pcm_f32le", "pcm_f64be", "pcm_f64le",
		"pcm_s16be", "pcm_s16le", "pcm_s24be", "pcm_s24le",
		"pcm_s32be", "pcm_s32le", "pcm_s64be", "pcm_s64le",
		"pcm_s8":
		return k, false
	default:
		return "", false
	}
}
