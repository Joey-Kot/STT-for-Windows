// Copyright (C) 2026 Joey Kot <joey.kot.x@gmail.com>
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed WITHOUT ANY WARRANTY; without even the
// implied warranty of MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.
// See <https://www.gnu.org/licenses/> for more details.

//go:build gui_ffmpeg_cgo

package ffmpeg

/*
#include <errno.h>
#include <stdarg.h>
#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>

#include <libavcodec/avcodec.h>
#include <libavformat/avformat.h>
#include <libavutil/audio_fifo.h>
#include <libavutil/channel_layout.h>
#include <libavutil/error.h>
#include <libavutil/frame.h>
#include <libavutil/mathematics.h>
#include <libavutil/opt.h>
#include <libavutil/samplefmt.h>
#include <libswresample/swresample.h>

static void stt_set_error(char *errbuf, int errbuf_size, const char *fmt, ...) {
	if (errbuf == NULL || errbuf_size <= 0) {
		return;
	}
	va_list args;
	va_start(args, fmt);
	vsnprintf(errbuf, errbuf_size, fmt, args);
	va_end(args);
}

static void stt_set_av_error(char *errbuf, int errbuf_size, const char *prefix, int err) {
	char av_error[AV_ERROR_MAX_STRING_SIZE] = {0};
	av_strerror(err, av_error, sizeof(av_error));
	stt_set_error(errbuf, errbuf_size, "%s: %s", prefix, av_error);
}

static int stt_pick_sample_fmt(const AVCodec *codec, const char *requested) {
	enum AVSampleFormat fmt = AV_SAMPLE_FMT_NONE;
	if (requested != NULL && requested[0] != '\0') {
		fmt = av_get_sample_fmt(requested);
	}
	if (fmt != AV_SAMPLE_FMT_NONE && codec->sample_fmts != NULL) {
		const enum AVSampleFormat *p = codec->sample_fmts;
		while (*p != AV_SAMPLE_FMT_NONE) {
			if (*p == fmt) {
				return fmt;
			}
			p++;
		}
	}
	if (fmt != AV_SAMPLE_FMT_NONE && codec->sample_fmts == NULL) {
		return fmt;
	}
	if (codec->sample_fmts != NULL) {
		return codec->sample_fmts[0];
	}
	return AV_SAMPLE_FMT_S16;
}

static int stt_alloc_audio_frame(
	AVFrame **frame,
	enum AVSampleFormat sample_fmt,
	const AVChannelLayout *ch_layout,
	int sample_rate,
	int nb_samples,
	char *errbuf,
	int errbuf_size
) {
	int ret;
	AVFrame *f = av_frame_alloc();
	if (f == NULL) {
		stt_set_error(errbuf, errbuf_size, "could not allocate audio frame");
		return AVERROR(ENOMEM);
	}
	f->format = sample_fmt;
	f->sample_rate = sample_rate;
	f->nb_samples = nb_samples;
	ret = av_channel_layout_copy(&f->ch_layout, ch_layout);
	if (ret < 0) {
		av_frame_free(&f);
		stt_set_av_error(errbuf, errbuf_size, "could not copy channel layout", ret);
		return ret;
	}
	if (nb_samples > 0) {
		ret = av_frame_get_buffer(f, 0);
		if (ret < 0) {
			av_frame_free(&f);
			stt_set_av_error(errbuf, errbuf_size, "could not allocate audio frame buffer", ret);
			return ret;
		}
	}
	*frame = f;
	return 0;
}

static int stt_encode_write(
	AVCodecContext *enc_ctx,
	AVFormatContext *ofmt_ctx,
	AVStream *out_stream,
	AVFrame *frame,
	char *errbuf,
	int errbuf_size
) {
	int ret = avcodec_send_frame(enc_ctx, frame);
	if (ret < 0) {
		stt_set_av_error(errbuf, errbuf_size, "could not send frame to encoder", ret);
		return ret;
	}

	while (1) {
		AVPacket *pkt = av_packet_alloc();
		if (pkt == NULL) {
			stt_set_error(errbuf, errbuf_size, "could not allocate encoded packet");
			return AVERROR(ENOMEM);
		}
		ret = avcodec_receive_packet(enc_ctx, pkt);
		if (ret == AVERROR(EAGAIN) || ret == AVERROR_EOF) {
			av_packet_free(&pkt);
			return 0;
		}
		if (ret < 0) {
			av_packet_free(&pkt);
			stt_set_av_error(errbuf, errbuf_size, "could not receive encoded packet", ret);
			return ret;
		}
		av_packet_rescale_ts(pkt, enc_ctx->time_base, out_stream->time_base);
		pkt->stream_index = out_stream->index;
		ret = av_interleaved_write_frame(ofmt_ctx, pkt);
		av_packet_free(&pkt);
		if (ret < 0) {
			stt_set_av_error(errbuf, errbuf_size, "could not write encoded packet", ret);
			return ret;
		}
	}
}

static int stt_write_fifo_to_encoder(
	AVAudioFifo *fifo,
	AVCodecContext *enc_ctx,
	AVFormatContext *ofmt_ctx,
	AVStream *out_stream,
	int flush_all,
	int64_t *next_pts,
	char *errbuf,
	int errbuf_size
) {
	int ret = 0;
	while (av_audio_fifo_size(fifo) > 0) {
		int available = av_audio_fifo_size(fifo);
		int nb_samples = enc_ctx->frame_size > 0 ? enc_ctx->frame_size : available;
		if (!flush_all && available < nb_samples) {
			return 0;
		}
		if (flush_all && available < nb_samples) {
			nb_samples = available;
		}

		AVFrame *frame = NULL;
		ret = stt_alloc_audio_frame(&frame, enc_ctx->sample_fmt, &enc_ctx->ch_layout, enc_ctx->sample_rate, nb_samples, errbuf, errbuf_size);
		if (ret < 0) {
			return ret;
		}
		ret = av_audio_fifo_read(fifo, (void **)frame->extended_data, nb_samples);
		if (ret < nb_samples) {
			av_frame_free(&frame);
			stt_set_error(errbuf, errbuf_size, "could not read converted samples from fifo");
			return AVERROR(EIO);
		}
		frame->pts = *next_pts;
		*next_pts += frame->nb_samples;
		ret = stt_encode_write(enc_ctx, ofmt_ctx, out_stream, frame, errbuf, errbuf_size);
		av_frame_free(&frame);
		if (ret < 0) {
			return ret;
		}
	}
	return 0;
}

static int stt_convert_and_queue_frame(
	SwrContext *swr,
	AVCodecContext *dec_ctx,
	AVCodecContext *enc_ctx,
	AVAudioFifo *fifo,
	AVFrame *decoded,
	char *errbuf,
	int errbuf_size
) {
	int64_t delay = swr_get_delay(swr, dec_ctx->sample_rate);
	int out_samples = (int)av_rescale_rnd(delay + decoded->nb_samples, enc_ctx->sample_rate, dec_ctx->sample_rate, AV_ROUND_UP);
	if (out_samples <= 0) {
		return 0;
	}

	AVFrame *converted = NULL;
	int ret = stt_alloc_audio_frame(&converted, enc_ctx->sample_fmt, &enc_ctx->ch_layout, enc_ctx->sample_rate, out_samples, errbuf, errbuf_size);
	if (ret < 0) {
		return ret;
	}
	ret = swr_convert(swr, converted->extended_data, out_samples, (const uint8_t **)decoded->extended_data, decoded->nb_samples);
	if (ret < 0) {
		av_frame_free(&converted);
		stt_set_av_error(errbuf, errbuf_size, "could not resample audio frame", ret);
		return ret;
	}
	converted->nb_samples = ret;
	if (ret > 0) {
		ret = av_audio_fifo_realloc(fifo, av_audio_fifo_size(fifo) + converted->nb_samples);
		if (ret < 0) {
			av_frame_free(&converted);
			stt_set_av_error(errbuf, errbuf_size, "could not grow audio fifo", ret);
			return ret;
		}
		ret = av_audio_fifo_write(fifo, (void **)converted->extended_data, converted->nb_samples);
		if (ret < converted->nb_samples) {
			av_frame_free(&converted);
			stt_set_error(errbuf, errbuf_size, "could not write converted samples to fifo");
			return AVERROR(EIO);
		}
	}
	av_frame_free(&converted);
	return 0;
}

static int stt_flush_resampler(
	SwrContext *swr,
	AVCodecContext *dec_ctx,
	AVCodecContext *enc_ctx,
	AVAudioFifo *fifo,
	char *errbuf,
	int errbuf_size
) {
	while (1) {
		int64_t delay = swr_get_delay(swr, dec_ctx->sample_rate);
		int out_samples = (int)av_rescale_rnd(delay, enc_ctx->sample_rate, dec_ctx->sample_rate, AV_ROUND_UP);
		if (out_samples <= 0) {
			return 0;
		}
		AVFrame *converted = NULL;
		int ret = stt_alloc_audio_frame(&converted, enc_ctx->sample_fmt, &enc_ctx->ch_layout, enc_ctx->sample_rate, out_samples, errbuf, errbuf_size);
		if (ret < 0) {
			return ret;
		}
		ret = swr_convert(swr, converted->extended_data, out_samples, NULL, 0);
		if (ret < 0) {
			av_frame_free(&converted);
			stt_set_av_error(errbuf, errbuf_size, "could not flush resampler", ret);
			return ret;
		}
		converted->nb_samples = ret;
		if (ret == 0) {
			av_frame_free(&converted);
			return 0;
		}
		ret = av_audio_fifo_realloc(fifo, av_audio_fifo_size(fifo) + converted->nb_samples);
		if (ret < 0) {
			av_frame_free(&converted);
			stt_set_av_error(errbuf, errbuf_size, "could not grow audio fifo", ret);
			return ret;
		}
		ret = av_audio_fifo_write(fifo, (void **)converted->extended_data, converted->nb_samples);
		if (ret < converted->nb_samples) {
			av_frame_free(&converted);
			stt_set_error(errbuf, errbuf_size, "could not write flushed samples to fifo");
			return AVERROR(EIO);
		}
		av_frame_free(&converted);
	}
}

static int stt_ffmpeg_convert(
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
) {
	AVFormatContext *ifmt_ctx = NULL;
	AVFormatContext *ofmt_ctx = NULL;
	AVCodecContext *dec_ctx = NULL;
	AVCodecContext *enc_ctx = NULL;
	AVPacket *packet = NULL;
	AVFrame *decoded = NULL;
	AVAudioFifo *fifo = NULL;
	SwrContext *swr = NULL;
	AVStream *out_stream = NULL;
	int audio_stream = -1;
	int ret = 0;
	int64_t next_pts = 0;

	av_log_set_level(debug ? AV_LOG_INFO : AV_LOG_ERROR);

	ret = avformat_open_input(&ifmt_ctx, in_path, NULL, NULL);
	if (ret < 0) {
		stt_set_av_error(errbuf, errbuf_size, "could not open input audio", ret);
		goto cleanup;
	}
	ret = avformat_find_stream_info(ifmt_ctx, NULL);
	if (ret < 0) {
		stt_set_av_error(errbuf, errbuf_size, "could not read input stream info", ret);
		goto cleanup;
	}
	audio_stream = av_find_best_stream(ifmt_ctx, AVMEDIA_TYPE_AUDIO, -1, -1, NULL, 0);
	if (audio_stream < 0) {
		ret = audio_stream;
		stt_set_av_error(errbuf, errbuf_size, "could not find input audio stream", ret);
		goto cleanup;
	}

	AVStream *in_stream = ifmt_ctx->streams[audio_stream];
	const AVCodec *decoder = avcodec_find_decoder(in_stream->codecpar->codec_id);
	if (decoder == NULL) {
		ret = AVERROR_DECODER_NOT_FOUND;
		stt_set_error(errbuf, errbuf_size, "could not find decoder for input audio");
		goto cleanup;
	}
	dec_ctx = avcodec_alloc_context3(decoder);
	if (dec_ctx == NULL) {
		ret = AVERROR(ENOMEM);
		stt_set_error(errbuf, errbuf_size, "could not allocate decoder context");
		goto cleanup;
	}
	ret = avcodec_parameters_to_context(dec_ctx, in_stream->codecpar);
	if (ret < 0) {
		stt_set_av_error(errbuf, errbuf_size, "could not copy decoder parameters", ret);
		goto cleanup;
	}
	if (dec_ctx->ch_layout.nb_channels <= 0) {
		int input_channels = in_stream->codecpar->ch_layout.nb_channels;
		if (input_channels <= 0) {
			input_channels = channels;
		}
		av_channel_layout_default(&dec_ctx->ch_layout, input_channels);
	}
	ret = avcodec_open2(dec_ctx, decoder, NULL);
	if (ret < 0) {
		stt_set_av_error(errbuf, errbuf_size, "could not open decoder", ret);
		goto cleanup;
	}

	ret = avformat_alloc_output_context2(&ofmt_ctx, NULL, NULL, out_path);
	if (ret < 0 || ofmt_ctx == NULL) {
		stt_set_av_error(errbuf, errbuf_size, "could not create output container", ret);
		goto cleanup;
	}
	const AVCodec *encoder = avcodec_find_encoder_by_name(codec_name);
	if (encoder == NULL) {
		ret = AVERROR_ENCODER_NOT_FOUND;
		stt_set_error(errbuf, errbuf_size, "could not find encoder '%s'", codec_name);
		goto cleanup;
	}
	enc_ctx = avcodec_alloc_context3(encoder);
	if (enc_ctx == NULL) {
		ret = AVERROR(ENOMEM);
		stt_set_error(errbuf, errbuf_size, "could not allocate encoder context");
		goto cleanup;
	}
	enc_ctx->sample_rate = sample_rate;
	enc_ctx->sample_fmt = stt_pick_sample_fmt(encoder, sample_fmt_name);
	enc_ctx->time_base = (AVRational){1, sample_rate};
	if (codec_has_bitrate) {
		enc_ctx->bit_rate = (int64_t)bitrate_kbps * 1000;
	}
	av_channel_layout_default(&enc_ctx->ch_layout, channels);
	if (ofmt_ctx->oformat->flags & AVFMT_GLOBALHEADER) {
		enc_ctx->flags |= AV_CODEC_FLAG_GLOBAL_HEADER;
	}
	ret = avcodec_open2(enc_ctx, encoder, NULL);
	if (ret < 0) {
		stt_set_av_error(errbuf, errbuf_size, "could not open encoder", ret);
		goto cleanup;
	}

	out_stream = avformat_new_stream(ofmt_ctx, NULL);
	if (out_stream == NULL) {
		ret = AVERROR(ENOMEM);
		stt_set_error(errbuf, errbuf_size, "could not allocate output stream");
		goto cleanup;
	}
	out_stream->time_base = enc_ctx->time_base;
	ret = avcodec_parameters_from_context(out_stream->codecpar, enc_ctx);
	if (ret < 0) {
		stt_set_av_error(errbuf, errbuf_size, "could not copy encoder parameters", ret);
		goto cleanup;
	}

	ret = swr_alloc_set_opts2(
		&swr,
		&enc_ctx->ch_layout,
		enc_ctx->sample_fmt,
		enc_ctx->sample_rate,
		&dec_ctx->ch_layout,
		dec_ctx->sample_fmt,
		dec_ctx->sample_rate,
		0,
		NULL
	);
	if (ret < 0) {
		stt_set_av_error(errbuf, errbuf_size, "could not allocate resampler", ret);
		goto cleanup;
	}
	ret = swr_init(swr);
	if (ret < 0) {
		stt_set_av_error(errbuf, errbuf_size, "could not initialize resampler", ret);
		goto cleanup;
	}

	fifo = av_audio_fifo_alloc(enc_ctx->sample_fmt, enc_ctx->ch_layout.nb_channels, enc_ctx->frame_size > 0 ? enc_ctx->frame_size : 1024);
	if (fifo == NULL) {
		ret = AVERROR(ENOMEM);
		stt_set_error(errbuf, errbuf_size, "could not allocate audio fifo");
		goto cleanup;
	}

	if (!(ofmt_ctx->oformat->flags & AVFMT_NOFILE)) {
		ret = avio_open(&ofmt_ctx->pb, out_path, AVIO_FLAG_WRITE);
		if (ret < 0) {
			stt_set_av_error(errbuf, errbuf_size, "could not open output audio", ret);
			goto cleanup;
		}
	}
	ret = avformat_write_header(ofmt_ctx, NULL);
	if (ret < 0) {
		stt_set_av_error(errbuf, errbuf_size, "could not write output header", ret);
		goto cleanup;
	}

	packet = av_packet_alloc();
	decoded = av_frame_alloc();
	if (packet == NULL || decoded == NULL) {
		ret = AVERROR(ENOMEM);
		stt_set_error(errbuf, errbuf_size, "could not allocate decode buffers");
		goto cleanup;
	}

	while ((ret = av_read_frame(ifmt_ctx, packet)) >= 0) {
		if (packet->stream_index != audio_stream) {
			av_packet_unref(packet);
			continue;
		}
		ret = avcodec_send_packet(dec_ctx, packet);
		av_packet_unref(packet);
		if (ret < 0) {
			stt_set_av_error(errbuf, errbuf_size, "could not send packet to decoder", ret);
			goto cleanup;
		}
		while ((ret = avcodec_receive_frame(dec_ctx, decoded)) >= 0) {
			ret = stt_convert_and_queue_frame(swr, dec_ctx, enc_ctx, fifo, decoded, errbuf, errbuf_size);
			av_frame_unref(decoded);
			if (ret < 0) {
				goto cleanup;
			}
			ret = stt_write_fifo_to_encoder(fifo, enc_ctx, ofmt_ctx, out_stream, 0, &next_pts, errbuf, errbuf_size);
			if (ret < 0) {
				goto cleanup;
			}
		}
		if (ret != AVERROR(EAGAIN) && ret != AVERROR_EOF) {
			stt_set_av_error(errbuf, errbuf_size, "could not receive decoded frame", ret);
			goto cleanup;
		}
	}
	if (ret != AVERROR_EOF) {
		stt_set_av_error(errbuf, errbuf_size, "could not read input packet", ret);
		goto cleanup;
	}

	ret = avcodec_send_packet(dec_ctx, NULL);
	if (ret < 0) {
		stt_set_av_error(errbuf, errbuf_size, "could not flush decoder", ret);
		goto cleanup;
	}
	while ((ret = avcodec_receive_frame(dec_ctx, decoded)) >= 0) {
		ret = stt_convert_and_queue_frame(swr, dec_ctx, enc_ctx, fifo, decoded, errbuf, errbuf_size);
		av_frame_unref(decoded);
		if (ret < 0) {
			goto cleanup;
		}
		ret = stt_write_fifo_to_encoder(fifo, enc_ctx, ofmt_ctx, out_stream, 0, &next_pts, errbuf, errbuf_size);
		if (ret < 0) {
			goto cleanup;
		}
	}
	if (ret != AVERROR_EOF && ret != AVERROR(EAGAIN)) {
		stt_set_av_error(errbuf, errbuf_size, "could not receive flushed decoded frame", ret);
		goto cleanup;
	}

	ret = stt_flush_resampler(swr, dec_ctx, enc_ctx, fifo, errbuf, errbuf_size);
	if (ret < 0) {
		goto cleanup;
	}
	ret = stt_write_fifo_to_encoder(fifo, enc_ctx, ofmt_ctx, out_stream, 1, &next_pts, errbuf, errbuf_size);
	if (ret < 0) {
		goto cleanup;
	}
	ret = stt_encode_write(enc_ctx, ofmt_ctx, out_stream, NULL, errbuf, errbuf_size);
	if (ret < 0) {
		goto cleanup;
	}
	ret = av_write_trailer(ofmt_ctx);
	if (ret < 0) {
		stt_set_av_error(errbuf, errbuf_size, "could not write output trailer", ret);
		goto cleanup;
	}
	ret = 0;

cleanup:
	if (decoded != NULL) {
		av_frame_free(&decoded);
	}
	if (packet != NULL) {
		av_packet_free(&packet);
	}
	if (fifo != NULL) {
		av_audio_fifo_free(fifo);
	}
	if (swr != NULL) {
		swr_free(&swr);
	}
	if (enc_ctx != NULL) {
		avcodec_free_context(&enc_ctx);
	}
	if (dec_ctx != NULL) {
		avcodec_free_context(&dec_ctx);
	}
	if (ofmt_ctx != NULL) {
		if (!(ofmt_ctx->oformat->flags & AVFMT_NOFILE) && ofmt_ctx->pb != NULL) {
			avio_closep(&ofmt_ctx->pb);
		}
		avformat_free_context(ofmt_ctx);
	}
	if (ifmt_ctx != NULL) {
		avformat_close_input(&ifmt_ctx);
	}
	return ret;
}
*/
import "C"

