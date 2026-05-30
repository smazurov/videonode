package api

import (
	"context"
	"fmt"

	"github.com/smazurov/videonode/internal/api/models"
	"github.com/smazurov/videonode/internal/streams/pipeline"
)

// StreamService is the contract the API layer requires of the stream
// service implementation. Mirrors SourceService/ComposerService: small,
// focused, persist-then-apply.
type StreamService interface {
	List(ctx context.Context) ([]pipeline.Stream, error)
	Get(ctx context.Context, id string) (*pipeline.Stream, error)
	Create(ctx context.Context, s pipeline.Stream) (*pipeline.Stream, error)
	// Update applies an arbitrary mutation to the stored stream and
	// re-applies the pipeline. The caller mutates the passed-in copy
	// in-place; the service handles validation, persistence, and rollback.
	Update(ctx context.Context, id string, patch func(*pipeline.Stream) error) (*pipeline.Stream, error)
	Delete(ctx context.Context, id string) error

	// EncoderStatus returns the process pool state for a stream's encoder.
	EncoderStatus(streamID string) models.ProcessStatus

	// PipelineEnabled / StartPipeline / StopPipeline drive the daemon-wide
	// pipeline master switch. Persisted on the store.
	PipelineEnabled() bool
	StartPipeline(ctx context.Context) (bool, error)
	StopPipeline(ctx context.Context) (bool, error)
}

// StreamNotFoundError indicates a stream id wasn't in the store. Mapped to 404.
type StreamNotFoundError struct {
	StreamID string
}

func (e *StreamNotFoundError) Error() string {
	return fmt.Sprintf("stream %q not found", e.StreamID)
}

// StreamExistsError indicates a duplicate stream id on create. Mapped to 409.
type StreamExistsError struct {
	StreamID string
}

func (e *StreamExistsError) Error() string {
	return fmt.Sprintf("stream %q already exists", e.StreamID)
}

// StreamInvalidError reports validation failures (missing upstream,
// dangling reference, malformed encoder config, etc.). Mapped to 400.
type StreamInvalidError struct {
	Message string
}

func (e *StreamInvalidError) Error() string {
	return e.Message
}

// StreamUpstreamMissingError reports a stream whose upstream ref points
// at a source or composer that doesn't exist. Mapped to 404.
type StreamUpstreamMissingError struct {
	StreamID string
	Upstream string
}

func (e *StreamUpstreamMissingError) Error() string {
	return fmt.Sprintf("stream %q: upstream %q does not exist", e.StreamID, e.Upstream)
}
