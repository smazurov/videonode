package services

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/smazurov/videonode/internal/api"
	"github.com/smazurov/videonode/internal/api/models"
	"github.com/smazurov/videonode/internal/logging"
	"github.com/smazurov/videonode/internal/process"
	"github.com/smazurov/videonode/internal/streams"
	"github.com/smazurov/videonode/internal/streams/pipeline"
	"github.com/smazurov/videonode/internal/streams/pipelinectl"
)

// ComposerServiceOptions wires the ComposerService to persistence and
// the supervised pipeline.
type ComposerServiceOptions struct {
	Store          streams.EntityStore
	Pipeline       *pipeline.Pipeline
	PipelineSwitch PipelineSwitch
}

// composerPipeline is the subset of *pipeline.Pipeline the composer
// service drives. Declaring it at the point of use keeps the service
// unit-testable with a small manual mock instead of a real supervised
// pipeline.
type composerPipeline interface {
	ApplyComposer(c pipeline.Composer) error
	UpdateComposerLayout(id string, layout []pipeline.LayoutSlot) error
	UpdateComposerCanvas(id string, canvas pipeline.CanvasDims) error
	UpdateComposerEffect(id, inputRef string, effect *pipeline.Effect) error
	DeleteComposer(id string) error
	RebuildStreamEncoder(s pipeline.Stream) error
	Pool() process.Pool
}

// composerService implements api.ComposerService backed by the v2
// EntityStore + pipeline.Pipeline.
type composerService struct {
	store  streams.EntityStore
	pipe   composerPipeline
	psw    PipelineSwitch
	logger logging.Logger
	mu     sync.Mutex
}

// NewComposerService constructs a ComposerService. Store is required;
// Pipeline is optional (nil = persistence-only).
func NewComposerService(opts ComposerServiceOptions) api.ComposerService {
	if opts.Store == nil {
		panic("services.NewComposerService: Store is required")
	}
	svc := &composerService{
		store:  opts.Store,
		psw:    opts.PipelineSwitch,
		logger: logging.GetLogger("composer_svc"),
	}
	// Assign through the nil check so a nil *pipeline.Pipeline doesn't become
	// a non-nil interface value (which would defeat the s.pipe == nil guards).
	if opts.Pipeline != nil {
		svc.pipe = opts.Pipeline
	}
	return svc
}

func (s *composerService) pipelineSwitchEnabled() bool {
	if s.psw == nil {
		return true
	}
	return s.psw.GetPipeline().Enabled
}

// editComposer is the shared shape of every in-place composer config edit:
// lock, load, apply mutate (which edits AND validates the working copy and
// returns the live-push thunk capturing exactly what changed), persist, then
// sync onto the pipeline. A nil push thunk means "nothing to push". The store
// write is the source of truth; the pipeline only mirrors it.
func (s *composerService) editComposer(op, id string, mutate func(c *pipeline.Composer) (push func() error, err error)) (*models.ComposerData, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	prev, ok := s.store.GetComposerEntity(id)
	if !ok {
		return nil, &api.ComposerError{Code: api.ComposerErrNotFound, Message: "composer " + id + " not found"}
	}
	c := prev
	push, err := mutate(&c)
	if err != nil {
		return nil, err
	}
	c.UpdatedAt = time.Now()
	if err := s.store.UpdateComposerEntity(id, c); err != nil {
		return nil, &api.ComposerError{Code: api.ComposerErrInternal, Message: err.Error()}
	}
	if err := s.syncComposerEdit(op, id, prev, push); err != nil {
		return nil, err
	}
	out := s.toEnrichedAPI(c)
	return &out, nil
}

