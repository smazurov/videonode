package pipeline

import (
	"errors"
	"log/slog"
	"path/filepath"
	"strconv"

	"github.com/smazurov/videonode/internal/ffmpeg"
	"github.com/smazurov/videonode/internal/logging"
	"github.com/smazurov/videonode/internal/process"
)

// EncoderStage is the per-Stream Encoder process: `vn-sink | ffmpeg`.
// VN-sink dials a producer's or composer's SCM socket, captures the
// stream's ALSA devices itself, and pipes paced NV12 video plus gapless PCM
// tracks as one NUT stream to ffmpeg, which encodes and publishes to the
// daemon's local RTSP relay (OutputURL). Audio device stalls or unplugs
// therefore never stall ffmpeg's video.
//
// One EncoderStage per stream, always present. Restart isolation works
// because vn-sink retry-dials its source SCM, so encoder respawn doesn't
// kill the producer or composer.
type EncoderStage struct {
	OwnerStreamID     string
	Media             MediaSource
	Cfg               EncoderConfig
	Resolved          EncoderResolution // populated by buildEncoder via Config.EncoderResolver
	OutputURL         string            // local RTSP relay URL, hardcoded by buildEncoder
	CustomEncoderArgs string            // user override; replaces -c:v onward when set
	VNSinkBin         string            // path to vn-sink binary
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

// Command builds the shell command `vn-sink --socket X --mux nut ... | ffmpeg ...`.
func (e *EncoderStage) Command() ([]string, []string, error) {
	if e.Media.Video == nil {
		return nil, nil, errors.New("encoder: media.video is nil")
	}
	if e.CustomEncoderArgs == "" && e.OutputURL == "" {
		return nil, nil, errors.New("encoder: OutputURL is required")
	}
	if e.Media.Video.SocketPath() == "" {
		return nil, nil, errors.New("encoder: media.video has no socket path")
	}
	if e.VNSinkBin == "" {
		return nil, nil, errors.New("encoder: VNSinkBin path is required")
	}

	sinkArgv := e.sinkArgv()
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

// defaultSinkFPS paces vn-sink when the upstream reports no frame rate.
const defaultSinkFPS = 30

// sinkArgv builds vn-sink's argv: NUT mux, the pacing frame rate, and one
// --audio-device per configured ALSA device (mapped through the capture
// config so dsnoop sharing applies).
func (e *EncoderStage) sinkArgv() []string {
	fps := e.Media.Video.FPS()
	if fps <= 0 {
		fps = defaultSinkFPS
	}
	argv := []string{
		e.VNSinkBin,
		"--socket", e.Media.Video.SocketPath(),
		"--mux", "nut",
		"--fps", strconv.Itoa(fps),
		"--status-socket", ProgressSocketPathFor(e.OwnerStreamID),
	}
	audio := e.Media.Audio.Config
	if len(audio.Devices) == 0 {
		return argv
	}
	for _, d := range audio.Devices {
		argv = append(argv, "--audio-device", ffmpeg.MapCaptureDevice(d))
	}
	argv = append(argv, "--audio-rate", "48000", "--audio-channels", "2")
	if offset := max(-MaxAudioOffsetMS, min(MaxAudioOffsetMS, audio.OffsetMS)); offset != 0 {
		argv = append(argv, "--audio-offset-ms", strconv.Itoa(offset))
	}
	return argv
}

// buildFFmpegParams projects EncoderConfig + MediaSource + OutputURL onto
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

	if audio := e.Media.Audio.Config; len(audio.Devices) > 0 {
		p.InputPipe.AudioTracks = len(audio.Devices)
		p.AudioCodec = audio.Codec
		p.AudioBitrate = audio.Bitrate
		p.AudioFilters = audio.Filters
	}

	if e.OutputURL != "" {
		p.Outputs = append(p.Outputs, ffmpeg.OutputTarget{Type: "rtsp", URL: e.OutputURL})
	}
	p.ProgressSocket = ProgressSocketPathFor(e.OwnerStreamID)
	return p
}

// ProgressSocketPathFor returns the Unix socket path where the
// EncoderCollector listens for ffmpeg's -progress reports and vn-sink's
// --status-socket reports.
func ProgressSocketPathFor(streamID string) string {
	return filepath.Join(NativeUdsDir, "progress-"+sanitizeForFilename(streamID)+".sock")
}

// pipeInputFor maps a FrameSource to the ffmpeg.PipeInput shape. The sink
// emits self-describing NUT (NV12 dims and rate in its headers); the
// colorimetry tag, which NUT cannot carry, distinguishes sources and composers.
func pipeInputFor(fs FrameSource) *ffmpeg.PipeInput {
	return &ffmpeg.PipeInput{
		Format: "nut",
		Color:  ffmpeg.ColorTagsForMatrix(fs.Colorimetry()),
	}
}

// LogParser delegates to ffmpeg.ParseLogLevel — ffmpeg's `[level] msg`
// format also matches vn-sink's vn::log output.
func (e *EncoderStage) LogParser() process.LogParser { return ffmpeg.ParseLogLine }

// LogAttrs tags every encoder log line with the stream id + pool-key.
func (e *EncoderStage) LogAttrs() []slog.Attr {
	return []slog.Attr{
		slog.String(logging.KeyStreamID, e.OwnerStreamID),
		slog.String(logging.KeyStageInstance, e.ID()),
	}
}
