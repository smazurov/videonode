//go:build planv2_tests

// Tests for the post-rewrite pipeline shape: Source / Composer / Stream
// are three independent entities reconciled by three Apply methods.
// Awaits B1 (Pipeline types + Apply* methods). Stubs in
// planv2_stubs_test.go let this file compile under the planv2_tests tag.
package pipeline

import (
	"strings"
	"testing"
)

// planRegistry is the test-time stand-in for the post-B1 Pipeline.
// Real Pipeline keeps a Source registry + Composer registry + Stream
// registry, each Apply method writes into its own table, and
// buildEncoder resolves Stream.Upstream against the first two. We
// mirror the same shape here so tests can pin the wiring semantics
// without depending on the still-in-flight Pipeline implementation.
type planRegistry struct {
	sources   map[string]PlanSource
	composers map[string]PlanComposer
	streams   map[string]PlanStream
}

func newPlanRegistry() *planRegistry {
	return &planRegistry{
		sources:   make(map[string]PlanSource),
		composers: make(map[string]PlanComposer),
		streams:   make(map[string]PlanStream),
	}
}

// ApplySource validates and stores a source. TestMode and Device are
// mutually exclusive; both empty is also illegal. Mirrors the
// validation contract in B1's Pipeline.ApplySource.
func (r *planRegistry) ApplySource(s PlanSource) error {
	if s.ID == "" {
		return errStub("source.ID required")
	}
	if s.TestMode && s.Device != "" {
		return errStub("source: TestMode and Device are mutually exclusive")
	}
	if !s.TestMode && s.Device == "" {
		return errStub("source: one of TestMode or Device required")
	}
	r.sources[s.ID] = s
	return nil
}

// ApplyComposer validates and stores a composer.
func (r *planRegistry) ApplyComposer(c PlanComposer) error {
	if c.ID == "" {
		return errStub("composer.ID required")
	}
	if c.Canvas.W <= 0 || c.Canvas.H <= 0 {
		return errStub("composer: canvas dims must be positive")
	}
	for _, in := range c.Inputs {
		kind, id, ok := ParseUpstreamRef(in.Ref)
		if !ok || kind != "source" {
			return errStub("composer input.ref must be source:<id>: " + in.Ref)
		}
		if _, exists := r.sources[id]; !exists {
			return errStub("composer references unknown source: " + id)
		}
	}
	// Layout entries reference inputs by Ref string, not slot index.
	for _, l := range c.Layout {
		found := false
		for _, in := range c.Inputs {
			if in.Ref == l.Input {
				found = true
				break
			}
		}
		if !found {
			return errStub("composer layout entry references unknown input: " + l.Input)
		}
	}
	r.composers[c.ID] = c
	return nil
}

// ApplyStream validates and stores a stream. Upstream must resolve
// against a known source or composer.
func (r *planRegistry) ApplyStream(s PlanStream) error {
	if s.ID == "" {
		return errStub("stream.ID required")
	}
	kind, id, ok := ParseUpstreamRef(s.Upstream)
	if !ok {
		return errStub("stream.Upstream malformed: " + s.Upstream)
	}
	switch kind {
	case "source":
		if _, found := r.sources[id]; !found {
			return errStub("stream upstream source not found: " + id)
		}
	case "composer":
		if _, found := r.composers[id]; !found {
			return errStub("stream upstream composer not found: " + id)
		}
	default:
		return errStub("stream.Upstream kind must be source|composer: " + kind)
	}
	r.streams[s.ID] = s
	return nil
}

type stubError string

func (e stubError) Error() string { return string(e) }
func errStub(s string) error      { return stubError(s) }

func TestParseUpstreamRef_ValidAndInvalidShapes(t *testing.T) {
	tests := []struct {
		name     string
		in       string
		wantKind string
		wantID   string
		wantOK   bool
	}{
		{"source ref", "source:hdmi0", "source", "hdmi0", true},
		{"composer ref", "composer:main-scene", "composer", "main-scene", true},
		{"empty string", "", "", "", false},
		{"no colon", "source-hdmi0", "", "", false},
		{"colon only", ":", "", "", true}, // technically parseable, semantic rejection later
		{"trailing colon", "source:", "source", "", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			kind, id, ok := ParseUpstreamRef(tc.in)
			if kind != tc.wantKind || id != tc.wantID || ok != tc.wantOK {
				t.Errorf("ParseUpstreamRef(%q) = (%q,%q,%v), want (%q,%q,%v)",
					tc.in, kind, id, ok, tc.wantKind, tc.wantID, tc.wantOK)
			}
		})
	}
}

func TestApplySource_Validates(t *testing.T) {
	tests := []struct {
		name    string
		src     PlanSource
		wantErr string
	}{
		{"empty id", PlanSource{Device: "/dev/video0"}, "ID required"},
		{"both device and test_mode", PlanSource{ID: "x", Device: "/dev/video0", TestMode: true}, "mutually exclusive"},
		{"neither device nor test_mode", PlanSource{ID: "x"}, "one of"},
		{"valid device", PlanSource{ID: "x", Device: "/dev/video0"}, ""},
		{"valid test_mode", PlanSource{ID: "x", TestMode: true}, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := newPlanRegistry()
			err := r.ApplySource(tc.src)
			if tc.wantErr == "" {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("want error containing %q, got %v", tc.wantErr, err)
			}
		})
	}
}

