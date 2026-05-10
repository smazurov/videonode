package metrics

import (
	"sync"
	"testing"
)

func TestFFmpegMetricsViaGatherer(t *testing.T) {
	streamID := "test-stream-1"

	// Clean state
	DeleteFFmpegMetrics(streamID)

	// Initially should return empty map
	m, err := GetFFmpegMetricsFromRegistry()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := m[streamID]; ok {
		t.Error("expected no metrics for non-existent stream")
	}

	// Set metrics
	SetFFmpegFPS(streamID, 30.0)
	SetFFmpegDroppedFrames(streamID, 5)
	SetFFmpegDuplicateFrames(streamID, 2)
	SetFFmpegSpeed(streamID, 1.5)

	// Verify values via gatherer
	m, err = GetFFmpegMetricsFromRegistry()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := m[streamID]
	if got == nil {
		t.Fatal("expected non-nil metrics")
	}
	if got.FPS != 30.0 {
		t.Errorf("FPS = %v, want 30.0", got.FPS)
	}
	if got.DroppedFrames != 5 {
		t.Errorf("DroppedFrames = %v, want 5", got.DroppedFrames)
	}
	if got.DuplicateFrames != 2 {
		t.Errorf("DuplicateFrames = %v, want 2", got.DuplicateFrames)
	}
	if got.Speed != 1.5 {
		t.Errorf("Speed = %v, want 1.5", got.Speed)
	}

	// Clean up
	DeleteFFmpegMetrics(streamID)
	m, err = GetFFmpegMetricsFromRegistry()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := m[streamID]; ok {
		t.Error("expected no metrics after delete")
	}
}

func TestGetFFmpegMetricsMultipleStreams(t *testing.T) {
	// Clean state
	DeleteFFmpegMetrics("stream-a")
	DeleteFFmpegMetrics("stream-b")

	SetFFmpegFPS("stream-a", 25.0)
	SetFFmpegFPS("stream-b", 60.0)

	all, err := GetFFmpegMetricsFromRegistry()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if all["stream-a"] == nil || all["stream-a"].FPS != 25.0 {
		t.Errorf("stream-a FPS = %v, want 25.0", all["stream-a"])
	}
	if all["stream-b"] == nil || all["stream-b"].FPS != 60.0 {
		t.Errorf("stream-b FPS = %v, want 60.0", all["stream-b"])
	}

	DeleteFFmpegMetrics("stream-a")
	DeleteFFmpegMetrics("stream-b")
}

func TestFFmpegMetricsConcurrency(t *testing.T) {
	streamID := "concurrent-stream"
	DeleteFFmpegMetrics(streamID)

	var wg sync.WaitGroup
	for i := range 100 {
		wg.Add(1)
		go func(val float64) {
			defer wg.Done()
			SetFFmpegFPS(streamID, val)
			SetFFmpegDroppedFrames(streamID, val)
			_, _ = GetFFmpegMetricsFromRegistry()
		}(float64(i))
	}
	wg.Wait()

	// Should not panic, final value is indeterminate
	m, err := GetFFmpegMetricsFromRegistry()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m[streamID] == nil {
		t.Error("expected non-nil metrics after concurrent access")
	}

	DeleteFFmpegMetrics(streamID)
}

func TestGetAllMetricsAsJSON(t *testing.T) {
	streamID := "json-test-stream"
	DeleteFFmpegMetrics(streamID)

	SetFFmpegFPS(streamID, 29.97)

	families, err := GetAllMetricsAsJSON()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var found bool
	for _, fam := range families {
		if fam.Name == "videonode_ffmpeg_fps" {
			for _, metric := range fam.Metrics {
				if metric.Labels["stream_id"] == streamID {
					found = true
					if metric.Value != 29.97 {
						t.Errorf("FPS value = %v, want 29.97", metric.Value)
					}
				}
			}
		}
	}
	if !found {
		t.Error("expected to find videonode_ffmpeg_fps metric")
	}

	DeleteFFmpegMetrics(streamID)
}
