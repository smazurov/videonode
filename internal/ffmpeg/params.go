package ffmpeg

// Params represents all parameters needed to generate an FFmpeg command
// This replaces the map[string]string approach with strongly typed fields
type Params struct {
	// Input Configuration
	DevicePath   string
	InputFormat  string // yuyv422, mjpeg, etc.
	Resolution   string // 1920x1080
	FPS          string // 30, 60, etc.
	IsTestSource bool   // Use test pattern instead of device
	TestOverlay  string // Text overlay for test mode (e.g. "TEST MODE", "NO SIGNAL")

	// Encoder Configuration
	Encoder string // h264_vaapi, libx264, etc.

	// Rate Control (only set what's needed)
	Bitrate    string // For CBR/VBR: 5.0M
	MinRate    string // For VBR: minimum bitrate
	MaxRate    string // For VBR/CBR: maximum bitrate
	BufferSize string // For CBR/VBR: buffer size
	CRF        int    // For CRF mode: 0-51 (0 = not set)
	QP         int    // For CQP mode: quantization (0 = not set)
	RCMode     string // rc_mode for hardware encoders

	// Encoder Options
	Preset  string // fast, medium, slow
	GOP     int    // Keyframe interval (0 = not set)
	BFrames int    // B-frame count (-1 = not set, 0 = no B-frames)

	// Hardware Acceleration
	GlobalArgs   []string // -vaapi_device, etc.
	VideoFilters string   // format=nv12,hwupload

	// Audio
	AudioDevice  string // hw:4,0
	AudioFilters string // aresample=async=1:min_hard_comp=0.100000:first_pts=0

	// Output
	ProgressSocket string // /tmp/ffmpeg-progress-xxx.sock
	OutputURL      string // rtsp://localhost:8554/$MTX_PATH

	// Behavior Options
	Options []OptionType // FFmpeg behavior flags
}
