package pipeline

import (
	"errors"
	"log/slog"

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
	StreamID_         string
	Media             MediaSource
	Cfg               EncoderConfig
	Publish           []PublishTarget
	CustomEncoderArgs string // user override; replaces -c:v onward when set
	VNSinkBin         string // path to vn-sink binary
}

// ID returns the stage's process.Pool key: "encoder:<stream-id>".
func (e *EncoderStage) ID() string { return EncoderIDFor(e.StreamID_) }

// Kind reports this as an Encoder stage.
func (e *EncoderStage) Kind() Kind { return KindEncoder }

// StreamID returns the user-facing stream id.
func (e *EncoderStage) StreamID() string { return e.StreamID_ }

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
// format also matches vn-sink's vn::log output.
func (e *EncoderStage) LogParser() process.LogParser { return ffmpeg.ParseLogLevel }

// LogAttrs tags every encoder log line with the stream id + pool-key.
func (e *EncoderStage) LogAttrs() []slog.Attr {
	return []slog.Attr{
		slog.String("stream_id", e.StreamID_),
		slog.String("stage_instance", e.ID()),
	}
}

// Reconfigure: encoder has no live control plane today; any change
// requires restart.
func (e *EncoderStage) Reconfigure(_ any) error { return ErrRequiresRestart }
