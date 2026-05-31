package services

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/smazurov/videonode/internal/api"
	"github.com/smazurov/videonode/internal/streams/pipeline"
)

// TestComposerService_ExportImport_RoundTrip exports a seeded composer and
// imports the bytes back, asserting the config survives the TOML round-trip.
func TestComposerService_ExportImport_RoundTrip(t *testing.T) {
	store := newStubStore()
	c := pipeline.Composer{
		ID:     "comp-1",
		Canvas: pipeline.CanvasDims{W: 1920, H: 1080, FPS: 60},
		Inputs: []pipeline.ComposerInput{{
			Ref:    "source:cam",
			Effect: &pipeline.Effect{Type: "perspective", Corners: [4][2]int{{0, 0}, {1920, 0}, {1920, 1080}, {0, 1080}}, SnapshotW: 1920, SnapshotH: 1080},
		}},
		Layout:    []pipeline.LayoutSlot{{Input: "source:cam", X: 0, Y: 0, W: 1920, H: 1080, Crop: &pipeline.CropConfig{X: 0.5, Y: 0.5, Scale: 1.0}}},
		CreatedAt: time.Unix(100, 0),
		UpdatedAt: time.Unix(100, 0),
	}
	if err := store.AddComposerEntity(c); err != nil {
		t.Fatalf("seed: %v", err)
	}
	svc := newComposerSvc(store, &stubComposerPipeline{})

	data, err := svc.ExportComposer(context.Background(), "comp-1")
	if err != nil {
		t.Fatalf("export: %v", err)
	}

	// Drop it, then re-import the exported bytes into a fresh store.
	store2 := newStubStore()
	svc2 := newComposerSvc(store2, &stubComposerPipeline{})
	out, created, err := svc2.ImportComposer(context.Background(), data)
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if !created {
		t.Errorf("created = false, want true for a new composer")
	}
	got, ok := store2.GetComposerEntity("comp-1")
	if !ok {
		t.Fatal("imported composer not in store")
	}
	if got.Canvas != c.Canvas {
		t.Errorf("canvas = %+v, want %+v", got.Canvas, c.Canvas)
	}
	if len(got.Inputs) != 1 || got.Inputs[0].Effect == nil || got.Inputs[0].Effect.Corners != c.Inputs[0].Effect.Corners {
		t.Errorf("inputs round-trip mismatch: %+v", got.Inputs)
	}
	if len(got.Layout) != 1 || got.Layout[0].Crop == nil || *got.Layout[0].Crop != *c.Layout[0].Crop {
		t.Errorf("layout round-trip mismatch: %+v", got.Layout)
	}
	if out.ID != "comp-1" {
		t.Errorf("returned id = %q, want comp-1", out.ID)
	}
}

// TestComposerService_Export_NotFound returns a typed not-found error.
func TestComposerService_Export_NotFound(t *testing.T) {
	svc := newComposerSvc(newStubStore(), &stubComposerPipeline{})
	_, err := svc.ExportComposer(context.Background(), "missing")
	ce := &api.ComposerError{}
	if !errors.As(err, &ce) || ce.Code != api.ComposerErrNotFound {
		t.Fatalf("err = %v, want ComposerErrNotFound", err)
	}
}

// TestComposerService_Import_OverwritesAndPreservesCreatedAt upserts onto an
// existing id: settings are replaced, CreatedAt is kept, created=false.
func TestComposerService_Import_OverwritesAndPreservesCreatedAt(t *testing.T) {
	store := newStubStore()
	orig := pipeline.Composer{
		ID:        "comp-1",
		Canvas:    pipeline.CanvasDims{W: 1280, H: 720},
		Inputs:    []pipeline.ComposerInput{{Ref: "source:old"}},
		CreatedAt: time.Unix(42, 0),
		UpdatedAt: time.Unix(42, 0),
	}
	if err := store.AddComposerEntity(orig); err != nil {
		t.Fatalf("seed: %v", err)
	}
	svc := newComposerSvc(store, &stubComposerPipeline{})

	doc := []byte(`id = "comp-1"
[canvas]
w = 1920
h = 1080
[[inputs]]
ref = "source:new"
`)
	_, created, err := svc.ImportComposer(context.Background(), doc)
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if created {
		t.Errorf("created = true, want false for an existing id")
	}
	got, _ := store.GetComposerEntity("comp-1")
	if got.Canvas.W != 1920 || got.Inputs[0].Ref != "source:new" {
		t.Errorf("overwrite did not apply: %+v", got)
	}
	if !got.CreatedAt.Equal(time.Unix(42, 0)) {
		t.Errorf("CreatedAt = %v, want preserved 42", got.CreatedAt)
	}
}

