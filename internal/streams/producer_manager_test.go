package streams

import (
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/smazurov/videonode/internal/process"
)

// shortenSocketReadyForTests must be called at the top of every test in
// this file so the post-Start socket-ready wait doesn't add 3 s per Acquire
// (no real sidecar is launched in unit tests). Package-level init is
// disallowed by lint; t.Cleanup is the alternative.
func shortenSocketReadyForTests(t *testing.T) {
	t.Helper()
	prev := socketReadyTimeout
	SetSocketReadyTimeout(10 * time.Millisecond)
	t.Cleanup(func() { SetSocketReadyTimeout(prev) })
}

// recPool implements process.Pool but only records Start/Stop calls. Unlike
// mockPool in process_manager_test.go, this one ignores the CommandProvider —
// producer-manager tests don't exercise command generation through the pool.
type recPool struct {
	mu         sync.Mutex
	startCalls []string
	stopCalls  []string
}

func (p *recPool) Start(id string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.startCalls = append(p.startCalls, id)
	return nil
}

func (p *recPool) Stop(id string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.stopCalls = append(p.stopCalls, id)
	return nil
}

func (p *recPool) Restart(_ string) error            { return nil }
func (p *recPool) IsRunning(_ string) bool           { return false }
func (p *recPool) GetStatus(id string) *process.Info { return &process.Info{ID: id} }
func (p *recPool) SetKind(_, _ string)               {}
func (p *recPool) IDs() []string                     { return nil }
func (p *recPool) StopAll()                          {}

func sampleSpec(deviceID, path string) ProducerSpec {
	return ProducerSpec{
		DeviceID:   deviceID,
		DevicePath: path,
		BinaryPath: "/usr/bin/echo", // exists on host; never actually exec'd by recPool
	}
}

func TestProducerManager_AcquireStartsFirstThenRefcounts(t *testing.T) {
	shortenSocketReadyForTests(t)
	pool := &recPool{}
	pm := NewProducerManager(pool)

	h1, err := pm.Acquire(sampleSpec("dev1", "/dev/video0"))
	if err != nil {
		t.Fatalf("first Acquire: %v", err)
	}
	if h1.SocketPath == "" {
		t.Fatal("expected non-empty SocketPath")
	}

	h2, err := pm.Acquire(sampleSpec("dev1", "/dev/video0"))
	if err != nil {
		t.Fatalf("second Acquire: %v", err)
	}
	if h2.SocketPath != h1.SocketPath {
		t.Errorf("second Acquire returned different socket: %q vs %q", h2.SocketPath, h1.SocketPath)
	}

	pool.mu.Lock()
	starts := append([]string{}, pool.startCalls...)
	pool.mu.Unlock()
	if len(starts) != 1 {
		t.Errorf("expected pool.Start called exactly once, got %d (%v)", len(starts), starts)
	}
	if starts[0] != ProducerProcessID("dev1") {
		t.Errorf("unexpected pool key: %q", starts[0])
	}
}

func TestProducerManager_ReleaseDecrementsAndStopsOnZero(t *testing.T) {
	shortenSocketReadyForTests(t)
	pool := &recPool{}
	pm := NewProducerManager(pool)

	if _, err := pm.Acquire(sampleSpec("dev1", "/dev/video0")); err != nil {
		t.Fatalf("acquire 1: %v", err)
	}
	if _, err := pm.Acquire(sampleSpec("dev1", "/dev/video0")); err != nil {
		t.Fatalf("acquire 2: %v", err)
	}

	pm.Release("dev1")
	pool.mu.Lock()
	stopsAfterFirst := len(pool.stopCalls)
	pool.mu.Unlock()
	if stopsAfterFirst != 0 {
		t.Errorf("expected no Stop after first Release (refcount still 1), got %d", stopsAfterFirst)
	}

	pm.Release("dev1")
	pool.mu.Lock()
	stops := append([]string{}, pool.stopCalls...)
	pool.mu.Unlock()
	if len(stops) != 1 || stops[0] != ProducerProcessID("dev1") {
		t.Errorf("expected single Stop(producer:dev1), got %v", stops)
	}

	if _, ok := pm.SocketPath("dev1"); ok {
		t.Error("expected SocketPath to return false after final Release")
	}
}

func TestProducerManager_ReleaseUnknownIsNoop(t *testing.T) {
	pool := &recPool{}
	pm := NewProducerManager(pool)
	pm.Release("never-acquired")
	pool.mu.Lock()
	defer pool.mu.Unlock()
	if len(pool.stopCalls) != 0 {
		t.Errorf("expected no pool.Stop for unknown device, got %v", pool.stopCalls)
	}
}

func TestProducerManager_ConcurrentAcquireStartsOnce(t *testing.T) {
	shortenSocketReadyForTests(t)
	pool := &recPool{}
	pm := NewProducerManager(pool)

	var wg sync.WaitGroup
	const N = 16
	wg.Add(N)
	for range N {
		go func() {
			defer wg.Done()
			_, _ = pm.Acquire(sampleSpec("dev1", "/dev/video0"))
		}()
	}
	wg.Wait()

	pool.mu.Lock()
	defer pool.mu.Unlock()
	if len(pool.startCalls) != 1 {
		t.Errorf("expected exactly one pool.Start across %d concurrent Acquires, got %d (%v)",
			N, len(pool.startCalls), pool.startCalls)
	}
}

func TestProducerManager_CommandRendersExec(t *testing.T) {
	shortenSocketReadyForTests(t)
	pool := &recPool{}
	pm := NewProducerManager(pool)

	if _, err := pm.Acquire(sampleSpec("dev1", "/dev/v4l/by-path/some-device-video-index0")); err != nil {
		t.Fatalf("acquire: %v", err)
	}
	cmd, err := pm.Command(ProducerProcessID("dev1"))
	if err != nil {
		t.Fatalf("Command: %v", err)
	}
	for _, want := range []string{"/usr/bin/echo", "--device", "/dev/v4l/by-path/some-device-video-index0", "--out-socket", "/tmp/vn-bus-dev1.sock"} {
		if !strings.Contains(cmd, want) {
			t.Errorf("Command missing %q\ngot: %s", want, cmd)
		}
	}
}

func TestProducerManager_CommandRejectsNonProducerKey(t *testing.T) {
	pool := &recPool{}
	pm := NewProducerManager(pool)
	if _, err := pm.Command("some-canvas"); err == nil {
		t.Error("expected error for non-producer key, got nil")
	}
}

func TestProducerManager_SanitizesSocketName(t *testing.T) {
	got := SocketPathFor("usb-1.2-port/3")
	want := "/tmp/vn-bus-usb-1_2-port_3.sock"
	if got != want {
		t.Errorf("SocketPathFor: got %q, want %q", got, want)
	}
}

func TestProducerManager_IsProducerKey(t *testing.T) {
	if !IsProducerKey("producer:dev1") {
		t.Error("expected IsProducerKey to return true for producer:dev1")
	}
	if IsProducerKey("dev1") {
		t.Error("expected IsProducerKey to return false for bare id")
	}
	if IsProducerKey("producer:") {
		t.Error("expected IsProducerKey to return false for bare prefix")
	}
}
