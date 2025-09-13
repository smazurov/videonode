package streams

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/pelletier/go-toml/v2"
	"github.com/smazurov/videonode/internal/types"
)

// TOMLRepository implements Repository using TOML file storage
type TOMLRepository struct {
	configPath string
	config     *StreamsConfig
}

// NewTOMLRepository creates a new TOML-based repository
func NewTOMLRepository(configPath string) *TOMLRepository {
	if configPath == "" {
		configPath = "streams.toml"
	}

	return &TOMLRepository{
		configPath: configPath,
		config: &StreamsConfig{
			Version: 1,
			Streams: make(map[string]StreamConfig),
		},
	}
}

// Load loads the streams configuration from file
func (r *TOMLRepository) Load() error {
	// Check if file exists
	if _, err := os.Stat(r.configPath); os.IsNotExist(err) {
		// File doesn't exist, use empty config
		return nil
	}

	data, err := os.ReadFile(r.configPath)
	if err != nil {
		return fmt.Errorf("failed to read streams config: %w", err)
	}

	if err := toml.Unmarshal(data, r.config); err != nil {
		return fmt.Errorf("failed to parse streams config: %w", err)
	}

	// Initialize streams map if nil
	if r.config.Streams == nil {
		r.config.Streams = make(map[string]StreamConfig)
	}

	// Set version if not set
	if r.config.Version == 0 {
		r.config.Version = 1
	}

	return nil
}

// Save saves the streams configuration to file
func (r *TOMLRepository) Save() error {
	// Ensure directory exists
	dir := filepath.Dir(r.configPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	data, err := toml.Marshal(r.config)
	if err != nil {
		return fmt.Errorf("failed to marshal streams config: %w", err)
	}

	if err := os.WriteFile(r.configPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write streams config: %w", err)
	}

	return nil
}

// AddStream adds a new stream to the configuration
func (r *TOMLRepository) AddStream(stream StreamConfig) error {
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

	r.config.Streams[stream.ID] = stream
	return r.Save()
}

// UpdateStream updates an existing stream configuration
func (r *TOMLRepository) UpdateStream(id string, updates StreamConfig) error {
	existing, exists := r.config.Streams[id]
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

	r.config.Streams[id] = updates
	return r.Save()
}

// RemoveStream removes a stream from the configuration
func (r *TOMLRepository) RemoveStream(id string) error {
	if _, exists := r.config.Streams[id]; !exists {
		return fmt.Errorf("stream %s not found", id)
	}

	delete(r.config.Streams, id)
	return r.Save()
}

// GetStream retrieves a stream by ID
func (r *TOMLRepository) GetStream(id string) (StreamConfig, bool) {
	stream, exists := r.config.Streams[id]
	return stream, exists
}

// GetAllStreams returns all streams
func (r *TOMLRepository) GetAllStreams() map[string]StreamConfig {
	return r.config.Streams
}

// GetEnabledStreams returns only enabled streams
func (r *TOMLRepository) GetEnabledStreams() map[string]StreamConfig {
	enabled := make(map[string]StreamConfig)
	for id, stream := range r.config.Streams {
		if stream.Enabled {
			enabled[id] = stream
		}
	}
	return enabled
}

// EnableStream enables a stream
func (r *TOMLRepository) EnableStream(id string) error {
	stream, exists := r.config.Streams[id]
	if !exists {
		return fmt.Errorf("stream %s not found", id)
	}

	stream.Enabled = true
	stream.UpdatedAt = time.Now()
	r.config.Streams[id] = stream
	return r.Save()
}

// DisableStream disables a stream
func (r *TOMLRepository) DisableStream(id string) error {
	stream, exists := r.config.Streams[id]
	if !exists {
		return fmt.Errorf("stream %s not found", id)
	}

	stream.Enabled = false
	stream.UpdatedAt = time.Now()
	r.config.Streams[id] = stream
	return r.Save()
}

// GetValidation returns the current validation data
func (r *TOMLRepository) GetValidation() *types.ValidationResults {
	return r.config.Validation
}

// UpdateValidation updates the validation data in the configuration
func (r *TOMLRepository) UpdateValidation(validation *types.ValidationResults) error {
	r.config.Validation = validation
	return r.Save()
}
