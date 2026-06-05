#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BUILD_DIR="${BUILD_DIR:-$ROOT_DIR/build/ffmpeg}"
PREFIX="${PREFIX:-$ROOT_DIR/build/ffmpeg-win}"
TARGET="${TARGET:-x86_64-w64-mingw32}"
JOBS="${JOBS:-$(nproc)}"

OGG_VERSION="${OGG_VERSION:-1.3.5}"
VORBIS_VERSION="${VORBIS_VERSION:-1.3.7}"
OPUS_REF="${OPUS_REF:-v1.5.2}"
LAME_VERSION="${LAME_VERSION:-3.100}"
OPENCORE_AMR_VERSION="${OPENCORE_AMR_VERSION:-0.1.6}"
FFMPEG_REF="${FFMPEG_REF:-n7.1.1}"

mkdir -p "$BUILD_DIR" "$PREFIX"

export PKG_CONFIG_ALLOW_CROSS=1
export PKG_CONFIG_PATH="$PREFIX/lib/pkgconfig"
export PKG_CONFIG_LIBDIR="$PREFIX/lib/pkgconfig"
export CFLAGS="-O2 -I$PREFIX/include"
export LDFLAGS="-L$PREFIX/lib"

fetch_tarball() {
	local url="$1"
	local out="$2"
	if [ ! -f "$out" ]; then
		curl -L "$url" -o "$out"
	fi
}

extract_once() {
	local tarball="$1"
	local dir="$2"
	if [ ! -d "$dir" ]; then
		tar -xf "$tarball" -C "$BUILD_DIR"
	fi
}

clone_once() {
	local url="$1"
	local ref="$2"
	local dir="$3"
	if [ ! -d "$dir/.git" ]; then
		git clone --depth 1 --branch "$ref" "$url" "$dir"
	fi
}

build_autotools() {
	local dir="$1"
	shift
	cd "$dir"
	if [ ! -x ./configure ]; then
		if [ -x ./autogen.sh ]; then
			./autogen.sh
		else
			autoreconf -fiv
		fi
	fi
	./configure \
		--host="$TARGET" \
		--prefix="$PREFIX" \
		--disable-shared \
		--enable-static \
		"$@"
	make -j"$JOBS"
	make install
}

cd "$BUILD_DIR"

fetch_tarball "https://downloads.xiph.org/releases/ogg/libogg-$OGG_VERSION.tar.xz" "$BUILD_DIR/libogg-$OGG_VERSION.tar.xz"
extract_once "$BUILD_DIR/libogg-$OGG_VERSION.tar.xz" "$BUILD_DIR/libogg-$OGG_VERSION"
build_autotools "$BUILD_DIR/libogg-$OGG_VERSION"

fetch_tarball "https://downloads.xiph.org/releases/vorbis/libvorbis-$VORBIS_VERSION.tar.xz" "$BUILD_DIR/libvorbis-$VORBIS_VERSION.tar.xz"
extract_once "$BUILD_DIR/libvorbis-$VORBIS_VERSION.tar.xz" "$BUILD_DIR/libvorbis-$VORBIS_VERSION"
build_autotools "$BUILD_DIR/libvorbis-$VORBIS_VERSION" --disable-oggtest

clone_once "https://github.com/xiph/opus.git" "$OPUS_REF" "$BUILD_DIR/opus"
build_autotools "$BUILD_DIR/opus" --disable-extra-programs --disable-doc

fetch_tarball "https://downloads.sourceforge.net/project/lame/lame/$LAME_VERSION/lame-$LAME_VERSION.tar.gz" "$BUILD_DIR/lame-$LAME_VERSION.tar.gz"
extract_once "$BUILD_DIR/lame-$LAME_VERSION.tar.gz" "$BUILD_DIR/lame-$LAME_VERSION"
build_autotools "$BUILD_DIR/lame-$LAME_VERSION" --disable-frontend --disable-decoder

fetch_tarball "https://downloads.sourceforge.net/project/opencore-amr/opencore-amr/opencore-amr-$OPENCORE_AMR_VERSION.tar.gz" "$BUILD_DIR/opencore-amr-$OPENCORE_AMR_VERSION.tar.gz"
extract_once "$BUILD_DIR/opencore-amr-$OPENCORE_AMR_VERSION.tar.gz" "$BUILD_DIR/opencore-amr-$OPENCORE_AMR_VERSION"
build_autotools "$BUILD_DIR/opencore-amr-$OPENCORE_AMR_VERSION"

clone_once "https://github.com/FFmpeg/FFmpeg.git" "$FFMPEG_REF" "$BUILD_DIR/ffmpeg"
cd "$BUILD_DIR/ffmpeg"

./configure \
	--prefix="$PREFIX" \
	--target-os=mingw32 \
	--arch=x86_64 \
	--cross-prefix="$TARGET-" \
	--pkg-config=pkg-config \
	--pkg-config-flags=--static \
	--enable-cross-compile \
	--disable-shared \
	--enable-static \
	--disable-debug \
	--disable-doc \
	--disable-programs \
	--disable-autodetect \
	--disable-everything \
	--enable-gpl \
	--enable-version3 \
	--enable-avcodec \
	--enable-avformat \
	--enable-avutil \
	--enable-swresample \
	--enable-protocol=file \
	--enable-demuxer=wav \
	--enable-decoder=pcm_f32be,pcm_f32le,pcm_f64be,pcm_f64le,pcm_s16be,pcm_s16le,pcm_s24be,pcm_s24le,pcm_s32be,pcm_s32le,pcm_s64be,pcm_s64le,pcm_s8 \
	--enable-encoder=libopus,wavpack,aac,ac3,eac3,libmp3lame,mp2,mp1,flac,alac,libvorbis,adpcm_ms,libopencore_amrnb,pcm_f32be,pcm_f32le,pcm_f64be,pcm_f64le,pcm_s16be,pcm_s16le,pcm_s24be,pcm_s24le,pcm_s32be,pcm_s32le,pcm_s64be,pcm_s64le,pcm_s8 \
	--enable-muxer=wav,ac3,ac4,ogg,mp3,flac,eac3,adts,ipod,mp4,opus,webm,s8,s16be,s16le,s24be,s24le,s32be,s32le,f32be,f32le,f64be,f64le \
	--enable-libopus \
	--enable-libmp3lame \
	--enable-libvorbis \
	--enable-libopencore-amrnb

make -j"$JOBS"
make install
