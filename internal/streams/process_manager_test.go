package streams

import (
	"errors"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/smazurov/videonode/internal/logging"
	"github.com/smazurov/videonode/internal/process"
)

// mockPool records Start/Stop/Restart calls and otherwise no-ops. Satisfies
// the minimum surface the ownership-aware Restart path exercises. Tests can
// set per-id IsRunning state via runningIDs.
type mockPool struct {
	mu           sync.Mutex
	startCalls   []string
	stopCalls    []string
	restartCalls []string
	callOrder    []string // every Start/Stop/Restart in arrival order, prefixed with op
	runningIDs   map[string]bool
}

func (m *mockPool) Start(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.startCalls = append(m.startCalls, id)
	m.callOrder = append(m.callOrder, "start:"+id)
	return nil
}

func (m *mockPool) Stop(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stopCalls = append(m.stopCalls, id)
	m.callOrder = append(m.callOrder, "stop:"+id)
	return nil
}

func (m *mockPool) Restart(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.restartCalls = append(m.restartCalls, id)
	m.callOrder = append(m.callOrder, "restart:"+id)
	return nil
}
func (m *mockPool) GetStatus(_ string) *process.Info { return &process.Info{} }
func (m *mockPool) IsRunning(id string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.runningIDs[id]
}
func (m *mockPool) SetKind(_, _ string) {}
func (m *mockPool) IDs() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]string, 0, len(m.runningIDs))
	for id := range m.runningIDs {
		out = append(out, id)
	}
	return out
}
func (m *mockPool) StopAll() {}

func newTestManager(pool process.Pool) *streamProcessManager {
	return &streamProcessManager{
		pool:            pool,
		crashedStreams:  map[string]bool{},
		canvasOwnership: map[string]string{},
		visionPipes:     map[string]*visionPipe{},
		logger:          logging.GetLogger("test"),
	}
}

// TestRestartCanvas_ClaimsAddedAndReleasesRemovedSources covers the canvas
// update flow: when sources are added to or removed from a canvas, the
// process manager must stop the newly-added source's individual ffmpeg,
// claim ownership of new sources, release ownership of removed sources,
// and restart the canvas pool process.
func TestRestartCanvas_ClaimsAddedAndReleasesRemovedSources(t *testing.T) {
	store := &mockStore{streams: map[string]StreamSpec{
		"src1": {ID: "src1", FFmpeg: FFmpegConfig{Codec: "h264"}},
		"src2": {ID: "src2", FFmpeg: FFmpegConfig{Codec: "h264"}},
		"cv1": {
			ID: "cv1",
			Canvas: &CanvasConfig{
				Width: 1920, Height: 1080, FPS: "30",
				SourceStreams: []string{"src1"},
			},
		},
	}}
	pool := &mockPool{runningIDs: map[string]bool{"src1": true}}
	m := newTestManager(pool)
	m.store = store

	if err := m.Start("cv1"); err != nil {
		t.Fatalf("Start(cv1) failed: %v", err)
	}
	if m.canvasOwnership["src1"] != "cv1" {
		t.Errorf("after Start: want canvasOwnership[src1]=cv1, got %q", m.canvasOwnership["src1"])
	}
	if !slices.Contains(pool.stopCalls, "src1") {
		t.Errorf("after Start: expected pool.Stop(src1), got stopCalls=%v", pool.stopCalls)
	}
	if !slices.Contains(pool.startCalls, "cv1") {
		t.Errorf("after Start: expected pool.Start(cv1), got startCalls=%v", pool.startCalls)
	}

	store.streams["cv1"] = StreamSpec{
		ID: "cv1",
		Canvas: &CanvasConfig{
			Width: 1920, Height: 1080, FPS: "30",
			SourceStreams: []string{"src1", "src2"},
		},
	}
	pool.runningIDs["src2"] = true
	pool.stopCalls = nil
	pool.startCalls = nil

	if err := m.RestartCanvas("cv1"); err != nil {
		t.Fatalf("RestartCanvas(add src2) failed: %v", err)
	}
	if m.canvasOwnership["src2"] != "cv1" {
		t.Errorf("after add: want canvasOwnership[src2]=cv1, got %q", m.canvasOwnership["src2"])
	}
	if !slices.Contains(pool.stopCalls, "src2") {
		t.Errorf("after add: expected pool.Stop(src2), got stopCalls=%v", pool.stopCalls)
	}
	if !slices.Contains(pool.startCalls, "cv1") {
		t.Errorf("after add: expected pool.Start(cv1), got startCalls=%v", pool.startCalls)
	}

	store.streams["cv1"] = StreamSpec{
		ID: "cv1",
		Canvas: &CanvasConfig{
			Width: 1920, Height: 1080, FPS: "30",
			SourceStreams: []string{"src2"},
		},
	}

	pool.startCalls = nil

	if err := m.RestartCanvas("cv1"); err != nil {
		t.Fatalf("RestartCanvas(drop src1) failed: %v", err)
	}
	if _, owned := m.canvasOwnership["src1"]; owned {
		t.Errorf("after drop: src1 should no longer be owned, got canvasOwnership=%v", m.canvasOwnership)
	}
	if m.canvasOwnership["src2"] != "cv1" {
		t.Errorf("after drop: src2 should remain owned by cv1, got %q", m.canvasOwnership["src2"])
	}
	if !slices.Contains(pool.startCalls, "src1") {
		t.Errorf("after drop: expected pool.Start(src1) to relaunch released source, got startCalls=%v", pool.startCalls)
	}
}

