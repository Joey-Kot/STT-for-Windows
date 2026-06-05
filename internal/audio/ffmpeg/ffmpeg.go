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

package ffmpeg

import (
	"fmt"
	"strconv"
	"strings"

	"stt/internal/config"
)

type conversionSettings struct {
	CodecKey        string
	FFCodec         string
	CodecHasBitrate bool
	Channels        int
	SampleRate      int
	Bitrate         int
	Depth           int
	SampleFormat    string
}

func settingsFor(cfg config.Config, rate int) (conversionSettings, error) {
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
		return conversionSettings{}, fmt.Errorf("unsupported codec: %s", cfg.CODECS)
	}

	settings := conversionSettings{
		CodecKey:        codecKey,
		FFCodec:         ffCodec,
		CodecHasBitrate: codecHasBitrate,
		Channels:        channels,
		SampleRate:      sr,
		Bitrate:         bitrate,
		Depth:           depth,
	}
	if !strings.HasPrefix(ffCodec, "pcm_") {
		settings.SampleFormat = sampleFormatForDepth(depth)
	}
	return settings, nil
}

func ffmpegArgsFor(settings conversionSettings, inPath, outPath string) []string {
	args := []string{"-y", "-i", inPath, "-ac", strconv.Itoa(settings.Channels), "-ar", strconv.Itoa(settings.SampleRate), "-c:a", settings.FFCodec}
	if !strings.HasPrefix(settings.FFCodec, "pcm_") {
		if settings.CodecHasBitrate {
			args = append(args, "-b:a", fmt.Sprintf("%dk", settings.Bitrate))
		}
		if settings.SampleFormat != "" {
			args = append(args, "-sample_fmt", settings.SampleFormat)
		}
	}
	return append(args, outPath)
}

func sampleFormatForDepth(depth int) string {
	switch depth {
	case 8:
		return "u8"
	case 16:
		return "s16"
	case 24:
		return "s24"
	case 32:
		return "s32"
	default:
		return ""
	}
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
