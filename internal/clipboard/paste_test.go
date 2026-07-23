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
	"reflect"
	"syscall"
	"testing"
	"time"
)

func TestPasteTextContextStopsBeforeClipboardAccessWhenCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	called := false
	ops := pasteOperations{
		readAll: func() (string, error) {
			called = true
			return "", nil
		},
	}

	err := pasteTextContext(ctx, "transcription", ops)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
	if called {
		t.Fatal("clipboard was accessed after cancellation")
	}
}

func TestPasteTextContextStopsBeforeWritingWhenCanceledAfterRead(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	writeCalled := false
	ops := pasteOperations{
		readAll: func() (string, error) {
			cancel()
			return "original", nil
		},
		writeAll: func(string) error {
			writeCalled = true
			return nil
		},
	}

	err := pasteTextContext(ctx, "transcription", ops)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
	if writeCalled {
		t.Fatal("clipboard was written after cancellation")
	}
}

func TestPasteTextContextCancelsBeforeSendingAndRestoresClipboard(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	var writes []string
	sendCalled := false
	ops := pasteOperations{
		readAll: func() (string, error) { return "original", nil },
		writeAll: func(text string) error {
			writes = append(writes, text)
			return nil
		},
		sendPaste: func() error {
			sendCalled = true
			return nil
		},
		wait: func(context.Context, time.Duration) error {
			cancel()
			return ctx.Err()
		},
	}

	err := pasteTextContext(ctx, "transcription", ops)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
	if sendCalled {
		t.Fatal("paste shortcut was sent after cancellation")
	}
	wantWrites := []string{"transcription", "original"}
	if !reflect.DeepEqual(writes, wantWrites) {
		t.Fatalf("writes = %#v, want %#v", writes, wantWrites)
	}
}

func TestPasteTextContextRestoresWhenCanceledAfterSending(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	var writes []string
	waits := 0
	ops := pasteOperations{
		readAll: func() (string, error) { return "original", nil },
		writeAll: func(text string) error {
			writes = append(writes, text)
			return nil
		},
		sendPaste: func() error { return nil },
		wait: func(context.Context, time.Duration) error {
			waits++
			if waits == 2 {
				cancel()
				return ctx.Err()
			}
			return nil
		},
	}

	err := pasteTextContext(ctx, "transcription", ops)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
	wantWrites := []string{"transcription", "original"}
	if !reflect.DeepEqual(writes, wantWrites) {
		t.Fatalf("writes = %#v, want %#v", writes, wantWrites)
	}
}

func TestPasteTextWritesSendsAndRestoresInOrder(t *testing.T) {
	var calls []string
	var sleeps []time.Duration
	ops := pasteOperations{
		readAll: func() (string, error) {
			calls = append(calls, "read")
			return "original", nil
		},
		writeAll: func(text string) error {
			calls = append(calls, "write:"+text)
			return nil
		},
		sendPaste: func() error {
			calls = append(calls, "send")
			return nil
		},
		sleep: func(delay time.Duration) {
			calls = append(calls, "sleep")
			sleeps = append(sleeps, delay)
		},
	}

	if err := pasteText("transcription", ops); err != nil {
		t.Fatalf("pasteText failed: %v", err)
	}

	wantCalls := []string{"read", "write:transcription", "sleep", "send", "sleep", "write:original"}
	if !reflect.DeepEqual(calls, wantCalls) {
		t.Fatalf("calls = %#v, want %#v", calls, wantCalls)
	}
	wantSleeps := []time.Duration{clipboardWriteDelay, clipboardRestoreDelay}
	if !reflect.DeepEqual(sleeps, wantSleeps) {
		t.Fatalf("sleeps = %#v, want %#v", sleeps, wantSleeps)
	}
}

func TestPasteTextStopsWhenClipboardReadFails(t *testing.T) {
	readErr := errors.New("read failed")
	writeCalled := false
	sendCalled := false
	ops := pasteOperations{
		readAll: func() (string, error) { return "", readErr },
		writeAll: func(string) error {
			writeCalled = true
			return nil
		},
		sendPaste: func() error {
			sendCalled = true
			return nil
		},
		sleep: func(time.Duration) {},
	}

	err := pasteText("transcription", ops)
	if !errors.Is(err, readErr) {
		t.Fatalf("error = %v, want wrapped read error", err)
	}
	if writeCalled || sendCalled {
		t.Fatalf("writeCalled=%v sendCalled=%v, want both false", writeCalled, sendCalled)
	}
}

func TestPasteTextRestoresAndStopsWhenClipboardWriteFails(t *testing.T) {
	writeErr := errors.New("write failed")
	writes := 0
	sendCalled := false
	ops := pasteOperations{
		readAll: func() (string, error) { return "original", nil },
		writeAll: func(string) error {
			writes++
			if writes == 1 {
				return writeErr
			}
			return nil
		},
		sendPaste: func() error {
			sendCalled = true
			return nil
		},
		sleep: func(time.Duration) {},
	}

	err := pasteText("transcription", ops)
	if !errors.Is(err, writeErr) {
		t.Fatalf("error = %v, want wrapped write error", err)
	}
	if writes != 2 || sendCalled {
		t.Fatalf("writes=%d sendCalled=%v, want failed write, restore, and no send", writes, sendCalled)
	}
}

