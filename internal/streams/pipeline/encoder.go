package pipeline

import (
	"errors"
	"log/slog"
	"strconv"

	"github.com/smazurov/videonode/internal/ffmpeg"
	"github.com/smazurov/videonode/internal/process"
)

// EncoderStage is the per-stream Encoder process: `vn-sink | ffmpeg`.
// VN-sink dials either the producer's SCM socket (NV12 → Y4M) or the
// composer's --scm-out socket (BGRA → raw), and pipes to ffmpeg which
// encodes and publishes to the configured PublishTarget(s).
//
// One EncoderStage per stream, always present. Restart isolation works
// because vn-sink retry-dials its source SCM (via scm_rights_source's
// 30s window) — encoder respawn doesn't kill the producer or composer.
type EncoderStage struct {
	StreamID_         string
	Media             MediaSource
	Cfg               EncoderConfig
	Publish           []PublishTarget
	CustomEncoderArgs string // user override; replaces -c:v onward when set
	VNSinkBin         string // path to vn-sink binary
}

// EncoderIDFor returns the stable pool key for a stream's encoder
// stage. Stream-id is the encoder identity end-to-end.
func EncoderIDFor(streamID string) string { return "encoder:" + streamID }

// ID returns the stage's process.Pool key: "encoder:<stream-id>".
func (e *EncoderStage) ID() string { return EncoderIDFor(e.StreamID_) }

// Kind reports this as an Encoder stage for logging and pool routing.
func (e *EncoderStage) Kind() Kind { return KindEncoder }

// StreamID returns the user-facing stream id.
func (e *EncoderStage) StreamID() string { return e.StreamID_ }

// composerInlineArgv returns the argv that should be the FIRST half of
// the shell pipe when the encoder bundles its own composer (inline
// composer mode). Returns nil for non-inline FrameSources.
//
// Seeds the composer's pre-ready canvas with --canvas-w / --canvas-h
// so the very first frame emitted matches what ffmpeg's `-s WxH` is
// expecting on the receive side. Without that, composer defaults to
// 1280x720 BGRA frames, ffmpeg parses them at 1920x1080-byte windows,
// and produces no valid output until the daemon's SetCanvas RPC lands
// (which can take longer than the smoke window allows).
func composerInlineArgv(fs FrameSource) []string {
	icfs, ok := fs.(InlineComposerFrameSource)
	if !ok {
		return nil
	}
	fps := icfs.Fps
	if fps <= 0 {
		fps = 30
	}
	argv := []string{
		icfs.ComposerBin,
		"--drm-device", icfs.DRMDevice,
		"--grpc-listen", icfs.GrpcUds,
		"--composer-id", icfs.ComposerID,
		"--target-fps", strconv.Itoa(fps),
	}
	if icfs.Width > 0 && icfs.Height > 0 {
		argv = append(argv,
			"--canvas-w", strconv.Itoa(icfs.Width),
			"--canvas-h", strconv.Itoa(icfs.Height),
		)
	}
	return argv
}

// Command builds the shell command `vn-sink --socket X | ffmpeg ...`.
// VN-sink's auto-detection (NV12 → Y4M vs BGRA → raw) means the ffmpeg
// input args differ per FrameSource kind; the rest of the encoder argv
// is identical regardless of upstream source.
//
// When CustomEncoderArgs is non-empty the user-supplied string is
// appended verbatim AFTER the daemon-owned input fragment (matching
// the legacy CustomFFmpegCommand contract — full shell expansion,
// quoting, etc.) The daemon prepends only `vn-sink --socket X |
// ffmpeg <input args>`; the user owns everything from there.
func (e *EncoderStage) Command() ([]string, []string, error) {
	if e.Media.Video == nil {
		return nil, nil, errors.New("encoder: media.video is nil")
	}
	if e.CustomEncoderArgs == "" && len(e.Publish) == 0 {
		return nil, nil, errors.New("encoder: at least one PublishTarget is required")
	}

	// Inline-composer mode: encoder spawns composer as the first half
	// of the shell pipe. Skip vn-sink entirely; composer writes BGRA
	// rawvideo to its stdout which the encoder ffmpeg consumes.
	inlineArgv := composerInlineArgv(e.Media.Video)
	var sinkArgv []string
	if inlineArgv != nil {
		sinkArgv = inlineArgv
	} else {
		if e.Media.Video.SocketPath() == "" {
			return nil, nil, errors.New("encoder: media.video has no socket path")
		}
		if e.VNSinkBin == "" {
			return nil, nil, errors.New("encoder: VNSinkBin path is required")
		}
		sinkArgv = []string{e.VNSinkBin, "--socket", e.Media.Video.SocketPath()}
	}

	params := e.buildFFmpegParams()

	var ffmpegCmd string
	if e.CustomEncoderArgs != "" {
		// User-owned tail as a raw shell string (verbatim). The user gets
		// $VAR / $(cmd) / single-quoted runs / backticks — same contract
		// as legacy CustomFFmpegCommand.
		ffmpegCmd = ffmpeg.BuildInputArgs(params) + " " + e.CustomEncoderArgs
	} else {
		ffmpegCmd = ffmpeg.BuildCommand(params)
	}

	// Wrap as `/bin/sh -c "vn-sink ... | ffmpeg ..."` so the pipe is
	// shell-managed. process.Pool supervises this as one entry; vn-sink
	// + ffmpeg share a death (the encoder stage is one logical thing).
	cmd := shellJoinArgv(sinkArgv) + " | " + ffmpegCmd
	return []string{"/bin/sh", "-c", cmd}, nil, nil
}

