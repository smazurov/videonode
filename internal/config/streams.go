package config

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/pelletier/go-toml/v2"
	"github.com/smazurov/videonode/internal/ffmpeg"
	"github.com/smazurov/videonode/internal/mediamtx"
	"github.com/smazurov/videonode/internal/types"
)

// FFmpegConfig contains all FFmpeg-specific settings for a stream.
// This structure holds everything needed to build the complete FFmpeg command,
// including hardware acceleration settings, encoder parameters, and filters.
type FFmpegConfig struct {
	// InputFormat specifies the V4L2 pixel format to request from the device
	// Common values: "yuyv422" (uncompressed), "mjpeg" (compressed)
	// This is passed as -input_format to FFmpeg
	InputFormat string `toml:"input_format,omitempty" json:"input_format,omitempty"`

	// Resolution specifies the video dimensions in WIDTHxHEIGHT format
	// Example: "1920x1080", "1280x720"
	// This is passed as -video_size to FFmpeg
	Resolution string `toml:"resolution,omitempty" json:"resolution,omitempty"`

	// FPS specifies the framerate to capture from the device
	// Example: "30", "60", "29.97"
	// This is passed as -framerate to FFmpeg
	FPS string `toml:"fps,omitempty" json:"fps,omitempty"`

	// Encoder specifies the actual FFmpeg encoder to use (NOT the codec standard)
	// Examples: "h264_vaapi" (hardware), "libx264" (software), "h264_nvenc" (NVIDIA)
	// This is the result of converting the API's generic codec (h264/h265) to a specific encoder
	// This is passed as -c:v to FFmpeg
	Encoder string `toml:"encoder,omitempty" json:"encoder,omitempty"`

	// Preset controls the encoding speed/quality tradeoff for software encoders
	// Values: "ultrafast", "superfast", "veryfast", "faster", "fast", "medium", "slow", "slower", "veryslow"
	// This is typically empty for hardware encoders
	// This is passed as -preset to FFmpeg when applicable
	Preset string `toml:"preset,omitempty" json:"preset,omitempty"`

	// Bitrate specifies the target video bitrate
	// Examples: "2M", "1500k", "4000000"
	// This is passed as -b:v to FFmpeg
	Bitrate string `toml:"bitrate,omitempty" json:"bitrate,omitempty"`

	// GlobalArgs contains FFmpeg arguments that must appear BEFORE the input
	// These are typically hardware device initialization parameters
	// Example for VAAPI: ["-vaapi_device", "/dev/dri/renderD128"]
	// Example for NVENC: ["-hwaccel", "cuda", "-hwaccel_output_format", "cuda"]
	GlobalArgs []string `toml:"global_args,omitempty" json:"global_args,omitempty"`

	// VideoFilters specifies the FFmpeg filter chain to apply to the video
	// This is used for pixel format conversion and hardware upload
	// Example for VAAPI: "format=nv12,hwupload"
	// Example for software: "format=yuv420p"
	// This is passed as -vf to FFmpeg
	VideoFilters string `toml:"video_filters,omitempty" json:"video_filters,omitempty"`

	// EncoderParams contains encoder-specific parameters as key-value pairs
	// These are parameters specific to the chosen encoder
	// Example for VAAPI: {"qp": "20", "bf": "0"}
	// Example for x264: {"crf": "23", "profile": "baseline"}
	// These are passed as individual -key value arguments to FFmpeg
	EncoderParams map[string]string `toml:"encoder_params,omitempty" json:"encoder_params,omitempty"`

	// Options contains FFmpeg behavior flags that affect input/output handling
	// These are predefined options from ffmpeg.OptionType
	// Examples: "thread_queue_1024" (buffer size), "copyts" (timestamp handling)
	Options []ffmpeg.OptionType `toml:"options,omitempty" json:"options,omitempty"`

	// QualityParams stores the original quality/rate control settings
	// This is used to regenerate EncoderParams if the encoder changes
	QualityParams *types.QualityParams `toml:"quality_params,omitempty" json:"quality_params,omitempty"`

	// AudioDevice specifies the ALSA device for audio capture
	// If set, enables audio passthrough with copy codec
	// Example: "hw:4,0" for card 4, device 0
	AudioDevice string `toml:"audio_device,omitempty" json:"audio_device,omitempty"`
}

