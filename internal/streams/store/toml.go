package store

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/pelletier/go-toml/v2"
	"github.com/smazurov/videonode/internal/streams"
	"github.com/smazurov/videonode/internal/types"
)

// config represents the complete streams configuration file for TOML marshaling.
type config struct {
	Version    int                           `toml:"version" json:"version"`
	Validation *types.ValidationResults      `toml:"validation,omitempty" json:"validation,omitempty"`
	Streams    map[string]streams.StreamSpec `toml:"streams" json:"streams"`
}

// tomlStore implements Store using TOML file storage.
type tomlStore struct {
	configPath string
	config     *config
}

// NewTOML creates a new TOML-based store.
func NewTOML(configPath string) streams.Store {
	if configPath == "" {
		configPath = "streams.toml"
	}

	return &tomlStore{
		configPath: configPath,
		config: &config{
			Version: 1,
			Streams: make(map[string]streams.StreamSpec),
		},
	}
}

// Load loads the streams configuration from file.
func (s *tomlStore) Load() error {
	// Check if file exists
	if _, err := os.Stat(s.configPath); os.IsNotExist(err) {
		// File doesn't exist, use empty config
		return nil
	}

	data, err := os.ReadFile(s.configPath)
	if err != nil {
		return fmt.Errorf("failed to read streams config: %w", err)
	}

	if unmarshalErr := toml.Unmarshal(data, s.config); unmarshalErr != nil {
		return fmt.Errorf("failed to parse streams config: %w", unmarshalErr)
	}

	// Initialize streams map if nil
	if s.config.Streams == nil {
		s.config.Streams = make(map[string]streams.StreamSpec)
	}

	// Set version if not set
	if s.config.Version == 0 {
		s.config.Version = 1
	}

	return nil
}

// Save saves the streams configuration to file.
func (s *tomlStore) Save() error {
	// Ensure directory exists
	dir := filepath.Dir(s.configPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	data, err := toml.Marshal(s.config)
	if err != nil {
		return fmt.Errorf("failed to marshal streams config: %w", err)
	}

	if writeErr := os.WriteFile(s.configPath, data, 0o644); writeErr != nil {
		return fmt.Errorf("failed to write streams config: %w", writeErr)
	}

	return nil
}

// AddStream adds a new stream to the configuration.
func (s *tomlStore) AddStream(stream streams.StreamSpec) error {
	s.config.Streams[stream.ID] = stream
	return s.Save()
}

// UpdateStream updates an existing stream configuration.
func (s *tomlStore) UpdateStream(id string, updates streams.StreamSpec) error {
	s.config.Streams[id] = updates
	return s.Save()
}

// RemoveStream removes a stream from the configuration.
func (s *tomlStore) RemoveStream(id string) error {
	delete(s.config.Streams, id)
	return s.Save()
}

// GetStream retrieves a stream by ID.
func (s *tomlStore) GetStream(id string) (streams.StreamSpec, bool) {
	stream, exists := s.config.Streams[id]
	return stream, exists
}

// GetAllStreams returns all streams.
func (s *tomlStore) GetAllStreams() map[string]streams.StreamSpec {
	return s.config.Streams
}

// GetValidation returns the current validation data.
func (s *tomlStore) GetValidation() *types.ValidationResults {
	return s.config.Validation
}

// UpdateValidation updates the validation data in the configuration.
func (s *tomlStore) UpdateValidation(validation *types.ValidationResults) error {
	s.config.Validation = validation
	return s.Save()
}
