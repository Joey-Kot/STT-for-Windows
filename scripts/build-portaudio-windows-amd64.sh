#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BUILD_DIR="${BUILD_DIR:-$ROOT_DIR/build/portaudio}"
PREFIX="${PREFIX:-$ROOT_DIR/build/portaudio-win}"
TARGET="${TARGET:-x86_64-w64-mingw32}"
JOBS="${JOBS:-$(nproc)}"
PORTAUDIO_REF="${PORTAUDIO_REF:-v19.7.0}"

mkdir -p "$BUILD_DIR" "$PREFIX"

if [ ! -d "$BUILD_DIR/source/.git" ]; then
    git clone --depth 1 --branch "$PORTAUDIO_REF" \
        https://github.com/PortAudio/portaudio.git "$BUILD_DIR/source"
fi

cd "$BUILD_DIR/source"
./configure \
    --host="$TARGET" \
    --prefix="$PREFIX" \
    --with-winapi=wmme \
    --disable-shared \
    --enable-static \
    CC="$TARGET-gcc"
make -j"$JOBS"
make install
