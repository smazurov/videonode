package pipeline

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// longRunningBin writes a tiny shell script that ignores its argv and
// sleeps for an hour. Use this as the fake binary path so the pool's
// supervised process stays alive long enough for assertions; t.Cleanup
// drains the pool to kill them at test end.
func longRunningBin(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "fake-bin.sh")
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexec sleep 3600\n"), 0o755); err != nil {
		t.Fatalf("write fake-bin: %v", err)
	}
	return path
}

func newTestPipeline(t *testing.T, resolveTo string) *Pipeline {
	t.Helper()
	bin := longRunningBin(t)
	p := New(Config{
		VNSourceBin:   bin,
		VNComposerBin: bin,
		VNSinkBin:     bin,
		DRMDevice:     "/dev/dri/renderD128",
		DeviceResolver: func(id string) string {
			if id == "" {
				return ""
			}
			return resolveTo
		},
	}, nil)
	t.Cleanup(func() { p.Pool().StopAll() })
	return p
}

// waitRunning polls IsRunning for up to 1 s, giving the pool's
// state-machine goroutine a chance to mark the process Running.
// Required because Pool.Start returns immediately after the spawn
// goroutine begins; IsRunning lags by a few µs.
func waitRunning(p *Pipeline, id string) bool {
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if p.Pool().IsRunning(id) {
			return true
		}
		time.Sleep(2 * time.Millisecond)
	}
	return false
}

// expectStopped polls IsRunning with a short timeout, returning true
// when the process is no longer running. Used for "should be gone"
// assertions where pool.Stop is synchronous but Pool's state-machine
// goroutine may still be in the process of writing State=Idle.
func expectStopped(p *Pipeline, id string) bool {
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if !p.Pool().IsRunning(id) {
			return true
		}
		time.Sleep(2 * time.Millisecond)
	}
	return false
}

