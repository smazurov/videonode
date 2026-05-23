package pipeline

import (
	"errors"
	"fmt"
	"log/slog"
	"strconv"

	"github.com/smazurov/videonode/internal/ffmpeg"
	"github.com/smazurov/videonode/internal/process"
)

// EncoderStage is the per-stream Encoder process: `vn-sink | ffmpeg`.
// vn-sink dials either the producer's SCM socket (NV12 → Y4M) or the
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

// ID returns the stage's process.Pool key: "encoder:<stream-id>".
func (e *EncoderStage) ID() string { return "encoder:" + e.StreamID_ }

// Kind reports this as an Encoder stage for logging and pool routing.
func (e *EncoderStage) Kind() Kind { return KindEncoder }

// StreamID returns the user-facing stream id.
func (e *EncoderStage) StreamID() string { return e.StreamID_ }

// Command builds the shell command `vn-sink --socket X | ffmpeg ...`.
// vn-sink's auto-detection (NV12 → Y4M vs BGRA → raw) means the ffmpeg
// input args differ per FrameSource kind; the rest of the encoder argv
// is identical regardless of upstream source.
func (e *EncoderStage) Command() ([]string, []string, error) {
	if e.Media.Video == nil {
		return nil, nil, errors.New("encoder: media.video is nil")
	}
	if e.Media.Video.SocketPath() == "" {
		return nil, nil, errors.New("encoder: media.video has no socket path")
	}
	if e.VNSinkBin == "" {
		return nil, nil, errors.New("encoder: VNSinkBin path is required")
	}
	if len(e.Publish) == 0 {
		return nil, nil, errors.New("encoder: at least one PublishTarget is required")
	}

	sinkArgv := []string{e.VNSinkBin, "--socket", e.Media.Video.SocketPath()}

	// Input fragment: daemon-owned. CustomEncoderArgs (when present)
	// only overrides the encoder + output side, never the input.
	ffmpegArgv := []string{
		"ffmpeg",
		"-hide_banner", "-loglevel", "warning",
	}
	ffmpegArgv = append(ffmpegArgv, videoInputArgs(e.Media.Video)...)

	// Audio input fragment (multiple -i if N>1).
	if e.Media.Audio != nil {
		ffmpegArgv = append(ffmpegArgv, e.Media.Audio.InputArgs()...)
	}

	if e.CustomEncoderArgs != "" {
		// User-owned tail. The split is whitespace-naive; users wanting
		// argv with embedded whitespace should pre-escape — same contract
		// as the legacy CustomFFmpegCommand.
		ffmpegArgv = append(ffmpegArgv, splitShellWords(e.CustomEncoderArgs)...)
	} else {
		ffmpegArgv = append(ffmpegArgv, encoderTailArgs(e.Cfg, e.Publish)...)
	}

	// Wrap as `/bin/sh -c "vn-sink ... | ffmpeg ..."` so the pipe is
	// shell-managed. process.Pool supervises this as one entry; vn-sink
	// + ffmpeg share a death (the encoder stage is one logical thing).
	cmd := shellPipe(sinkArgv, ffmpegArgv)
	return []string{"/bin/sh", "-c", cmd}, nil, nil
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

// videoInputArgs returns the ffmpeg `-f ... -i pipe:0` fragment for
// vn-sink's upstream. NV12-Y4M is self-describing (no -s/-framerate);
// BGRA-raw needs explicit dims + framerate at the input stage.
func videoInputArgs(fs FrameSource) []string {
	switch fs.Kind() {
	case FrameKindBGRARaw:
		w, h := fs.Dims()
		return []string{
			"-f", "rawvideo",
			"-pix_fmt", "bgra",
			"-s", fmt.Sprintf("%dx%d", w, h),
			"-framerate", strconv.Itoa(fs.FPS()),
			"-i", "pipe:0",
		}
	case FrameKindNV12Y4M, FrameKindUnknown:
		fallthrough
	default:
		return []string{"-f", "yuv4mpegpipe", "-i", "pipe:0"}
	}
}

// encoderTailArgs maps the backend-agnostic EncoderConfig + Publish
// targets to ffmpeg's `-c:v ... -f rtsp <url>` tail. Picks a backend
// encoder name based on Codec; today defaults to rkmpp variants for
// h264/h265. Future work threads a probed-encoder argument through.
func encoderTailArgs(cfg EncoderConfig, publish []PublishTarget) []string {
	encName := encoderNameFor(cfg.Codec)
	bitrate := cfg.Bitrate
	if bitrate == "" {
		bitrate = "4M"
	}
	gop := cfg.GOP
	if gop <= 0 {
		gop = 60
	}
	rcMode := cfg.RateControl
	if rcMode == "" {
		rcMode = "vbr"
	}

	args := []string{
		"-c:v", encName,
		"-rc_mode", rcModeFFmpeg(rcMode),
		"-b:v", bitrate,
		"-g", strconv.Itoa(gop),
		"-bf", strconv.Itoa(cfg.BFrames),
		"-bsf:v", "dump_extra=freq=keyframe",
	}

	// For audio, default to AAC passthrough when an audio input is
	// present (the encoder stage's audio mux is the same regardless of
	// publish target). Bitrate/codec come from AudioConfig but the
	// MediaSource layer set the input args — here we just pick the
	// audio encoder if the caller wants one. Today: copy is wrong
	// (input is raw PCM), so emit aac always when there's audio.
	// TODO: derive from AudioConfig once it's plumbed in here.

	args = append(args, "-rtsp_transport", "tcp")
	for i, pt := range publish {
		args = append(args, "-f", publishFFmpegFormat(pt.Type), pt.URL)
		_ = i
	}
	return args
}

// encoderNameFor maps a backend-agnostic codec name to today's default
// ffmpeg encoder. Lives behind a function so future work (encoder
// probe) can pick libx264/libx265/etc. based on what's actually
// available on the host.
func encoderNameFor(codec string) string {
	switch codec {
	case "h265", "hevc":
		return "hevc_rkmpp"
	case "h264", "":
		return "h264_rkmpp"
	default:
		return codec
	}
}

// rcModeFFmpeg maps EncoderConfig.RateControl values to the strings
// rkmpp accepts. Other backends ignore -rc_mode.
func rcModeFFmpeg(rc string) string {
	switch rc {
	case "cbr", "CBR":
		return "CBR"
	case "cqp", "CQP":
		return "CQP"
	case "vbr", "VBR", "":
		return "VBR"
	default:
		return rc
	}
}

// publishFFmpegFormat picks the ffmpeg muxer name for each PublishTarget
// type. Unknown types fall through to the raw type string so the user
// gets ffmpeg's own "Unknown output format" error.
func publishFFmpegFormat(t string) string {
	switch t {
	case "rtsp":
		return "rtsp"
	case "srt":
		return "mpegts" // SRT carries mpegts; URL prefix selects transport
	case "hls":
		return "hls"
	default:
		return t
	}
}