// TestRestartCanvas_StopsCanvasBeforeStartingReleasedSource asserts the
// release-race fix: when a source is removed from a canvas, the canvas
// process must be stopped (releasing its v4l2 fds) before the released
// source's standalone ffmpeg is started. Otherwise the released source
// races the canvas's close, hits EBUSY, and gets stranded on the CRASH
// placeholder.
func TestRestartCanvas_StopsCanvasBeforeStartingReleasedSource(t *testing.T) {
	store := &mockStore{streams: map[string]StreamSpec{
		"src1": {ID: "src1", FFmpeg: FFmpegConfig{Codec: "h264"}},
		"cv1": {
			ID: "cv1",
			Canvas: &CanvasConfig{
				Width: 1920, Height: 1080, FPS: "30",
				SourceStreams: []string{}, // src1 just dropped
			},
		},
	}}
	pool := &mockPool{runningIDs: map[string]bool{"cv1": true}}
	m := newTestManager(pool)
	m.store = store
	m.canvasOwnership["src1"] = "cv1"

	if err := m.RestartCanvas("cv1"); err != nil {
		t.Fatalf("RestartCanvas: %v", err)
	}

	stopCanvasIdx := slices.Index(pool.callOrder, "stop:cv1")
	startSrcIdx := slices.Index(pool.callOrder, "start:src1")
	startCanvasIdx := slices.Index(pool.callOrder, "start:cv1")

	if stopCanvasIdx < 0 {
		t.Fatalf("expected stop:cv1 in call order, got %v", pool.callOrder)
	}
	if startSrcIdx < 0 {
		t.Fatalf("expected start:src1 (released source) in call order, got %v", pool.callOrder)
	}
	if startCanvasIdx < 0 {
		t.Fatalf("expected start:cv1 (canvas relaunch) in call order, got %v", pool.callOrder)
	}
	if stopCanvasIdx > startSrcIdx {
		t.Errorf("canvas must be stopped BEFORE released source is started; got order %v", pool.callOrder)
	}
	if startSrcIdx > startCanvasIdx {
		t.Errorf("released source must start before canvas relaunch (so device is free, then canvas reopens its remaining devices); got order %v", pool.callOrder)
	}
}

func TestRestart_OwnedStreamIsNoOp(t *testing.T) {
	pool := &mockPool{}
	m := newTestManager(pool)
	m.canvasOwnership["cam1"] = "canvas1"

	if err := m.Restart("cam1"); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if len(pool.restartCalls) != 0 {
		t.Errorf("expected no pool.Restart calls for owned stream, got %v", pool.restartCalls)
	}
}

