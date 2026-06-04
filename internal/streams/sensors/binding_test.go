package sensors

import (
	"testing"

	"github.com/smazurov/videonode/internal/streams/pipeline"
)

type fakeComposerReader struct{ comps map[string]pipeline.Composer }

func (f *fakeComposerReader) GetComposerEntity(id string) (pipeline.Composer, bool) {
	c, ok := f.comps[id]
	return c, ok
}

func composerWithSensor(id, inputRef, sensorRef string) pipeline.Composer {
	return pipeline.Composer{
		ID: id,
		Inputs: []pipeline.ComposerInput{{
			Ref:    inputRef,
			Effect: &pipeline.Effect{Type: "auto_crop", AutoCrop: &pipeline.AutoCropEffect{Sensor: sensorRef}},
		}},
	}
}

func TestBindingAddsAndRemovesTarget(t *testing.T) {
	reader := &fakeComposerReader{comps: map[string]pipeline.Composer{
		"display": composerWithSensor("display", "source:cam", "sensor:playfield"),
	}}
	router := NewRouter(&fakeApplier{}, nil, nil, nil)
	br := NewBindingReconciler(reader, router, nil)

	br.ReconcileComposer("display")
	st := router.state["playfield"]
	if st == nil || len(st.targets) != 1 {
		t.Fatalf("expected one target bound, got %+v", st)
	}

	// Idempotent: re-reconcile keeps exactly one target.
	br.ReconcileComposer("display")
	if len(router.state["playfield"].targets) != 1 {
		t.Fatalf("re-reconcile duplicated target")
	}

	// Clear the effect → target removed.
	reader.comps["display"] = pipeline.Composer{
		ID:     "display",
		Inputs: []pipeline.ComposerInput{{Ref: "source:cam"}},
	}
	br.ReconcileComposer("display")
	if cleared := router.state["playfield"]; cleared != nil && len(cleared.targets) != 0 {
		t.Fatalf("target not removed: %+v", cleared.targets)
	}
}

func TestBindingComposerGoneRemovesTargets(t *testing.T) {
	reader := &fakeComposerReader{comps: map[string]pipeline.Composer{
		"display": composerWithSensor("display", "source:cam", "sensor:playfield"),
	}}
	router := NewRouter(&fakeApplier{}, nil, nil, nil)
	br := NewBindingReconciler(reader, router, nil)

	br.ReconcileComposer("display")
	delete(reader.comps, "display")
	br.ReconcileComposer("display")
	if st := router.state["playfield"]; st != nil && len(st.targets) != 0 {
		t.Fatalf("targets not removed when composer deleted: %+v", st.targets)
	}
}

func TestBindingIgnoresMalformedSensorRef(t *testing.T) {
	reader := &fakeComposerReader{comps: map[string]pipeline.Composer{
		"display": composerWithSensor("display", "source:cam", "playfield"), // missing sensor: prefix
	}}
	router := NewRouter(&fakeApplier{}, nil, nil, nil)
	br := NewBindingReconciler(reader, router, nil)

	br.ReconcileComposer("display")
	if len(router.state) != 0 {
		t.Fatalf("malformed sensor ref should bind nothing, got %+v", router.state)
	}
}
