# STT compatibility contract

This document records the behavior preserved by the Rust implementation. The
release binaries are `stt.exe` and `STT.exe`.

## Deliberately removed behavior

The Rust implementation removes these former public features:

- `beeep` and all Windows notifications;
- the `NOTIFICATION` configuration field when serializing or saving;
- the `--notification` CLI argument.

The legacy `TEXT_PATH` dot-path syntax and automatic field fallback have also
been replaced by standard JSONPath with exactly-one-result validation.

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
| `TEXT_PATH` | `"$.text"` | Standard JSONPath selecting exactly one scalar; no fallback |
| `ExtraConfig` | `""` | Must be a JSON object when non-empty |
| `OPACITY` | `1.0` | GUI floating-window opacity; `0.10`–`1.00` in `0.01` steps, where `1.0` is fully opaque |
| `WINDOW_SCALE` | `1.0` | GUI floating-window scale; `0.3`–`2.0` in `0.1` steps, shared by full and minimal modes |
| `INPUT_DEVICE` | `""` | Stable Windows capture endpoint ID; empty follows the system default |
| `INPUT_DEVICE_NAME` | `""` | Display-only cached name, not used for device resolution |
| `CHANNELS` | `1` | Inclusive range 1–8; output channels only |
| `SAMPLING_RATE` | `16000` | Final upload rate, greater than zero |
| `ENABLE_VAD` | `false` | Shared by GUI, CLI recording and CLI file mode |
| `VAD_PADDING_MS` | `100` | Integer 0–1000 milliseconds; validated even while VAD is off |
| `VAD_START_THRESHOLD` | `0.6` | Range 0.5–1.0 (inclusive); validated even while VAD is off |
| `SAMPLING_RATE_DEPTH` | `16` | 8, 16, 24, or 32 |
| `BIT_RATE` | `128` | Greater than zero |
| `CODECS` | `"opus"` | Existing alias list, case-insensitive |
| `CONTAINER` | `"opus"` | Existing container list, case-insensitive |
| `REQUEST_TIMEOUT` | `60` | Seconds; positive values set the client timeout |
| `MAX_RETRY` | `3` | Attempt limit, including the first request |
| `RETRY_BASE_DELAY` | `0.5` | Seconds, doubled after each failure |
| `ENABLE_HTTP2` | `true` | Explicit HTTP/2 control |
| `VERIFY_SSL` | `true` | Explicit TLS certificate validation control |
| `HOTKEY_HOOK` | `true` | Low-level hook when true, RegisterHotKey otherwise |
| `START_KEY` | `"ctrl+alt+q"` | Start/stop recording |
| `PAUSE_KEY` | `"ctrl+alt+s"` | Pause/resume |
| `CANCEL_OR_RETRY_KEY` | `"alt+esc"` | Cancel recording/request, or retry the buffered recording while idle |
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

The configured path uses standard JSONPath through `serde_json_path`, for example
`$.results[0].alternatives[0].transcript`. The default is `$.text`; legacy paths
without the root identifier are not supported. Configuration validation rejects
empty or malformed paths before any request, and each ASR client reuses its parsed
path. Queries must match exactly one node. String, number, and boolean results
are converted to text; objects, arrays, and null are errors. An empty string is a
successful extraction and produces no paste in GUI/hotkey mode.

Invalid JSON responses, zero matches, multiple matches (with their count), and
unsupported value types produce explicit errors without fallback or automatic
upload retries. The original HTTP 200 response remains available for caching,
even if it is not valid JSON. CLI file mode returns an error without writing the
transcription file. See the README for selector examples and result semantics.

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

The default clipboard channel uses `keybd_event` for `Ctrl+V`. The exact v1.1.2 sequence is:

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

The current optional `USE_SENDINPUT` setting supersedes the original prohibition
on SendInput. It defaults to false, including in older configuration files.
When true, shared core output uses SendInput with KEYEVENTF_UNICODE, without
clipboard access, clipboard delays, fallback, or automatic delivery retries.
This applies to transcriptions and `[request failed]` text in GUI and CLI hotkey
mode. CLI `--use-sendinput <BOOL>` overrides the setting. GUI Hotkeys exposes a
checkbox below Restore delay and disables both clipboard delay controls while
selected, retaining their values. Unicode scalars stay intact across batches;
line endings normalize to CR, and tabs remain Unicode characters. Partial
delivery and cancellation after delivery report that text may already exist.

Hotkeys reject unknown or repeated modifiers and normalize aliases/casing for
duplicate detection. RegisterHotKey mode uses `MOD_NOREPEAT`, a dedicated
message thread, `WM_QUIT`, and unregisters all bindings. Hook mode uses
`WH_KEYBOARD_LL`, ignores `LLKHF_INJECTED`, checks only required modifiers with
`GetAsyncKeyState`, swallows held-repeat keydown and its matching keyup, and
does not forbid extra modifiers. `CANCEL_OR_RETRY_KEY` cancels while recording
or uploading; when idle with a buffered recording, it retries that recording.

## Shared WASAPI recorder

- Core enumerates active capture endpoints and opens them by stable ID. Empty
  `INPUT_DEVICE` resolves the current system default at each recording start.
- GUI and CLI share the configuration and backend. `--list-input-devices` exits
  before configuration lookup; `--input-device ID|default` overrides this run only.
- A fixed unavailable endpoint fails without switching devices. Device names
  and enumeration indices are never used as identifiers.
- WASAPI shared mode prefers the endpoint's Windows-configured default format;
  if unavailable or unsupported, use the same endpoint's engine mix format.
- Capture sample rate, precision and channels are independent of output settings.
  Temporary WAVs preserve effective precision and channel layout; integer
  padding bytes may be packed out losslessly for decoder compatibility.