func TestRestart_UnownedStreamCallsPool(t *testing.T) {
	pool := &mockPool{}
	m := newTestManager(pool)

	if err := m.Restart("cam1"); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if len(pool.restartCalls) != 1 || pool.restartCalls[0] != "cam1" {
		t.Errorf("expected pool.Restart(\"cam1\"), got %v", pool.restartCalls)
	}
}

func TestRestart_ClearsCrashedAndVisionPipesOnlyWhenNotOwned(t *testing.T) {
	pool := &mockPool{}
	m := newTestManager(pool)
	m.crashedStreams["cam1"] = true
	m.visionPipes["cam1"] = &visionPipe{ownerStreamID: "cam1"}

	// Owned: should not touch crashedStreams or visionPipes.
	m.canvasOwnership["cam1"] = "canvas1"
	_ = m.Restart("cam1")
	if !m.crashedStreams["cam1"] {
		t.Error("owned restart should not clear crashedStreams")
	}
	if _, ok := m.visionPipes["cam1"]; !ok {
		t.Error("owned restart should not clear visionPipes")
	}

	// Unowned: should clear both.
	delete(m.canvasOwnership, "cam1")
	_ = m.Restart("cam1")
	if m.crashedStreams["cam1"] {
		t.Error("unowned restart should clear crashedStreams")
	}
	if _, ok := m.visionPipes["cam1"]; ok {
		t.Error("unowned restart should clear visionPipes")
	}
}

// failingRestartPool returns an error from Restart so we can observe the
// state-error → restart-failure → stale-pipe scenario.
type failingRestartPool struct {
	mockPool
	restartErr error
}

func (m *failingRestartPool) Restart(id string) error {
	m.mu.Lock()
	m.restartCalls = append(m.restartCalls, id)
	m.callOrder = append(m.callOrder, "restart:"+id)
	m.mu.Unlock()
	return m.restartErr
}

// TestQuarantinedInputUsesPlaceholder asserts the canvas builder substitutes
// lavfi testsrc2 for a disabled input (e.g., the device-detector hotplug
// path flips InputsEnabled[src] to false on unplug). Without this the
// per-source NO SIGNAL placeholder logic in canvas_processor would not work.
func TestQuarantinedInputUsesPlaceholder(t *testing.T) {
	store := &mockStore{streams: map[string]StreamSpec{
		"src1": {
			ID:     "src1",
			Device: "usb-bad-camera-0001-video-index0",
			FFmpeg: FFmpegConfig{
				Codec:      "h264",
				Resolution: "1280x720",
				FPS:        "30",
			},
		},
		"src2": {
			ID:     "src2",
			Device: "usb-good-camera-0002-video-index0",
			FFmpeg: FFmpegConfig{
				Codec:      "h264",
				Resolution: "1920x1080",
				FPS:        "60",
			},
		},
		"cv1": {
			ID: "cv1",
			Canvas: &CanvasConfig{
				Width: 1920, Height: 1080, FPS: "60",
				SourceStreams: []string{"src1", "src2"},
			},
			FFmpeg: FFmpegConfig{Codec: "h264"},
		},
	}}

	cp := newCanvasProcessor(store)
	cp.deviceResolver = func(deviceID string) string {
		// Echo back as device path to make assertions easy.
		return "/dev/by-id/" + deviceID
	}
	cp.getStreamState = func(id string) (*Stream, bool) {
		if id == "cv1" {
			return &Stream{
				ID:            "cv1",
				Enabled:       true,
				InputsEnabled: map[string]bool{"src1": false, "src2": true},
			}, true
		}
		return nil, false
	}

	out, err := cp.processStream("cv1")
	if err != nil {
		t.Fatalf("processStream: %v", err)
	}
	if !strings.Contains(out.FFmpegCommand, "testsrc2") {
		t.Errorf("expected testsrc2 placeholder for disabled src1, got command:\n%s", out.FFmpegCommand)
	}
	if strings.Contains(out.FFmpegCommand, "/dev/by-id/usb-bad-camera-0001-video-index0") {
		t.Errorf("expected NOT to find src1's device path, got command:\n%s", out.FFmpegCommand)
	}
	if !strings.Contains(out.FFmpegCommand, "/dev/by-id/usb-good-camera-0002-video-index0") {
		t.Errorf("expected src2's device path to be present, got command:\n%s", out.FFmpegCommand)
	}
	if !strings.Contains(out.FFmpegCommand, "NO SIGNAL") {
		t.Errorf("expected NO SIGNAL drawtext for disabled src1, got command:\n%s", out.FFmpegCommand)
	}
}

