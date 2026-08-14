#ifndef STT_FFMPEG_BRIDGE_H
#define STT_FFMPEG_BRIDGE_H

#ifdef __cplusplus
extern "C" {
#endif

int stt_ffmpeg_convert(
    const char *in_path,
    const char *out_path,
    const char *codec_name,
    int channels,
    int sample_rate,
    int bitrate_kbps,
    int codec_has_bitrate,
    const char *sample_fmt_name,
    int debug,
    char *errbuf,
    int errbuf_size
);

#ifdef __cplusplus
}
#endif

#endif

