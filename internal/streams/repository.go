package streams

import (
	"github.com/smazurov/videonode/internal/streams/pipeline"
	"github.com/smazurov/videonode/internal/types"
)

// Source re-exports the canonical producer descriptor from the pipeline
// package. The service layer treats sources as first-class entities; this
// alias lets api/service consumers use one set of types.
type Source = pipeline.Source

// Composer re-exports the canonical composer descriptor.
type Composer = pipeline.Composer

// ComposerInput re-exports the canonical composer input descriptor.
type ComposerInput = pipeline.ComposerInput

// ComposerLayoutSlot re-exports the canonical composer layout slot.
type ComposerLayoutSlot = pipeline.LayoutSlot

// ComposerEffect re-exports the canonical composer effect descriptor.
type ComposerEffect = pipeline.Effect

// ComposerCanvasDims re-exports the canonical composer canvas dimensions.
type ComposerCanvasDims = pipeline.CanvasDims

// PipelineStream re-exports the canonical slim stream descriptor.
type PipelineStream = pipeline.Stream

// PipelineConfig is the persisted, daemon-wide pipeline master switch.
// When Enabled is false, no processes of any kind (sources, composers,
// or streams) are spawned.
type PipelineConfig struct {
	Enabled bool `toml:"enabled" json:"enabled"`
}

// Store is the persistence + validation surface every concrete stream
// store implements. The TOML store ([[sources]] / [[composers]] /
// [[streams]] tables) is the only production implementation today.
type Store interface {
	Load() error

	GetValidation() *types.ValidationResults
	UpdateValidation(validation *types.ValidationResults) error

	// GetPipeline returns the daemon-wide pipeline master switch. When the
	// table is absent in the underlying config, returns {Enabled: true} so
	// existing installs preserve their auto-start behavior.
	GetPipeline() PipelineConfig
	// SetPipeline writes the daemon-wide pipeline master switch and persists.
	SetPipeline(cfg PipelineConfig) error

	EntityStore
}

// EntityStore is the v2 entity-CRUD surface (sources / composers /
// streams as independent top-level entries). Every stream service in
// the api layer routes through this interface.
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
