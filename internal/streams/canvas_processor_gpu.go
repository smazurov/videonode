package streams

import (
	"fmt"
	"strconv"
	"strings"
)

const (
	gpuDRMDevice = "/dev/dri/renderD130" // panthor on the rig
	gpuRTSPHost  = "127.0.0.1:8554"      // daemon's embedded RTSP
)

// processStreamGPU builds the **sink** command for the GPU composer path:
//
//	videonode-composer --source-a-scm-path <S> --no-source-b \
//	  | ffmpeg -f rawvideo -pix_fmt bgra ... -f rtsp rtsp://127.0.0.1:8554/<id>
//
// The producer side (videonode-source sidecar) is supervised separately by the
// ProducerManager — see internal/streams/producer_manager.go. Acquire
// happens in streamProcessManager.Start before this function is called; we
// only need to read back the socket path. If the producer takes a moment
// to bind, the scm_rights_source dial retries for 30 s, so the sink
// survives a slightly cold start.
//
// Single source for now; videonode-composer supports 2 slots via
// --source-b-* flags and we'll extend when source-b is wired in.
func (cp *canvasProcessor) processStreamGPU(
	canvasID string,
	canvas *CanvasConfig,
	sourceSpecs map[string]*StreamSpec,
) (*ProcessedStream, error) {
	if len(canvas.SourceStreams) != 1 {
		return nil, fmt.Errorf("canvas %s: GPU path currently supports exactly 1 source (got %d)",
			canvasID, len(canvas.SourceStreams))
	}
	if cp.producerMgr == nil {
		return nil, fmt.Errorf("canvas %s: GPU path requires a ProducerManager (not wired)", canvasID)
	}
	if !cp.native.CanvasReady() {
		return nil, fmt.Errorf("canvas %s: GPU path requires videonode-composer + videonode-source binaries", canvasID)
	}
	srcID := canvas.SourceStreams[0]
	src := sourceSpecs[srcID]
	if src == nil {
		return nil, fmt.Errorf("canvas %s: source %q not found", canvasID, srcID)
	}
	socketPath, ok := cp.producerMgr.SocketPath(srcID)
	if !ok {
		return nil, fmt.Errorf("canvas %s: producer for source %q not acquired", canvasID, srcID)
	}
	canvasFPS, err := strconv.Atoi(canvas.FPS)
	if err != nil || canvasFPS <= 0 {
		return nil, fmt.Errorf("canvas %s: GPU composer needs integer FPS, got %q",
			canvasID, canvas.FPS)
	}

	composerArgv := []string{
		cp.native.Composer,
		"--drm-device", gpuDRMDevice,
		"--canvas-w", strconv.Itoa(canvas.Width),
		"--canvas-h", strconv.Itoa(canvas.Height),
		"--fps", strconv.Itoa(canvasFPS),
		"--no-source-b",
		"--source-a-scm-path", socketPath,
	}
	ffmpegArgv := []string{
		"ffmpeg",
		"-hide_banner", "-loglevel", "warning",
		"-f", "rawvideo", "-pix_fmt", "bgra",
		"-s", fmt.Sprintf("%dx%d", canvas.Width, canvas.Height),
		"-framerate", strconv.Itoa(canvasFPS),
		"-i", "pipe:0",
		"-c:v", "h264_rkmpp",
		"-profile:v", "high", "-level:v", "5.2",
		"-rc_mode", "VBR", "-b:v", "6M",
		"-g", strconv.Itoa(canvasFPS * 2), "-bf", "0",
		"-bsf:v", "dump_extra=freq=keyframe",
		"-rtsp_transport", "tcp",
		"-f", "rtsp", fmt.Sprintf("rtsp://%s/%s", gpuRTSPHost, canvasID),
	}

	// sh -c so the composer | ffmpeg pipe is set up by the shell. No
	// background sidecar, no trap — producer is its own supervised process.
	var sb strings.Builder
	sb.WriteString(`/bin/sh -c "`)
	sb.WriteString(shellJoin(composerArgv))
	sb.WriteString(` | `)
	sb.WriteString(shellJoin(ffmpegArgv))
	sb.WriteString(`"`)

	return &ProcessedStream{
		StreamID:      canvasID,
		FFmpegCommand: sb.String(),
	}, nil
}

func shellJoin(argv []string) string {
	parts := make([]string, 0, len(argv))
	for _, a := range argv {
		parts = append(parts, shellQuote(a))
	}
	return strings.Join(parts, " ")
}

func shellQuote(s string) string {
	if s == "" {
		return "''"
	}
	if !strings.ContainsAny(s, " \t\n\r'\"\\$`|&;<>(){}[]*?#~!") {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
