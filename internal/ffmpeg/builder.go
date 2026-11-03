package ffmpeg

import (
	"fmt"
	"slices"
	"strings"
)

// alsaThreadQueueSize sets ffmpeg's -thread_queue_size for the legacy direct
// ALSA input so the demuxer keeps draining the capture ring during pacing gaps.
const alsaThreadQueueSize = 8192

// pipeAudioResample stretches each nut audio track to its pts. The sink
// stamps audio with the device's smoothed capture clock, so async keeps device
// clock drift from accumulating against video without inserting or dropping
// samples.
const pipeAudioResample = "aresample=async=1000:min_hard_comp=0.100000:first_pts=0"

// commandHead returns the ffmpeg base prefix. Strips `-nostdin` when the
// caller is feeding ffmpeg via a stdin pipe (InputPipe), otherwise uses
// Base() as-is so v4l2 / lavfi callers keep their current head.
func commandHead(p *Params) string {
	if p.InputPipe != nil {
		return strings.Replace(Base(), " -nostdin", "", 1)
	}
	return Base()
}

// audioEncoder maps a logical audio codec name to its ffmpeg encoder. Empty
// (and any unknown value) defaults to Opus, the only codec the WebRTC fan-out
// can carry.
func audioEncoder(codec string) string {
	switch codec {
	case "aac":
		return "aac"
	default:
		return "libopus"
	}
}

// BuildCommand builds an FFmpeg command from structured parameters.
func BuildCommand(p *Params) string {
	var cmd strings.Builder

	cmd.WriteString(commandHead(p))

	for _, arg := range p.GlobalArgs {
		cmd.WriteString(" " + arg)
	}

	if p.InputPipe != nil {
		writePipeInput(&cmd, p.InputPipe)
	} else {
		cmd.WriteString(" -f v4l2")

		ApplyOptionsToCommand(p.Options, &cmd)

		if p.InputFormat != "" {
			cmd.WriteString(" -input_format " + p.InputFormat)
		}
		if p.Resolution != "" {
			cmd.WriteString(" -video_size " + p.Resolution)
		}
		if p.FPS != "" {
			cmd.WriteString(" -framerate " + p.FPS)
		}
		cmd.WriteString(" -i " + p.DevicePath)
	}

	// The single AudioDevice path keeps legacy behavior verbatim for back-compat;
	// nut pipe audio needs no extra inputs.
	if pipeAudioTracks(p) == 0 && p.AudioDevice != "" {
		fmt.Fprintf(&cmd, " -thread_queue_size %d", alsaThreadQueueSize)

		if slices.Contains(p.Options, OptionWallclockWithGenpts) {
			cmd.WriteString(" -use_wallclock_as_timestamps 1 -fflags +genpts+igndts")
		}

		cmd.WriteString(" -f alsa -sample_fmt s16 -ar 48000 -ac 2")
		cmd.WriteString(" -i " + p.AudioDevice)

		cmd.WriteString(" -map 0:v -map 1:a")
	}

	if slices.Contains(p.Options, OptionVsyncPassthrough) {
		cmd.WriteString(" -fps_mode passthrough")
	}

	// Single-device audio takes an -af chain; pipe audio routes its filter
	// through the filter_complex mix below instead.
	if p.AudioFilters != "" && p.AudioDevice != "" && pipeAudioTracks(p) == 0 {
		cmd.WriteString(" -af " + p.AudioFilters)
	}

	// Pipe audio uses filter_complex (aresample per track + per-track map).
	// Must come before -c:v so ffmpeg sees the stream selection first.
	if n := pipeAudioTracks(p); n > 0 {
		writePipeAudioFilterComplex(&cmd, n, p.AudioFilters)
	}

	if p.VideoFilters != "" {
		cmd.WriteString(" -vf " + p.VideoFilters)
	}

	cmd.WriteString(" -c:v " + p.Encoder)

	// Tag the encoded stream VUI to match the input colorimetry (raw pipe
	// carries none, so the encoder would otherwise leave it unspecified).
	if p.InputPipe != nil && !p.InputPipe.Color.IsZero() {
		writeColorTags(&cmd, p.InputPipe.Color)
	}

	if strings.Contains(p.Encoder, "h264") {
		cmd.WriteString(" -profile:v high -level:v 5.2")
	}

	if p.RCMode != "" && isHardwareEncoder(p.Encoder) {
		cmd.WriteString(" -rc_mode " + p.RCMode)
	}

	if p.Bitrate != "" {
		cmd.WriteString(" -b:v " + p.Bitrate)
	}
	if p.MinRate != "" {
		cmd.WriteString(" -minrate " + p.MinRate)
	}
	if p.MaxRate != "" {
		cmd.WriteString(" -maxrate " + p.MaxRate)
	}
	if p.BufferSize != "" {
		cmd.WriteString(" -bufsize " + p.BufferSize)
	}
	if p.CRF > 0 {
		fmt.Fprintf(&cmd, " -crf %d", p.CRF)
	}
	if p.QP > 0 {
		fmt.Fprintf(&cmd, " -qp %d", p.QP)
	}

	if p.Preset != "" {
		cmd.WriteString(" -preset " + p.Preset)
	}
	if p.GOP > 0 {
		fmt.Fprintf(&cmd, " -g %d", p.GOP)
	} else {
		cmd.WriteString(" -g 60")
	}
	if p.BFrames >= 0 {
		fmt.Fprintf(&cmd, " -bf %d", p.BFrames)
	}

	if !isHardwareEncoder(p.Encoder) {
		cmd.WriteString(" -tune zerolatency")
		cmd.WriteString(" -keyint_min 15")
		cmd.WriteString(" -sc_threshold 0")
	}

	// Repeat VPS/SPS/PPS at every keyframe so late MPEG-TS/SRT joiners can decode immediately.
	if isHevcOrH264(p.Encoder) {
		cmd.WriteString(" -bsf:v dump_extra=freq=keyframe")
	}

	if p.AudioDevice != "" || pipeAudioTracks(p) > 0 {
		bitrate := p.AudioBitrate
		if bitrate == "" {
			bitrate = "128k"
		}
		fmt.Fprintf(&cmd, " -c:a %s -b:a %s -ar 48000", audioEncoder(p.AudioCodec), bitrate)
	}

	if p.ProgressSocket != "" {
		cmd.WriteString(" -progress unix://" + p.ProgressSocket)
	}

	// Outputs: multi-target mode (Outputs slice) supersedes single
	// OutputURL. Both modes pick mux flags by transport type.
	switch {
	case len(p.Outputs) > 0:
		writeOutputs(&cmd, p.Outputs)
	case strings.HasPrefix(p.OutputURL, "rtsp://"):
		cmd.WriteString(" -rtsp_transport tcp -f rtsp " + p.OutputURL)
	default:
		cmd.WriteString(" -muxdelay 0 -muxpreload 0 -flush_packets 1 -f mpegts " + p.OutputURL)
	}

	return cmd.String()
}

