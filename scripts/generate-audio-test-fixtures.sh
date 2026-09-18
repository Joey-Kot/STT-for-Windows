#!/usr/bin/env bash
# Test-only generator. The shipped applications never invoke an FFmpeg process.
set -euo pipefail
folder="${1:?usage: generate-audio-test-fixtures.sh OUTPUT_DIRECTORY}"
mkdir -p "$folder"
ffmpeg -nostdin -v error -y -f lavfi -i "flite=text='This is a speech activity detection test. Please preserve every word.':voice=slt" -ar 48000 -ac 2 "$folder/pcm.wav"
# FFmpeg must not consume the format list inherited as the loop's stdin.
while read -r codec filename; do
    ffmpeg -nostdin -v error -y -i "$folder/pcm.wav" -c:a "$codec" "$folder/$filename"
done <<'FORMATS'
libmp3lame audio.mp3
flac audio.flac
libopus opus.ogg
libvorbis vorbis.ogg
aac aac.m4a
alac alac.m4a
libopus audio.webm
flac audio.mka
wavpack audio.wv
ac3 audio.ac3
eac3 audio.eac3
FORMATS

# Raw ADTS segments permit midstream format changes without remuxing.
ffmpeg -nostdin -v error -y -i "$folder/pcm.wav" -ar 48000 -ac 2 -c:a aac -f adts "$folder/aac-stereo-48k.aac"
ffmpeg -nostdin -v error -y -i "$folder/pcm.wav" -ar 24000 -ac 2 -c:a aac -f adts "$folder/aac-stereo-24k.aac"
ffmpeg -nostdin -v error -y -i "$folder/pcm.wav" -ar 48000 -ac 1 -c:a aac -f adts "$folder/aac-mono-48k.aac"
