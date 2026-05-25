package pipeline

import (
	"errors"
	"log/slog"
	"path/filepath"

	"github.com/smazurov/videonode/internal/ffmpeg"
	"github.com/smazurov/videonode/internal/process"
)

// EncoderStage is the per-Stream Encoder process: `vn-sink | ffmpeg`.
// VN-sink dials either a producer's SCM socket (NV12 → Y4M) or a
// composer's `--scm-out` socket (BGRA → raw), and pipes to ffmpeg which
// encodes and publishes to the configured PublishTargets.
//
// One EncoderStage per stream, always present. Restart isolation works
// because vn-sink retry-dials its source SCM, so encoder respawn doesn't
// kill the producer or composer.
type EncoderStage struct {
	OwnerStreamID     string
	Media             MediaSource
	Cfg               EncoderConfig
	Resolved          EncoderResolution // populated by buildEncoder via Config.EncoderResolver
	Publish           []PublishTarget
	CustomEncoderArgs string // user override; replaces -c:v onward when set
	VNSinkBin         string // path to vn-sink binary
}

// EncoderIDFor returns the stable pool key for a stream's encoder
// stage. Stream-id is the encoder identity end-to-end.
func EncoderIDFor(streamID string) string { return "encoder:" + streamID }

// ID returns the stage's process.Pool key: "encoder:<stream-id>".
func (e *EncoderStage) ID() string { return EncoderIDFor(e.OwnerStreamID) }

// Kind reports this as an Encoder stage.
func (e *EncoderStage) Kind() Kind { return KindEncoder }

// StreamID returns the user-facing stream id.
func (e *EncoderStage) StreamID() string { return e.OwnerStreamID }

// Command builds the shell command `vn-sink --socket X | ffmpeg ...`.
func (e *EncoderStage) Command() ([]string, []string, error) {
	if e.Media.Video == nil {
		return nil, nil, errors.New("encoder: media.video is nil")
	}
	if e.CustomEncoderArgs == "" && len(e.Publish) == 0 {
		return nil, nil, errors.New("encoder: at least one PublishTarget is required")
	}
	if e.Media.Video.SocketPath() == "" {
		return nil, nil, errors.New("encoder: media.video has no socket path")
	}
	if e.VNSinkBin == "" {
		return nil, nil, errors.New("encoder: VNSinkBin path is required")
	}

	sinkArgv := []string{e.VNSinkBin, "--socket", e.Media.Video.SocketPath()}
	params := e.buildFFmpegParams()

	var ffmpegCmd string
	if e.CustomEncoderArgs != "" {
		ffmpegCmd = ffmpeg.BuildInputArgs(params) + " " + e.CustomEncoderArgs
	} else {
		ffmpegCmd = ffmpeg.BuildCommand(params)
	}

	cmd := shellJoinArgv(sinkArgv) + " | " + ffmpegCmd
	return []string{"/bin/sh", "-c", cmd}, nil, nil
}

// buildFFmpegParams projects EncoderConfig + MediaSource + Publish onto
// ffmpeg.Params. Applies the same defaults the legacy arg builder did
// (bitrate=4M, rc_mode=VBR for HW) so the shared builder sees a fully
// populated struct.
func (e *EncoderStage) buildFFmpegParams() *ffmpeg.Params {
	p := &ffmpeg.Params{
		InputPipe:    pipeInputFor(e.Media.Video),
		Encoder:      e.Resolved.EncoderName,
		GlobalArgs:   append([]string(nil), e.Resolved.GlobalArgs...),
		VideoFilters: e.Resolved.VideoFilters,
		Bitrate:      e.Cfg.Bitrate,
		GOP:          e.Cfg.GOP,
		BFrames:      e.Cfg.BFrames,
	}
	if p.Bitrate == "" {
		p.Bitrate = "4M"
	}
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
	p.ProgressSocket = ProgressSocketPathFor(e.OwnerStreamID)
	return p
}

// ProgressSocketPathFor returns the Unix socket path where the
// FFmpegCollector listens for progress data from ffmpeg's -progress flag.
func ProgressSocketPathFor(streamID string) string {
	return filepath.Join(NativeUdsDir, "progress-"+sanitizeForFilename(streamID)+".sock")
}

// pipeInputFor maps a FrameSource to the ffmpeg.PipeInput shape:
// NV12-Y4M is self-describing; BGRA-raw needs explicit dims + framerate.
func pipeInputFor(fs FrameSource) *ffmpeg.PipeInput {
	w, h := fs.Dims()
	switch fs.Kind() {
	case FrameKindNV12Raw:
		return &ffmpeg.PipeInput{
			Format:      "rawvideo",
			PixelFormat: "nv12",
			Width:       w,
			Height:      h,
			FPS:         fs.FPS(),
		}
	case FrameKindBGRARaw:
		return &ffmpeg.PipeInput{
			Format:      "rawvideo",
			PixelFormat: "bgra",
			Width:       w,
			Height:      h,
			FPS:         fs.FPS(),
		}
	default:
		return &ffmpeg.PipeInput{
			Format:      "rawvideo",
			PixelFormat: "nv12",
			Width:       w,
			Height:      h,
			FPS:         fs.FPS(),
		}
	}
}

// LogParser delegates to ffmpeg.ParseLogLevel — ffmpeg's `[level] msg`
// format also matches vn-sink's vn::log output.
func (e *EncoderStage) LogParser() process.LogParser { return ffmpeg.ParseLogLevel }

// LogAttrs tags every encoder log line with the stream id + pool-key.
func (e *EncoderStage) LogAttrs() []slog.Attr {
	return []slog.Attr{
		slog.String("stream_id", e.OwnerStreamID),
		slog.String("stage_instance", e.ID()),
	}
}

// Reconfigure always returns ErrRequiresRestart: encoder has no live
// control plane today; any change requires restart.
func (e *EncoderStage) Reconfigure(_ any) error { return ErrRequiresRestart }