func TestPasteTextPreservesInitialWriteAndRestoreErrors(t *testing.T) {
	writeErr := errors.New("write failed")
	restoreErr := errors.New("restore failed")
	writes := 0
	sendCalled := false
	ops := pasteOperations{
		readAll: func() (string, error) { return "original", nil },
		writeAll: func(string) error {
			writes++
			if writes == 1 {
				return writeErr
			}
			return restoreErr
		},
		sendPaste: func() error {
			sendCalled = true
			return nil
		},
		sleep: func(time.Duration) {},
	}

	err := pasteText("transcription", ops)
	if !errors.Is(err, writeErr) || !errors.Is(err, restoreErr) {
		t.Fatalf("error = %v, want both write and restore errors", err)
	}
	var typedRestoreErr *RestoreError
	if !errors.As(err, &typedRestoreErr) {
		t.Fatalf("error = %v, want RestoreError", err)
	}
	if typedRestoreErr.PasteSent {
		t.Fatalf("PasteSent = true after write failure, want false")
	}
	if writes != 2 || sendCalled {
		t.Fatalf("writes=%d sendCalled=%v, want failed write, failed restore, and no send", writes, sendCalled)
	}
}

func TestPasteTextRestoresClipboardWhenSendingFails(t *testing.T) {
	sendErr := errors.New("send failed")
	var writes []string
	ops := pasteOperations{
		readAll: func() (string, error) { return "original", nil },
		writeAll: func(text string) error {
			writes = append(writes, text)
			return nil
		},
		sendPaste: func() error { return sendErr },
		sleep:     func(time.Duration) {},
	}

	err := pasteText("transcription", ops)
	if !errors.Is(err, sendErr) {
		t.Fatalf("error = %v, want wrapped send error", err)
	}
	wantWrites := []string{"transcription", "original"}
	if !reflect.DeepEqual(writes, wantWrites) {
		t.Fatalf("writes = %#v, want %#v", writes, wantWrites)
	}
}

func TestPasteTextPreservesSendAndRestoreErrors(t *testing.T) {
	sendErr := errors.New("send failed")
	restoreErr := errors.New("restore failed")
	writes := 0
	ops := pasteOperations{
		readAll: func() (string, error) { return "original", nil },
		writeAll: func(string) error {
			writes++
			if writes == 2 {
				return restoreErr
			}
			return nil
		},
		sendPaste: func() error { return sendErr },
		sleep:     func(time.Duration) {},
	}

	err := pasteText("transcription", ops)
	if !errors.Is(err, sendErr) || !errors.Is(err, restoreErr) {
		t.Fatalf("error = %v, want both send and restore errors", err)
	}
	var typedRestoreErr *RestoreError
	if !errors.As(err, &typedRestoreErr) {
		t.Fatalf("error = %v, want RestoreError", err)
	}
	if typedRestoreErr.PasteSent {
		t.Fatalf("PasteSent = true after send failure, want false")
	}
}

func TestPasteTextReportsRestoreFailureAfterPasteWasSent(t *testing.T) {
	restoreErr := errors.New("restore failed")
	writes := 0
	ops := pasteOperations{
		readAll: func() (string, error) { return "original", nil },
		writeAll: func(string) error {
			writes++
			if writes == 2 {
				return restoreErr
			}
			return nil
		},
		sendPaste: func() error { return nil },
		sleep:     func(time.Duration) {},
	}

	err := pasteText("transcription", ops)
	if !errors.Is(err, restoreErr) {
		t.Fatalf("error = %v, want wrapped restore error", err)
	}
	var typedRestoreErr *RestoreError
	if !errors.As(err, &typedRestoreErr) {
		t.Fatalf("error = %v, want RestoreError", err)
	}
	if !typedRestoreErr.PasteSent {
		t.Fatalf("PasteSent = false after successful send, want true")
	}
}

func TestPasteTextIgnoresZeroWindowsErrno(t *testing.T) {
	var writes []string
	ops := pasteOperations{
		readAll: func() (string, error) { return "original", syscall.Errno(0) },
		writeAll: func(text string) error {
			writes = append(writes, text)
			return nil
		},
		sendPaste: func() error { return nil },
		sleep:     func(time.Duration) {},
	}

	if err := pasteText("transcription", ops); err != nil {
		t.Fatalf("pasteText failed for Errno(0): %v", err)
	}
	wantWrites := []string{"transcription", "original"}
	if !reflect.DeepEqual(writes, wantWrites) {
		t.Fatalf("writes = %#v, want %#v", writes, wantWrites)
	}
}