// StreamConfig represents a single stream configuration in the TOML file.
// This is the top-level structure for each stream, containing metadata
// and the nested FFmpeg configuration.
type StreamConfig struct {
	// ID is the unique identifier for this stream
	// This becomes the MediaMTX path name and must be unique
	ID string `toml:"id" json:"id"`

	// Name is a human-readable name for the stream
	// If not specified, defaults to the ID
	Name string `toml:"name" json:"name"`

	// Device is the stable USB device identifier
	// Format: "usb-BUS-PORT" (e.g., "usb-0000:00:14.0-1.2")
	// This is resolved to a /dev/videoX path at runtime
	Device string `toml:"device" json:"device"`

	// Enabled determines if this stream should be active
	// Disabled streams are kept in config but not started
	Enabled bool `toml:"enabled" json:"enabled"`

	// FFmpeg contains all FFmpeg-specific configuration for this stream
	// This includes encoder selection, hardware acceleration, and encoding parameters
	FFmpeg FFmpegConfig `toml:"ffmpeg" json:"ffmpeg"`

	// CreatedAt timestamp when the stream was first created
	CreatedAt time.Time `toml:"created_at" json:"created_at"`

	// UpdatedAt timestamp when the stream was last modified
	UpdatedAt time.Time `toml:"updated_at" json:"updated_at"`
}

// StreamsConfig represents the complete streams configuration file
type StreamsConfig struct {
	Version    int                      `toml:"version" json:"version"`
	Validation *types.ValidationResults `toml:"validation,omitempty" json:"validation,omitempty"`
	Streams    map[string]StreamConfig  `toml:"streams" json:"streams"`
}

// StreamManager manages stream configurations
type StreamManager struct {
	configPath string
	config     *StreamsConfig
}

// NewStreamManager creates a new stream manager
func NewStreamManager(configPath string) *StreamManager {
	if configPath == "" {
		configPath = "streams.toml"
	}

	return &StreamManager{
		configPath: configPath,
		config: &StreamsConfig{
			Version: 1,
			Streams: make(map[string]StreamConfig),
		},
	}
}

// Load loads the streams configuration from file
func (sm *StreamManager) Load() error {
	// Check if file exists
	if _, err := os.Stat(sm.configPath); os.IsNotExist(err) {
		// File doesn't exist, use empty config
		return nil
	}

	data, err := os.ReadFile(sm.configPath)
	if err != nil {
		return fmt.Errorf("failed to read streams config: %w", err)
	}

	if err := toml.Unmarshal(data, sm.config); err != nil {
		return fmt.Errorf("failed to parse streams config: %w", err)
	}

	// Initialize streams map if nil
	if sm.config.Streams == nil {
		sm.config.Streams = make(map[string]StreamConfig)
	}

	// Set version if not set
	if sm.config.Version == 0 {
		sm.config.Version = 1
	}

	return nil
}

