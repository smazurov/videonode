package streams

import "github.com/smazurov/videonode/internal/types"

// Repository defines the interface for stream data access
type Repository interface {
	// Load loads the configuration from storage
	Load() error

	// Save saves the configuration to storage
	Save() error

	// AddStream adds a new stream to the configuration
	AddStream(stream StreamConfig) error

	// UpdateStream updates an existing stream configuration
	UpdateStream(id string, stream StreamConfig) error

	// RemoveStream removes a stream from the configuration
	RemoveStream(id string) error

	// GetStream retrieves a stream by ID
	GetStream(id string) (StreamConfig, bool)

	// GetAllStreams returns all streams
	GetAllStreams() map[string]StreamConfig

	// GetEnabledStreams returns only enabled streams
	GetEnabledStreams() map[string]StreamConfig

	// EnableStream enables a stream by ID
	EnableStream(id string) error

	// DisableStream disables a stream by ID
	DisableStream(id string) error
}

// ValidationRepository defines the interface for validation data access
type ValidationRepository interface {
	// GetValidation returns the current validation data
	GetValidation() *types.ValidationResults

	// UpdateValidation updates the validation data
	UpdateValidation(validation *types.ValidationResults) error
}
