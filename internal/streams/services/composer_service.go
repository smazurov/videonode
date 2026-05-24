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
		Canvas:    pipeline.CanvasDims{W: data.Canvas.W, H: data.Canvas.H, FPS: data.Canvas.FPS},
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
			// Roll back the insert so the persisted state matches what
			// the pipeline accepts.
			if rmErr := s.store.RemoveComposerEntity(entity.ID); rmErr != nil {
				s.logger.Error("CreateComposer: rollback after ApplyComposer failure also failed",
					"composer_id", entity.ID, "apply_error", err, "rollback_error", rmErr)
			}
			return nil, &api.ComposerError{Code: api.ComposerErrInvalid, Message: "pipeline rejected composer: " + err.Error()}
		}
	}
	out := composerToAPI(entity)
	return &out, nil
}

// UpdateComposer applies a partial patch to an existing composer.
// Layout-only and effect-only diffs hot-apply over the gRPC control
// plane; canvas-dim or input-list diffs require a process restart and
// re-apply via ApplyComposer.
func (s *composerService) UpdateComposer(_ context.Context, id string, patch models.ComposerUpdateRequestData) (*models.ComposerData, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	prev, ok := s.store.GetComposerEntity(id)
	if !ok {
		return nil, &api.ComposerError{Code: api.ComposerErrNotFound, Message: "composer " + id + " not found"}
	}
	c := prev
	canvasChanged := false
	inputsChanged := false
	layoutChanged := false
	if patch.Canvas != nil {
		if patch.Canvas.W <= 0 || patch.Canvas.H <= 0 {
			return nil, &api.ComposerError{Code: api.ComposerErrInvalid, Message: "canvas dimensions must be positive"}
		}
		if patch.Canvas.FPS < 0 || patch.Canvas.FPS > 240 {
			return nil, &api.ComposerError{Code: api.ComposerErrInvalid, Message: "canvas fps must be in 0..240 (0 = daemon default)"}
		}
		next := pipeline.CanvasDims{W: patch.Canvas.W, H: patch.Canvas.H, FPS: patch.Canvas.FPS}
		if next != c.Canvas {
			canvasChanged = true
		}
		c.Canvas = next
	}
	if patch.Inputs != nil {
		next := apiInputsToEntity(patch.Inputs)
		if !inputsEqual(c.Inputs, next) {
			inputsChanged = true
		}
		c.Inputs = next
	}
	if patch.Layout != nil {
		next := apiLayoutToEntity(patch.Layout)
		if !layoutEqual(c.Layout, next) {
			layoutChanged = true
		}
		c.Layout = next
	}
	if err := validateComposerLayout(c); err != nil {
		return nil, err
	}
	c.UpdatedAt = time.Now()
	if err := s.store.UpdateComposerEntity(id, c); err != nil {
		return nil, &api.ComposerError{Code: api.ComposerErrInternal, Message: err.Error()}
	}
	if s.pipe != nil {
		var applyErr error
		switch {
		case canvasChanged || inputsChanged:
			applyErr = s.pipe.ApplyComposer(c)
		case layoutChanged:
			applyErr = s.pipe.UpdateComposerLayout(id, c.Layout)
		}
		if applyErr != nil {
			if restoreErr := s.store.UpdateComposerEntity(id, prev); restoreErr != nil {
				s.logger.Error("UpdateComposer: rollback after pipeline failure also failed",
					"composer_id", id, "apply_error", applyErr, "rollback_error", restoreErr)
			}
			return nil, &api.ComposerError{Code: api.ComposerErrInvalid, Message: "pipeline rejected composer: " + applyErr.Error()}
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

// ReplaceLayout swaps the composer's full layout array. Hot-applies via
// the gRPC control plane — no process restart, so downstream vn-sink
// consumers stay connected.
func (s *composerService) ReplaceLayout(_ context.Context, id string, layout []models.LayoutSlotData) (*models.ComposerData, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	prev, ok := s.store.GetComposerEntity(id)
	if !ok {
		return nil, &api.ComposerError{Code: api.ComposerErrNotFound, Message: "composer " + id + " not found"}
	}
	c := prev
	c.Layout = apiLayoutToEntity(layout)
	if err := validateComposerLayout(c); err != nil {
		return nil, err
	}
	c.UpdatedAt = time.Now()
	if err := s.store.UpdateComposerEntity(id, c); err != nil {
		return nil, &api.ComposerError{Code: api.ComposerErrInternal, Message: err.Error()}
	}
	if s.pipe != nil {
		if err := s.pipe.UpdateComposerLayout(id, c.Layout); err != nil {
			if restoreErr := s.store.UpdateComposerEntity(id, prev); restoreErr != nil {
				s.logger.Error("ReplaceLayout: rollback after UpdateComposerLayout failure also failed",
					"composer_id", id, "apply_error", err, "rollback_error", restoreErr)
			}
			return nil, &api.ComposerError{Code: api.ComposerErrInvalid, Message: "pipeline rejected composer: " + err.Error()}
		}
	}
	out := composerToAPI(c)
	return &out, nil
}

// SetInputEffect sets or clears the per-input effect for one input ref.
// Hot-applies via the gRPC control plane — no process restart.
func (s *composerService) SetInputEffect(_ context.Context, id, ref string, effect *models.EffectData) (*models.ComposerData, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	prev, ok := s.store.GetComposerEntity(id)
	if !ok {
		return nil, &api.ComposerError{Code: api.ComposerErrNotFound, Message: "composer " + id + " not found"}
	}
	// Deep-copy Inputs so the in-flight mutation doesn't poison prev (used
	// for rollback on pipeline failure).
	c := prev
	c.Inputs = make([]pipeline.ComposerInput, len(prev.Inputs))
	copy(c.Inputs, prev.Inputs)
	var applied *pipeline.Effect
	found := false
	for i := range c.Inputs {
		if c.Inputs[i].Ref == ref {
			if effect == nil {
				c.Inputs[i].Effect = nil
			} else {
				c.Inputs[i].Effect = &pipeline.Effect{Type: effect.Type, Corners: effect.Corners}
			}
			applied = c.Inputs[i].Effect
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
		if err := s.pipe.UpdateComposerEffect(id, ref, applied); err != nil {
			if restoreErr := s.store.UpdateComposerEntity(id, prev); restoreErr != nil {
				s.logger.Error("SetInputEffect: rollback after UpdateComposerEffect failure also failed",
					"composer_id", id, "apply_error", err, "rollback_error", restoreErr)
			}
			return nil, &api.ComposerError{Code: api.ComposerErrInvalid, Message: "pipeline rejected composer: " + err.Error()}
		}
	}
	out := composerToAPI(c)
	return &out, nil
}

// inputsEqual returns true when two ComposerInput slices have the same
// elements in the same order (ref + effect both compared field-wise).
func inputsEqual(a, b []pipeline.ComposerInput) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Ref != b[i].Ref {
			return false
		}
		ae, be := a[i].Effect, b[i].Effect
		if (ae == nil) != (be == nil) {
			return false
		}
		if ae != nil && (ae.Type != be.Type || ae.Corners != be.Corners) {
			return false
		}
	}
	return true
}

// layoutEqual returns true when two LayoutSlot slices match field-wise
// in declared order.
func layoutEqual(a, b []pipeline.LayoutSlot) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func validateComposerCreate(data models.ComposerCreateRequestData) error {
	if strings.TrimSpace(data.ID) == "" {
		return &api.ComposerError{Code: api.ComposerErrInvalid, Message: "id is required"}
	}
	if data.Canvas.W <= 0 || data.Canvas.H <= 0 {
		return &api.ComposerError{Code: api.ComposerErrInvalid, Message: "canvas dimensions must be positive"}
	}
	if data.Canvas.FPS < 0 || data.Canvas.FPS > 240 {
		return &api.ComposerError{Code: api.ComposerErrInvalid, Message: "canvas fps must be in 0..240 (0 = daemon default)"}
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
		Canvas:    models.CanvasDimsData{W: c.Canvas.W, H: c.Canvas.H, FPS: c.Canvas.FPS},
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
