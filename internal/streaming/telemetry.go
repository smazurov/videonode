package streaming

import (
	"time"

	"github.com/smazurov/videonode/internal/metrics"
)

// StreamHealth joins independent encoder and media-output observations.
type StreamHealth struct {
	SampledAt            time.Time             `json:"sampled_at"`
	StallAfterMS         int64                 `json:"stall_after_ms"`
	Encoder              metrics.EncoderHealth `json:"encoder"`
	PublisherStartedAtUS int64                 `json:"publisher_started_at_us" doc:"Publisher identity; zero when no current publisher has announced"`
	Tracks               []TrackHealth         `json:"tracks" nullable:"false"`
}

// EncoderHealthProvider supplies current-run progress and process state.
type EncoderHealthProvider interface {
	EncoderHealth(streamID string, now time.Time) metrics.EncoderHealth
}

// HealthProvider supplies identical snapshots to REST and periodic SSE publication.
type HealthProvider interface {
	Snapshot(streamID string) StreamHealth
	Snapshots() map[string]StreamHealth
}

// Telemetry joins health at read time; collecting media never depends on UI subscribers.
type Telemetry struct {
	encoders  EncoderHealthProvider
	streams   StreamProvider
	streamIDs func() []string
}

// NewTelemetry constructs a read-only health provider with IDs for idle and unpublished streams.
func NewTelemetry(encoders EncoderHealthProvider, streams StreamProvider, streamIDs func() []string) *Telemetry {
	return &Telemetry{encoders: encoders, streams: streams, streamIDs: streamIDs}
}

// Snapshot returns a current observation without lazily starting the encoder.
func (t *Telemetry) Snapshot(streamID string) StreamHealth {
	return t.snapshot(streamID, time.Now())
}

func (t *Telemetry) snapshot(streamID string, now time.Time) StreamHealth {
	enc := t.encoders.EncoderHealth(streamID, now)
	out := StreamHealth{SampledAt: now, StallAfterMS: metrics.HealthTimeout.Milliseconds(), Encoder: enc, Tracks: []TrackHealth{}}
	if enc.State != "running" && enc.State != "starting" {
		return out
	}
	s := t.streams.GetStream(streamID)
	if s == nil || s.health.startedAt.UnixMicro() < enc.StartedAtUS {
		return out
	}
	out.PublisherStartedAtUS = s.health.startedAt.UnixMicro()
	out.Tracks = s.Health(now)
	return out
}

// Snapshots includes streams with no progress or publisher, so startup silence stays visible.
func (t *Telemetry) Snapshots() map[string]StreamHealth {
	now := time.Now()
	ids := t.streamIDs()
	out := make(map[string]StreamHealth, len(ids))
	for _, id := range ids {
		out[id] = t.snapshot(id, now)
	}
	return out
}
