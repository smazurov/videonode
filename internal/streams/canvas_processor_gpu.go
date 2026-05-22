package streams

import (
	"fmt"
	"net"
	"strconv"
	"strings"

	"github.com/smazurov/videonode/internal/streams/pipelinectl"
)

const (
	gpuDRMDevice       = "/dev/dri/renderD130" // panthor on the rig
	defaultGPURTSPHost = "127.0.0.1:8554"      // fallback when ServiceOptions.RTSPPort is unset
)

// resolveRTSPHost normalizes the daemon's --streaming-rtsp-port flag
// to the host:port the GPU compose path's ffmpeg sink dials.
//
// Accepted forms:
//
//	""              → "127.0.0.1:8554" (well-known default)
//	":8654"         → "127.0.0.1:8654" (bind-on-all → dial loopback)
//	"0.0.0.0:8654"  → "127.0.0.1:8654" (any-address → dial loopback)
//	"host:port"     → unchanged
//
// Bare-port input ("8654" — no colon, no host) is rejected by falling
// back to the default rather than producing "rtsp://8654/<id>", which
// ffmpeg would (mis)interpret as host=8654 with no port and fail DNS.
func resolveRTSPHost(rtspPort string) string {
	if rtspPort == "" || rtspPort == ":" {
		return defaultGPURTSPHost
	}
	host, port, err := net.SplitHostPort(rtspPort)
	if err != nil {
		// Not parseable as host:port — most likely a bare number like
		// "8554". Fall back to the default and let ffmpeg fail loudly
		// on the next restart cycle (rather than silently dialing a
		// bogus URL).
		return defaultGPURTSPHost
	}
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}
	return net.JoinHostPort(host, port)
}

// composerOrchestration carries the daemon-pushed config for a GPU
// composer stream: the params the daemon sends over pipelinectl AFTER
// videonode-composer is spawned and identifies itself. ProcessStreamGPU
// builds this alongside the shell command; streamProcessManager hands
// it to a goroutine that waits for the composer's identify and then
// dispatches the set_canvas / set_source / set_layout / set_effects /
// set_source_state pushes.
type composerOrchestration struct {
	ComposerID string
	// GrpcUds is the per-instance UDS the composer binary is listening
	// on; the daemon dials in via Manager.RegisterComposer before pushing
	// the rest of the plan.
	GrpcUds string
	Canvas  pipelinectl.SetCanvasParams
	Sources []pipelinectl.SetSourceParams
	Layout  pipelinectl.SetLayoutParams
	Effects []pipelinectl.SetEffectsParams
	States  []pipelinectl.SetSourceStateParams
}

// composerIDFor returns the stable composer-id the daemon will send the
// composer with, matching what we tell it to identify as.
func composerIDFor(canvasID string) string {
	return canvasID + "-composer"
}