// syncComposerEdit mirrors a persisted composer edit onto the pipeline.
// Switch off (or no live push) → nothing to do: the new spec is already
// persisted and the pipeline reads it through the store on its next spawn.
// Switch on → best-effort hot-apply: a not-live composer
// (pipelinectl.ErrNoSuchComposer) is a non-fatal skip, while any other RPC
// error rolls the store back to prev and surfaces so the composer's rejection
// isn't lost.
func (s *composerService) syncComposerEdit(op, id string, prev pipeline.Composer, push func() error) error {
	if s.pipe == nil {
		return nil
	}
	if !s.pipelineSwitchEnabled() || push == nil {
		// Switch off (or no live edit): the store already holds the new spec
		// and the pipeline reads through to it; nothing to push.
		return nil
	}
	err := push()
	if err == nil {
		return nil
	}
	if errors.Is(err, pipelinectl.ErrNoSuchComposer) {
		s.logger.Debug(op+": composer not live; config persisted, skipping live push",
			logging.KeyComposerID, id, logging.KeyError, err)
		return nil
	}
	if restoreErr := s.store.UpdateComposerEntity(id, prev); restoreErr != nil {
		s.logger.Error(op+": rollback after pipeline failure also failed",
			logging.KeyComposerID, id, logging.KeyApplyError, err, logging.KeyRollbackError, restoreErr)
	}
	return &api.ComposerError{Code: api.ComposerErrInvalid, Message: "pipeline rejected composer: " + err.Error()}
}

