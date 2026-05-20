package streams

import (
	"fmt"
	"strconv"
	"strings"
)

// processStreamNative builds the sink-side command for a single V4L2
// stream that's routed through the native dma-buf pipeline:
//
//	videonode-sink --socket /tmp/vn-bus-<deviceID>.sock
//	  | ffmpeg -f rawvideo -pix_fmt nv12 -s WxH -framerate N -i pipe:0
//	           <encoder ...> -f rtsp rtsp://127.0.0.1:8554/<streamID>
//
// The producer side (videonode-source) is started independently by
// streamProcessManager.Start via ProducerManager.Acquire. We look up its
// socket path through cp.producerMgr; if it's not yet acquired we surface
// an error and the caller falls back to legacy. The single-stream native
// path otherwise reuses the producer-manager refcount: multiple sinks
// (canvases + single streams) can share one sidecar per device.
//
// Resolution / FPS for the rawvideo pipe come from the stream's FFmpeg
// config. If omitted the sidecar's auto-detected geometry isn't echoed
// back to us; we default to 1920x1080@30 (matches videonode-source's
// Placeholder default; videonode-sink prints the actual NV12 dimensions on
// stderr so journald confirms what's flowing.
//
// DevicePath is recorded by the caller (which Acquired the producer) and
// not used here — we read the socket back from ProducerManager.
func (p *processor) processStreamNative(streamID string, spec *StreamSpec, _ string) (*ProcessedStream, error) {
	if p.native == nil || !p.native.SingleStreamReady() {
		return nil, fmt.Errorf("stream %s: native pipeline requested but binaries not available", streamID)
	}
	// Producer Acquire happens in streamProcessManager.Start. Here we only
	// need to know what socket path the sidecar bound. Default to the
	// canonical per-device path if the manager doesn't know yet (the sink
	// will retry-dial via scm_rights_source's 30 s window).
	socketPath := SocketPathFor(streamID)

	width, height := parseResolutionWH(spec.FFmpeg.Resolution)
	if width <= 0 || height <= 0 {
		width, height = 1920, 1080
	}
	fps := parseFPSDefault(spec.FFmpeg.FPS, 30)

	_ = width
	_ = height
	sinkArgv := []string{
		p.native.VNSink,
		"--socket", socketPath,
	}

	bitrate := "2M"
	if spec.FFmpeg.QualityParams != nil && spec.FFmpeg.QualityParams.TargetBitrate != nil {
		bitrate = fmt.Sprintf("%.0fM", *spec.FFmpeg.QualityParams.TargetBitrate)
	}

	encoder := "h264_rkmpp"
	if spec.FFmpeg.Codec == "h265" {
		encoder = "hevc_rkmpp"
	}

	// vn-sink writes YUV4MPEG2 (self-describing dims); -f yuv4mpegpipe
	// lets ffmpeg auto-detect resolution regardless of what the producer
	// negotiated with the V4L2 device.
	ffmpegArgv := []string{
		"ffmpeg",
		"-hide_banner", "-loglevel", "warning",
		"-f", "yuv4mpegpipe",
		"-i", "pipe:0",
		"-c:v", encoder,
		"-rc_mode", "VBR", "-b:v", bitrate,
		"-g", strconv.Itoa(fps * 2), "-bf", "0",
		"-bsf:v", "dump_extra=freq=keyframe",
		"-rtsp_transport", "tcp",
		"-f", "rtsp", fmt.Sprintf("rtsp://127.0.0.1:8554/%s", streamID),
	}

	var sb strings.Builder
	sb.WriteString(`/bin/sh -c "`)
	sb.WriteString(shellJoin(sinkArgv))
	sb.WriteString(` | `)
	sb.WriteString(shellJoin(ffmpegArgv))
	sb.WriteString(`"`)

	return &ProcessedStream{
		StreamID:      streamID,
		FFmpegCommand: sb.String(),
	}, nil
}

// parseFPSDefault parses a string fps; falls back to def on any error.
func parseFPSDefault(s string, def int) int {
	if s == "" {
		return def
	}
	if v, err := strconv.Atoi(s); err == nil && v > 0 {
		return v
	}
	return def
}
