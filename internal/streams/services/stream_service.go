package services

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/smazurov/videonode/internal/api"
	"github.com/smazurov/videonode/internal/logging"
	"github.com/smazurov/videonode/internal/streams"
	"github.com/smazurov/videonode/internal/streams/pipeline"
)

// StreamServiceOptions wires the StreamService to persistence and the
// supervised pipeline.
type StreamServiceOptions struct {
	Store    streams.EntityStore
	Pipeline *pipeline.Pipeline
	// PipelineSwitch toggles + reads the daemon-wide pipeline master
	// switch. Optional; nil disables the /api/pipeline endpoints.
	PipelineSwitch PipelineSwitch
}

// PipelineSwitch is the small surface the StreamService needs to drive
// the daemon-wide pipeline on/off toggle.
type PipelineSwitch interface {
	GetPipeline() streams.PipelineConfig
	SetPipeline(cfg streams.PipelineConfig) error
}

// streamService implements api.StreamService backed by the v2 EntityStore
// + pipeline.Pipeline. Mirrors sourceService/composerService.
type streamService struct {
	store  streams.EntityStore
	pipe   *pipeline.Pipeline
	psw    PipelineSwitch
	logger logging.Logger
	mu     sync.Mutex
}

// NewStreamService constructs a StreamService. Store is required;
// Pipeline is optional (nil = persistence-only).
func NewStreamService(opts StreamServiceOptions) api.StreamService {
	if opts.Store == nil {
		panic("services.NewStreamService: Store is required")
	}
	return &streamService{
		store:  opts.Store,
		pipe:   opts.Pipeline,
		psw:    opts.PipelineSwitch,
		logger: logging.GetLogger("stream_svc"),
	}
}

// List returns all configured streams.
func (s *streamService) List(_ context.Context) ([]pipeline.Stream, error) {
	entries := s.store.ListPipelineStreams()
	out := make([]pipeline.Stream, len(entries))
	copy(out, entries)
	return out, nil
}

// Get returns one stream by id.
func (s *streamService) Get(_ context.Context, id string) (*pipeline.Stream, error) {
	st, ok := s.store.GetPipelineStream(id)
	if !ok {
		return nil, &api.StreamNotFoundError{StreamID: id}
	}
	return &st, nil
}

