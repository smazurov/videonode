package pipeline

import (
	"strings"
	"testing"
)

func TestEncoderIDFor(t *testing.T) {
	tests := []struct {
		name     string
		streamID string
		want     string
	}{
		{"simple", "main", "encoder:main"},
		{"kebab", "host-solo", "encoder:host-solo"},
		{"empty", "", "encoder:"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := EncoderIDFor(tt.streamID); got != tt.want {
				t.Errorf("EncoderIDFor(%q) = %q, want %q", tt.streamID, got, tt.want)
			}
		})
	}
}

func TestPipeline_StopEncoder_RequiresStreamID(t *testing.T) {
	p := newTestPipeline(t, "/bin/true")
	if err := p.StopEncoder(""); err == nil {
		t.Fatal("expected error for empty streamID")
	}
}

func TestPipeline_EnsureEncoder_RequiresStreamID(t *testing.T) {
	p := newTestPipeline(t, "/bin/true")
	if err := p.EnsureEncoder(""); err == nil {
		t.Fatal("expected error for empty streamID")
	}
}

// TestPipeline_StopEncoder_NoopWhenNotRunning covers two cases:
// (1) totally unknown stream, (2) stream that's been Applied then
// stopped — second StopEncoder must return nil.
func TestPipeline_StopEncoder_NoopWhenNotRunning(t *testing.T) {
	p := newTestPipeline(t, "/bin/true")
	if err := p.StopEncoder("never-applied"); err != nil {
		t.Errorf("StopEncoder on unknown stream = %v, want nil", err)
	}

	must(t, p.Apply(Stream{
		ID:      "solo",
		Inputs:  []InputRef{{ID: "i", Device: "dev-a"}},
		Publish: []PublishTarget{{Type: "rtsp", URL: "rtsp://x/solo"}},
	}))
	if !waitRunning(p, EncoderIDFor("solo")) {
		t.Fatal("setup: encoder should be up after Apply")
	}
	if err := p.StopEncoder("solo"); err != nil {
		t.Fatalf("first StopEncoder: %v", err)
	}
	if !expectStopped(p, EncoderIDFor("solo")) {
		t.Fatal("encoder should be stopped after StopEncoder")
	}
	// Second call is a no-op.
	if err := p.StopEncoder("solo"); err != nil {
		t.Errorf("second StopEncoder = %v, want nil", err)
	}
}

func TestPipeline_EnsureEncoder_ErrorWhenNoCachedStage(t *testing.T) {
	p := newTestPipeline(t, "/bin/true")
	err := p.EnsureEncoder("never-applied")
	if err == nil {
		t.Fatal("expected error for stream with no cached stage")
	}
	if !strings.Contains(err.Error(), "no cached encoder stage") {
		t.Errorf("unexpected error message: %v", err)
	}
}

// TestPipeline_LazyEncoderCycle exercises the start → stop → ensure →
// stop sequence the streaming server relies on for the "encoder idles
// when no readers" behavior.
func TestPipeline_LazyEncoderCycle(t *testing.T) {
	p := newTestPipeline(t, "/bin/true")
	must(t, p.Apply(Stream{
		ID:      "cycle",
		Inputs:  []InputRef{{ID: "i", Device: "dev-c"}},
		Publish: []PublishTarget{{Type: "rtsp", URL: "rtsp://x/cycle"}},
	}))
	encID := EncoderIDFor("cycle")
	if !waitRunning(p, encID) {
		t.Fatal("encoder should be up after Apply")
	}

	// Stop it.
	must(t, p.StopEncoder("cycle"))
	if !expectStopped(p, encID) {
		t.Fatal("encoder should be stopped after StopEncoder")
	}

	// EnsureEncoder on a stopped-but-cached stage brings it back.
	must(t, p.EnsureEncoder("cycle"))
	if !waitRunning(p, encID) {
		t.Fatal("encoder should be up after EnsureEncoder")
	}

	// EnsureEncoder while running is a no-op.
	if err := p.EnsureEncoder("cycle"); err != nil {
		t.Errorf("EnsureEncoder while running = %v, want nil", err)
	}
	if !p.Pool().IsRunning(encID) {
		t.Error("encoder should still be running after no-op EnsureEncoder")
	}
}
