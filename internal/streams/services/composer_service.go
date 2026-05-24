package services

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/smazurov/videonode/internal/api"
	"github.com/smazurov/videonode/internal/api/models"
	"github.com/smazurov/videonode/internal/logging"
	"github.com/smazurov/videonode/internal/streams"
	"github.com/smazurov/videonode/internal/streams/pipeline"
)

// ComposerServiceOptions wires the ComposerService to persistence and
// the supervised pipeline.
type ComposerServiceOptions struct {
	Store    streams.EntityStore
	Pipeline *pipeline.Pipeline
}

// composerService implements api.ComposerService backed by the v2
// EntityStore + pipeline.Pipeline.
type composerService struct {
	store  streams.EntityStore
	pipe   *pipeline.Pipeline
	logger logging.Logger
	mu     sync.Mutex
}

// NewComposerService constructs a ComposerService. Store is required;
// Pipeline is optional (nil = persistence-only).
func NewComposerService(opts ComposerServiceOptions) api.ComposerService {
	if opts.Store == nil {
		panic("services.NewComposerService: Store is required")
	}
	return &composerService{
		store:  opts.Store,
		pipe:   opts.Pipeline,
		logger: logging.GetLogger("composer_svc"),
	}
}

// ListComposers returns all persisted composers in API wire shape.
func (s *composerService) ListComposers(_ context.Context) ([]models.ComposerData, error) {
	entries := s.store.ListComposerEntities()
	out := make([]models.ComposerData, len(entries))
	for i, e := range entries {
		out[i] = composerToAPI(e)
	}
	return out, nil
}

// GetComposer fetches a single composer by id.
func (s *composerService) GetComposer(_ context.Context, id string) (*models.ComposerData, error) {
	c, ok := s.store.GetComposerEntity(id)
	if !ok {
		return nil, &api.ComposerError{Code: api.ComposerErrNotFound, Message: "composer " + id + " not found"}
	}
	out := composerToAPI(c)
	return &out, nil
}

