package streams

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/smazurov/videonode/internal/events"
	"github.com/smazurov/videonode/internal/logging"
)

// stubProcMgr implements StreamProcessManager with no-op defaults; tests
// override only the methods they care about by overlaying counters/maps.
type stubProcMgr struct {
	releaseCalls []string
	startCalls   []string
	running      map[string]bool
	ownedBy      map[string]string
}

func (s *stubProcMgr) Start(id string) error {
	s.startCalls = append(s.startCalls, id)
	return nil
}
func (s *stubProcMgr) Stop(string) error          { return nil }
func (s *stubProcMgr) Restart(string) error       { return nil }
func (s *stubProcMgr) RestartCanvas(string) error { return nil }
func (s *stubProcMgr) ReleaseCanvas(id string) error {
	s.releaseCalls = append(s.releaseCalls, id)
	return nil
}
func (s *stubProcMgr) GetStatus(string) (*ProcessInfo, error)    { return &ProcessInfo{}, nil }
func (s *stubProcMgr) StartAll() error                           { return nil }
func (s *stubProcMgr) StopAll()                                  {}
func (s *stubProcMgr) IsRunning(id string) bool                  { return s.running[id] }
func (s *stubProcMgr) IsCrashed(string) bool                     { return false }
func (s *stubProcMgr) CaptureRawSnapshot(string) ([]byte, error) { return nil, nil }
func (s *stubProcMgr) OwnedBy(id string) string                  { return s.ownedBy[id] }

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

// TestService_ReleaseCanvas_FlipsEnabledAndEmitsEvent verifies the
// disband path: ReleaseCanvas delegates to the process manager, sets the
// canvas's runtime Enabled to false, and publishes a StreamStateChangedEvent.
func TestService_ReleaseCanvas_FlipsEnabledAndEmitsEvent(t *testing.T) {
	store := &mockStore{streams: map[string]StreamSpec{
		"src1": {ID: "src1", FFmpeg: FFmpegConfig{Codec: "h264"}},
		"cv1": {
			ID: "cv1",
			Canvas: &CanvasConfig{
				Width: 1920, Height: 1080, FPS: "30",
				SourceStreams: []string{"src1"},
			},
		},
	}}
	svc := newTestService(t, store)
	svc.streams["cv1"] = &Stream{ID: "cv1", Enabled: true}
	pm := &stubProcMgr{running: map[string]bool{"cv1": true}}
	svc.processManager = pm
	bus := events.New()
	svc.eventBus = bus

	var (
		mu    sync.Mutex
		gotEv *events.StreamStateChangedEvent
	)
	unsub := bus.Subscribe(func(ev events.StreamStateChangedEvent) {
		mu.Lock()
		defer mu.Unlock()
		gotEv = &ev
	})
	defer unsub()

	if err := svc.ReleaseCanvas(context.Background(), "cv1"); err != nil {
		t.Fatalf("ReleaseCanvas: %v", err)
	}

	if len(pm.releaseCalls) != 1 || pm.releaseCalls[0] != "cv1" {
		t.Errorf("expected process manager ReleaseCanvas(cv1), got %v", pm.releaseCalls)
	}
	if svc.streams["cv1"].Enabled {
		t.Errorf("canvas Enabled should be false after release")
	}

	// Bus delivers synchronously via kelindar/event, but allow a brief settle.
	for range 50 {
		mu.Lock()
		if gotEv != nil {
			mu.Unlock()
			break
		}
		mu.Unlock()
	}
	if gotEv == nil {
		t.Fatalf("expected StreamStateChangedEvent on release")
	}
	if gotEv.StreamID != "cv1" || gotEv.Enabled {
		t.Errorf("event mismatch: got %+v", *gotEv)
	}
}