- Reads return whole interleaved packets in the actual capture format. The COM
  apartment and interfaces are owned by the recorder thread.
- Start returns only after initialize, open, stream start, and WAV creation.
- Pause stops capture and polls state every ~100 ms; resume resets old buffered
  packets before restarting the stream.
- Ten consecutive read errors terminate recording, with ~10 ms between errors;
  a successful read resets the count.
- Available packets are drained promptly; empty reads wait ~10 ms.
- Stop finalizes the WAV; cancel/lifecycle cancellation removes it.
- Shutdown cancellation is non-blocking.

## Conversion

Both frontends use `stt-core::embedded_ffmpeg::EmbeddedFfmpegConverter` through
`prepare_audio_for_upload`. Neither searches PATH nor launches FFmpeg.
Release builds enable `stt-core/static-libav`; builds without native libraries
remain usable for type checks and report LibAvUnavailable on conversion.

The native bridge streams arbitrary supported inputs to 16 kHz mono int16
callbacks for a fresh Earshot 1.2.2 detector per input. Frames contain 256 samples;
the final partial frame is zero-padded, but interval endpoints exclude padding.
Segmentation requires three consecutive frames at or above VAD_START_THRESHOLD
(default 0.6), with up to six candidate frames of lookback including confirmation.
Before activation, a frame below both the start and continuation thresholds clears
the candidate. Continuation uses 0.5; at least four continuation-level frames are
required per segment, and ten silent frames close it. EOF flushes active speech.
The lookback voice count excludes discarded frames. GUI and CLI pass the shared
core configuration to the detector.

Half-open intervals count frames per channel. Starts map down and ends map up
from 16 kHz using u128 arithmetic. Empty intervals are removed; sorted overlapping
intervals merge, clamp to decoded length and receive padding. The first and last
boundaries receive full padding; internal boundaries share one padding, splitting
floor(ms/2) after the preceding segment and the remainder before the next.
Gaps at most one padding remain intact. No fixed interval count limit exists.

The final pass always decodes the original input once, selects intervals with a
forward-only cursor and av_samples_copy, then resamples, queues and encodes.
Packed and planar audio are supported. Output PTS starts at zero and remains
continuous. No libavfilter, filtergraph or intermediate analysis/cropped WAV is
used. Any native failure fails the entire operation and deletes partial output.
Source format changes during decoding fail explicitly.

A blocking worker owns callback state; cancellation is polled during packet,
frame and interval processing and via AVIO interrupt callbacks. Cleanup waits
until native output is closed. Identical input/output paths are rejected.

Input support includes WAV/PCM, MP3, FLAC, Ogg/Opus, Ogg/Vorbis, AAC/M4A/MP4,
ALAC/M4A, WebM/Matroska, WavPack, AC3/EAC3. The build explicitly selects
demuxers, decoders and parsers plus file protocol.

With VAD off, no analysis or trimming occurs. With VAD on and no speech, no ASR
request or text output is created; temporary audio is removed, retry state is
cleared and GUI/hotkey mode returns to Idle. CLI file mode reports the outcome
and exits successfully. KEEP_CACHE otherwise retains original audio, final
audio and successful responses under the existing rules. Both manual retry and
automatic HTTP retry with VAD enabled reanalyze and reconvert the original;
disabled VAD preserves existing HTTP retry behavior.

## Runtime state machine

Visible states are Idle, Recording, Paused, Uploading, and Error. Normal actions
use one action lock. GUI and hotkey events use a non-blocking try-lock and are
dropped when busy; they never queue for a later state. Upload
cancellation bypasses that lock and cancels the active request token directly.

- Idle/Error: start is allowed.
- Idle with a retry buffer: `CANCEL_OR_RETRY_KEY` retries the buffered WAV.
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

GUI and CLI hotkey mode retain the latest completed WAV in memory for retry,
up to 100,000,000 bytes. Except for the no-speech result, the buffer survives failed, successful, and manually
canceled requests, is replaced only by another completed recording, and is
released during shutdown. Retry attempts recreate a temporary WAV and do not
create a persistent retry cache.

Shutdown cancels the lifecycle, requests recorder cancellation without waiting,
unregisters hotkeys, and waits at most about 250 ms for the action lock.

## Native GUI

The GUI uses a Win32 message loop, Direct2D/DirectWrite, native controls,
`Shell_NotifyIconW`, and `ITaskbarList.AddTab/DeleteTab`. It contains no
WebView or embedded browser runtime.

- Full window: 222×94 logical pixels.
- Minimal window: 170×46 logical pixels.
- Settings window: 760×620 logical pixels.
- Frameless, per-pixel-alpha layered, always on top, per-monitor DPI aware.
- Drag threshold is about 4 px; dragging minimal mode from the microphone does
  not activate recording on release.
- Minimal hides settings/status and removes the taskbar tab.
- Full and minimal floating panels always use the same 10 logical pixel Direct2D
  continuous-corner profile, with a fully inset 1 px outline. The layered HWND
  is composed from Direct2D premultiplied-alpha pixels on Windows 10 and Windows
  11, preserving antialiased edges; native DWM rounding and the Windows 11 border
  are disabled so no second outer shape is added.
- Tray menu remains Minimal, Settings, Quit and emits no balloon.
- Audio settings expose ENABLE_VAD, VAD_PADDING_MS and VAD_START_THRESHOLD in all five languages.
  Padding and start threshold are disabled while VAD is off, retain their values and are validated on save.
- Settings use native tab/edit/button/checkbox/combobox controls. Token is a
  password edit. Its outer frame uses the same 10 logical pixel continuous-corner
  profile as the floating panels while preserving native child controls. Display
  languages are English, Simplified Chinese, German, Japanese, and French.
- The Display page controls the shared opacity and scale of the full and minimal
  floating windows.
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
