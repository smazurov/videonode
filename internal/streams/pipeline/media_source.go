package pipeline

// MediaSource is the encoder-input abstraction. Wraps the upstream frame
// source (a Source's SCM socket or a Composer's SCM-out socket) and the
// audio source (today: direct ALSA).
type MediaSource struct {
	Video FrameSource
	Audio ALSADirectAudio
}

// FrameKind tags the wire format the source emits, so consumers (the
// EncoderStage argv builder) can pick the right ffmpeg input args.
type FrameKind int

// FrameKind values select the wire format vn-sink emits for ffmpeg.
const (
	FrameKindUnknown FrameKind = iota
	// FrameKindNV12Raw — raw NV12 bytes (Y + UV planes) on stdout.
	FrameKindNV12Raw
	// FrameKindBGRARaw is what vn-sink emits when consuming a composer
	// socket: raw BGRA bytes. ffmpeg consumes via
	// `-f rawvideo -pix_fmt bgra -s WxH -framerate N -i pipe:0`.
	FrameKindBGRARaw
)

// FrameSource describes one upstream video source the encoder dials.
type FrameSource interface {
	Kind() FrameKind
	SocketPath() string
	Dims() (w int, h int)
	FPS() int
}

// ProducerFrameSource — encoder dialing a Source's SCM socket directly.
type ProducerFrameSource struct {
	Socket string
	Width  int
	Height int
	Fps    int
}

// Kind reports raw NV12 wire format.
func (p ProducerFrameSource) Kind() FrameKind { return FrameKindNV12Raw }

// SocketPath returns the producer SCM socket vn-sink dials.
func (p ProducerFrameSource) SocketPath() string { return p.Socket }

// Dims returns the source capture resolution.
func (p ProducerFrameSource) Dims() (int, int) { return p.Width, p.Height }

// FPS returns the source capture framerate.
func (p ProducerFrameSource) FPS() int { return p.Fps }

// ComposerFrameSource — encoder dialing a Composer's `--scm-out` socket.
type ComposerFrameSource struct {
	Socket string
	Width  int
	Height int
	Fps    int
}

// Kind reports the wire format: raw BGRA.
func (c ComposerFrameSource) Kind() FrameKind { return FrameKindBGRARaw }

// SocketPath returns the composer --scm-out socket vn-sink dials.
func (c ComposerFrameSource) SocketPath() string { return c.Socket }

// Dims returns the canvas (width, height) ffmpeg needs to interpret raw BGRA.
func (c ComposerFrameSource) Dims() (int, int) { return c.Width, c.Height }

// FPS returns the canvas framerate ffmpeg uses on its `-framerate` flag.
func (c ComposerFrameSource) FPS() int { return c.Fps }

// ALSADirectAudio opens ALSA from the ffmpeg process. Its devices are
// projected onto ffmpeg.Params.AudioInputs in EncoderStage.buildFFmpegParams;
// the shared builder emits the per-device input fragments.
type ALSADirectAudio struct {
	Config AudioConfig
}
