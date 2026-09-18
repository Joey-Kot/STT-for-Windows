# Rust technical validation record

## Optional SendInput channel validation

- Linux: 52 tests pass (46 core, 6 CLI), including Unicode batching, partial/zero
  injection, cancellation, channel selection and configuration overrides.
- Formatting and workspace Clippy with warnings denied pass.
- An isolated Windows-target crate importing the actual core `keyboard.rs`
  passes type-checking with windows 0.61.3, including SendInput and modifier APIs.
- Full Windows cross-check was attempted but blocked by the missing
  `x86_64-w64-mingw32-gcc` compiler required by `ring`. Windows GUI behavior and
  release binaries have not been validated for this change on this Linux host.
  Downloading isolated MinGW packages was rejected by the tool approval policy.

The following automated validation record describes the earlier rewrite.

## Automated validation completed

- `cargo test --workspace`: 38 behavioral tests pass on Linux
  (34 core tests and 4 CLI tests).
- `cargo fmt --all --check` and
  `cargo clippy --workspace --all-targets -- -D warnings` pass.
- `cargo check --workspace --target x86_64-pc-windows-gnu --features
  stt-gui/native-gui`: Win32, Direct2D/DirectWrite, clipboard, hotkey, and
  PortAudio FFI signatures type-check with the GNU Windows target.
- PortAudio v19.7.0 was cross-built using MinGW-w64. Its configuration summary
  reported `WMME=yes` and `WASAPI=no`.
- FFmpeg n7.1.1 and the Ogg, Vorbis, Opus 1.5.2, LAME 3.100, and
  OpenCore AMR 0.1.6 dependencies were cross-built as static MinGW archives.
- The libav bridge is built only for the GUI `static-libav` feature. A real
  release link completed for both `stt.exe` and `STT.exe`.
- PE inspection reports the CLI as Windows CUI and the GUI as Windows GUI.
  The GUI imports no PortAudio/libav DLL and contains the static libav backend
  marker but no external `ffmpeg.exe` marker; the CLI has the inverse markers.
- The original GUI import table contained only `keybd_event`; the optional
  Unicode input channel now requires `SendInput` in both CLI and GUI. CI checks both imports.
- CI checks source/binary markers for notifications, external
  FFmpeg in the GUI, and dynamic PortAudio/libav DLL imports.

## Windows hardware validation checklist

These checks cannot be proven on the Ubuntu build host and must be run on real
Windows 10 and Windows 11 systems:

- blocking PortAudio recording on the default device;
- playable WAV output and pause intervals containing no captured samples;
- static libav representative conversions: Ogg/Opus, MP3, FLAC, AAC, PCM;
- RegisterHotKey and WH_KEYBOARD_LL behavior, held-key suppression, and focus;
- Ctrl+V paste into multiple foreground controls, configurable wait intervals,
  and clipboard restoration;
- optional SendInput in GUI and CLI: Notepad, browser, VS Code, terminal and chat
  fields; Chinese, emoji, CR/LF, tabs and long text; preserve clipboard text,
  images and files; modifier release, elevated targets, partial input and cancel;
- SendInput checkbox save/reopen, both delay controls disabled while enabled,
  retained delay values, five languages and DPI layout. These require Windows
  desktop validation; cross-compilation does not prove application compatibility;
- full/minimal sizing, 4 px drag threshold, tray synchronization, taskbar tab;
- matching antialiased, per-pixel-alpha self-drawn rounded corners on Windows 10
  and Windows 11 for both floating and settings windows, with no native
  rectangular border or second DWM corner layer;
- per-monitor high-DPI movement and rendering;
- settings tabs, password token edit, five display languages, save/reload;
- cancel button and Cancel or Retry hotkey abort a blocked ASR request and return to Idle; verify the same hotkey retries the buffered recording while idle in both GUI and CLI;
- busy quit confirmation and bounded shutdown during upload/libav/driver work.

The native GUI dimensions are 222×94 for the full window, 170×46 for minimal
mode, and 760×620 for the settings window.
