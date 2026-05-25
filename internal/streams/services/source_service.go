package services

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/smazurov/videonode/internal/api"
	"github.com/smazurov/videonode/internal/api/models"
	"github.com/smazurov/videonode/internal/logging"
	"github.com/smazurov/videonode/internal/streams"
	"github.com/smazurov/videonode/internal/streams/pipeline"
)

// SourceServiceOptions wires the SourceService to persistence and the
// supervised pipeline.
type SourceServiceOptions struct {
	Store          streams.EntityStore
	Pipeline       *pipeline.Pipeline
	PipelineSwitch PipelineSwitch
}

// sourceService implements api.SourceService backed by the v2 EntityStore
// + pipeline.Pipeline. Cross-entity reference checks read composers and
// streams from the same store so Delete can refuse cleanly.
type sourceService struct {
	store  streams.EntityStore
	pipe   *pipeline.Pipeline
	psw    PipelineSwitch
	logger logging.Logger
	mu     sync.Mutex
}

// NewSourceService constructs a SourceService. Store is required;
// Pipeline is optional (nil = persistence-only).
func NewSourceService(opts SourceServiceOptions) api.SourceService {
	if opts.Store == nil {
		panic("services.NewSourceService: Store is required")
	}
	return &sourceService{
		store:  opts.Store,
		pipe:   opts.Pipeline,
		psw:    opts.PipelineSwitch,
		logger: logging.GetLogger("source_svc"),
	}
}

func (s *sourceService) pipelineSwitchEnabled() bool {
	if s.psw == nil {
		return true
	}
	return s.psw.GetPipeline().Enabled
}

// List returns all configured sources, each with Consumers denormalized.
func (s *sourceService) List(_ context.Context) ([]api.Source, error) {
	entries := s.store.ListSourceEntities()
	out := make([]api.Source, len(entries))
	for i, e := range entries {
		out[i] = sourceToAPI(e)
		out[i].Consumers = s.findReferences(e.ID)
		s.enrichStatus(&out[i])
	}
	return out, nil
}

// Get returns one source by id, with Consumers denormalized so a single
// `entity{type:source}` SSE event carries the full picture (no
// client-side joins).
func (s *sourceService) Get(_ context.Context, id string) (*api.Source, error) {
	src, ok := s.store.GetSourceEntity(id)
	if !ok {
		return nil, &api.SourceNotFoundError{SourceID: id}
	}
	out := sourceToAPI(src)
	out.Consumers = s.findReferences(id)
	s.enrichStatus(&out)
	return &out, nil
}

