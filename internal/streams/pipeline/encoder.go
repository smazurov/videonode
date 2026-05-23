package pipeline

import (
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"

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

// ID returns the stage's process.Pool key: "encoder:<stream-id>".
func (e *EncoderStage) ID() string { return "encoder:" + e.StreamID_ }

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

	// Daemon-owned input fragment. Custom args (when present) take
	// over from -c:v onward but never touch the input plumbing.
	ffmpegHead := []string{
		"ffmpeg",
		"-hide_banner", "-loglevel", "warning",
	}
	ffmpegHead = append(ffmpegHead, videoInputArgs(e.Media.Video)...)
	if e.Media.Audio != nil {
		ffmpegHead = append(ffmpegHead, e.Media.Audio.InputArgs()...)
	}

	var ffmpegCmd string
	if e.CustomEncoderArgs != "" {
		// Append user-owned tail as a raw shell string (verbatim). The
		// user gets $VAR / $(cmd) / single-quoted runs / backticks —
		// same contract as legacy CustomFFmpegCommand.
		ffmpegCmd = shellJoinArgv(ffmpegHead) + " " + e.CustomEncoderArgs
	} else {
		audioCount := 0
		if alsa, ok := e.Media.Audio.(ALSADirectAudio); ok {
			audioCount = len(alsa.Config.Devices)
		}
		ffmpegFull := make([]string, 0, len(ffmpegHead)+12+4*audioCount)
		ffmpegFull = append(ffmpegFull, ffmpegHead...)
		ffmpegFull = append(ffmpegFull, encoderTailArgs(e.Cfg, e.Publish, audioCount)...)
		ffmpegCmd = shellJoinArgv(ffmpegFull)
	}

	// Wrap as `/bin/sh -c "vn-sink ... | ffmpeg ..."` so the pipe is
	// shell-managed. process.Pool supervises this as one entry; vn-sink
	// + ffmpeg share a death (the encoder stage is one logical thing).
	cmd := shellJoinArgv(sinkArgv) + " | " + ffmpegCmd
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
// h264/h265.
//
// audioCount is the number of ALSA inputs that precede the encoder
// (each declared via `-f alsa -i hw:X,Y`). For audioCount > 0 we
// emit:
//   - a `-filter_complex` with one aresample chain per audio input
//     (matches legacy composite.go: prevents A/V drift on long runs)
//   - explicit `-map 0:v` + `-map "[aN]"` per track so each input
//     becomes its own OUTPUT TRACK (NOT mixed)
//   - one `-c:a libopus -b:a 128k -ar 48000` covering all tracks
//
// Mixing semantics (what we explicitly do NOT do): the rip incorrectly
// introduced an `amix=inputs=N:duration=longest` filter, collapsing
// N audio devices to one track. Legacy behavior + the API doc on
// `CanvasConfig.AudioDevices` says "one output audio track per
// entry"; that is what this function emits.
func encoderTailArgs(cfg EncoderConfig, publish []PublishTarget, audioCount int) []string {
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

	args := make([]string, 0, 16+4*audioCount)

	// Audio: aresample per input + explicit map for each output track.
	// Must come before -c:v so ffmpeg knows the stream selection
	// before the encoder picks them up. Skipped entirely when no
	// audio inputs were attached.
	if audioCount > 0 {
		args = append(args, "-filter_complex", buildAudioFilterComplex(audioCount))
		args = append(args, "-map", "0:v")
		for k := 0; k < audioCount; k++ {
			args = append(args, "-map", fmt.Sprintf("[a%d]", k))
		}
	}

	args = append(args,
		"-c:v", encName,
		"-rc_mode", rcModeFFmpeg(rcMode),
		"-b:v", bitrate,
		"-g", strconv.Itoa(gop),
		"-bf", strconv.Itoa(cfg.BFrames),
		"-bsf:v", "dump_extra=freq=keyframe",
	)

	if audioCount > 0 {
		args = append(args, "-c:a", "libopus", "-b:a", "128k", "-ar", "48000")
	}

	args = append(args, "-rtsp_transport", "tcp")
	for _, pt := range publish {
		args = append(args, "-f", publishFFmpegFormat(pt.Type), pt.URL)
	}
	return args
}

// buildAudioFilterComplex returns the `-filter_complex` value that
// applies aresample per audio input. Audio inputs are at ffmpeg
// indices [1, audioCount] (video is input 0). Output labels are
// [a0]..[aN-1] for the per-track map references.
//
// The aresample params (`async=1:min_hard_comp=0.100000:first_pts=0`)
// match what legacy composite.go used — they soften A/V drift over
// long streams without dropping samples.
func buildAudioFilterComplex(audioCount int) string {
	var b strings.Builder
	for k := 0; k < audioCount; k++ {
		if k > 0 {
			b.WriteString(";")
		}
		// ffmpeg input index = k+1 (input 0 is video from pipe:0)
		fmt.Fprintf(&b, "[%d:a]aresample=async=1:min_hard_comp=0.100000:first_pts=0[a%d]",
			k+1, k)
	}
	return b.String()
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
