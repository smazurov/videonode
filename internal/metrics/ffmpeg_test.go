package metrics

import (
	"sync"
	"testing"
)

func TestFFmpegMetricsCache(t *testing.T) {
	streamID := "test-stream-1"

	// Clean state
	DeleteFFmpegMetrics(streamID)

	// Initially should return nil
	if m := GetFFmpegMetrics(streamID); m != nil {
		t.Error("expected nil for non-existent stream")
	}

	// Set metrics
	SetFFmpegFPS(streamID, 30.0)
	SetFFmpegDroppedFrames(streamID, 5)
	SetFFmpegDuplicateFrames(streamID, 2)
	SetFFmpegSpeed(streamID, 1.5)

	// Verify cached values
	m := GetFFmpegMetrics(streamID)
	if m == nil {
		t.Fatal("expected non-nil metrics")
	}
	if m.FPS != 30.0 {
		t.Errorf("FPS = %v, want 30.0", m.FPS)
	}
	if m.DroppedFrames != 5 {
		t.Errorf("DroppedFrames = %v, want 5", m.DroppedFrames)
	}
	if m.DuplicateFrames != 2 {
		t.Errorf("DuplicateFrames = %v, want 2", m.DuplicateFrames)
	}
	if m.Speed != 1.5 {
		t.Errorf("Speed = %v, want 1.5", m.Speed)
	}

	// Verify returned copy is independent
	m.FPS = 999
	m2 := GetFFmpegMetrics(streamID)
	if m2.FPS != 30.0 {
		t.Errorf("cache was modified, FPS = %v, want 30.0", m2.FPS)
	}

	// Clean up
	DeleteFFmpegMetrics(streamID)
	if deleted := GetFFmpegMetrics(streamID); deleted != nil {
		t.Error("expected nil after delete")
	}
}

func TestGetAllFFmpegMetrics(t *testing.T) {
	// Clean state
	DeleteFFmpegMetrics("stream-a")
	DeleteFFmpegMetrics("stream-b")

	SetFFmpegFPS("stream-a", 25.0)
	SetFFmpegFPS("stream-b", 60.0)

	all := GetAllFFmpegMetrics()
	if len(all) < 2 {
		t.Fatalf("expected at least 2 streams, got %d", len(all))
	}

	if all["stream-a"] == nil || all["stream-a"].FPS != 25.0 {
		t.Errorf("stream-a FPS = %v, want 25.0", all["stream-a"])
	}
	if all["stream-b"] == nil || all["stream-b"].FPS != 60.0 {
		t.Errorf("stream-b FPS = %v, want 60.0", all["stream-b"])
	}

	// Verify returned map is independent
	all["stream-a"].FPS = 999
	fresh := GetAllFFmpegMetrics()
	if fresh["stream-a"].FPS != 25.0 {
		t.Errorf("cache was modified")
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
			_ = GetFFmpegMetrics(streamID)
			_ = GetAllFFmpegMetrics()
		}(float64(i))
	}
	wg.Wait()

	// Should not panic, final value is indeterminate
	m := GetFFmpegMetrics(streamID)
	if m == nil {
		t.Error("expected non-nil metrics after concurrent access")
	}

	DeleteFFmpegMetrics(streamID)
}
