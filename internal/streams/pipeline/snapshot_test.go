//go:build planv2_tests

// Post-rewrite snapshot test — uses new Source/Composer/Stream Apply API.
package pipeline

import (
	"strings"
	"testing"
)

func TestSnapshot_EmitsAllStagesWithKinds(t *testing.T) {
	p := newTestPipeline(t, "/bin/true")
	mustApplySource(t, p, Source{ID: "d1", Device: "d1"})
	mustApplySource(t, p, Source{ID: "d2", Device: "d2"})
	mustApplyComposer(t, p, Composer{
		ID:     "main",
		Canvas: CanvasDims{W: 1920, H: 1080},
		Inputs: []ComposerInput{
			{Ref: SourceIDFor("d1")},
			{Ref: SourceIDFor("d2")},
		},
	})
	mustApplyStream(t, p, Stream{
		ID:       "canvas",
		Upstream: "composer:main",
	})
	for _, want := range []string{
		"producer:d1", "producer:d2",
		"composer:main", "encoder:canvas",
	} {
		if !waitRunning(p, want) {
			t.Fatalf("setup: %s not running", want)
		}
	}

	views := p.Snapshot()
	if len(views) != 4 {
		t.Fatalf("Snapshot len = %d, want 4: %+v", len(views), views)
	}

	byID := map[string]ProcessView{}
	for _, v := range views {
		byID[v.ID] = v
	}

	for _, src := range []string{"d1", "d2"} {
		key := "producer:" + src
		v := byID[key]
		if v.Kind != "producer" {
			t.Errorf("%s kind = %q, want producer", key, v.Kind)
		}
		if v.SourceID != src {
			t.Errorf("%s source_id = %q, want %s", key, v.SourceID, src)
		}
		if v.PID == 0 {
			t.Errorf("%s should have a PID", key)
		}
	}

	enc := byID["encoder:canvas"]
	if enc.StreamID != "canvas" {
		t.Errorf("encoder stream_id = %q, want canvas", enc.StreamID)
	}
	if enc.PID == 0 {
		t.Errorf("encoder should have a PID")
	}

	for i := 1; i < len(views); i++ {
		if !sortedAscByID(views[i-1], views[i]) {
			t.Errorf("Snapshot not sorted by ID: %s before %s", views[i-1].ID, views[i].ID)
		}
	}
}

func sortedAscByID(a, b ProcessView) bool {
	return strings.Compare(a.ID, b.ID) <= 0
}

func TestSnapshot_EmptyPipelineReturnsEmptySlice(t *testing.T) {
	p := newTestPipeline(t, "/bin/true")
	if got := p.Snapshot(); len(got) != 0 {
		t.Errorf("Snapshot on empty pipeline = %v, want []", got)
	}
}
