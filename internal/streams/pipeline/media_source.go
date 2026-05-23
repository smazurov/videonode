package pipeline

// MediaSource is the encoder-input abstraction. Wraps the upstream
// frame source (composer-output socket, or producer-output socket at
// N=1+no-effects) plus the audio source (today: direct ALSA; future:
// SCM-fanned-out producer audio).
//
// The Encoder stage constructs a MediaSource from these two pieces and
// uses it to build the encoder argv. Audio is currently always
// ALSADirectAudio — moving audio capture to the producer side later is
// a swap of AudioSource implementations without touching the encoder.
type MediaSource struct {
	Video FrameSource
	Audio AudioSource
}

// FrameKind tags the wire format the source emits, so consumers (e.g.
// the EncoderStage argv builder) can pick the right ffmpeg input args.
type FrameKind int

const (
	FrameKindUnknown FrameKind = iota
	// FrameKindNV12Y4M is what vn-sink emits when consuming a producer
	// socket: YUV4MPEG2 wrapping NV12 input. ffmpeg consumes via
	// `-f yuv4mpegpipe -i pipe:0` (self-describing dims).
	FrameKindNV12Y4M
	// FrameKindBGRARaw is what vn-sink emits when consuming a composer
	// socket: raw BGRA bytes. ffmpeg consumes via
	// `-f rawvideo -pix_fmt bgra -s WxH -framerate N -i pipe:0`.
	FrameKindBGRARaw
)

// FrameSource describes one upstream video source the encoder dials.
type FrameSource interface {
	// Kind is the wire format vn-sink will emit; the encoder argv
	// builder selects ffmpeg input args from this.
	Kind() FrameKind
	// SocketPath is the SCM_RIGHTS socket vn-sink dials.
	SocketPath() string
	// Dims returns canvas dims. Required for FrameKindBGRARaw (ffmpeg
	// needs -s WxH at the input stage); zero-valued for NV12 (Y4M is
	// self-describing).
	Dims() (w int, h int)
	// FPS returns the target frame rate. Required for FrameKindBGRARaw
	// (-framerate N at the input stage); zero-valued for NV12.
	FPS() int
}

// ProducerFrameSource — encoder dialing a producer's SCM socket directly
// (the N=1+no-effects path). Producer emits NV12, vn-sink wraps it as
// Y4M, ffmpeg auto-detects dims.
type ProducerFrameSource struct {
	Socket string
}

func (p ProducerFrameSource) Kind() FrameKind     { return FrameKindNV12Y4M }
func (p ProducerFrameSource) SocketPath() string  { return p.Socket }
func (p ProducerFrameSource) Dims() (int, int)    { return 0, 0 }
func (p ProducerFrameSource) FPS() int            { return 0 }

// ComposerFrameSource — encoder dialing a composer's --scm-out socket
// (the N>1-or-effects path). Composer emits BGRA, vn-sink passes bytes
// through, ffmpeg needs -s WxH -framerate N at the input stage.
type ComposerFrameSource struct {
	Socket string
	Width  int
	Height int
	Fps    int
}

func (c ComposerFrameSource) Kind() FrameKind     { return FrameKindBGRARaw }
func (c ComposerFrameSource) SocketPath() string  { return c.Socket }
func (c ComposerFrameSource) Dims() (int, int)    { return c.Width, c.Height }
func (c ComposerFrameSource) FPS() int            { return c.Fps }

// AudioSource emits the encoder's audio-input argv fragment. Today the
// only impl is ALSADirectAudio (opens ALSA from the ffmpeg process);
// future audio fanout via the producer adds a sibling impl without
// touching encoder code.
type AudioSource interface {
	// InputArgs returns the ffmpeg argv slice that selects this audio
	// input. Caller embeds the result before its encoder/output args.
	// Returns empty when the stream has no audio configured.
	InputArgs() []string
}

// ALSADirectAudio opens ALSA from the ffmpeg process. Wraps an
// AudioConfig. Returns empty argv when no devices are configured.
type ALSADirectAudio struct {
	Config AudioConfig
}

// InputArgs returns the `-f alsa -i <dev>` fragment(s). With a single
// device, one `-i`; with N devices, N `-i` plus the audio filter chain
// in the encoder's `-filter_complex` (caller's responsibility — this
// only returns the inputs).
func (a ALSADirectAudio) InputArgs() []string {
	if len(a.Config.Devices) == 0 {
		return nil
	}
	out := make([]string, 0, 3*len(a.Config.Devices))
	for _, dev := range a.Config.Devices {
		out = append(out, "-f", "alsa", "-i", dev)
	}
	return out
}
