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
	"github.com/smazurov/videonode/internal/process"
	"github.com/smazurov/videonode/internal/streams"
	"github.com/smazurov/videonode/internal/streams/pipeline"
)

// SourceServiceOptions wires the SourceService to persistence and the
// supervised pipeline.
type SourceServiceOptions struct {
	Store          streams.EntityStore
	Pipeline       *pipeline.Pipeline
	PipelineSwitch PipelineSwitch
	// ColorMatrix, when set, delivers a source's detected YCbCr matrix once
	// the first status frame resolves it, so dependent encoders can be
	// rebuilt off the height-default they were frozen on at apply time.
	ColorMatrix ColorMatrixNotifier
}

// sourcePipeline is the subset of *pipeline.Pipeline the source service
// drives. Declaring it at the point of use keeps the service unit-testable
// with a small manual mock instead of a real supervised pipeline.
type sourcePipeline interface {
	ApplySource(s pipeline.Source) error
	UpdateSourceFormat(id string, f pipeline.SourceFormat) error
	DeleteSource(id string) error
	RebuildStreamEncoder(s pipeline.Stream) error
	SourceLiveness(id string) string
	SourceConsumerCount(id string) int
	SourceColorMatrix(id string) string
	Pool() process.Pool
}

// ColorMatrixNotifier delivers a source's detected YCbCr matrix once the
// first status frame resolves it (or it later changes), letting the source
// service correct dependent encoders' VUI tags without a manual re-apply.
type ColorMatrixNotifier interface {
	SetColorMatrixHandler(func(sourceID string))
}

// sourceService implements api.SourceService backed by the v2 EntityStore
// + pipeline.Pipeline. Cross-entity reference checks read composers and
// streams from the same store so Delete can refuse cleanly.
type sourceService struct {
	store  streams.EntityStore
	pipe   sourcePipeline
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
	svc := &sourceService{
		store:  opts.Store,
		psw:    opts.PipelineSwitch,
		logger: logging.GetLogger("source_svc"),
	}
	// Assign through the nil check so a nil *pipeline.Pipeline doesn't become
	// a non-nil interface value (which would defeat the s.pipe == nil guards).
	if opts.Pipeline != nil {
		svc.pipe = opts.Pipeline
	}
	if opts.ColorMatrix != nil {
		opts.ColorMatrix.SetColorMatrixHandler(svc.onColorMatrixResolved)
	}
	return svc
}

// onColorMatrixResolved rebuilds the encoders of every stream consuming the
// named source when its detected YCbCr matrix first becomes known. At
// startup ApplyStream bakes the encoder's color tag from the SD/HD height
// default because the first gRPC status frame hasn't populated the source's
// real matrix yet; this corrects an already-running encoder's VUI tag
// without a manual re-apply. Idle encoders only get a fresh cached stage.
func (s *sourceService) onColorMatrixResolved(sourceID string) {
	if s.pipe == nil || !s.pipelineSwitchEnabled() {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rebuildDependentEncoders(sourceID)
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
	if s.pipe != nil && s.pipelineSwitchEnabled() {
		if err := s.pipe.ApplySource(entity); err != nil {
			if rmErr := s.store.RemoveSourceEntity(src.ID); rmErr != nil {
				s.logger.Error("Create: rollback after ApplySource failure also failed",
					logging.KeySourceID, src.ID, logging.KeyApplyError, err, logging.KeyRollbackError, rmErr)
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
					logging.KeySourceID, id, logging.KeyError, err)
			}
		}
		if !applied {
			if err := s.pipe.ApplySource(src); err != nil {
				if restoreErr := s.store.UpdateSourceEntity(id, prev); restoreErr != nil {
					s.logger.Error("Update: rollback after ApplySource failure also failed",
						logging.KeySourceID, id, logging.KeyApplyError, err, logging.KeyRollbackError, restoreErr)
				}
				return nil, &api.SourceInvalidError{Message: "pipeline rejected source: " + err.Error()}
			}
		}
	}
	// Switch off: the patch is persisted and the pipeline reads source specs
	// through the store, so no registry refresh or spawn is needed here.
	// A resolution/framerate change is baked into each consuming stream's
	// ffmpeg `-s`/`-framerate` at encoder-build time and the source hot-apply
	// keeps those encoders attached, so they'd otherwise keep the stale
	// geometry. Rebuild dependents (bounces only the running ones). Gated on an
	// actual dims change so a no-op edit doesn't disturb connected readers.
	if s.pipe != nil && s.pipelineSwitchEnabled() && patch.Format != nil &&
		sourceDimsChanged(prev.Format, src.Format) {
		s.rebuildDependentEncoders(id)
	}
	out := sourceToAPI(src)
	return &out, nil
}

// sourceDimsChanged reports whether the capture geometry/rate that a
// downstream encoder bakes into ffmpeg's `-s`/`-framerate` differs between
// two source formats.
func sourceDimsChanged(a, b *pipeline.SourceFormat) bool {
	if a == nil || b == nil {
		return a != b
	}
	return a.Width != b.Width || a.Height != b.Height || a.FPS != b.FPS
}

// rebuildDependentEncoders rebuilds the encoder of every stream that consumes
// this source so a resolution change reaches their launch-time ffmpeg `-s`.
// Best-effort: logs and continues on per-stream failure.
func (s *sourceService) rebuildDependentEncoders(id string) {
	if s.pipe == nil {
		return
	}
	target := pipeline.SourceIDFor(id)
	for _, st := range s.store.ListPipelineStreams() {
		if st.Upstream != target {
			continue
		}
		if err := s.pipe.RebuildStreamEncoder(st); err != nil {
			s.logger.Warn("Update: rebuild dependent encoder failed",
				logging.KeyStreamID, st.ID, logging.KeyError, err)
		}
	}
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
			s.logger.Warn("Delete: DeleteSource failed", logging.KeySourceID, id, logging.KeyError, err)
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
		out.Liveness = models.SourceLiveness(s.pipe.SourceLiveness(out.ID))
		out.ConsumerCount = s.pipe.SourceConsumerCount(out.ID)
		if out.Format != nil {
			out.Format.ColorMatrix = s.pipe.SourceColorMatrix(out.ID)
		}
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