func TestPipeline_Apply_RequiresStreamID(t *testing.T) {
	p := newTestPipeline(t, "/dev/video0")
	err := p.Apply(Stream{})
	if err == nil {
		t.Fatal("expected error for missing stream ID")
	}
	if !strings.Contains(err.Error(), "stream.ID is required") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestPipeline_Apply_FailsOnUnresolvedDevice(t *testing.T) {
	bin := longRunningBin(t)
	p := New(Config{
		VNSourceBin:    bin,
		VNComposerBin:  bin,
		VNSinkBin:      bin,
		DRMDevice:      "/dev/dri/renderD128",
		DeviceResolver: func(string) string { return "" }, // never resolves
	}, nil)
	t.Cleanup(func() { p.Pool().StopAll() })
	err := p.Apply(Stream{
		ID:      "cam",
		Inputs:  []InputRef{{ID: "inp1", Device: "usb-1-2"}},
		Publish: []PublishTarget{{Type: "rtsp", URL: "rtsp://x/y"}},
	})
	if err == nil || !strings.Contains(err.Error(), "did not resolve to a path") {
		t.Errorf("expected unresolved-device error, got %v", err)
	}
}

func TestPipeline_NeedsComposerPickerDecidesTopology(t *testing.T) {
	p := newTestPipeline(t, "/bin/true")
	// Solo source, no effects → no composer expected
	if err := p.Apply(Stream{
		ID:      "solo",
		Inputs:  []InputRef{{ID: "a", Device: "dev-a"}},
		Publish: []PublishTarget{{Type: "rtsp", URL: "rtsp://x/solo"}},
	}); err != nil {
		t.Fatalf("solo Apply: %v", err)
	}
	if p.Pool().IsRunning(ComposerPoolKey("solo")) {
		t.Error("composer should NOT be running for solo+no-effects stream")
	}
	if !waitRunning(p, "encoder:solo") {
		t.Error("encoder for solo should be running")
	}
	if !waitRunning(p, "producer:dev-a") {
		t.Error("producer for dev-a should be running")
	}

	// Two-input stream → composer expected
	if err := p.Apply(Stream{
		ID: "two",
		Inputs: []InputRef{
			{ID: "a", Device: "dev-a"},
			{ID: "b", Device: "dev-b"},
		},
		Publish: []PublishTarget{{Type: "rtsp", URL: "rtsp://x/two"}},
	}); err != nil {
		t.Fatalf("two Apply: %v", err)
	}
	if !waitRunning(p, ComposerPoolKey("two")) {
		t.Error("composer should be running for N=2 stream")
	}
}

func TestPipeline_ProducerSharingAcrossStreams(t *testing.T) {
	p := newTestPipeline(t, "/bin/true")
	must(t, p.Apply(Stream{
		ID:      "a",
		Inputs:  []InputRef{{ID: "i", Device: "shared"}},
		Publish: []PublishTarget{{Type: "rtsp", URL: "rtsp://x/a"}},
	}))
	must(t, p.Apply(Stream{
		ID:      "b",
		Inputs:  []InputRef{{ID: "i", Device: "shared"}},
		Publish: []PublishTarget{{Type: "rtsp", URL: "rtsp://x/b"}},
	}))
	if p.Producers().Refcount("shared") != 2 {
		t.Errorf("shared device refcount = %d, want 2", p.Producers().Refcount("shared"))
	}
	if !waitRunning(p, "producer:shared") {
		t.Error("shared producer should be up")
	}
	// Delete stream a — producer stays up for stream b.
	must(t, p.Delete("a"))
	if !waitRunning(p, "producer:shared") {
		t.Error("shared producer should stay running for stream b")
	}
	if p.Producers().Refcount("shared") != 1 {
		t.Errorf("refcount after a delete = %d, want 1", p.Producers().Refcount("shared"))
	}
	// Delete stream b — producer stops.
	must(t, p.Delete("b"))
	if p.Pool().IsRunning("producer:shared") {
		t.Error("shared producer should be stopped after last consumer")
	}
	if p.Producers().Refcount("shared") != 0 {
		t.Errorf("refcount after b delete = %d, want 0", p.Producers().Refcount("shared"))
	}
}

func TestPipeline_DeleteStopsComposerAndEncoder(t *testing.T) {
	p := newTestPipeline(t, "/bin/true")
	must(t, p.Apply(Stream{
		ID: "canvas",
		Inputs: []InputRef{
			{ID: "i1", Device: "d1"},
			{ID: "i2", Device: "d2"},
		},
		Publish: []PublishTarget{{Type: "rtsp", URL: "rtsp://x/c"}},
	}))
	if !waitRunning(p, "composer:canvas") {
		t.Fatal("setup: composer should be running")
	}
	if !waitRunning(p, "encoder:canvas") {
		t.Fatal("setup: encoder should be running")
	}
	must(t, p.Delete("canvas"))
	if p.Pool().IsRunning("composer:canvas") {
		t.Error("composer should be stopped post-Delete")
	}
	if waitRunning(p, "encoder:canvas") {
		t.Error("encoder should be stopped post-Delete")
	}
}

func TestPipeline_DeleteUnknownStreamIsNoop(t *testing.T) {
	p := newTestPipeline(t, "/bin/true")
	if err := p.Delete("nobody-home"); err != nil {
		t.Errorf("Delete unknown stream returned %v, want nil", err)
	}
}

func TestPipeline_EnsureUdsDirCreated(t *testing.T) {
	p := newTestPipeline(t, "/bin/true")
	must(t, p.Apply(Stream{
		ID:      "x",
		Inputs:  []InputRef{{ID: "i", Device: "d"}},
		Publish: []PublishTarget{{Type: "rtsp", URL: "rtsp://x/y"}},
	}))
	if _, err := os.Stat(NativeUdsDir); err != nil {
		t.Errorf("UDS dir %s not created: %v", NativeUdsDir, err)
	}
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
