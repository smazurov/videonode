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
	Store    streams.EntityStore
	Pipeline *pipeline.Pipeline
}

// sourceService implements api.SourceService backed by the v2 EntityStore
// + pipeline.Pipeline. Cross-entity reference checks read composers and
// streams from the same store so Delete can refuse cleanly.
type sourceService struct {
	store  streams.EntityStore
	pipe   *pipeline.Pipeline
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
		logger: logging.GetLogger("source_svc"),
	}
}

// List returns all configured sources.
func (s *sourceService) List(_ context.Context) ([]api.Source, error) {
	entries := s.store.ListSourceEntities()
	out := make([]api.Source, len(entries))
	for i, e := range entries {
		out[i] = sourceToAPI(e)
	}
	return out, nil
}

// Get returns one source by id.
func (s *sourceService) Get(_ context.Context, id string) (*api.Source, error) {
	src, ok := s.store.GetSourceEntity(id)
	if !ok {
		return nil, &api.SourceNotFoundError{SourceID: id}
	}
	out := sourceToAPI(src)
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
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := s.store.AddSourceEntity(entity); err != nil {
		return nil, fmt.Errorf("persist source: %w", err)
	}
	if s.pipe != nil {
		if err := s.pipe.ApplySource(entity); err != nil {
			// Roll back the store insert so the operator doesn't see a
			// persisted source that the pipeline never accepted.
			if rmErr := s.store.RemoveSourceEntity(src.ID); rmErr != nil {
				s.logger.Error("Create: rollback after ApplySource failure also failed",
					"source_id", src.ID, "apply_error", err, "rollback_error", rmErr)
			}
			return nil, &api.SourceInvalidError{Message: "pipeline rejected source: " + err.Error()}
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
	if err := validateSourcePayload(src.Device, src.TestMode); err != nil {
		return nil, err
	}
	src.UpdatedAt = time.Now()
	if err := s.store.UpdateSourceEntity(id, src); err != nil {
		return nil, fmt.Errorf("persist source update: %w", err)
	}
	if s.pipe != nil {
		if err := s.pipe.ApplySource(src); err != nil {
			// Roll back to the previous spec so the persisted state stays
			// consistent with what the pipeline accepts.
			if restoreErr := s.store.UpdateSourceEntity(id, prev); restoreErr != nil {
				s.logger.Error("Update: rollback after ApplySource failure also failed",
					"source_id", id, "apply_error", err, "rollback_error", restoreErr)
			}
			return nil, &api.SourceInvalidError{Message: "pipeline rejected source: " + err.Error()}
		}
	}
	out := sourceToAPI(src)
	return &out, nil
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
	return api.Source{
		ID:        src.ID,
		Device:    src.Device,
		TestMode:  src.TestMode,
		CreatedAt: src.CreatedAt,
		UpdatedAt: src.UpdatedAt,
	}
}
