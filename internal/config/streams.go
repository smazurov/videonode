package config

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/pelletier/go-toml/v2"
	"github.com/smazurov/videonode/internal/ffmpeg"
	"github.com/smazurov/videonode/internal/mediamtx"
)

// StreamConfig represents a single stream configuration
type StreamConfig struct {
	ID      string `toml:"id" json:"id"`
	Name    string `toml:"name" json:"name"`
	Device  string `toml:"device" json:"device"` // Stable device identifier (USB bus/port)
	Enabled bool   `toml:"enabled" json:"enabled"`

	// FFmpeg settings
	InputFormat   string                      `toml:"input_format,omitempty" json:"input_format,omitempty"` // FFmpeg input format
	Resolution    string                      `toml:"resolution,omitempty" json:"resolution,omitempty"`
	FPS           string                      `toml:"fps,omitempty" json:"fps,omitempty"`
	Codec         string                      `toml:"codec,omitempty" json:"codec,omitempty"`
	Preset        string                      `toml:"preset,omitempty" json:"preset,omitempty"`
	Bitrate       string                      `toml:"bitrate,omitempty" json:"bitrate,omitempty"`
	FFmpegOptions []ffmpeg.OptionType `toml:"ffmpeg_options,omitempty" json:"ffmpeg_options,omitempty"` // FFmpeg feature flags

	// Monitoring
	ProgressSocket string `toml:"progress_socket,omitempty" json:"progress_socket,omitempty"`

	// Metadata
	CreatedAt time.Time `toml:"created_at" json:"created_at"`
	UpdatedAt time.Time `toml:"updated_at" json:"updated_at"`
}

// StreamsConfig represents the complete streams configuration file
type StreamsConfig struct {
	Version int                     `toml:"version" json:"version"`
	Streams map[string]StreamConfig `toml:"streams" json:"streams"`
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
func (sm *StreamManager) ToMediaMTXConfig(deviceResolver func(string) string) (*mediamtx.Config, error) {
	mtxConfig := mediamtx.NewConfig()

	for _, stream := range sm.GetEnabledStreams() {
		// Resolve device stable ID to actual device path
		devicePath := deviceResolver(stream.Device)
		if devicePath == "" {
			// Skip if device not found - could log this
			continue
		}

		streamConfig := mediamtx.StreamConfig{
			DevicePath:     devicePath,
			InputFormat:    stream.InputFormat,
			Resolution:     stream.Resolution,
			FPS:            stream.FPS,
			Codec:          stream.Codec,
			Preset:         stream.Preset,
			FFmpegOptions:  stream.FFmpegOptions,
			ProgressSocket: stream.ProgressSocket, // Use socket path from TOML config
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
