#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TARGET="${TARGET:-x86_64-pc-windows-gnu}"
MINGW_TARGET="${MINGW_TARGET:-x86_64-w64-mingw32}"
PORTAUDIO_PREFIX="${PORTAUDIO_PREFIX:-$ROOT_DIR/build/portaudio-win}"
FFMPEG_PREFIX="${FFMPEG_PREFIX:-$ROOT_DIR/build/ffmpeg-win}"

export CARGO_TARGET_X86_64_PC_WINDOWS_GNU_LINKER="$MINGW_TARGET-gcc"
export CC_x86_64_pc_windows_gnu="$MINGW_TARGET-gcc"
export AR_x86_64_pc_windows_gnu="$MINGW_TARGET-ar"
export PKG_CONFIG_ALLOW_CROSS=1
export PKG_CONFIG_PATH="$PORTAUDIO_PREFIX/lib/pkgconfig:$FFMPEG_PREFIX/lib/pkgconfig"
export STT_REQUIRE_PORTAUDIO=1

cd "$ROOT_DIR"
cargo build --release --target "$TARGET" -p stt-cli
cargo build --release --target "$TARGET" -p stt-gui \
    --features native-gui,static-libav

mkdir -p "$ROOT_DIR/dist/cli" "$ROOT_DIR/dist/gui"
cp "$ROOT_DIR/target/$TARGET/release/stt.exe" \
    "$ROOT_DIR/dist/cli/stt.exe"
cp "$ROOT_DIR/target/$TARGET/release/STT.exe" \
    "$ROOT_DIR/dist/gui/STT.exe"
