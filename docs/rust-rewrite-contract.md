# STT compatibility contract

This document records the behavior preserved by the Rust implementation. The
release binaries are `stt.exe` and `STT.exe`.

## Deliberately removed behavior

The Rust implementation removes these former public features:

- `beeep` and all Windows notifications;
- the `NOTIFICATION` configuration field when serializing or saving;
- the `--notification` CLI argument.

Old JSON files containing `NOTIFICATION` remain readable because unknown
fields are ignored. `REQUEST_FAILED_NOTIFICATION` remains supported and means
“paste `[request failed]` after retries are exhausted”; it is not a system
notification setting.

## Configuration

Configuration precedence is CLI overrides, then the selected JSON file, then
defaults. Missing fields receive defaults and unknown fields are ignored.

| JSON field | Default | Validation / behavior |
|---|---:|---|
| `API_ENDPOINT` | `""` | Required at upload time |
| `TOKEN` | `""` | Sent as Bearer token when non-empty |
| `MODEL` | `""` | Multipart field only when non-empty |
| `LANGUAGE` | `""` | Multipart field only when non-empty |
| `PROMPT` | `""` | Multipart field only when non-empty |
| `TEXT_PATH` | `"text"` | Dot path with repeated array indexes |
| `ExtraConfig` | `""` | Must be a JSON object when non-empty |
| `CHANNELS` | `1` | Inclusive range 1–8 |
| `SAMPLING_RATE` | `16000` | Greater than zero |
| `SAMPLING_RATE_DEPTH` | `16` | 8, 16, 24, or 32 |
| `BIT_RATE` | `32` | Greater than zero |
| `CODECS` | `"opus"` | Existing alias list, case-insensitive |
| `CONTAINER` | `"ogg"` | Existing container list, case-insensitive |
| `REQUEST_TIMEOUT` | `60` | Seconds; positive values set the client timeout |
| `MAX_RETRY` | `3` | Attempt limit, including the first request |
| `RETRY_BASE_DELAY` | `0.5` | Seconds, doubled after each failure |
| `ENABLE_HTTP2` | `true` | Explicit HTTP/2 control |
| `VERIFY_SSL` | `true` | Explicit TLS certificate validation control |
| `HOTKEY_HOOK` | `true` | Low-level hook when true, RegisterHotKey otherwise |
| `START_KEY` | `"ctrl+alt+q"` | Start/stop recording |
| `PAUSE_KEY` | `"ctrl+alt+s"` | Pause/resume |
| `CANCEL_KEY` | `"alt+esc"` | Cancel recording |
| `CLIPBOARD_WRITE_DELAY` | `80` | Milliseconds between writing text and sending Ctrl+V |
| `CLIPBOARD_RESTORE_DELAY` | `120` | Milliseconds between Ctrl+V and clipboard restoration |
| `CACHE_DIR` | `""` | Empty falls back to current directory |
| `KEEP_CACHE` | `false` | Effective only with a non-empty usable cache dir |
| `REQUEST_FAILED_NOTIFICATION` | `false` | Paste failure placeholder after retry exhaustion |
| debug flags | existing values | FFmpeg false, record false, hotkey true, upload false |

The Rust CLI uses only standard long options. Boolean options require an
explicit `true` or `false`. `--rate` is a hidden-compatible alias for
`--sampling-rate`; old single-dash long forms are not accepted. Long help is
organized into General, API, Audio, Network, Hotkeys, Cache, and Debug groups.

## TEXT_PATH

The configured path supports dot-separated object keys and any number of
array indexes in a token, for example
`results[0].alternatives[0].transcript`. String, number, and boolean results
are converted to text. A failed configured path falls back to top-level
`text`, then to any non-empty top-level string.

## HTTP and multipart

- Each retry reopens the file and reconstructs the multipart body.
- The file field is `file`; its uploaded filename is the local basename.
- `model`, `language`, and `prompt` are omitted when empty.
- `ExtraConfig` shallowly overrides base fields; `null` removes a base field.
- Nested extra values are serialized as compact JSON strings.
- User-Agent remains `stt-go-client/1.0`.
- Only HTTP 200 succeeds; the original response body is retained.
- Requests and exponential retry waits are cancellable.
- System proxies, redirects, and automatic compression are disabled.
- HTTP/2 and TLS verification follow their explicit settings.

## Cache and filenames

The cache path is made absolute and created when possible. Failure clears the
setting and uses the current directory. Startup removes every file or directory
whose name starts with `RecordTemp_` in the active temporary directory.

Temporary names are `RecordTemp_<16 hex chars>.<ext>`. Recorded WAV and
converted audio are renamed to `audio-YYYY-MM-DD-HH.MM.SS.<ext>` when caching
is enabled. If both are WAV, the converted file receives `_convert`. A response
JSON is saved only after an HTTP-200 upload. Otherwise temporary audio is
removed.

## Keyboard, clipboard, and hotkeys

`Ctrl+V` uses `keybd_event`, never `SendInput`. The exact v1.1.2 sequence is:

1. virtual-key Ctrl down: `(0x11, 0x91, 0)`;
2. scan-code V down: `(47, 175, KEYEVENTF_SCANCODE)`;
3. virtual-key Ctrl up: `(0x11, 0x91, KEYEVENTF_KEYUP)`;
4. scan-code V up: `(47, 175, KEYEVENTF_KEYUP | KEYEVENTF_SCANCODE)`.

