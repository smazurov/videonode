package pipeline

import (
	"strings"
	"testing"
)

func TestSnapshot_EmitsAllStagesWithKinds(t *testing.T) {
	p := newTestPipeline(t, "/bin/true")
	must(t, p.Apply(Stream{
		ID: "canvas",
		Inputs: []InputRef{
			{ID: "i1", Device: "d1"},
			{ID: "i2", Device: "d2"},
		},
		Publish: []PublishTarget{{Type: "rtsp", URL: "rtsp://x/c"}},
	}))
	// Wait until all four expected pool entries have shown up Running.
	for _, want := range []string{
		"producer:d1", "producer:d2",
		"composer:canvas", "encoder:canvas",
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

	// Producer rows expose device + refcount + consumers.
	for _, dev := range []string{"d1", "d2"} {
		key := "producer:" + dev
		v := byID[key]
		if v.Kind != "producer" {
			t.Errorf("%s kind = %q, want producer", key, v.Kind)
		}
		if v.Device != dev {
			t.Errorf("%s device = %q, want %s", key, v.Device, dev)
		}
		if v.Refcount != 1 {
			t.Errorf("%s refcount = %d, want 1", key, v.Refcount)
		}
		if len(v.Consumers) != 1 || v.Consumers[0] != "canvas" {
			t.Errorf("%s consumers = %v, want [canvas]", key, v.Consumers)
		}
		if v.PID == 0 {
			t.Errorf("%s should have a PID", key)
		}
	}

	// Composer + encoder rows expose stream_id.
	for _, key := range []string{"composer:canvas", "encoder:canvas"} {
		v := byID[key]
		if v.StreamID != "canvas" {
			t.Errorf("%s stream_id = %q, want canvas", key, v.StreamID)
		}
		if v.PID == 0 {
			t.Errorf("%s should have a PID", key)
		}
	}

	// Sorted output.
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