// TestOnStateChange_StateErrorClearsStaleVisionPipe asserts that when the
// pool reports an unexpected exit and the async restart fails, the stale
// visionPipes entry pointing to the dead process's pipe is dropped — readers
// of CaptureRawSnapshot would otherwise block on a closed fd for any stream
// whose restart never recovers.
func TestOnStateChange_StateErrorClearsStaleVisionPipe(t *testing.T) {
	pool := &failingRestartPool{restartErr: errors.New("restart failed")}
	m := newTestManager(pool)
	m.visionPipes["cam1"] = &visionPipe{ownerStreamID: "cam1"}

	m.onStateChange("cam1", process.StateRunning, process.StateError, errors.New("ffmpeg died"))

	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		m.mu.Lock()
		_, present := m.visionPipes["cam1"]
		m.mu.Unlock()
		if !present {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Errorf("stale vision pipe entry still present after StateError + failed restart")
}

// TestReleaseCanvas_StopsCanvasAndStartsSourcesStandalone asserts the disband
// path: a running canvas is stopped, ownership is cleared for every source,
// and each previously-owned source is started as a standalone stream. The
// canvas spec is left in the store untouched.
func TestReleaseCanvas_StopsCanvasAndStartsSourcesStandalone(t *testing.T) {
	store := &mockStore{streams: map[string]StreamSpec{
		"src1": {ID: "src1", FFmpeg: FFmpegConfig{Codec: "h264"}},
		"src2": {ID: "src2", FFmpeg: FFmpegConfig{Codec: "h264"}},
		"src3": {ID: "src3", FFmpeg: FFmpegConfig{Codec: "h264"}},
		"cv1": {
			ID: "cv1",
			Canvas: &CanvasConfig{
				Width: 1920, Height: 1080, FPS: "30",
				SourceStreams: []string{"src1", "src2", "src3"},
			},
		},
	}}
	pool := &mockPool{runningIDs: map[string]bool{"cv1": true}}
	m := newTestManager(pool)
	m.store = store
	m.canvasOwnership["src1"] = "cv1"
	m.canvasOwnership["src2"] = "cv1"
	m.canvasOwnership["src3"] = "cv1"

	if err := m.ReleaseCanvas("cv1"); err != nil {
		t.Fatalf("ReleaseCanvas: %v", err)
	}

	for _, srcID := range []string{"src1", "src2", "src3"} {
		if owner, ok := m.canvasOwnership[srcID]; ok {
			t.Errorf("after release: %s still owned by %q", srcID, owner)
		}
	}
	if !slices.Contains(pool.stopCalls, "cv1") {
		t.Errorf("after release: expected pool.Stop(cv1), got stopCalls=%v", pool.stopCalls)
	}
	for _, srcID := range []string{"src1", "src2", "src3"} {
		if !slices.Contains(pool.startCalls, srcID) {
			t.Errorf("after release: expected pool.Start(%s), got startCalls=%v", srcID, pool.startCalls)
		}
	}
	if _, ok := store.streams["cv1"]; !ok {
		t.Errorf("canvas spec should remain in store after release")
	}
}

// TestReleaseCanvas_StopsCanvasBeforeStartingSources guards the v4l2 EBUSY race:
// the canvas must release its v4l2 fds (pool.Stop) before any source's
// standalone ffmpeg attempts to reopen the same device.
func TestReleaseCanvas_StopsCanvasBeforeStartingSources(t *testing.T) {
	store := &mockStore{streams: map[string]StreamSpec{
		"src1": {ID: "src1", FFmpeg: FFmpegConfig{Codec: "h264"}},
		"src2": {ID: "src2", FFmpeg: FFmpegConfig{Codec: "h264"}},
		"cv1": {
			ID: "cv1",
			Canvas: &CanvasConfig{
				Width: 1920, Height: 1080, FPS: "30",
				SourceStreams: []string{"src1", "src2"},
			},
		},
	}}
	pool := &mockPool{runningIDs: map[string]bool{"cv1": true}}
	m := newTestManager(pool)
	m.store = store
	m.canvasOwnership["src1"] = "cv1"
	m.canvasOwnership["src2"] = "cv1"

	if err := m.ReleaseCanvas("cv1"); err != nil {
		t.Fatalf("ReleaseCanvas: %v", err)
	}

	stopCanvasIdx := slices.Index(pool.callOrder, "stop:cv1")
	if stopCanvasIdx < 0 {
		t.Fatalf("expected stop:cv1 in call order, got %v", pool.callOrder)
	}
	for _, srcID := range []string{"src1", "src2"} {
		startIdx := slices.Index(pool.callOrder, "start:"+srcID)
		if startIdx < 0 {
			t.Fatalf("expected start:%s in call order, got %v", srcID, pool.callOrder)
		}
		if stopCanvasIdx > startIdx {
			t.Errorf("canvas Stop must precede source Start; got order %v", pool.callOrder)
		}
	}
}

// TestReleaseCanvas_SkipsMissingSources verifies that a source removed from
// the store between ownership claim and release does not cause an error or
// panic — it is silently skipped.
func TestReleaseCanvas_SkipsMissingSources(t *testing.T) {
	store := &mockStore{streams: map[string]StreamSpec{
		"src1": {ID: "src1", FFmpeg: FFmpegConfig{Codec: "h264"}},
		"cv1": {
			ID: "cv1",
			Canvas: &CanvasConfig{
				Width: 1920, Height: 1080, FPS: "30",
				SourceStreams: []string{"src1", "ghost"},
			},
		},
	}}
	pool := &mockPool{runningIDs: map[string]bool{"cv1": true}}
	m := newTestManager(pool)
	m.store = store
	m.canvasOwnership["src1"] = "cv1"
	m.canvasOwnership["ghost"] = "cv1"

	if err := m.ReleaseCanvas("cv1"); err != nil {
		t.Fatalf("ReleaseCanvas: %v", err)
	}

	if !slices.Contains(pool.startCalls, "src1") {
		t.Errorf("expected pool.Start(src1), got startCalls=%v", pool.startCalls)
	}
	if slices.Contains(pool.startCalls, "ghost") {
		t.Errorf("ghost source not in store, must not be started; got startCalls=%v", pool.startCalls)
	}
}

// TestReleaseCanvas_RejectsNonCanvas ensures the operation is canvas-only.
func TestReleaseCanvas_RejectsNonCanvas(t *testing.T) {
	store := &mockStore{streams: map[string]StreamSpec{
		"src1": {ID: "src1", FFmpeg: FFmpegConfig{Codec: "h264"}},
	}}
	pool := &mockPool{}
	m := newTestManager(pool)
	m.store = store

	if err := m.ReleaseCanvas("src1"); err == nil {
		t.Fatalf("ReleaseCanvas on non-canvas: expected error, got nil")
	}
}

// TestReleaseCanvas_IdempotentOnDormantCanvas asserts releasing an
// already-released (or never-started) canvas is a no-op without error.
func TestReleaseCanvas_IdempotentOnDormantCanvas(t *testing.T) {
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
	pool := &mockPool{}
	m := newTestManager(pool)
	m.store = store

	if err := m.ReleaseCanvas("cv1"); err != nil {
		t.Fatalf("first ReleaseCanvas: %v", err)
	}
	if err := m.ReleaseCanvas("cv1"); err != nil {
		t.Fatalf("second ReleaseCanvas (idempotent): %v", err)
	}
}
