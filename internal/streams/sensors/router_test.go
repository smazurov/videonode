package sensors

import "testing"

type fakeApplier struct {
	calls []Crop
}

func (f *fakeApplier) ApplyCrop(_, _ string, crop Crop) error {
	f.calls = append(f.calls, crop)
	return nil
}

func bbox(conf float64) Finding {
	return Finding{SensorID: "a1", Confidence: conf, Kind: "bbox", BBox: BBox{0.25, 0.25, 0.5, 0.5}}
}

func TestRouterAutoAppliesOnCommit(t *testing.T) {
	app := &fakeApplier{}
	r := NewRouter(app, nil, nil, nil)
	r.Configure("a1", newTestCommitter(), "auto")
	r.AddTarget("a1", "display", "source:cam")

	r.OnFinding(bbox(0.9)) // first confident → commit + apply
	if len(app.calls) != 1 {
		t.Fatalf("expected 1 apply, got %d", len(app.calls))
	}
	// Same crop again → committer reports no change → no extra apply.
	r.OnFinding(bbox(0.9))
	if len(app.calls) != 1 {
		t.Fatalf("steady-state re-applied: %d calls", len(app.calls))
	}
}

func TestRouterProposeDoesNotApply(t *testing.T) {
	app := &fakeApplier{}
	var proposed int
	r := NewRouter(app, func(_, _, _ string, _ Crop) { proposed++ }, nil, nil)
	r.Configure("a1", newTestCommitter(), "propose")
	r.AddTarget("a1", "display", "source:cam")

	r.OnFinding(bbox(0.9))
	if len(app.calls) != 0 {
		t.Fatalf("propose mode applied a crop: %d calls", len(app.calls))
	}
	if proposed != 1 {
		t.Fatalf("expected 1 proposal, got %d", proposed)
	}
}

// An unattached sensor (no targets) applies nothing but is still observable:
// every finding emits a FindingEvent so the UI can watch it work.
func TestRouterUnattachedStillObserves(t *testing.T) {
	app := &fakeApplier{}
	var events int
	r := NewRouter(app, nil, func(FindingEvent) { events++ }, nil)
	r.Configure("a1", newTestCommitter(), "auto")

	r.OnFinding(bbox(0.9))
	if len(app.calls) != 0 {
		t.Fatalf("unattached sensor applied a crop: %d calls", len(app.calls))
	}
	if events != 1 {
		t.Fatalf("expected 1 observe-only event, got %d", events)
	}
}

func TestRouterRemoveTargetStops(t *testing.T) {
	app := &fakeApplier{}
	r := NewRouter(app, nil, nil, nil)
	r.Configure("a1", newTestCommitter(), "auto")
	r.AddTarget("a1", "display", "source:cam")
	r.RemoveTarget("a1", "display", "source:cam")
	r.OnFinding(bbox(0.9))
	if len(app.calls) != 0 {
		t.Fatalf("applied after RemoveTarget")
	}
}

func TestRouterFanOutAppliesToAllTargets(t *testing.T) {
	app := &fakeApplier{}
	r := NewRouter(app, nil, nil, nil)
	r.Configure("a1", newTestCommitter(), "auto")
	r.AddTarget("a1", "display-a", "source:cam")
	r.AddTarget("a1", "display-b", "source:cam")

	r.OnFinding(bbox(0.9))
	if len(app.calls) != 2 {
		t.Fatalf("expected crop applied to both targets, got %d calls", len(app.calls))
	}
}
