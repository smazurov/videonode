package validation

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// ProbeTimeout caps a single probe ffmpeg invocation.
const ProbeTimeout = 5 * time.Second

// Probe is one ffmpeg invocation exercising a single backend capability.
type Probe struct {
	Name string
	Args []string
	// ProducerArgs runs a second ffmpeg whose stdout pipes into the consumer's stdin (no shell involved).
	ProducerArgs []string
}

// runProbe execs the probe; returns ok=consumer exited 0, plus condensed stderr.
func runProbe(p Probe) (ok bool, stderr string) {
	_, ok, stderr = runProbeWithStdout(p, false)
	return ok, stderr
}

// runProbeStdout execs the probe and additionally returns captured stdout.
func runProbeStdout(p Probe) (stdout []byte, ok bool, stderr string) {
	return runProbeWithStdout(p, true)
}

// runHevcAlignmentProbe inspects bottom-row RGB; clean encoders ~(252,0,0), AMD VCN bug ~(0,135,0).
func runHevcAlignmentProbe(p Probe) (ok bool, stderr string) {
	out, ran, err := runProbeStdout(p)
	if !ran {
		return false, err
	}
	if len(out) < 3 {
		return false, fmt.Sprintf("alignment probe: short stdout (got %d bytes, want 3)", len(out))
	}
	r, g, _ := out[0], out[1], out[2]
	if r < 128 && g > 64 {
		return false, fmt.Sprintf("alignment defect: bottom row decoded as green (R=%d G=%d)", r, g)
	}
	return true, ""
}

func runProbeWithStdout(p Probe, captureStdout bool) (stdout []byte, ok bool, stderr string) {
	ctx, cancel := context.WithTimeout(context.Background(), ProbeTimeout)
	defer cancel()

	consumer := exec.CommandContext(ctx, "ffmpeg", p.Args...)
	var errBuf, outBuf bytes.Buffer
	consumer.Stderr = &errBuf
	if captureStdout {
		consumer.Stdout = &outBuf
	}

	var producer *exec.Cmd
	if len(p.ProducerArgs) > 0 {
		producer = exec.CommandContext(ctx, "ffmpeg", p.ProducerArgs...)
		producer.Stderr = nil
		pipe, err := producer.StdoutPipe()
		if err != nil {
			return nil, false, "probe stdout pipe: " + err.Error()
		}
		consumer.Stdin = pipe
		if err := producer.Start(); err != nil {
			return nil, false, "probe producer start: " + err.Error()
		}
		defer func() { _ = producer.Wait() }()
	}

	err := consumer.Run()

	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return nil, false, "probe timed out after " + ProbeTimeout.String()
	}
	if err != nil {
		return nil, false, condenseStderr(errBuf.String())
	}
	return outBuf.Bytes(), true, ""
}

// buildDecoderProbe pipes a synthetic encoded stream through hwaccel decode + hwdownload to null.
// PixFmt is backend-specific: rkmpp mjpeg needs yuvj420p; vaapi mjpeg needs yuvj422p (catches radeonsi VPP gap).
func buildDecoderProbe(hwaccel, hwOutputFormat, codec, pixFmt string) Probe {
	var producerCodec, demuxer string
	switch codec {
	case "h264":
		producerCodec, demuxer = pickH264Producer(hwaccel), "h264"
	case "hevc":
		producerCodec, demuxer = pickHEVCProducer(hwaccel), "hevc"
	case "mjpeg":
		producerCodec, demuxer = "mjpeg", "mjpeg"
	default:
		producerCodec, demuxer = "libx264", "h264"
	}

	producerArgs := []string{
		"-hide_banner", "-nostats", "-loglevel", "error",
	}
	// HW producers need their device set up too.
	switch {
	case strings.HasSuffix(producerCodec, "_vaapi"):
		producerArgs = append(producerArgs,
			"-vaapi_device", "/dev/dri/renderD128",
			"-f", "lavfi", "-i", "testsrc2=size=320x240:rate=10:duration=0.3",
			"-vf", "format=nv12,hwupload",
			"-c:v", producerCodec)
	case strings.HasSuffix(producerCodec, "_rkmpp"):
		producerArgs = append(producerArgs,
			"-init_hw_device", "rkmpp=hw", "-filter_hw_device", "hw",
			"-f", "lavfi", "-i", "testsrc2=size=320x240:rate=10:duration=0.3",
			"-c:v", producerCodec)
	default:
		producerArgs = append(producerArgs,
			"-f", "lavfi", "-i", "testsrc2=size=320x240:rate=10:duration=0.3",
			"-c:v", producerCodec)
	}
	if pixFmt != "" {
		producerArgs = append(producerArgs, "-pix_fmt", pixFmt)
	}
	producerArgs = append(producerArgs, "-f", demuxer, "-")

	return Probe{
		Name: codec,
		Args: []string{
			"-hide_banner", "-nostats", "-loglevel", "error",
			"-hwaccel", hwaccel, "-hwaccel_output_format", hwOutputFormat,
			"-f", demuxer, "-i", "-",
			"-vf", "hwdownload,format=nv12",
			"-frames:v", "1", "-f", "null", "-",
		},
		ProducerArgs: producerArgs,
	}
}

// pickH264Producer prefers the matching HW encoder; falls back to libx264.
func pickH264Producer(hwaccel string) string {
	switch hwaccel {
	case "rkmpp":
		if isEncoderCompiled("h264_rkmpp") {
			return "h264_rkmpp"
		}
	case "vaapi":
		if isEncoderCompiled("h264_vaapi") {
			return "h264_vaapi"
		}
	}
	return "libx264"
}

func pickHEVCProducer(hwaccel string) string {
	switch hwaccel {
	case "rkmpp":
		if isEncoderCompiled("hevc_rkmpp") {
			return "hevc_rkmpp"
		}
	case "vaapi":
		if isEncoderCompiled("hevc_vaapi") {
			return "hevc_vaapi"
		}
	}
	return "libx265"
}

// condenseStderr keeps non-empty stderr lines (last 8) joined by "; ".
func condenseStderr(s string) string {
	var out []string
	for line := range strings.SplitSeq(s, "\n") {
		t := strings.TrimSpace(line)
		if t != "" {
			out = append(out, t)
		}
	}
	const maxLines = 8
	if len(out) > maxLines {
		out = out[len(out)-maxLines:]
	}
	return strings.Join(out, "; ")
}
