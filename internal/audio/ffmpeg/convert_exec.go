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

//go:build !gui_ffmpeg_cgo

package ffmpeg

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"

	"stt/internal/config"
)

// Convert converts input audio into the configured codec/container.
func Convert(cfg config.Config, inPath, outPath string, rate int) error {
	settings, err := settingsFor(cfg, rate)
	if err != nil {
		return err
	}
	args := ffmpegArgsFor(settings, inPath, outPath)

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