// writeColorTags emits the non-empty colorimetry flags. Used on both the
// raw input (before -i, so ffmpeg interprets the untagged pipe correctly)
// and the encoder output (sets the encoded stream VUI).
func writeColorTags(cmd *strings.Builder, c ColorTags) {
	if c.Space != "" {
		cmd.WriteString(" -colorspace " + c.Space)
	}
	if c.Primaries != "" {
		cmd.WriteString(" -color_primaries " + c.Primaries)
	}
	if c.TRC != "" {
		cmd.WriteString(" -color_trc " + c.TRC)
	}
	if c.Range != "" {
		cmd.WriteString(" -color_range " + c.Range)
	}
}

// writePipeInput emits the `-f <muxer> [-pix_fmt -s -framerate] -i pipe:0`
// fragment for the InputPipe case. Y4M is self-describing; rawvideo
// needs explicit pix_fmt + dims + framerate.
func writePipeInput(cmd *strings.Builder, pi *PipeInput) {
	switch pi.Format {
	case "rawvideo":
		cmd.WriteString(" -f rawvideo")
		if pi.PixelFormat != "" {
			cmd.WriteString(" -pix_fmt " + pi.PixelFormat)
		}
		if pi.Width > 0 && pi.Height > 0 {
			fmt.Fprintf(cmd, " -s %dx%d", pi.Width, pi.Height)
		}
		if pi.FPS > 0 {
			fmt.Fprintf(cmd, " -framerate %d", pi.FPS)
		}
		writeColorTags(cmd, pi.Color)
		cmd.WriteString(" -i pipe:0")
	case "nut":
		// Headers describe every stream, so skip probing to start encoding sooner.
		cmd.WriteString(" -probesize 32 -analyzeduration 0 -f nut")
		writeColorTags(cmd, pi.Color)
		cmd.WriteString(" -i pipe:0")
	case "yuv4mpegpipe", "":
		cmd.WriteString(" -f yuv4mpegpipe -i pipe:0")
	default:
		cmd.WriteString(" -f " + pi.Format + " -i pipe:0")
	}
}