func TestApplyComposer_ValidatesInputsAndLayout(t *testing.T) {
	r := newPlanRegistry()
	mustApplySource(t, r, PlanSource{ID: "hdmi0", Device: "/dev/video0"})
	mustApplySource(t, r, PlanSource{ID: "cam-host", Device: "/dev/video1"})

	tests := []struct {
		name    string
		c       PlanComposer
		wantErr string
	}{
		{
			name: "valid two-input composer",
			c: PlanComposer{
				ID:     "main",
				Canvas: PlanCanvasDims{W: 1920, H: 1080},
				Inputs: []PlanComposerInput{
					{Ref: "source:hdmi0"},
					{Ref: "source:cam-host"},
				},
				Layout: []PlanLayoutSlot{
					{Input: "source:hdmi0", X: 0, Y: 0, W: 1920, H: 1080},
					{Input: "source:cam-host", X: 20, Y: 740, W: 320, H: 180},
				},
			},
		},
		{
			name: "empty id",
			c: PlanComposer{
				Canvas: PlanCanvasDims{W: 1920, H: 1080},
				Inputs: []PlanComposerInput{{Ref: "source:hdmi0"}},
			},
			wantErr: "ID required",
		},
		{
			name: "zero canvas dims",
			c: PlanComposer{
				ID:     "broken",
				Canvas: PlanCanvasDims{},
				Inputs: []PlanComposerInput{{Ref: "source:hdmi0"}},
			},
			wantErr: "canvas dims must be positive",
		},
		{
			name: "input ref points at composer (illegal)",
			c: PlanComposer{
				ID:     "broken",
				Canvas: PlanCanvasDims{W: 1920, H: 1080},
				Inputs: []PlanComposerInput{{Ref: "composer:other"}},
			},
			wantErr: "must be source:",
		},
		{
			name: "input ref points at unknown source",
			c: PlanComposer{
				ID:     "broken",
				Canvas: PlanCanvasDims{W: 1920, H: 1080},
				Inputs: []PlanComposerInput{{Ref: "source:nobody"}},
			},
			wantErr: "unknown source",
		},
		{
			name: "layout references unknown input",
			c: PlanComposer{
				ID:     "broken",
				Canvas: PlanCanvasDims{W: 1920, H: 1080},
				Inputs: []PlanComposerInput{{Ref: "source:hdmi0"}},
				Layout: []PlanLayoutSlot{{Input: "source:cam-host", X: 0, Y: 0, W: 1, H: 1}},
			},
			wantErr: "unknown input",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := r.ApplyComposer(tc.c)
			if tc.wantErr == "" {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("want error containing %q, got %v", tc.wantErr, err)
			}
		})
	}
}

func TestApplyStream_UpstreamResolution(t *testing.T) {
	r := newPlanRegistry()
	mustApplySource(t, r, PlanSource{ID: "hdmi0", Device: "/dev/video0"})
	mustApplySource(t, r, PlanSource{ID: "cam-host", Device: "/dev/video1"})
	mustApplyComposer(t, r, PlanComposer{
		ID:     "main",
		Canvas: PlanCanvasDims{W: 1920, H: 1080},
		Inputs: []PlanComposerInput{{Ref: "source:hdmi0"}, {Ref: "source:cam-host"}},
	})

	tests := []struct {
		name    string
		s       PlanStream
		wantErr string
	}{
		{
			"resolves to source",
			PlanStream{ID: "solo", Upstream: "source:cam-host"},
			"",
		},
		{
			"resolves to composer",
			PlanStream{ID: "archive", Upstream: "composer:main"},
			"",
		},
		{
			"empty id",
			PlanStream{Upstream: "source:hdmi0"},
			"ID required",
		},
		{
			"malformed upstream",
			PlanStream{ID: "x", Upstream: "no-colon-here"},
			"malformed",
		},
		{
			"unknown kind",
			PlanStream{ID: "x", Upstream: "device:hdmi0"},
			"kind must be",
		},
		{
			"dangling source ref",
			PlanStream{ID: "x", Upstream: "source:ghost"},
			"upstream source not found",
		},
		{
			"dangling composer ref",
			PlanStream{ID: "x", Upstream: "composer:ghost"},
			"upstream composer not found",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := r.ApplyStream(tc.s)
			if tc.wantErr == "" {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("want error containing %q, got %v", tc.wantErr, err)
			}
		})
	}
}

func TestApplyStream_MultipleStreamsShareUpstream(t *testing.T) {
	// Two streams pointing at the same composer is the headline win from
	// the rewrite — one GPU compose feeds N ffmpeg encodes. The registry
	// must permit this without complaint.
	r := newPlanRegistry()
	mustApplySource(t, r, PlanSource{ID: "hdmi0", Device: "/dev/video0"})
	mustApplyComposer(t, r, PlanComposer{
		ID:     "main",
		Canvas: PlanCanvasDims{W: 1920, H: 1080},
		Inputs: []PlanComposerInput{{Ref: "source:hdmi0"}},
	})
	for _, id := range []string{"archive", "low-latency"} {
		if err := r.ApplyStream(PlanStream{ID: id, Upstream: "composer:main"}); err != nil {
			t.Errorf("ApplyStream %s: %v", id, err)
		}
	}
	if len(r.streams) != 2 {
		t.Errorf("want 2 streams sharing composer:main, got %d", len(r.streams))
	}
}

func mustApplySource(t *testing.T, r *planRegistry, s PlanSource) {
	t.Helper()
	if err := r.ApplySource(s); err != nil {
		t.Fatalf("ApplySource %s: %v", s.ID, err)
	}
}

func mustApplyComposer(t *testing.T, r *planRegistry, c PlanComposer) {
	t.Helper()
	if err := r.ApplyComposer(c); err != nil {
		t.Fatalf("ApplyComposer %s: %v", c.ID, err)
	}
}