import (
	"fmt"
	"unsafe"

	"stt/internal/config"
)

// Convert converts input audio into the configured codec/container using the
// statically linked libav* libraries in GUI builds.
func Convert(cfg config.Config, inPath, outPath string, rate int) error {
	settings, err := settingsFor(cfg, rate)
	if err != nil {
		return err
	}

	in := C.CString(inPath)
	out := C.CString(outPath)
	codec := C.CString(settings.FFCodec)
	sampleFormat := C.CString(settings.SampleFormat)
	defer C.free(unsafe.Pointer(in))
	defer C.free(unsafe.Pointer(out))
	defer C.free(unsafe.Pointer(codec))
	defer C.free(unsafe.Pointer(sampleFormat))

	errbuf := make([]C.char, 4096)
	codecHasBitrate := 0
	if settings.CodecHasBitrate {
		codecHasBitrate = 1
	}
	debug := 0
	if cfg.FFMPEG_DEBUG {
		debug = 1
		fmt.Printf("[ffmpeg] libav convert: %s -> %s codec=%s channels=%d rate=%d bitrate=%dk sample_fmt=%s\n",
			inPath, outPath, settings.FFCodec, settings.Channels, settings.SampleRate, settings.Bitrate, settings.SampleFormat)
	}

	ret := C.stt_ffmpeg_convert(
		in,
		out,
		codec,
		C.int(settings.Channels),
		C.int(settings.SampleRate),
		C.int(settings.Bitrate),
		C.int(codecHasBitrate),
		sampleFormat,
		C.int(debug),
		&errbuf[0],
		C.int(len(errbuf)),
	)
	if ret < 0 {
		msg := C.GoString(&errbuf[0])
		if msg == "" {
			msg = fmt.Sprintf("libav conversion failed: %d", int(ret))
		}
		return fmt.Errorf("ffmpeg failed: %s", msg)
	}
	return nil
}