// Save saves the streams configuration to file
func (sm *StreamManager) Save() error {
	// Ensure directory exists
	dir := filepath.Dir(sm.configPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	data, err := toml.Marshal(sm.config)
	if err != nil {
		return fmt.Errorf("failed to marshal streams config: %w", err)
	}

	if err := os.WriteFile(sm.configPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write streams config: %w", err)
	}

	return nil
}

// AddStream adds a new stream to the configuration
func (sm *StreamManager) AddStream(stream StreamConfig) error {
	if stream.ID == "" {
		return fmt.Errorf("stream ID cannot be empty")
	}

	if stream.Name == "" {
		stream.Name = stream.ID
	}

	if stream.Device == "" {
		return fmt.Errorf("device identifier cannot be empty")
	}

	// Set timestamps
	now := time.Now()
	if stream.CreatedAt.IsZero() {
		stream.CreatedAt = now
	}
	stream.UpdatedAt = now

	// Set enabled by default
	if !stream.Enabled {
		stream.Enabled = true
	}

	sm.config.Streams[stream.ID] = stream
	return sm.Save()
}

// UpdateStream updates an existing stream configuration
func (sm *StreamManager) UpdateStream(id string, updates StreamConfig) error {
	existing, exists := sm.config.Streams[id]
	if !exists {
		return fmt.Errorf("stream %s not found", id)
	}

	// Preserve creation time and ID
	updates.ID = existing.ID
	updates.CreatedAt = existing.CreatedAt
	updates.UpdatedAt = time.Now()

	// Use existing values if not provided
	if updates.Name == "" {
		updates.Name = existing.Name
	}
	if updates.Device == "" {
		updates.Device = existing.Device
	}

	sm.config.Streams[id] = updates
	return sm.Save()
}

// RemoveStream removes a stream from the configuration
func (sm *StreamManager) RemoveStream(id string) error {
	if _, exists := sm.config.Streams[id]; !exists {
		return fmt.Errorf("stream %s not found", id)
	}

	delete(sm.config.Streams, id)
	return sm.Save()
}

// GetStream retrieves a stream by ID
func (sm *StreamManager) GetStream(id string) (StreamConfig, bool) {
	stream, exists := sm.config.Streams[id]
	return stream, exists
}

// GetStreams returns all streams
func (sm *StreamManager) GetStreams() map[string]StreamConfig {
	return sm.config.Streams
}

// GetEnabledStreams returns only enabled streams
func (sm *StreamManager) GetEnabledStreams() map[string]StreamConfig {
	enabled := make(map[string]StreamConfig)
	for id, stream := range sm.config.Streams {
		if stream.Enabled {
			enabled[id] = stream
		}
	}
	return enabled
}

// ToMediaMTXConfig converts stream configurations to MediaMTX configuration
func (sm *StreamManager) ToMediaMTXConfig(deviceResolver func(string) string, socketPaths map[string]string) (*mediamtx.Config, error) {
	mtxConfig := mediamtx.NewConfig()

	for _, stream := range sm.GetEnabledStreams() {
		// Resolve device stable ID to actual device path
		devicePath := deviceResolver(stream.Device)
		if devicePath == "" {
			// Skip if device not found - could log this
			continue
		}

		// Use the fresh socket path provided, not the one from TOML
		socketPath := ""
		if socketPaths != nil {
			socketPath = socketPaths[stream.ID]
		}

		streamConfig := mediamtx.StreamConfig{
			DevicePath:     devicePath,
			InputFormat:    stream.FFmpeg.InputFormat,
			Resolution:     stream.FFmpeg.Resolution,
			FPS:            stream.FFmpeg.FPS,
			Codec:          stream.FFmpeg.Encoder, // Use the actual encoder, not generic codec
			Preset:         stream.FFmpeg.Preset,
			Bitrate:        stream.FFmpeg.Bitrate,
			FFmpegOptions:  stream.FFmpeg.Options,
			ProgressSocket: socketPath, // Use fresh socket path
			GlobalArgs:     stream.FFmpeg.GlobalArgs,
			EncoderParams:  stream.FFmpeg.EncoderParams,
			VideoFilters:   stream.FFmpeg.VideoFilters,
			AudioDevice:    stream.FFmpeg.AudioDevice,
		}

		// Use stream ID as path name
		if err := mtxConfig.AddStream(stream.ID, streamConfig); err != nil {
			return nil, fmt.Errorf("failed to add stream %s: %w", stream.ID, err)
		}
	}

	return mtxConfig, nil
}

// EnableStream enables a stream
func (sm *StreamManager) EnableStream(id string) error {
	stream, exists := sm.config.Streams[id]
	if !exists {
		return fmt.Errorf("stream %s not found", id)
	}

	stream.Enabled = true
	stream.UpdatedAt = time.Now()
	sm.config.Streams[id] = stream
	return sm.Save()
}

// DisableStream disables a stream
func (sm *StreamManager) DisableStream(id string) error {
	stream, exists := sm.config.Streams[id]
	if !exists {
		return fmt.Errorf("stream %s not found", id)
	}

	stream.Enabled = false
	stream.UpdatedAt = time.Now()
	sm.config.Streams[id] = stream
	return sm.Save()
}

// UpdateValidation updates the validation data in the configuration
func (sm *StreamManager) UpdateValidation(validation *types.ValidationResults) error {
	sm.config.Validation = validation
	return sm.Save()
}

// GetValidation returns the current validation data
func (sm *StreamManager) GetValidation() *types.ValidationResults {
	return sm.config.Validation
}
