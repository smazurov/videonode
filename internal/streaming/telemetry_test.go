package streaming

import (
	"testing"
	"time"

	"github.com/smazurov/videonode/internal/metrics"
)

type healthTestEncoder struct{ health metrics.EncoderHealth }

func (e *healthTestEncoder) EncoderHealth(_ string, _ time.Time) metrics.EncoderHealth {
	return e.health
}

type healthTestStreams struct {
	StreamProvider
	stream *Stream
}

func (s healthTestStreams) GetStream(string) *Stream { return s.stream }

func TestTelemetry_StartupIdleAndPublisherReplacement(t *testing.T) {
	now := time.Now()
	enc := &healthTestEncoder{health: metrics.EncoderHealth{State: "running", StartedAtUS: now.UnixMicro()}}
	provider := NewTelemetry(enc, healthTestStreams{}, func() []string { return []string{"unpublished"} })
	h := provider.Snapshots()["unpublished"]
	if h.Encoder.State != "running" || h.PublisherStartedAtUS != 0 || len(h.Tracks) != 0 {
		t.Fatalf("startup snapshot = %+v", h)
	}
	s := newHealthTestStream()
	provider.streams = healthTestStreams{stream: s}
	if got := provider.Snapshot("unpublished"); len(got.Tracks) != 3 {
		t.Fatalf("announced tracks missing: %+v", got)
	}
	enc.health.StartedAtUS = s.health.startedAt.Add(time.Second).UnixMicro()
	if got := provider.Snapshot("unpublished"); got.PublisherStartedAtUS != 0 || len(got.Tracks) != 0 {
		t.Fatal("old publisher was attributed to the new encoder run")
	}
	enc.health.State = "idle"
	if got := provider.Snapshot("unpublished"); got.Encoder.State != "idle" || len(got.Tracks) != 0 {
		t.Fatal("idle encoder reported stalled media")
	}
}
