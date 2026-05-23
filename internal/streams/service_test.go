package streams

import (
	"context"
	"sync"
	"testing"

	"github.com/smazurov/videonode/internal/logging"
)

// newTestService builds a minimal *service usable for unit tests that exercise
// only store-backed concurrency primitives. Process manager, processors, and
// event bus are intentionally nil.
func newTestService(t *testing.T, store Store) *service {
	t.Helper()
	return &service{
		store:   store,
		streams: make(map[string]*Stream),
		logger:  logging.GetLogger("streams-test"),
	}
}

// TestService_UpdatePartial_SyncsCanvasInputsEnabled asserts that updating a
// canvas's source list rewrites the runtime InputsEnabled map: new sources
// take their value from the source stream's current Enabled flag, prior
// entries are preserved, and removed sources drop out. Without this,
// canvas_processor.go falls through to the "NO SIGNAL" branch for newly-added
// sources whose key is missing from the stale map.
func TestService_UpdatePartial_SyncsCanvasInputsEnabled(t *testing.T) {
	canvasSpec := func(sources []string) StreamSpec {
		return StreamSpec{
			ID: "cv1",
			Canvas: &CanvasConfig{
				Width:         1920,
				Height:        1080,
				FPS:           "30",
				SourceStreams: sources,
			},
		}
	}
	store := &mockStore{streams: map[string]StreamSpec{
		"src1": {ID: "src1", FFmpeg: FFmpegConfig{Codec: "h264"}},
		"src2": {ID: "src2", FFmpeg: FFmpegConfig{Codec: "h264"}},
		"cv1":  canvasSpec([]string{"src1"}),
	}}
	svc := newTestService(t, store)
	svc.streams["src1"] = &Stream{ID: "src1", Enabled: true}
	svc.streams["src2"] = &Stream{ID: "src2", Enabled: true}
	svc.streams["cv1"] = &Stream{
		ID:            "cv1",
		Enabled:       true,
		InputsEnabled: map[string]bool{"src1": true},
	}

	if _, err := svc.UpdatePartial(context.Background(), "cv1", func(spec *StreamSpec) error {
		spec.Canvas.SourceStreams = []string{"src1", "src2"}
		return nil
	}); err != nil {
		t.Fatalf("UpdatePartial(add src2) failed: %v", err)
	}

	got := svc.streams["cv1"].InputsEnabled
	if len(got) != 2 || !got["src1"] || !got["src2"] {
		t.Fatalf("after add: want InputsEnabled={src1:true, src2:true}, got %v", got)
	}

	if _, err := svc.UpdatePartial(context.Background(), "cv1", func(spec *StreamSpec) error {
		spec.Canvas.SourceStreams = []string{"src2"}
		return nil
	}); err != nil {
		t.Fatalf("UpdatePartial(drop src1) failed: %v", err)
	}

	got = svc.streams["cv1"].InputsEnabled
	if _, present := got["src1"]; present {
		t.Errorf("after drop: src1 should be absent from InputsEnabled, got %v", got)
	}
	if !got["src2"] {
		t.Errorf("after drop: src2 should remain enabled, got %v", got)
	}
}

// TestService_UpdatePartial_AppliesUnderMutex verifies that two goroutines
// mutating disjoint fields of the same stream both end up persisted. Without
// the mutex, the second writer's stale snapshot overwrites the first writer's
// change.
func TestService_UpdatePartial_AppliesUnderMutex(t *testing.T) {
	store := &mockStore{streams: map[string]StreamSpec{
		"s1": {
			ID: "s1",
			FFmpeg: FFmpegConfig{
				Codec:      "h264",
				Resolution: "1280x720",
				FPS:        "30",
			},
		},
	}}
	svc := newTestService(t, store)
	svc.streams["s1"] = &Stream{ID: "s1", Enabled: true}

	const iterations = 100
	for i := range iterations {
		store.streams["s1"] = StreamSpec{
			ID: "s1",
			FFmpeg: FFmpegConfig{
				Codec:      "h264",
				Resolution: "1280x720",
				FPS:        "30",
			},
		}

		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			_, _ = svc.UpdatePartial(context.Background(), "s1", func(spec *StreamSpec) error {
				spec.FFmpeg.Resolution = "1920x1080"
				return nil
			})
		}()
		go func() {
			defer wg.Done()
			_, _ = svc.UpdatePartial(context.Background(), "s1", func(spec *StreamSpec) error {
				spec.FFmpeg.FPS = "60"
				return nil
			})
		}()
		wg.Wait()

		got, _ := store.GetStream("s1")
		if got.FFmpeg.Resolution != "1920x1080" {
			t.Fatalf("iter %d: lost Resolution update, got %q", i, got.FFmpeg.Resolution)
		}
		if got.FFmpeg.FPS != "60" {
			t.Fatalf("iter %d: lost FPS update, got %q", i, got.FFmpeg.FPS)
		}
	}
}