// Create validates, persists, and applies a new source.
func (s *sourceService) Create(_ context.Context, src api.Source) (*api.Source, error) {
	if err := validateSourceCreate(src); err != nil {
		return nil, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.store.GetSourceEntity(src.ID); exists {
		return nil, &api.SourceExistsError{SourceID: src.ID}
	}

	now := time.Now()
	entity := pipeline.Source{
		ID:        src.ID,
		Device:    src.Device,
		TestMode:  src.TestMode,
		Format:    apiFormatToPipeline(src.Format),
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := s.store.AddSourceEntity(entity); err != nil {
		return nil, fmt.Errorf("persist source: %w", err)
	}
	if s.pipe != nil {
		if s.pipelineSwitchEnabled() {
			if err := s.pipe.ApplySource(entity); err != nil {
				if rmErr := s.store.RemoveSourceEntity(src.ID); rmErr != nil {
					s.logger.Error("Create: rollback after ApplySource failure also failed",
						"source_id", src.ID, "apply_error", err, "rollback_error", rmErr)
				}
				return nil, &api.SourceInvalidError{Message: "pipeline rejected source: " + err.Error()}
			}
		} else {
			_ = s.pipe.RegisterSource(entity)
		}
	}
	out := sourceToAPI(entity)
	return &out, nil
}

// Update applies a partial patch to an existing source.
func (s *sourceService) Update(_ context.Context, id string, patch api.SourcePatch) (*api.Source, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	prev, ok := s.store.GetSourceEntity(id)
	if !ok {
		return nil, &api.SourceNotFoundError{SourceID: id}
	}
	src := prev
	if patch.Device != nil {
		src.Device = *patch.Device
	}
	if patch.TestMode != nil {
		src.TestMode = *patch.TestMode
	}
	if patch.Format != nil {
		src.Format = apiFormatToPipeline(patch.Format)
	}
	// Flipping to test_mode clears any previously-persisted V4L2 format —
	// the two are mutually exclusive, and toggling test_mode on shouldn't
	// need a separate "clear format" step from the client.
	if src.TestMode {
		src.Format = nil
	}
	if err := validateSourcePayload(src.Device, src.TestMode); err != nil {
		return nil, err
	}
	if src.TestMode && src.Format != nil {
		return nil, &api.SourceInvalidError{Message: "format cannot be set while test_mode is true"}
	}
	src.UpdatedAt = time.Now()
	if err := s.store.UpdateSourceEntity(id, src); err != nil {
		return nil, fmt.Errorf("persist source update: %w", err)
	}
	if s.pipe != nil && s.pipelineSwitchEnabled() {
		// Format-only edit on a real device: hot-apply via gRPC so
		// connected consumers (composer, vn-sink) stay attached. Falls
		// back to ApplySource (restart) if the hot-apply path can't
		// reach the source (not registered yet, RPC error).
		formatOnly := !src.TestMode &&
			patch.Device == nil &&
			patch.TestMode == nil &&
			patch.Format != nil &&
			src.Format != nil
		applied := false
		if formatOnly {
			if err := s.pipe.UpdateSourceFormat(id, *src.Format); err == nil {
				applied = true
			} else {
				s.logger.Warn("Update: hot-apply SetFormat failed; falling back to restart",
					"source_id", id, "error", err)
			}
		}
		if !applied {
			if err := s.pipe.ApplySource(src); err != nil {
				if restoreErr := s.store.UpdateSourceEntity(id, prev); restoreErr != nil {
					s.logger.Error("Update: rollback after ApplySource failure also failed",
						"source_id", id, "apply_error", err, "rollback_error", restoreErr)
				}
				return nil, &api.SourceInvalidError{Message: "pipeline rejected source: " + err.Error()}
			}
		}
	} else if s.pipe != nil {
		_ = s.pipe.RegisterSource(src)
	}
	out := sourceToAPI(src)
	return &out, nil
}

// apiFormatToPipeline maps the API SourceFormat (FourCC-keyed, validated
// at the handler boundary) to the canonical pipeline shape.
func apiFormatToPipeline(f *api.SourceFormat) *pipeline.SourceFormat {
	if f == nil {
		return nil
	}
	return &pipeline.SourceFormat{
		FourCC: f.FourCC,
		Width:  f.Width,
		Height: f.Height,
		FPS:    f.FPS,
	}
}

// Delete refuses when the source is still referenced by a composer or
// stream; otherwise stops the producer and persists removal.
func (s *sourceService) Delete(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.store.GetSourceEntity(id); !ok {
		return &api.SourceNotFoundError{SourceID: id}
	}

	refs := s.findReferences(id)
	if len(refs) > 0 {
		return &api.SourceInUseError{SourceID: id, References: refs}
	}

	if s.pipe != nil {
		if err := s.pipe.DeleteSource(id); err != nil {
			s.logger.Warn("Delete: DeleteSource failed", "source_id", id, "error", err)
		}
	}
	if err := s.store.RemoveSourceEntity(id); err != nil {
		return fmt.Errorf("remove source: %w", err)
	}
	return nil
}

// findReferences scans composers and streams for inputs/upstreams that
// still name this source.
func (s *sourceService) findReferences(id string) []models.SourceReference {
	target := pipeline.SourceIDFor(id)
	var refs []models.SourceReference
	for _, c := range s.store.ListComposerEntities() {
		for _, in := range c.Inputs {
			if in.Ref == target {
				refs = append(refs, models.SourceReference{
					Kind: models.SourceReferenceKindComposer,
					ID:   c.ID,
				})
				break
			}
		}
	}
	for _, st := range s.store.ListPipelineStreams() {
		if st.Upstream == target {
			refs = append(refs, models.SourceReference{
				Kind: models.SourceReferenceKindStream,
				ID:   st.ID,
			})
		}
	}
	return refs
}

func (s *sourceService) enrichStatus(out *api.Source) {
	if s.pipe != nil {
		out.Status = models.ProcessStatus(s.pipe.Pool().GetStatus(pipeline.SourcePoolKey(out.ID)).State)
	}
}

func validateSourceCreate(src api.Source) error {
	if strings.TrimSpace(src.ID) == "" {
		return &api.SourceInvalidError{Message: "id is required"}
	}
	return validateSourcePayload(src.Device, src.TestMode)
}

func validateSourcePayload(device string, testMode bool) error {
	switch {
	case device != "" && testMode:
		return &api.SourceInvalidError{Message: "device and test_mode are mutually exclusive"}
	case device == "" && !testMode:
		return &api.SourceInvalidError{Message: "one of device or test_mode is required"}
	}
	return nil
}

func sourceToAPI(src pipeline.Source) api.Source {
	out := api.Source{
		ID:        src.ID,
		Device:    src.Device,
		TestMode:  src.TestMode,
		CreatedAt: src.CreatedAt,
		UpdatedAt: src.UpdatedAt,
	}
	if src.Format != nil {
		out.Format = &api.SourceFormat{
			FourCC: src.Format.FourCC,
			Width:  src.Format.Width,
			Height: src.Format.Height,
			FPS:    src.Format.FPS,
		}
	}
	return out
}