// Create validates, persists, and applies a new stream.
func (s *streamService) Create(_ context.Context, in pipeline.Stream) (*pipeline.Stream, error) {
	if err := s.validateStream(in); err != nil {
		return nil, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.store.GetPipelineStream(in.ID); exists {
		return nil, &api.StreamExistsError{StreamID: in.ID}
	}
	if err := s.validateUpstreamExists(in.ID, in.Upstream); err != nil {
		return nil, err
	}

	now := time.Now()
	entity := in
	entity.CreatedAt = now
	entity.UpdatedAt = now
	if entity.Name == "" {
		entity.Name = entity.ID
	}

	if err := s.store.AddPipelineStream(entity); err != nil {
		return nil, fmt.Errorf("persist stream: %w", err)
	}
	if s.pipe != nil && s.pipelineSwitchEnabled() {
		if err := s.pipe.ApplyStream(entity); err != nil {
			// Roll back so persisted state matches what the pipeline accepts.
			if rmErr := s.store.RemovePipelineStream(entity.ID); rmErr != nil {
				s.logger.Error("Create: rollback after ApplyStream failure also failed",
					"stream_id", entity.ID, "apply_error", err, "rollback_error", rmErr)
			}
			return nil, &api.StreamInvalidError{Message: "pipeline rejected stream: " + err.Error()}
		}
	}
	return &entity, nil
}

// Update fetches the current stream, runs the caller's patch, validates,
// persists, and re-applies. On apply failure the previous spec is restored.
func (s *streamService) Update(_ context.Context, id string, patch func(*pipeline.Stream) error) (*pipeline.Stream, error) {
	if patch == nil {
		return nil, &api.StreamInvalidError{Message: "patch is required"}
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	prev, ok := s.store.GetPipelineStream(id)
	if !ok {
		return nil, &api.StreamNotFoundError{StreamID: id}
	}

	next := prev
	if err := patch(&next); err != nil {
		return nil, err
	}
	if next.ID != id {
		return nil, &api.StreamInvalidError{Message: "stream id is immutable"}
	}
	if err := s.validateStream(next); err != nil {
		return nil, err
	}
	if next.Upstream != prev.Upstream {
		if err := s.validateUpstreamExists(id, next.Upstream); err != nil {
			return nil, err
		}
	}
	next.UpdatedAt = time.Now()

	if err := s.store.UpdatePipelineStream(id, next); err != nil {
		return nil, fmt.Errorf("persist stream update: %w", err)
	}
	if s.pipe != nil && s.pipelineSwitchEnabled() {
		if err := s.pipe.ApplyStream(next); err != nil {
			if restoreErr := s.store.UpdatePipelineStream(id, prev); restoreErr != nil {
				s.logger.Error("Update: rollback after ApplyStream failure also failed",
					"stream_id", id, "apply_error", err, "rollback_error", restoreErr)
			}
			return nil, &api.StreamInvalidError{Message: "pipeline rejected stream: " + err.Error()}
		}
	}
	return &next, nil
}

// Delete stops the encoder process and removes the stream from the store.
func (s *streamService) Delete(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.store.GetPipelineStream(id); !ok {
		return &api.StreamNotFoundError{StreamID: id}
	}

	if s.pipe != nil {
		if err := s.pipe.DeleteStream(id); err != nil {
			s.logger.Warn("Delete: DeleteStream failed", "stream_id", id, "error", err)
		}
	}
	if err := s.store.RemovePipelineStream(id); err != nil {
		return fmt.Errorf("remove stream: %w", err)
	}
	return nil
}

// Restart re-applies the persisted spec to the pipeline. ApplyStream is
// idempotent — it stops the existing encoder stage and starts a fresh one.
// When the pipeline master switch is off, Restart is a no-op (the encoder
// would refuse to spawn anyway).
func (s *streamService) Restart(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	st, ok := s.store.GetPipelineStream(id)
	if !ok {
		return &api.StreamNotFoundError{StreamID: id}
	}
	if s.pipe == nil || !s.pipelineSwitchEnabled() {
		return nil
	}
	if err := s.pipe.ApplyStream(st); err != nil {
		return &api.StreamInvalidError{Message: "pipeline rejected stream: " + err.Error()}
	}
	return nil
}

// pipelineSwitchEnabled reports the daemon-wide pipeline master switch.
// Defaults to true when no switch is wired (production main.go always
// wires the streamStore as the switch).
func (s *streamService) pipelineSwitchEnabled() bool {
	if s.psw == nil {
		return true
	}
	return s.psw.GetPipeline().Enabled
}

// PipelineEnabled returns the persisted daemon-wide master switch state.
func (s *streamService) PipelineEnabled() bool {
	return s.pipelineSwitchEnabled()
}

// StartPipeline flips the persisted master switch on and re-applies every
// persisted stream so the encoder pool comes back up.
func (s *streamService) StartPipeline(_ context.Context) (bool, error) {
	if s.psw == nil {
		return false, nil
	}
	wasEnabled := s.psw.GetPipeline().Enabled
	if err := s.psw.SetPipeline(streams.PipelineConfig{Enabled: true}); err != nil {
		return false, fmt.Errorf("persist pipeline state: %w", err)
	}
	if s.pipe != nil {
		for _, st := range s.store.ListPipelineStreams() {
			if err := s.pipe.ApplyStream(st); err != nil {
				s.logger.Warn("StartPipeline: ApplyStream failed", "stream_id", st.ID, "error", err)
			}
		}
	}
	return !wasEnabled, nil
}

// StopPipeline flips the persisted master switch off and stops every
// supervised encoder process. Sources/composers stay warm so the user can
// re-engage without spawning them again.
func (s *streamService) StopPipeline(_ context.Context) (bool, error) {
	if s.psw == nil {
		return false, nil
	}
	wasEnabled := s.psw.GetPipeline().Enabled
	if err := s.psw.SetPipeline(streams.PipelineConfig{Enabled: false}); err != nil {
		return false, fmt.Errorf("persist pipeline state: %w", err)
	}
	if s.pipe != nil {
		for _, st := range s.store.ListPipelineStreams() {
			if err := s.pipe.DeleteStream(st.ID); err != nil {
				s.logger.Warn("StopPipeline: DeleteStream failed", "stream_id", st.ID, "error", err)
			}
		}
	}
	return wasEnabled, nil
}

// validateStream runs static-shape validation that doesn't depend on the
// store. Cross-entity checks (upstream exists) live in validateUpstreamExists.
func (s *streamService) validateStream(st pipeline.Stream) error {
	if strings.TrimSpace(st.ID) == "" {
		return &api.StreamInvalidError{Message: "id is required"}
	}
	if st.Upstream == "" {
		return &api.StreamInvalidError{Message: "upstream is required (\"source:<id>\" or \"composer:<id>\")"}
	}
	kind, _, ok := pipeline.SplitUpstream(st.Upstream)
	if !ok || (kind != "source" && kind != "composer") {
		return &api.StreamInvalidError{
			Message: fmt.Sprintf("malformed upstream %q (want \"source:<id>\" or \"composer:<id>\")", st.Upstream),
		}
	}
	return nil
}

// validateUpstreamExists confirms the referenced source/composer is in
// the store. Returns StreamUpstreamMissingError so the API surfaces a 404.
func (s *streamService) validateUpstreamExists(streamID, upstream string) error {
	kind, id, ok := pipeline.SplitUpstream(upstream)
	if !ok {
		return &api.StreamInvalidError{
			Message: fmt.Sprintf("malformed upstream %q (want \"source:<id>\" or \"composer:<id>\")", upstream),
		}
	}
	switch kind {
	case "source":
		if _, found := s.store.GetSourceEntity(id); !found {
			return &api.StreamUpstreamMissingError{StreamID: streamID, Upstream: upstream}
		}
	case "composer":
		if _, found := s.store.GetComposerEntity(id); !found {
			return &api.StreamUpstreamMissingError{StreamID: streamID, Upstream: upstream}
		}
	default:
		return &api.StreamInvalidError{
			Message: fmt.Sprintf("upstream kind must be source or composer (got %q)", kind),
		}
	}
	return nil
}
