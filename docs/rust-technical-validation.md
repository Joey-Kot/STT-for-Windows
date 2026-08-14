# Rust technical validation record

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
- The GUI import table contains `keybd_event` and no `SendInput` import.
- CI checks source/binary markers for `SendInput`, notifications, external
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
- full/minimal sizing, 4 px drag threshold, tray synchronization, taskbar tab;
- Windows 11 rounded corners versus Windows 10 square behavior;
- per-monitor high-DPI movement and rendering;
- settings tabs, password token edit, five display languages, save/reload;
- cancel button and cancel hotkey abort a blocked ASR request and return to Idle;
- busy quit confirmation and bounded shutdown during upload/libav/driver work.

The native GUI dimensions are 222×94 for the full window, 170×46 for minimal
mode, and 760×620 for the settings window.
