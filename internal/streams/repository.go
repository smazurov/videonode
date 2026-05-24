package streams

import "github.com/smazurov/videonode/internal/types"

// Store is the interface for stream and validation data access.
//
// The v2 entity accessors (Sources/Composers/Streams) live on the concrete
// store implementation rather than this interface. B9 swaps service-layer
// callers from the legacy StreamSpec methods over to the v2 accessors and
// will widen this interface at that time; today they coexist so the build
// passes across the parallel B-unit worktrees.
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

	// GetPipeline returns the daemon-wide pipeline master switch. When the
	// table is absent in the underlying config, returns {Enabled: true} so
	// existing installs preserve their auto-start behavior.
	GetPipeline() PipelineConfig
	// SetPipeline writes the daemon-wide pipeline master switch and persists.
	SetPipeline(cfg PipelineConfig) error
}