Clipboard transport is `CF_UNICODETEXT`. Opening waits for up to one second
and the open/close transaction stays on one OS thread. The runtime reads the
old text, writes the transcription, waits `CLIPBOARD_WRITE_DELAY` milliseconds,
sends Ctrl+V, waits `CLIPBOARD_RESTORE_DELAY` milliseconds, and unconditionally
tries to restore the old text. The defaults remain 80 ms and 120 ms. Both waits
are cancellable. “Paste sent; clipboard restore failed” remains distinct from
pre-paste failure.

Hotkeys reject unknown or repeated modifiers and normalize aliases/casing for
duplicate detection. RegisterHotKey mode uses `MOD_NOREPEAT`, a dedicated
message thread, `WM_QUIT`, and unregisters all bindings. Hook mode uses
`WH_KEYBOARD_LL`, ignores `LLKHF_INJECTED`, checks only required modifiers with
`GetAsyncKeyState`, swallows held-repeat keydown and its matching keyup, and
does not forbid extra modifiers.

## PortAudio recorder

- PortAudio C blocking API; no WASAPI implementation.
- Initialize for each recording and terminate after it.
- Default input device, configured channels/rate, interleaved signed int16.
- Buffer length remains 1024 samples; frames are buffer length divided by
  channel count.
- WAV is always PCM 16-bit. `SAMPLING_RATE_DEPTH` affects conversion only.
- Start returns only after initialize, open, stream start, and WAV creation.
- Pause does not call `Pa_ReadStream`; it polls state every ~100 ms.
- Ten consecutive read errors terminate recording, with ~10 ms between errors;
  a successful read resets the count.
- A successful WAV write retains the existing ~10 ms delay.
- Stop finalizes the WAV; cancel/lifecycle cancellation removes it.
- Shutdown cancellation is non-blocking.

## Conversion

The CLI executes `ffmpeg.exe` from `PATH` with the existing argument order:
`-y -i INPUT -ac CHANNELS -ar RATE -c:a CODEC`, optional bitrate and sample
format, then output. Cancellation terminates the child process. Input and output
paths may not be equal.

The GUI links `native/ffmpeg_bridge.c` and calls only its C ABI. It checks
cancellation before and after the synchronous call and never searches for or
starts `ffmpeg.exe`.

## Runtime state machine

Visible states are Idle, Recording, Paused, Uploading, and Error. Normal actions
use one action lock. GUI and hotkey events use a non-blocking try-lock and are
dropped when busy; they never queue for a later state. Upload
cancellation bypasses that lock and cancels the active request token directly.

- Idle/Error: start is allowed.
- Recording/Paused: stop and cancel are allowed.
- Pause outside recording is silent except debug output.
- Uploading: cancel is allowed; other inputs are dropped.

Stop flow is WAV finalize, Uploading, conversion, ASR, extraction, clipboard
paste, cache handling, then Idle or Error. Empty ASR text goes to Idle without
an `[empty result]` paste. Only exhausted retries plus
`REQUEST_FAILED_NOTIFICATION=true` paste `[request failed]`. Manual request
cancellation signals the active conversion/upload pipeline, immediately aborts
ASR upload, response, and retry waits, performs cache cleanup, and returns to
Idle without reporting an upload failure.

Shutdown cancels the lifecycle, requests recorder cancellation without waiting,
unregisters hotkeys, and waits at most about 250 ms for the action lock.

## Native GUI

The GUI uses a Win32 message loop, Direct2D/DirectWrite, native controls,
`Shell_NotifyIconW`, and `ITaskbarList.AddTab/DeleteTab`. It contains no
WebView or embedded browser runtime.

- Full window: 222×94 logical pixels.
- Minimal window: 170×46 logical pixels.
- Settings window: 760×620 logical pixels.
- Frameless, transparent/color-keyed, always on top, per-monitor DPI aware.
- Drag threshold is about 4 px; dragging minimal mode from the microphone does
  not activate recording on release.
- Minimal hides settings/status and removes the taskbar tab.
- Full and minimal floating panels always use the same 8 logical pixel Direct2D
  corner radius and request rounded DWM corners; Windows versions without that
  DWM attribute keep the app-drawn shape.
- Tray menu remains Minimal, Settings, Quit and emits no balloon.
- Settings use native tab/edit/button/checkbox/combobox controls. Token is a
  password edit. Display languages are English, Simplified Chinese, German,
  Japanese, and French.
- Config writes `%APPDATA%\stt\config.json`, then validates and reloads runtime
  dependencies/hotkeys. Saving is allowed only in Idle or Error.
- Escape closes settings first; busy quit shows a native confirmation dialog.

## Verification split

Cross-platform unit tests cover config, JSON path, request fields, cache,
hotkey parsing, clipboard transaction, recorder failure/pause/cancel behavior,
conversion arguments, CLI parsing, and runtime action dropping. The MinGW
build verifies Windows API signatures and static linkage. Real Windows 10/11
manual verification remains required for audio hardware, foreground paste,
low-level hooks, taskbar behavior, DWM corners, tray interaction, and high-DPI
visual comparison.
