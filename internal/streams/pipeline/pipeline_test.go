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
// supervised process stays alive long enough for assertions.
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

// waitRunning polls IsRunning for up to 1s, giving the pool's
// state-machine goroutine a chance to mark the process Running.
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

func mustApplySource(t *testing.T, p *Pipeline, s Source) {
	t.Helper()
	if err := p.ApplySource(s); err != nil {
		t.Fatalf("ApplySource %s: %v", s.ID, err)
	}
}

func mustApplyComposer(t *testing.T, p *Pipeline, c Composer) {
	t.Helper()
	if err := p.ApplyComposer(c); err != nil {
		t.Fatalf("ApplyComposer %s: %v", c.ID, err)
	}
}

func mustApplyStream(t *testing.T, p *Pipeline, s Stream) {
	t.Helper()
	if err := p.ApplyStream(s); err != nil {
		t.Fatalf("ApplyStream %s: %v", s.ID, err)
	}
}

func TestPipeline_ApplySource_RequiresID(t *testing.T) {
	p := newTestPipeline(t, "/dev/video0")
	if err := p.ApplySource(Source{Device: "x"}); err == nil ||
		!strings.Contains(err.Error(), "source.ID is required") {
		t.Errorf("expected source.ID required error, got %v", err)
	}
}

func TestPipeline_ApplySource_TestModeAndDeviceMutuallyExclusive(t *testing.T) {
	p := newTestPipeline(t, "/dev/video0")
	tests := []struct {
		name    string
		src     Source
		wantErr string
	}{
		{
			name:    "both set",
			src:     Source{ID: "s", Device: "x", TestMode: true},
			wantErr: "both Device and TestMode",
		},
		{
			name:    "neither set",
			src:     Source{ID: "s"},
			wantErr: "requires one of Device or TestMode",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := p.ApplySource(tt.src)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("got %v, want substring %q", err, tt.wantErr)
			}
		})
	}
}

func TestPipeline_ApplySource_DeviceMode(t *testing.T) {
	p := newTestPipeline(t, "/bin/true")
	mustApplySource(t, p, Source{ID: "hdmi0", Device: "hdmi-real"})
	if !waitRunning(p, "producer:hdmi0") {
		t.Fatal("source process should be running")
	}
	got, ok := p.Sources().Get("hdmi0")
	if !ok || got.Device != "hdmi-real" {
		t.Errorf("registry missing source: got=%+v ok=%v", got, ok)
	}
}

func TestPipeline_ApplySource_TestMode(t *testing.T) {
	// TestMode: no DeviceResolver needed; should spawn with --test-pattern
	// argv (verified via ProducerStage.Command unit test below).
	p := New(Config{
		VNSourceBin:   longRunningBin(t),
		VNComposerBin: longRunningBin(t),
		VNSinkBin:     longRunningBin(t),
		DRMDevice:     "/dev/dri/renderD128",
		// DeviceResolver intentionally nil — TestMode must not require it.
	}, nil)
	t.Cleanup(func() { p.Pool().StopAll() })
	if err := p.ApplySource(Source{ID: "test", TestMode: true}); err != nil {
		t.Fatalf("ApplySource test: %v", err)
	}
	if !waitRunning(p, "producer:test") {
		t.Fatal("test-pattern producer should be running")
	}
}

func TestPipeline_ApplySource_UnresolvedDeviceErrors(t *testing.T) {
	bin := longRunningBin(t)
	p := New(Config{
		VNSourceBin:    bin,
		VNComposerBin:  bin,
		VNSinkBin:      bin,
		DRMDevice:      "/dev/dri/renderD128",
		DeviceResolver: func(string) string { return "" },
	}, nil)
	t.Cleanup(func() { p.Pool().StopAll() })
	err := p.ApplySource(Source{ID: "s", Device: "usb-1-2"})
	if err == nil || !strings.Contains(err.Error(), "did not resolve to a path") {
		t.Errorf("expected unresolved-device error, got %v", err)
	}
}

func TestPipeline_ApplyStream_RequiresUpstream(t *testing.T) {
	p := newTestPipeline(t, "/bin/true")
	err := p.ApplyStream(Stream{ID: "x"})
	if err == nil || !strings.Contains(err.Error(), "requires Upstream") {
		t.Errorf("expected upstream-required error, got %v", err)
	}
}

func TestPipeline_ApplyStream_DanglingUpstreamErrors(t *testing.T) {
	p := newTestPipeline(t, "/bin/true")
	tests := []struct {
		name     string
		upstream string
		wantSub  string
	}{
		{"unknown source", "source:ghost", "upstream source"},
		{"unknown composer", "composer:ghost", "upstream composer"},
		{"bad format", "ghost", "invalid upstream"},
		{"unknown kind", "bogus:x", "unknown upstream kind"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := p.ApplyStream(Stream{
				ID:       "s",
				Upstream: tt.upstream,
				Publish:  []PublishTarget{{Type: "rtsp", URL: "rtsp://x/y"}},
			})
			if err == nil || !strings.Contains(err.Error(), tt.wantSub) {
				t.Errorf("got %v, want substring %q", err, tt.wantSub)
			}
		})
	}
}