// CreateComposer validates, persists, and applies a new composer.
func (s *composerService) CreateComposer(_ context.Context, data models.ComposerCreateRequestData) (*models.ComposerData, error) {
	if err := validateComposerCreate(data); err != nil {
		return nil, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.store.GetComposerEntity(data.ID); exists {
		return nil, &api.ComposerError{Code: api.ComposerErrExists, Message: "composer " + data.ID + " already exists"}
	}

	now := time.Now()
	entity := pipeline.Composer{
		ID:        data.ID,
		Canvas:    pipeline.CanvasDims{W: data.Canvas.W, H: data.Canvas.H},
		Inputs:    apiInputsToEntity(data.Inputs),
		Layout:    apiLayoutToEntity(data.Layout),
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := validateComposerLayout(entity); err != nil {
		return nil, err
	}
	if err := s.store.AddComposerEntity(entity); err != nil {
		return nil, &api.ComposerError{Code: api.ComposerErrInternal, Message: err.Error()}
	}
	if s.pipe != nil {
		if err := s.pipe.ApplyComposer(entity); err != nil {
			s.logger.Warn("CreateComposer: ApplyComposer failed", "composer_id", entity.ID, "error", err)
		}
	}
	out := composerToAPI(entity)
	return &out, nil
}

// UpdateComposer applies a partial patch to an existing composer.
func (s *composerService) UpdateComposer(_ context.Context, id string, patch models.ComposerUpdateRequestData) (*models.ComposerData, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	c, ok := s.store.GetComposerEntity(id)
	if !ok {
		return nil, &api.ComposerError{Code: api.ComposerErrNotFound, Message: "composer " + id + " not found"}
	}
	if patch.Canvas != nil {
		c.Canvas = pipeline.CanvasDims{W: patch.Canvas.W, H: patch.Canvas.H}
	}
	if patch.Inputs != nil {
		c.Inputs = apiInputsToEntity(patch.Inputs)
	}
	if patch.Layout != nil {
		c.Layout = apiLayoutToEntity(patch.Layout)
	}
	if err := validateComposerLayout(c); err != nil {
		return nil, err
	}
	c.UpdatedAt = time.Now()
	if err := s.store.UpdateComposerEntity(id, c); err != nil {
		return nil, &api.ComposerError{Code: api.ComposerErrInternal, Message: err.Error()}
	}
	if s.pipe != nil {
		if err := s.pipe.ApplyComposer(c); err != nil {
			s.logger.Warn("UpdateComposer: ApplyComposer failed", "composer_id", id, "error", err)
		}
	}
	out := composerToAPI(c)
	return &out, nil
}

// DeleteComposer refuses when any stream still references this composer;
// otherwise stops the composer process and persists removal.
func (s *composerService) DeleteComposer(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.store.GetComposerEntity(id); !ok {
		return &api.ComposerError{Code: api.ComposerErrNotFound, Message: "composer " + id + " not found"}
	}

	target := "composer:" + id
	var refs []string
	for _, st := range s.store.ListPipelineStreams() {
		if st.Upstream == target {
			refs = append(refs, st.ID)
		}
	}
	if len(refs) > 0 {
		return &api.ComposerError{
			Code:               api.ComposerErrInUse,
			Message:            "composer " + id + " is referenced by streams",
			ReferencingStreams: refs,
		}
	}

	if s.pipe != nil {
		if err := s.pipe.DeleteComposer(id); err != nil {
			s.logger.Warn("DeleteComposer: DeleteComposer failed", "composer_id", id, "error", err)
		}
	}
	if err := s.store.RemoveComposerEntity(id); err != nil {
		return &api.ComposerError{Code: api.ComposerErrInternal, Message: err.Error()}
	}
	return nil
}

// ReplaceLayout swaps the composer's full layout array.
func (s *composerService) ReplaceLayout(_ context.Context, id string, layout []models.LayoutSlotData) (*models.ComposerData, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	c, ok := s.store.GetComposerEntity(id)
	if !ok {
		return nil, &api.ComposerError{Code: api.ComposerErrNotFound, Message: "composer " + id + " not found"}
	}
	c.Layout = apiLayoutToEntity(layout)
	if err := validateComposerLayout(c); err != nil {
		return nil, err
	}
	c.UpdatedAt = time.Now()
	if err := s.store.UpdateComposerEntity(id, c); err != nil {
		return nil, &api.ComposerError{Code: api.ComposerErrInternal, Message: err.Error()}
	}
	if s.pipe != nil {
		if err := s.pipe.ApplyComposer(c); err != nil {
			s.logger.Warn("ReplaceLayout: ApplyComposer failed", "composer_id", id, "error", err)
		}
	}
	out := composerToAPI(c)
	return &out, nil
}

// SetInputEffect sets or clears the per-input effect for one input ref.
func (s *composerService) SetInputEffect(_ context.Context, id, ref string, effect *models.EffectData) (*models.ComposerData, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	c, ok := s.store.GetComposerEntity(id)
	if !ok {
		return nil, &api.ComposerError{Code: api.ComposerErrNotFound, Message: "composer " + id + " not found"}
	}
	found := false
	for i := range c.Inputs {
		if c.Inputs[i].Ref == ref {
			if effect == nil {
				c.Inputs[i].Effect = nil
			} else {
				c.Inputs[i].Effect = &pipeline.Effect{Type: effect.Type, Corners: effect.Corners}
			}
			found = true
			break
		}
	}
	if !found {
		return nil, &api.ComposerError{
			Code:    api.ComposerErrInputNotFound,
			Message: "input " + ref + " not found in composer " + id,
		}
	}
	c.UpdatedAt = time.Now()
	if err := s.store.UpdateComposerEntity(id, c); err != nil {
		return nil, &api.ComposerError{Code: api.ComposerErrInternal, Message: err.Error()}
	}
	if s.pipe != nil {
		if err := s.pipe.ApplyComposer(c); err != nil {
			s.logger.Warn("SetInputEffect: ApplyComposer failed", "composer_id", id, "error", err)
		}
	}
	out := composerToAPI(c)
	return &out, nil
}

func validateComposerCreate(data models.ComposerCreateRequestData) error {
	if strings.TrimSpace(data.ID) == "" {
		return &api.ComposerError{Code: api.ComposerErrInvalid, Message: "id is required"}
	}
	if data.Canvas.W <= 0 || data.Canvas.H <= 0 {
		return &api.ComposerError{Code: api.ComposerErrInvalid, Message: "canvas dimensions must be positive"}
	}
	if len(data.Inputs) == 0 {
		return &api.ComposerError{Code: api.ComposerErrInvalid, Message: "at least one input is required"}
	}
	return nil
}

// validateComposerLayout checks each layout slot references a declared
// input. Empty layout is allowed (composer creates with no positioning).
func validateComposerLayout(c pipeline.Composer) error {
	if len(c.Layout) == 0 {
		return nil
	}
	known := make(map[string]struct{}, len(c.Inputs))
	for _, in := range c.Inputs {
		known[in.Ref] = struct{}{}
	}
	for _, slot := range c.Layout {
		if _, ok := known[slot.Input]; !ok {
			return &api.ComposerError{
				Code:    api.ComposerErrInvalid,
				Message: "layout slot references unknown input: " + slot.Input,
			}
		}
	}
	return nil
}

func apiInputsToEntity(inputs []models.ComposerInputData) []pipeline.ComposerInput {
	if len(inputs) == 0 {
		return nil
	}
	out := make([]pipeline.ComposerInput, len(inputs))
	for i, in := range inputs {
		out[i] = pipeline.ComposerInput{Ref: in.Ref}
		if in.Effect != nil {
			out[i].Effect = &pipeline.Effect{Type: in.Effect.Type, Corners: in.Effect.Corners}
		}
	}
	return out
}

func apiLayoutToEntity(layout []models.LayoutSlotData) []pipeline.LayoutSlot {
	if len(layout) == 0 {
		return nil
	}
	out := make([]pipeline.LayoutSlot, len(layout))
	for i, l := range layout {
		out[i] = pipeline.LayoutSlot{Input: l.Input, X: l.X, Y: l.Y, W: l.W, H: l.H}
	}
	return out
}

func composerToAPI(c pipeline.Composer) models.ComposerData {
	out := models.ComposerData{
		ID:        c.ID,
		Canvas:    models.CanvasDimsData{W: c.Canvas.W, H: c.Canvas.H},
		CreatedAt: c.CreatedAt,
		UpdatedAt: c.UpdatedAt,
	}
	out.Inputs = make([]models.ComposerInputData, len(c.Inputs))
	for i, in := range c.Inputs {
		out.Inputs[i] = models.ComposerInputData{Ref: in.Ref}
		if in.Effect != nil {
			e := models.EffectData{Type: in.Effect.Type, Corners: in.Effect.Corners}
			out.Inputs[i].Effect = &e
		}
	}
	out.Layout = make([]models.LayoutSlotData, len(c.Layout))
	for i, l := range c.Layout {
		out.Layout[i] = models.LayoutSlotData{Input: l.Input, X: l.X, Y: l.Y, W: l.W, H: l.H}
	}
	return out
}
