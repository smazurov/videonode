package streams

import "github.com/smazurov/videonode/internal/types"

// Store defines the interface for stream and validation data access.
// This interface allows for different storage backends (TOML, SQLite, etc.)
// This is an internal interface - external code should use service types.
type Store interface {
	// Load loads the configuration from storage
	Load() error

	// Save saves the configuration to storage
	Save() error

	// AddStream adds a new stream to the configuration
	AddStream(stream StreamSpec) error

	// UpdateStream updates an existing stream configuration
	UpdateStream(id string, stream StreamSpec) error

	// RemoveStream removes a stream from the configuration
	RemoveStream(id string) error

	// GetStream retrieves a stream by ID
	GetStream(id string) (StreamSpec, bool)

	// GetAllStreams returns all streams
	GetAllStreams() map[string]StreamSpec

	// GetValidation returns the current validation data
	GetValidation() *types.ValidationResults

	// UpdateValidation updates the validation data
	UpdateValidation(validation *types.ValidationResults) error
}