// TestService_EngageCanvas_FlipsEnabledAndCallsStart verifies the engage path:
// EngageCanvas calls processManager.Start (which handles ownership claim +
// canvas start), flips Enabled to true, and emits a StreamStateChangedEvent.
func TestService_EngageCanvas_FlipsEnabledAndCallsStart(t *testing.T) {
	store := &mockStore{streams: map[string]StreamSpec{
		"src1": {ID: "src1", FFmpeg: FFmpegConfig{Codec: "h264"}},
		"cv1": {
			ID: "cv1",
			Canvas: &CanvasConfig{
				Width: 1920, Height: 1080, FPS: "30",
				SourceStreams: []string{"src1"},
			},
		},
	}}
	svc := newTestService(t, store)
	svc.streams["cv1"] = &Stream{ID: "cv1", Enabled: false}
	pm := &stubProcMgr{}
	svc.processManager = pm
	bus := events.New()
	svc.eventBus = bus

	var (
		mu    sync.Mutex
		gotEv *events.StreamStateChangedEvent
	)
	unsub := bus.Subscribe(func(ev events.StreamStateChangedEvent) {
		mu.Lock()
		defer mu.Unlock()
		gotEv = &ev
	})
	defer unsub()

	if err := svc.EngageCanvas(context.Background(), "cv1"); err != nil {
		t.Fatalf("EngageCanvas: %v", err)
	}

	if len(pm.startCalls) != 1 || pm.startCalls[0] != "cv1" {
		t.Errorf("expected process manager Start(cv1), got %v", pm.startCalls)
	}
	if !svc.streams["cv1"].Enabled {
		t.Errorf("canvas Enabled should be true after engage")
	}
	mu.Lock()
	defer mu.Unlock()
	if gotEv == nil || gotEv.StreamID != "cv1" || !gotEv.Enabled {
		t.Errorf("expected StreamStateChangedEvent{cv1, true}, got %+v", gotEv)
	}
}

// TestService_EngageCanvas_IdempotentWhenAlreadyOwned verifies that calling
// EngageCanvas on an already-engaged canvas is a no-op: the process manager
// is not called again. The Enabled flag is still confirmed true.
func TestService_EngageCanvas_IdempotentWhenAlreadyOwned(t *testing.T) {
	store := &mockStore{streams: map[string]StreamSpec{
		"src1": {ID: "src1", FFmpeg: FFmpegConfig{Codec: "h264"}},
		"cv1": {
			ID: "cv1",
			Canvas: &CanvasConfig{
				Width: 1920, Height: 1080, FPS: "30",
				SourceStreams: []string{"src1"},
			},
		},
	}}
	svc := newTestService(t, store)
	svc.streams["cv1"] = &Stream{ID: "cv1", Enabled: true}
	pm := &stubProcMgr{
		running: map[string]bool{"cv1": true},
		ownedBy: map[string]string{"src1": "cv1"},
	}
	svc.processManager = pm

	if err := svc.EngageCanvas(context.Background(), "cv1"); err != nil {
		t.Fatalf("EngageCanvas (idempotent): %v", err)
	}
	if len(pm.startCalls) != 0 {
		t.Errorf("expected no Start calls on idempotent engage, got %v", pm.startCalls)
	}
	if !svc.streams["cv1"].Enabled {
		t.Errorf("Enabled should remain true after idempotent engage")
	}
}

// TestService_ReleaseCanvas_RejectsNonCanvas ensures the operation rejects
// non-canvas streams with an InvalidParams error.
func TestService_ReleaseCanvas_RejectsNonCanvas(t *testing.T) {
	store := &mockStore{streams: map[string]StreamSpec{
		"src1": {ID: "src1", FFmpeg: FFmpegConfig{Codec: "h264"}},
	}}
	svc := newTestService(t, store)
	svc.streams["src1"] = &Stream{ID: "src1", Enabled: true}

	err := svc.ReleaseCanvas(context.Background(), "src1")
	if err == nil {
		t.Fatalf("expected error releasing non-canvas, got nil")
	}
	var streamErr *StreamError
	if !errors.As(err, &streamErr) || streamErr.Code != ErrCodeInvalidParams {
		t.Errorf("expected ErrCodeInvalidParams, got %v", err)
	}
}
