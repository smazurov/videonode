package streams

import "github.com/smazurov/videonode/internal/types"

// Store is the interface for stream and validation data access.
//
// Legacy StreamSpec accessors (Add/Update/Remove/GetStream/GetAllStreams)
// remain on the interface so the existing StreamService keeps working
// during the source/composer/stream split. V2 entity accessors (sources,
// composers, v2 streams) live on EntityStore — every concrete store is
// expected to implement both surfaces.
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

// EntityStore is the v2 entity-CRUD surface (sources / composers /
// streams as independent top-level entries). The B9 service split uses
// this surface directly; the concrete TOML store implements both Store
// and EntityStore.
type EntityStore interface {
	ListSourceEntities() []Source
	GetSourceEntity(id string) (Source, bool)
	AddSourceEntity(src Source) error
	UpdateSourceEntity(id string, src Source) error
	RemoveSourceEntity(id string) error

	ListComposerEntities() []Composer
	GetComposerEntity(id string) (Composer, bool)
	AddComposerEntity(c Composer) error
	UpdateComposerEntity(id string, c Composer) error
	RemoveComposerEntity(id string) error

	ListPipelineStreams() []PipelineStream
	GetPipelineStream(id string) (PipelineStream, bool)
	AddPipelineStream(s PipelineStream) error
	UpdatePipelineStream(id string, s PipelineStream) error
	RemovePipelineStream(id string) error
}