// processStreamGPU builds the **sink** command for the GPU composer path.
// The composer is launched with bare argv — no source / canvas / layout
// flags — and dials the daemon's pipelinectl UDS for its runtime config.
// Right after the process pool spawns the shell pipeline, the
// streamProcessManager kicks off a goroutine that watches for the
// composer's identify and pushes the orchestration plan we return here.
//
// The producer side (videonode-source process) is supervised separately
// by ProducerManager — see internal/streams/producer_manager.go. Acquire
// happens in streamProcessManager.Start before this function is called;
// we only need to read back the socket path so the daemon can push it
// to composer as part of set_source.
//
// Single source for now; the orchestration plan shape supports N slots
// and extends cleanly when the canvas processor wires multi-source GPU.
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
	// Composer is daemon-driven over pipelinectl. Without a live control
	// plane it would dial a non-listening socket forever and render solid
	// black. Refuse the GPU path here so the failure is visible (caller
	// surfaces the error) instead of silently emitting a black RTSP feed.
	if !cp.producerMgr.HasControlManager() {
		return nil, fmt.Errorf("canvas %s: GPU path requires pipelinectl (control plane not started)", canvasID)
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

	composerID := composerIDFor(canvasID)
	composerUds := GrpcSocketPathFor("composer", composerID)
	composerArgv := []string{
		cp.native.Composer,
		"--drm-device", gpuDRMDevice,
		"--grpc-listen", composerUds,
		"--composer-id", composerID,
		"--target-fps", strconv.Itoa(canvasFPS),
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
		"-f", "rtsp", fmt.Sprintf("rtsp://%s/%s", cp.rtspHostOrDefault(), canvasID),
	}

	// sh -c so the composer | ffmpeg pipe is set up by the shell. No
	// background process, no trap — producer is its own supervised process.
	var sb strings.Builder
	sb.WriteString(`/bin/sh -c "`)
	sb.WriteString(shellJoin(composerArgv))
	sb.WriteString(` | `)
	sb.WriteString(shellJoin(ffmpegArgv))
	sb.WriteString(`"`)

	// Build the orchestration plan the streamProcessManager will push to
	// the composer once it identifies. Today: one slot "a", layout
	// covers the full canvas, perspective effect if spec.Perspective is
	// non-nil. Source state starts "live"; future work subscribes to the
	// source's pipelinectl status feed and pushes set_source_state on
	// health change.
	plan := &composerOrchestration{
		ComposerID: composerID,
		GrpcUds:    composerUds,
		Canvas: pipelinectl.SetCanvasParams{
			W:   uint32(canvas.Width),
			H:   uint32(canvas.Height),
			FPS: uint32(canvasFPS),
		},
		Sources: []pipelinectl.SetSourceParams{{
			Slot:     "a",
			SourceID: srcID,
			ScmPath:  socketPath,
			Width:    uint32(parseDimOrZero(src.FFmpeg.Resolution, 0)),
			Height:   uint32(parseDimOrZero(src.FFmpeg.Resolution, 1)),
			FPS:      uint32(parseFPSOrZero(src.FFmpeg.FPS)),
		}},
		Layout: pipelinectl.SetLayoutParams{
			Slots: []pipelinectl.LayoutSlotEntry{{
				Slot: "a",
				X:    0, Y: 0,
				W: int32(canvas.Width),
				H: int32(canvas.Height),
			}},
		},
		States: []pipelinectl.SetSourceStateParams{{
			SourceID: srcID,
			State:    "live",
		}},
	}
	if src.Perspective != nil {
		eff := pipelinectl.EffectParams{
			Type:           "perspective",
			Corners:        src.Perspective.Corners,
			SnapshotWidth:  src.Perspective.SnapshotWidth,
			SnapshotHeight: src.Perspective.SnapshotHeight,
		}
		// Fall back to source's currently-configured input dims if the
		// stored perspective predates the snapshot_w/h fields. Logged
		// in the API layer (one-time warning); here we just substitute.
		if eff.SnapshotWidth == 0 || eff.SnapshotHeight == 0 {
			if w, h := parseInputDims(src.FFmpeg.Resolution); w > 0 && h > 0 {
				eff.SnapshotWidth = w
				eff.SnapshotHeight = h
			}
		}
		plan.Effects = append(plan.Effects, pipelinectl.SetEffectsParams{
			SourceID: srcID,
			Effects:  []pipelinectl.EffectParams{eff},
		})
	}

	return &ProcessedStream{
		StreamID:      canvasID,
		FFmpegCommand: sb.String(),
		ComposerPlan:  plan,
	}, nil
}

// rtspHostOrDefault returns the configured RTSP target, falling back to
// the well-known default for tests and other call sites that construct a
// canvasProcessor without going through NewStreamService.
func (cp *canvasProcessor) rtspHostOrDefault() string {
	if cp.rtspHost == "" {
		return defaultGPURTSPHost
	}
	return cp.rtspHost
}

// parseInputDims splits a "WxH" resolution string into its components.
// Returns (0,0) if the format is unrecognized — caller treats that as
// "unknown; don't fill snapshot dims.".
func parseInputDims(resolution string) (int, int) {
	parts := strings.SplitN(resolution, "x", 2)
	if len(parts) != 2 {
		return 0, 0
	}
	w, err1 := strconv.Atoi(parts[0])
	h, err2 := strconv.Atoi(parts[1])
	if err1 != nil || err2 != nil {
		return 0, 0
	}
	return w, h
}

// parseDimOrZero returns the i-th component (0=W, 1=H) of a "WxH"
// string, or 0 if the format is unrecognized.
func parseDimOrZero(resolution string, i int) int {
	w, h := parseInputDims(resolution)
	if i == 0 {
		return w
	}
	return h
}

// parseFPSOrZero parses a string like "30" into an int, returning 0 on
// failure (composer treats 0 as "driver decides").
func parseFPSOrZero(s string) int {
	if s == "" {
		return 0
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < 0 {
		return 0
	}
	return n
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
