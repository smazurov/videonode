package pipeline

// MediaSource is the encoder-input abstraction. Wraps the upstream frame
// source (a Source's SCM socket or a Composer's SCM-out socket) and the
// audio source (today: direct ALSA).
type MediaSource struct {
	Video FrameSource
	Audio AudioSource
}

// FrameKind tags the wire format the source emits, so consumers (the
// EncoderStage argv builder) can pick the right ffmpeg input args.
type FrameKind int

// FrameKind values: Unknown is the zero value reserved for unset; the
// other two select the wire format vn-sink emits for ffmpeg.
const (
	FrameKindUnknown FrameKind = iota
	// FrameKindNV12Y4M is what vn-sink emits when consuming a producer
	// socket: YUV4MPEG2 wrapping NV12. ffmpeg consumes via
	// `-f yuv4mpegpipe -i pipe:0`.
	FrameKindNV12Y4M
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
}

// Kind reports the wire format: NV12 wrapped in YUV4MPEG2.
func (p ProducerFrameSource) Kind() FrameKind { return FrameKindNV12Y4M }

// SocketPath returns the producer SCM socket vn-sink dials.
func (p ProducerFrameSource) SocketPath() string { return p.Socket }

// Dims returns (0, 0) — producer dims come from the y4m header at runtime.
func (p ProducerFrameSource) Dims() (int, int) { return 0, 0 }

// FPS returns 0 — producer framerate comes from the y4m header at runtime.
func (p ProducerFrameSource) FPS() int { return 0 }

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

// AudioSource emits the encoder's audio-input argv fragment.
type AudioSource interface {
	InputArgs() []string
}

// ALSADirectAudio opens ALSA from the ffmpeg process.
type ALSADirectAudio struct {
	Config AudioConfig
}

// InputArgs returns one ffmpeg input fragment per audio device. Each
// device produces one OUTPUT TRACK in the published stream.
func (a ALSADirectAudio) InputArgs() []string {
	if len(a.Config.Devices) == 0 {
		return nil
	}
	out := make([]string, 0, 14*len(a.Config.Devices))
	for _, dev := range a.Config.Devices {
		out = append(out,
			"-thread_queue_size", "1024",
			"-f", "alsa",
			"-sample_fmt", "s16",
			"-ar", "48000",
			"-ac", "2",
			"-i", dev,
		)
	}
	return out
}
