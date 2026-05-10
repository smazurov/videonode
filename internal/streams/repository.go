package streams

import "github.com/smazurov/videonode/internal/types"

// Store is the interface for stream and validation data access.
type Store interface {
	Load() error
	Save() error
	AddStream(stream StreamSpec) error
	UpdateStream(id string, stream StreamSpec) error
	RemoveStream(id string) error
	GetStream(id string) (StreamSpec, bool)
	GetAllStreams() map[string]StreamSpec
	GetValidation() *types.ValidationResults
	UpdateValidation(validation *types.ValidationResults) error
}