func TestPipeline_ApplyStream_AgainstSource(t *testing.T) {
	p := newTestPipeline(t, "/bin/true")
	mustApplySource(t, p, Source{ID: "cam", Device: "cam"})
	mustApplyStream(t, p, Stream{
		ID:       "solo",
		Upstream: "source:cam",
		Publish:  []PublishTarget{{Type: "rtsp", URL: "rtsp://x/solo"}},
	})
	if !waitRunning(p, "encoder:solo") {
		t.Error("encoder should be running")
	}
	if !waitRunning(p, "producer:cam") {
		t.Error("source should remain running")
	}
}

func TestPipeline_ApplyStream_AgainstComposer(t *testing.T) {
	p := newTestPipeline(t, "/bin/true")
	mustApplySource(t, p, Source{ID: "cam", Device: "cam"})
	mustApplyComposer(t, p, Composer{
		ID:     "scene",
		Canvas: CanvasDims{W: 1280, H: 720},
		Inputs: []ComposerInput{{Ref: SourceIDFor("cam")}},
	})
	mustApplyStream(t, p, Stream{
		ID:       "out",
		Upstream: "composer:scene",
		Publish:  []PublishTarget{{Type: "rtsp", URL: "rtsp://x/out"}},
	})
	if !waitRunning(p, "encoder:out") {
		t.Error("encoder should be running")
	}
	if !waitRunning(p, "composer:scene") {
		t.Error("composer should be running")
	}
}

func TestPipeline_DeleteStreamLeavesUpstreamWarm(t *testing.T) {
	p := newTestPipeline(t, "/bin/true")
	mustApplySource(t, p, Source{ID: "cam", Device: "cam"})
	mustApplyStream(t, p, Stream{
		ID:       "s",
		Upstream: "source:cam",
		Publish:  []PublishTarget{{Type: "rtsp", URL: "rtsp://x/s"}},
	})
	if !waitRunning(p, "encoder:s") {
		t.Fatal("setup: encoder should be running")
	}
	if err := p.DeleteStream("s"); err != nil {
		t.Fatalf("DeleteStream: %v", err)
	}
	if p.Pool().IsRunning("encoder:s") {
		t.Error("encoder should be stopped post-Delete")
	}
	if !p.Pool().IsRunning("producer:cam") {
		t.Error("source should remain warm after stream delete")
	}
}

func TestPipeline_DeleteSource(t *testing.T) {
	p := newTestPipeline(t, "/bin/true")
	mustApplySource(t, p, Source{ID: "cam", Device: "cam"})
	if !waitRunning(p, "producer:cam") {
		t.Fatal("setup: source should be running")
	}
	if err := p.DeleteSource("cam"); err != nil {
		t.Fatalf("DeleteSource: %v", err)
	}
	if p.Pool().IsRunning("producer:cam") {
		t.Error("source should be stopped post-Delete")
	}
	if _, ok := p.Sources().Get("cam"); ok {
		t.Error("registry should not contain deleted source")
	}
}

func TestPipeline_DeleteComposer(t *testing.T) {
	p := newTestPipeline(t, "/bin/true")
	mustApplyComposer(t, p, Composer{ID: "scene", Canvas: CanvasDims{W: 1920, H: 1080}})
	if !waitRunning(p, "composer:scene") {
		t.Fatal("setup: composer should be running")
	}
	if err := p.DeleteComposer("scene"); err != nil {
		t.Fatalf("DeleteComposer: %v", err)
	}
	if p.Pool().IsRunning("composer:scene") {
		t.Error("composer should be stopped post-Delete")
	}
}

func TestPipeline_DeleteUnknownIsNoop(t *testing.T) {
	p := newTestPipeline(t, "/bin/true")
	if err := p.DeleteSource("nobody"); err != nil {
		t.Errorf("DeleteSource unknown: %v", err)
	}
	if err := p.DeleteComposer("nobody"); err != nil {
		t.Errorf("DeleteComposer unknown: %v", err)
	}
	if err := p.DeleteStream("nobody"); err != nil {
		t.Errorf("DeleteStream unknown: %v", err)
	}
}

func TestPipeline_EnsureUdsDirCreated(t *testing.T) {
	p := newTestPipeline(t, "/bin/true")
	mustApplySource(t, p, Source{ID: "x", Device: "d"})
	if _, err := os.Stat(NativeUdsDir); err != nil {
		t.Errorf("UDS dir %s not created: %v", NativeUdsDir, err)
	}
}
