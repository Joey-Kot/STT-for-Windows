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

package ffmpeg

import (
	"reflect"
	"testing"

	"stt/internal/config"
)

func TestSettingsForAppliesDefaultsAndMapsCodec(t *testing.T) {
	cfg := config.Config{CODECS: "MP3"}
	settings, err := settingsFor(cfg, 44100)
	if err != nil {
		t.Fatalf("settingsFor failed: %v", err)
	}

	if settings.CodecKey != "mp3" || settings.FFCodec != "libmp3lame" || !settings.CodecHasBitrate {
		t.Fatalf("unexpected codec settings: %#v", settings)
	}
	if settings.Channels != 1 || settings.SampleRate != 44100 || settings.Bitrate != 128 || settings.Depth != 16 || settings.SampleFormat != "s16" {
		t.Fatalf("unexpected defaults: %#v", settings)
	}
}

func TestSettingsForRejectsUnsupportedCodec(t *testing.T) {
	_, err := settingsFor(config.Config{CODECS: "not-real"}, 16000)
	if err == nil {
		t.Fatalf("settingsFor succeeded, want unsupported codec error")
	}
}

func TestFFmpegArgsForBitrateCodec(t *testing.T) {
	settings := conversionSettings{
		FFCodec:         "libopus",
		CodecHasBitrate: true,
		Channels:        2,
		SampleRate:      48000,
		Bitrate:         64,
		SampleFormat:    "s16",
	}
	got := ffmpegArgsFor(settings, "in.wav", "out.ogg")
	want := []string{"-y", "-i", "in.wav", "-ac", "2", "-ar", "48000", "-c:a", "libopus", "-b:a", "64k", "-sample_fmt", "s16", "out.ogg"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ffmpegArgsFor = %#v, want %#v", got, want)
	}
}

func TestFFmpegArgsForPCMCodecOmitsBitrateAndSampleFormat(t *testing.T) {
	settings := conversionSettings{
		FFCodec:         "pcm_s16le",
		CodecHasBitrate: true,
		Channels:        1,
		SampleRate:      16000,
		Bitrate:         64,
		SampleFormat:    "s16",
	}
	got := ffmpegArgsFor(settings, "in.wav", "out.wav")
	want := []string{"-y", "-i", "in.wav", "-ac", "1", "-ar", "16000", "-c:a", "pcm_s16le", "out.wav"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ffmpegArgsFor = %#v, want %#v", got, want)
	}
}

func TestSampleFormatForDepth(t *testing.T) {
	tests := map[int]string{8: "u8", 16: "s16", 24: "s24", 32: "s32", 12: ""}
	for depth, want := range tests {
		if got := sampleFormatForDepth(depth); got != want {
			t.Fatalf("sampleFormatForDepth(%d) = %q, want %q", depth, got, want)
		}
	}
}

func TestFFmpegCodecFor(t *testing.T) {
	tests := []struct {
		key     string
		codec   string
		bitrate bool
	}{
		{key: "opus", codec: "libopus", bitrate: true},
		{key: "VORB", codec: "libvorbis", bitrate: true},
		{key: "flac", codec: "flac", bitrate: false},
		{key: "pcm_s24le", codec: "pcm_s24le", bitrate: false},
		{key: "unknown", codec: "", bitrate: false},
	}
	for _, tt := range tests {
		codec, bitrate := ffmpegCodecFor(tt.key)
		if codec != tt.codec || bitrate != tt.bitrate {
			t.Fatalf("ffmpegCodecFor(%q) = (%q, %v), want (%q, %v)", tt.key, codec, bitrate, tt.codec, tt.bitrate)
		}
	}
}
