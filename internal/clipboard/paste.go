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

package clipboard

import (
	"context"
	"errors"
	"fmt"
	"syscall"
	"time"
)

const (
	// These fixed margins target ordinary short-text pastes. The transaction
	// does not attempt readiness detection for unusually large clipboard
	// payloads or application-specific paste latency.
	clipboardWriteDelay   = 80 * time.Millisecond
	clipboardRestoreDelay = 120 * time.Millisecond
)

type pasteOperations struct {
	readAll   func() (string, error)
	writeAll  func(string) error
	sendPaste func() error
	sleep     func(time.Duration)
	wait      func(context.Context, time.Duration) error
}

// RestoreError reports that the temporary clipboard contents could not be
// replaced with the contents saved before the paste operation.
type RestoreError struct {
	PasteSent bool
	Err       error
}

func (e *RestoreError) Error() string {
	if e.PasteSent {
		return fmt.Sprintf("paste shortcut sent, but restoring clipboard failed: %v", e.Err)
	}
	return fmt.Sprintf("restoring clipboard failed: %v", e.Err)
}

// Unwrap returns the underlying clipboard error.
func (e *RestoreError) Unwrap() error {
	return e.Err
}

func pasteText(text string, ops pasteOperations) error {
	return pasteTextContext(context.Background(), text, ops)
}

func pasteTextContext(ctx context.Context, text string, ops pasteOperations) (err error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	original, readErr := ops.readAll()
	if readErr = normalizeClipboardError(readErr); readErr != nil {
		return fmt.Errorf("read original clipboard: %w", readErr)
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	pasteSent := false
	// Restoration is intentionally unconditional. On Windows, the text-only
	// adapter maps an empty or non-text clipboard to "", which can leave a
	// cosmetic blank item in clipboard history. That known behavior is retained
	// instead of trying to infer or manipulate the separately managed history.
	defer func() {
		restoreErr := ops.writeAll(original)
		if restoreErr == nil {
			return
		}

		restoreFailure := &RestoreError{PasteSent: pasteSent, Err: restoreErr}
		if err == nil {
			err = restoreFailure
			return
		}
		err = errors.Join(err, restoreFailure)
	}()

	if writeErr := ops.writeAll(text); writeErr != nil {
		return fmt.Errorf("write paste text to clipboard: %w", writeErr)
	}

	if waitErr := waitForPaste(ctx, clipboardWriteDelay, ops); waitErr != nil {
		return waitErr
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if sendErr := ops.sendPaste(); sendErr != nil {
		return fmt.Errorf("send paste shortcut: %w", sendErr)
	}
	pasteSent = true

	if waitErr := waitForPaste(ctx, clipboardRestoreDelay, ops); waitErr != nil {
		return waitErr
	}
	return nil
}

func waitForPaste(ctx context.Context, delay time.Duration, ops pasteOperations) error {
	if ops.wait != nil {
		return ops.wait(ctx, delay)
	}
	if ops.sleep != nil {
		ops.sleep(delay)
	}
	return ctx.Err()
}

func normalizeClipboardError(err error) error {
	if err == nil {
		return nil
	}

	// syscall.Proc.Call always returns a non-nil error on Windows. Some
	// clipboard-library paths therefore surface Errno(0), which indicates
	// that no Win32 error occurred and must not abort the paste operation.
	var errno syscall.Errno
	if errors.As(err, &errno) && errno == 0 {
		return nil
	}
	return err
}
