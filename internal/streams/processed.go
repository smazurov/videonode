package streams

// ProcessedStream represents a stream configuration with all runtime data injected.
// This includes encoder selection, device resolution, and monitoring paths.
type ProcessedStream struct {
	// StreamID is the unique identifier for this stream
	StreamID string

	// FFmpegCommand is the complete FFmpeg command ready to execute
	FFmpegCommand string

	// DevicePath is the resolved device path (e.g., /dev/video0)
	DevicePath string

	// Encoder is the selected encoder (e.g., h264_vaapi, libx264)
	Encoder string

	// GlobalArgs are FFmpeg arguments that appear before input
	GlobalArgs []string

	// VideoFilters is the video filter chain
	VideoFilters string

	// SocketPath is the monitoring socket path if applicable
	SocketPath string

	// Metadata for debugging/logging
	InputFormat string
	Resolution  string
	FPS         string
	Bitrate     string
	AudioDevice string
	Preset      string
	Options     []string
}