// pipeAudioTracks reports how many PCM tracks ride inside a nut pipe input.
func pipeAudioTracks(p *Params) int {
	if p.InputPipe == nil || p.InputPipe.Format != "nut" {
		return 0
	}
	return p.InputPipe.AudioTracks
}

// writePipeAudioFilterComplex emits the filter_complex audio chain plus the
// output map flags for N audio tracks inside pipe input 0 (after the video
// stream). Each track first goes through aresample (drift correction).
// Without mixFilter, every track becomes its own output track. With mixFilter
// (e.g. "amix=inputs=2:duration=shortest"), the resampled tracks feed that
// filtergraph and collapse into a single mixed output track.
func writePipeAudioFilterComplex(cmd *strings.Builder, audioCount int, mixFilter string) {
	var fc strings.Builder
	resampleLabel := "a"
	if mixFilter != "" {
		resampleLabel = "s"
	}
	for k := range audioCount {
		if k > 0 {
			fc.WriteString(";")
		}
		fmt.Fprintf(&fc, "[0:a:%d]%s[%s%d]", k, pipeAudioResample, resampleLabel, k)
	}

	if mixFilter != "" {
		fc.WriteString(";")
		for k := range audioCount {
			fmt.Fprintf(&fc, "[s%d]", k)
		}
		fc.WriteString(mixFilter)
		fc.WriteString("[aout]")
		cmd.WriteString(" -filter_complex " + fc.String())
		cmd.WriteString(" -map 0:v:0 -map [aout]")
		return
	}

	cmd.WriteString(" -filter_complex " + fc.String())
	cmd.WriteString(" -map 0:v:0")
	for k := range audioCount {
		fmt.Fprintf(cmd, " -map [a%d]", k)
	}
}

// writeOutputs emits one output fragment per OutputTarget. Per-format
// tail flags (rtsp_transport, mpegts mux delays) are applied per output.
func writeOutputs(cmd *strings.Builder, outs []OutputTarget) {
	rtspEmitted := false
	for _, o := range outs {
		switch o.Type {
		case "rtsp":
			if !rtspEmitted {
				cmd.WriteString(" -rtsp_transport tcp")
				rtspEmitted = true
			}
			cmd.WriteString(" -f rtsp " + o.URL)
		case "srt", "mpegts":
			cmd.WriteString(" -muxdelay 0 -muxpreload 0 -flush_packets 1 -f mpegts " + o.URL)
		case "hls":
			cmd.WriteString(" -f hls " + o.URL)
		default:
			cmd.WriteString(" -f " + o.Type + " " + o.URL)
		}
	}
}

// BuildInputArgs returns just the ffmpeg head (base flags + global args +
// input + audio inputs) as a single shell-ready string. The pipeline's
// CustomEncoderArgs path uses this to compose `head + " " + custom_tail`.
func BuildInputArgs(p *Params) string {
	var cmd strings.Builder
	cmd.WriteString(commandHead(p))

	for _, arg := range p.GlobalArgs {
		cmd.WriteString(" " + arg)
	}

	if p.InputPipe != nil {
		writePipeInput(&cmd, p.InputPipe)
	} else {
		cmd.WriteString(" -f v4l2")
		ApplyOptionsToCommand(p.Options, &cmd)
		if p.InputFormat != "" {
			cmd.WriteString(" -input_format " + p.InputFormat)
		}
		if p.Resolution != "" {
			cmd.WriteString(" -video_size " + p.Resolution)
		}
		if p.FPS != "" {
			cmd.WriteString(" -framerate " + p.FPS)
		}
		cmd.WriteString(" -i " + p.DevicePath)
	}

	if pipeAudioTracks(p) == 0 && p.AudioDevice != "" {
		fmt.Fprintf(&cmd, " -thread_queue_size %d -f alsa -sample_fmt s16 -ar 48000 -ac 2 -i %s", alsaThreadQueueSize, p.AudioDevice)
	}

	return cmd.String()
}
