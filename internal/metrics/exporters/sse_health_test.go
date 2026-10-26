package exporters

import (
	"context"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/smazurov/videonode/internal/metrics"
	"github.com/smazurov/videonode/internal/streaming"
)

type testHealthProvider struct {
	streaming.HealthProvider
	last  time.Time
	calls atomic.Int32
}

func (p *testHealthProvider) Snapshots() map[string]streaming.StreamHealth {
	p.calls.Add(1)
	return map[string]streaming.StreamHealth{
		"no-gauges": {
			SampledAt: time.Now(),
			Encoder: metrics.EncoderHealth{
				State: "running", StartedAtUS: 123,
				FFmpeg: metrics.FFmpegHealth{Progress: (metrics.Activity{Count: 1, Last: p.last}).Snapshot(time.Now())},
			},
		},
	}
}

func TestSSEHealth_PublishesExpiryWithoutNewMedia(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		registry := newMockRegistry()
		health := &testHealthProvider{last: time.Now()}
		exporter := NewSSEExporter(registry, nil, health)
		exporter.Start(t.Context())
		time.Sleep(time.Second)
		synctest.Wait()
		time.Sleep(metrics.HealthTimeout)
		synctest.Wait()
		exporter.Stop()

		var first, last string
		for _, event := range registry.getEvents() {
			if event.id != "no-gauges" {
				continue
			}
			payload := event.payload.(streaming.StreamMetricsPayload)
			if payload.Health == nil {
				t.Fatal("health missing from SSE")
			}
			state := payload.Health.Encoder.FFmpeg.Progress.State
			if first == "" {
				first = state
			}
			last = state
		}
		if first != "flowing" || last != "stalled" {
			t.Fatalf("published freshness %q -> %q, want flowing -> stalled", first, last)
		}
	})
}

func TestSSEHealth_SkipsUnsubscribedWork(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		health := &testHealthProvider{}
		exporter := NewSSEExporter(newMockRegistry(), func() bool { return false }, health)
		exporter.Start(context.Background())
		time.Sleep(3 * time.Second)
		synctest.Wait()
		exporter.Stop()
		if health.calls.Load() != 0 {
			t.Fatal("gathered health without subscribers")
		}
	})
}

func TestSSEHealth_DoesNotRepublishDeletedStreamGauges(t *testing.T) {
	metrics.SetFFmpegFPS("deleted-health-test", 30)
	defer metrics.DeleteFFmpegMetrics("deleted-health-test")
	registry := newMockRegistry()
	exporter := NewSSEExporter(registry, nil, &testHealthProvider{})
	exporter.publishMetrics()
	for _, event := range registry.getEvents() {
		if event.id == "deleted-health-test" {
			t.Fatal("republished deleted stream")
		}
	}
}
