#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
OUTPUT_FILE="${OUTPUT_FILE:-$ROOT_DIR/scripts/connectivity_test.pcm.b64}"
FFMPEG="${FFMPEG:-ffmpeg}"
RAW_FILE="$(mktemp)"
ENCODED_FILE="$(mktemp)"

cleanup() {
    rm -f "$RAW_FILE" "$ENCODED_FILE"
}
trap cleanup EXIT

if ! command -v "$FFMPEG"; then
    echo "ffmpeg is required."
    exit 1
fi

FILTER_LIST="$("$FFMPEG" -hide_banner -filters)"
if ! grep -qE '(^|[[:space:]])flite[[:space:]]' <<< "$FILTER_LIST"; then
    echo "The installed ffmpeg must include the libflite filter."
    exit 1
fi

"$FFMPEG" \
    -hide_banner \
    -loglevel error \
    -y \
    -f lavfi \
    -i "flite=text='test':voice=slt" \
    -ar 16000 \
    -ac 1 \
    -f s16le \
    "$RAW_FILE"

base64 --wrap=76 "$RAW_FILE" > "$ENCODED_FILE"
mkdir -p "$(dirname "$OUTPUT_FILE")"
install -m 0644 "$ENCODED_FILE" "$OUTPUT_FILE"

sha256sum "$RAW_FILE"
wc -c "$RAW_FILE"
echo "Generated $OUTPUT_FILE"