// buildFFmpegParams projects the pipeline's EncoderConfig + MediaSource +
// Publish targets onto ffmpeg.Params. Defaults that the legacy pipeline
// arg builder applied (bitrate=4M, gop=60, rc_mode=VBR for HW) are
// applied here at the boundary so the shared builder sees a fully
// populated struct.
func (e *EncoderStage) buildFFmpegParams() *ffmpeg.Params {
	p := &ffmpeg.Params{
		InputPipe:    pipeInputFor(e.Media.Video),
		Encoder:      e.Cfg.EncoderName,
		GlobalArgs:   append([]string(nil), e.Cfg.GlobalArgs...),
		VideoFilters: e.Cfg.VideoFilters,
		Bitrate:      e.Cfg.Bitrate,
		GOP:          e.Cfg.GOP,
		BFrames:      e.Cfg.BFrames,
	}
	if p.Encoder == "" {
		p.Encoder = "libx264"
	}
	if p.Bitrate == "" {
		p.Bitrate = "4M"
	}
	// rkmpp requires -rc_mode; other backends ignore it via the builder.
	switch e.Cfg.RateControl {
	case "cbr", "CBR":
		p.RCMode = "CBR"
	case "cqp", "CQP":
		p.RCMode = "CQP"
	default:
		p.RCMode = "VBR"
	}

	if alsa, ok := e.Media.Audio.(ALSADirectAudio); ok && len(alsa.Config.Devices) > 0 {
		p.AudioInputs = append([]string(nil), alsa.Config.Devices...)
	}

	for _, pt := range e.Publish {
		p.Outputs = append(p.Outputs, ffmpeg.OutputTarget{Type: pt.Type, URL: pt.URL})
	}
	return p
}

// pipeInputFor maps a FrameSource to the ffmpeg.PipeInput shape:
// NV12-Y4M is self-describing; BGRA-raw needs explicit dims + framerate.
func pipeInputFor(fs FrameSource) *ffmpeg.PipeInput {
	switch fs.Kind() {
	case FrameKindBGRARaw:
		w, h := fs.Dims()
		return &ffmpeg.PipeInput{
			Format:      "rawvideo",
			PixelFormat: "bgra",
			Width:       w,
			Height:      h,
			FPS:         fs.FPS(),
		}
	case FrameKindNV12Y4M, FrameKindUnknown:
		fallthrough
	default:
		return &ffmpeg.PipeInput{Format: "yuv4mpegpipe"}
	}
}

// LogParser delegates to ffmpeg.ParseLogLevel — ffmpeg's `[level] msg`
// format is also what vn-sink emits (via vn::log helpers), so one
// parser handles both halves of the shell-piped pair.
func (e *EncoderStage) LogParser() process.LogParser {
	return ffmpeg.ParseLogLevel
}

// LogAttrs tags every encoder log line with the user-facing stream id
// and the pool-key instance.
func (e *EncoderStage) LogAttrs() []slog.Attr {
	return []slog.Attr{
		slog.String("stream_id", e.StreamID_),
		slog.String("stage_instance", e.ID()),
	}
}

// Reconfigure: today's encoder has no live control plane. Bitrate /
// codec / publish target changes all require restart. Follow-up work
// (rkmpp hot bitrate) can wire SetBitrate via the future encoder gRPC,
// at which point this returns nil for in-place updates.
func (e *EncoderStage) Reconfigure(_ any) error { return ErrRequiresRestart }