// ListComposers returns all persisted composers in API wire shape.
func (s *composerService) ListComposers(_ context.Context) ([]models.ComposerData, error) {
	entries := s.store.ListComposerEntities()
	streams := s.store.ListPipelineStreams()
	out := make([]models.ComposerData, len(entries))
	for i, e := range entries {
		out[i] = composerToAPI(e)
		s.enrichRuntime(&out[i], streams)
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
	s.enrichRuntime(&out, s.store.ListPipelineStreams())
	return &out, nil
}

// enrichRuntime layers in the denormalized DownstreamStreamIDs (cheap
// store-side join across streams whose upstream == "composer:<id>")
// enrichRuntime layers in the denormalized DownstreamStreamIDs and
// the process pool state. Kept out of composerToAPI so the static
// helper stays pure for tests.
func (s *composerService) enrichRuntime(out *models.ComposerData, streams []streams.PipelineStream) {
	downstream := make([]string, 0)
	wanted := "composer:" + out.ID
	for _, st := range streams {
		if st.Upstream == wanted {
			downstream = append(downstream, st.ID)
		}
	}
	out.DownstreamStreamIDs = downstream
	if s.pipe != nil {
		if pool := s.pipe.Pool(); pool != nil {
			out.Status = models.ProcessStatus(pool.GetStatus(pipeline.ComposerPoolKey(out.ID)).State)
		}
	}
}

// toEnrichedAPI converts a persisted composer to the wire shape with the
// denormalized DownstreamStreamIDs (and pool status) populated. Every method
// that returns a ComposerData to a handler must use this — the handlers
// publish the returned value as the composer.updated SSE payload, and a payload
// missing downstream_stream_ids would stomp the field in the UI's store until a
// reload re-fetches via the (enriched) Get/List path.
func (s *composerService) toEnrichedAPI(c pipeline.Composer) models.ComposerData {
	out := composerToAPI(c)
	s.enrichRuntime(&out, s.store.ListPipelineStreams())
	return out
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
	if s.pipe != nil && s.pipelineSwitchEnabled() {
		if err := s.pipe.ApplyComposer(entity); err != nil {
			if rmErr := s.store.RemoveComposerEntity(entity.ID); rmErr != nil {
				s.logger.Error("CreateComposer: rollback after ApplyComposer failure also failed",
					logging.KeyComposerID, entity.ID, logging.KeyApplyError, err, logging.KeyRollbackError, rmErr)
			}
			return nil, &api.ComposerError{Code: api.ComposerErrInvalid, Message: "pipeline rejected composer: " + err.Error()}
		}
	}
	out := s.toEnrichedAPI(entity)
	return &out, nil
}

// UpdateComposer applies a partial patch to an existing composer.
// Layout-only and effect-only diffs hot-apply over the gRPC control
// plane; canvas-dim or input-list diffs require a process restart and
// re-apply via ApplyComposer.
func (s *composerService) UpdateComposer(_ context.Context, id string, patch models.ComposerUpdateRequestData) (*models.ComposerData, error) {
	return s.editComposer("UpdateComposer", id, func(c *pipeline.Composer) (func() error, error) {
		dimsChanged, bgChanged, inputsChanged, layoutChanged := false, false, false, false
		if patch.Canvas != nil {
			if patch.Canvas.W <= 0 || patch.Canvas.H <= 0 {
				return nil, &api.ComposerError{Code: api.ComposerErrInvalid, Message: "canvas dimensions must be positive"}
			}
			if patch.Canvas.FPS < 0 || patch.Canvas.FPS > 240 {
				return nil, &api.ComposerError{Code: api.ComposerErrInvalid, Message: "canvas fps must be in 0..240 (0 = daemon default)"}
			}
			next := pipeline.CanvasDims{W: patch.Canvas.W, H: patch.Canvas.H, FPS: patch.Canvas.FPS, Background: patch.Canvas.Background}
			// Background recolors live via SetCanvas; only W/H/FPS need a
			// process restart (broadcast dims are fixed at encoder build).
			dimsChanged = next.W != c.Canvas.W || next.H != c.Canvas.H || next.FPS != c.Canvas.FPS
			bgChanged = next.Background != c.Canvas.Background
			c.Canvas = next
		}
		if patch.Inputs != nil {
			next := apiInputsToEntity(patch.Inputs)
			inputsChanged = !inputsEqual(c.Inputs, next)
			c.Inputs = next
		}
		if patch.Layout != nil {
			next := apiLayoutToEntity(patch.Layout)
			layoutChanged = !layoutEqual(c.Layout, next)
			c.Layout = next
		}
		if err := validateComposerLayout(*c); err != nil {
			return nil, err
		}
		return func() error {
			// A dims/input change requires a process restart; ApplyComposer
			// re-pushes SetCanvas (background included), so bgChanged needs no
			// separate handling on that path.
			if dimsChanged || inputsChanged {
				if err := s.pipe.ApplyComposer(*c); err != nil {
					return err
				}
				// A canvas resize changes the composer's broadcast dims, which each
				// consuming stream's ffmpeg `-s` is fixed to at encoder-build time.
				// Rebuild dependents (bounces only the running ones); gated on
				// dimsChanged so layout/input/background-only edits leave readers undisturbed.
				if dimsChanged {
					s.rebuildDependentEncoders(id)
				}
				return nil
			}
			if bgChanged {
				if err := s.pipe.UpdateComposerCanvas(id, c.Canvas); err != nil {
					return err
				}
			}
			if layoutChanged {
				return s.pipe.UpdateComposerLayout(id, c.Layout)
			}
			return nil
		}, nil
	})
}

// rebuildDependentEncoders rebuilds the encoder of every stream that consumes
// this composer so a canvas resize reaches their launch-time ffmpeg `-s`.
// Best-effort: logs and continues on per-stream failure.
func (s *composerService) rebuildDependentEncoders(id string) {
	if s.pipe == nil {
		return
	}
	target := "composer:" + id
	for _, st := range s.store.ListPipelineStreams() {
		if st.Upstream != target {
			continue
		}
		if err := s.pipe.RebuildStreamEncoder(st); err != nil {
			s.logger.Warn("UpdateComposer: rebuild dependent encoder failed",
				logging.KeyStreamID, st.ID, logging.KeyError, err)
		}
	}
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
			s.logger.Warn("DeleteComposer: DeleteComposer failed", logging.KeyComposerID, id, logging.KeyError, err)
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
	return s.editComposer("ReplaceLayout", id, func(c *pipeline.Composer) (func() error, error) {
		c.Layout = apiLayoutToEntity(layout)
		if err := validateComposerLayout(*c); err != nil {
			return nil, err
		}
		return func() error { return s.pipe.UpdateComposerLayout(id, c.Layout) }, nil
	})
}

// SetInputEffect sets or clears the per-input effect for one input ref.
// Hot-applies via the gRPC control plane — no process restart.
func (s *composerService) SetInputEffect(_ context.Context, id, ref string, effect *models.EffectData) (*models.ComposerData, error) {
	return s.editComposer("SetInputEffect", id, func(c *pipeline.Composer) (func() error, error) {
		// Copy Inputs so the mutation doesn't alias prev (used for rollback).
		c.Inputs = append([]pipeline.ComposerInput(nil), c.Inputs...)
		var applied *pipeline.Effect
		for i := range c.Inputs {
			if c.Inputs[i].Ref != ref {
				continue
			}
			if effect != nil {
				c.Inputs[i].Effect = &pipeline.Effect{
					Type:      effect.Type,
					Corners:   effect.Corners,
					SnapshotW: effect.SnapshotW,
					SnapshotH: effect.SnapshotH,
				}
			} else {
				c.Inputs[i].Effect = nil
			}
			applied = c.Inputs[i].Effect
			return func() error { return s.pipe.UpdateComposerEffect(id, ref, applied) }, nil
		}
		return nil, &api.ComposerError{
			Code:    api.ComposerErrInputNotFound,
			Message: "input " + ref + " not found in composer " + id,
		}
	})
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
		if ae != nil && (ae.Type != be.Type || ae.Corners != be.Corners ||
			ae.SnapshotW != be.SnapshotW || ae.SnapshotH != be.SnapshotH) {
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
		if a[i].Input != b[i].Input || a[i].X != b[i].X || a[i].Y != b[i].Y ||
			a[i].W != b[i].W || a[i].H != b[i].H || a[i].Rotation != b[i].Rotation ||
			a[i].AspectRatioMode != b[i].AspectRatioMode {
			return false
		}
		if !cropConfigEqual(a[i].Crop, b[i].Crop) {
			return false
		}
	}
	return true
}

func cropConfigEqual(a, b *pipeline.CropConfig) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return a.X == b.X && a.Y == b.Y && a.Scale == b.Scale
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
		switch slot.Rotation {
		case 0, 90, 180, 270:
		default:
			return &api.ComposerError{
				Code:    api.ComposerErrInvalid,
				Message: fmt.Sprintf("layout slot rotation must be 0, 90, 180, or 270; got %d", slot.Rotation),
			}
		}
		switch slot.AspectRatioMode {
		case "", "stretch", "fit", "crop":
		default:
			return &api.ComposerError{
				Code:    api.ComposerErrInvalid,
				Message: fmt.Sprintf("layout slot aspect_ratio_mode must be stretch, fit, or crop; got %q", slot.AspectRatioMode),
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
			out[i].Effect = &pipeline.Effect{
				Type:      in.Effect.Type,
				Corners:   in.Effect.Corners,
				SnapshotW: in.Effect.SnapshotW,
				SnapshotH: in.Effect.SnapshotH,
			}
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
		slot := pipeline.LayoutSlot{Input: l.Input, X: l.X, Y: l.Y, W: l.W, H: l.H, Rotation: l.Rotation, AspectRatioMode: l.AspectRatioMode}
		if l.Crop != nil {
			slot.Crop = &pipeline.CropConfig{X: l.Crop.X, Y: l.Crop.Y, Scale: l.Crop.Scale}
		}
		out[i] = slot
	}
	return out
}

func composerToAPI(c pipeline.Composer) models.ComposerData {
	out := models.ComposerData{
		ID:        c.ID,
		Canvas:    models.CanvasDimsData{W: c.Canvas.W, H: c.Canvas.H, FPS: c.Canvas.FPS, Background: c.Canvas.Background},
		CreatedAt: c.CreatedAt,
		UpdatedAt: c.UpdatedAt,
	}
	out.Inputs = make([]models.ComposerInputData, len(c.Inputs))
	for i, in := range c.Inputs {
		out.Inputs[i] = models.ComposerInputData{Ref: in.Ref}
		if in.Effect != nil {
			e := models.EffectData{
				Type:      in.Effect.Type,
				Corners:   in.Effect.Corners,
				SnapshotW: in.Effect.SnapshotW,
				SnapshotH: in.Effect.SnapshotH,
			}
			out.Inputs[i].Effect = &e
		}
	}
	out.Layout = make([]models.LayoutSlotData, len(c.Layout))
	for i, l := range c.Layout {
		slot := models.LayoutSlotData{Input: l.Input, X: l.X, Y: l.Y, W: l.W, H: l.H, Rotation: l.Rotation, AspectRatioMode: l.AspectRatioMode}
		if l.Crop != nil {
			slot.Crop = &models.CropConfigData{X: l.Crop.X, Y: l.Crop.Y, Scale: l.Crop.Scale}
		}
		out.Layout[i] = slot
	}
	return out
}