// TestComposerService_Import_Invalid rejects malformed or invalid documents
// with a typed invalid error and does not touch the store.
func TestComposerService_Import_Invalid(t *testing.T) {
	cases := map[string]string{
		"bad toml":      `id = "x" [canvas`,
		"empty id":      "[canvas]\nw = 1920\nh = 1080\n[[inputs]]\nref = \"source:cam\"\n",
		"bad id chars":  "id = \"has space\"\n[canvas]\nw = 1920\nh = 1080\n[[inputs]]\nref = \"source:cam\"\n",
		"zero canvas":   "id = \"c\"\n[canvas]\nw = 0\nh = 1080\n[[inputs]]\nref = \"source:cam\"\n",
		"no inputs":     "id = \"c\"\n[canvas]\nw = 1920\nh = 1080\n",
		"layout orphan": "id = \"c\"\n[canvas]\nw = 1920\nh = 1080\n[[inputs]]\nref = \"source:cam\"\n[[layout]]\ninput = \"source:ghost\"\nx = 0\ny = 0\nw = 1\nh = 1\n",
	}
	for name, doc := range cases {
		t.Run(name, func(t *testing.T) {
			store := newStubStore()
			svc := newComposerSvc(store, &stubComposerPipeline{})
			_, _, err := svc.ImportComposer(context.Background(), []byte(doc))
			ce := &api.ComposerError{}
			if !errors.As(err, &ce) || ce.Code != api.ComposerErrInvalid {
				t.Fatalf("err = %v, want ComposerErrInvalid", err)
			}
			if len(store.composers) != 0 {
				t.Errorf("store mutated on invalid import: %+v", store.composers)
			}
		})
	}
}

// TestComposerService_Import_PipelineRejectRollsBack ensures a pipeline
// rejection of a brand-new import leaves the store empty.
func TestComposerService_Import_PipelineRejectRollsBack(t *testing.T) {
	store := newStubStore()
	pipe := &stubComposerPipeline{applyErr: errors.New("daemon nope")}
	svc := newComposerSvc(store, pipe)
	doc := []byte("id = \"c\"\n[canvas]\nw = 1920\nh = 1080\n[[inputs]]\nref = \"source:cam\"\n")
	_, _, err := svc.ImportComposer(context.Background(), doc)
	ce := &api.ComposerError{}
	if !errors.As(err, &ce) || ce.Code != api.ComposerErrInvalid {
		t.Fatalf("err = %v, want ComposerErrInvalid", err)
	}
	if len(store.composers) != 0 {
		t.Errorf("store should be empty after rollback, got %+v", store.composers)
	}
}

// TestComposerService_ImportInto_ForcesPathIDOntoTarget overwrites an existing
// composer from a document carrying a *different* id, asserting the path id
// wins and the source-named composer is untouched (copy-paste between composers).
func TestComposerService_ImportInto_ForcesPathIDOntoTarget(t *testing.T) {
	store := newStubStore()
	target := pipeline.Composer{
		ID:        "dest",
		Canvas:    pipeline.CanvasDims{W: 1280, H: 720},
		Inputs:    []pipeline.ComposerInput{{Ref: "source:old"}},
		CreatedAt: time.Unix(7, 0),
		UpdatedAt: time.Unix(7, 0),
	}
	if err := store.AddComposerEntity(target); err != nil {
		t.Fatalf("seed: %v", err)
	}
	svc := newComposerSvc(store, &stubComposerPipeline{})

	doc := []byte("id = \"some-other-name\"\n[canvas]\nw = 1920\nh = 1080\n[[inputs]]\nref = \"source:new\"\n")
	out, err := svc.ImportComposerInto(context.Background(), "dest", doc)
	if err != nil {
		t.Fatalf("import into: %v", err)
	}
	if out.ID != "dest" {
		t.Errorf("returned id = %q, want dest", out.ID)
	}
	if _, ok := store.GetComposerEntity("some-other-name"); ok {
		t.Error("document id leaked a new composer; path id should win")
	}
	got, _ := store.GetComposerEntity("dest")
	if got.Canvas.W != 1920 || got.Inputs[0].Ref != "source:new" {
		t.Errorf("overwrite did not apply to target: %+v", got)
	}
	if !got.CreatedAt.Equal(time.Unix(7, 0)) {
		t.Errorf("CreatedAt = %v, want preserved 7", got.CreatedAt)
	}
}

// TestComposerService_ImportInto_MissingTarget rejects with not-found when the
// path id does not exist (import-into requires an existing composer).
func TestComposerService_ImportInto_MissingTarget(t *testing.T) {
	store := newStubStore()
	svc := newComposerSvc(store, &stubComposerPipeline{})
	doc := []byte("id = \"x\"\n[canvas]\nw = 1920\nh = 1080\n[[inputs]]\nref = \"source:cam\"\n")
	_, err := svc.ImportComposerInto(context.Background(), "ghost", doc)
	ce := &api.ComposerError{}
	if !errors.As(err, &ce) || ce.Code != api.ComposerErrNotFound {
		t.Fatalf("err = %v, want ComposerErrNotFound", err)
	}
	if len(store.composers) != 0 {
		t.Errorf("store mutated on missing-target import: %+v", store.composers)
	}
}
