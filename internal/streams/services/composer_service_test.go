package services

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/smazurov/videonode/internal/api"
	"github.com/smazurov/videonode/internal/api/models"
	"github.com/smazurov/videonode/internal/logging"
	"github.com/smazurov/videonode/internal/process"
	"github.com/smazurov/videonode/internal/streams"
	"github.com/smazurov/videonode/internal/streams/pipeline"
	"github.com/smazurov/videonode/internal/streams/pipelinectl"
)

// stubComposerPipeline is a manual mock of the composerPipeline seam. It
// records hot-apply attempts and returns the configured error so tests can
// drive the persist-then-best-effort logic without a real supervised
// process.
type stubComposerPipeline struct {
	layoutErr     error
	effectErr     error
	applyErr      error
	layoutCalls   int
	effectCalls   int
	registerCalls int
}

func (p *stubComposerPipeline) ApplyComposer(_ pipeline.Composer) error { return p.applyErr }
func (p *stubComposerPipeline) RegisterComposer(_ pipeline.Composer) error {
	p.registerCalls++
	return nil
}

func (p *stubComposerPipeline) UpdateComposerLayout(_ string, _ []pipeline.LayoutSlot) error {
	p.layoutCalls++
	return p.layoutErr
}

func (p *stubComposerPipeline) UpdateComposerEffect(_, _ string, _ *pipeline.Effect) error {
	p.effectCalls++
	return p.effectErr
}
func (p *stubComposerPipeline) DeleteComposer(_ string) error                { return nil }
func (p *stubComposerPipeline) RebuildStreamEncoder(_ pipeline.Stream) error { return nil }
func (p *stubComposerPipeline) Pool() process.Pool                           { return nil }

// newComposerSvc builds a composerService wired to the given store and
// pipeline mock with the master switch enabled.
func newComposerSvc(store streams.EntityStore, pipe composerPipeline) *composerService {
	return &composerService{
		store:  store,
		pipe:   pipe,
		psw:    &stubPipelineSwitch{cfg: streams.PipelineConfig{Enabled: true}},
		logger: logging.GetLogger("composer_svc_test"),
	}
}

// seedComposer adds a composer with one input and no layout to the store.
func seedComposer(t *testing.T, store *stubEntityStore) {
	t.Helper()
	c := pipeline.Composer{
		ID:        "comp-1",
		Canvas:    pipeline.CanvasDims{W: 1920, H: 1080},
		Inputs:    []pipeline.ComposerInput{{Ref: "source:cam"}},
		CreatedAt: time.Unix(0, 0),
		UpdatedAt: time.Unix(0, 0),
	}
	if err := store.AddComposerEntity(c); err != nil {
		t.Fatalf("seed composer: %v", err)
	}
}

func validLayout() []models.LayoutSlotData {
	return []models.LayoutSlotData{{Input: "source:cam", X: 0, Y: 0, W: 1920, H: 1080}}
}

// TestComposerService_ReplaceLayout_PersistsWhenNotLive covers the reported
// bug: with the live composer process absent, the manager reports the
// ErrNoSuchComposer sentinel, yet the layout must still persist and the call
// must succeed.
func TestComposerService_ReplaceLayout_PersistsWhenNotLive(t *testing.T) {
	store := newStubStore()
	seedComposer(t, store)
	pipe := &stubComposerPipeline{
		layoutErr: fmt.Errorf("daemon: %w %q", pipelinectl.ErrNoSuchComposer, "comp-1"),
	}
	svc := newComposerSvc(store, pipe)

	out, err := svc.ReplaceLayout(context.Background(), "comp-1", validLayout())
	if err != nil {
		t.Fatalf("ReplaceLayout returned error, want success: %v", err)
	}
	if len(out.Layout) != 1 || out.Layout[0].Input != "source:cam" {
		t.Fatalf("returned layout = %+v, want one slot for source:cam", out.Layout)
	}
	if pipe.layoutCalls != 1 {
		t.Errorf("layout hot-apply attempts = %d, want 1 (best-effort push attempted)", pipe.layoutCalls)
	}
	got, _ := store.GetComposerEntity("comp-1")
	if len(got.Layout) != 1 || got.Layout[0].Input != "source:cam" {
		t.Errorf("persisted layout = %+v, want one slot for source:cam", got.Layout)
	}
}

// TestComposerService_SetInputEffect_PersistsWhenNotLive mirrors the layout
// case for the per-input effect edit path.
func TestComposerService_SetInputEffect_PersistsWhenNotLive(t *testing.T) {
	store := newStubStore()
	seedComposer(t, store)
	pipe := &stubComposerPipeline{
		effectErr: fmt.Errorf("daemon: %w %q", pipelinectl.ErrNoSuchComposer, "comp-1"),
	}
	svc := newComposerSvc(store, pipe)

	effect := &models.EffectData{Type: "perspective", SnapshotW: 1920, SnapshotH: 1080}
	out, err := svc.SetInputEffect(context.Background(), "comp-1", "source:cam", effect)
	if err != nil {
		t.Fatalf("SetInputEffect returned error, want success: %v", err)
	}
	if out.Inputs[0].Effect == nil || out.Inputs[0].Effect.Type != "perspective" {
		t.Fatalf("returned input effect = %+v, want perspective", out.Inputs[0].Effect)
	}
	if pipe.effectCalls != 1 {
		t.Errorf("effect hot-apply attempts = %d, want 1", pipe.effectCalls)
	}
	got, _ := store.GetComposerEntity("comp-1")
	if got.Inputs[0].Effect == nil || got.Inputs[0].Effect.Type != "perspective" {
		t.Errorf("persisted effect = %+v, want perspective", got.Inputs[0].Effect)
	}
}

// TestComposerService_ReplaceLayout_ValidationRejectedNotPersisted ensures
// genuine validation errors are rejected BEFORE persistence and never reach
// the live push.
func TestComposerService_ReplaceLayout_ValidationRejectedNotPersisted(t *testing.T) {
	tests := []struct {
		name   string
		layout []models.LayoutSlotData
	}{
		{
			name:   "unknown input ref",
			layout: []models.LayoutSlotData{{Input: "source:ghost", W: 100, H: 100}},
		},
		{
			name:   "invalid rotation",
			layout: []models.LayoutSlotData{{Input: "source:cam", W: 100, H: 100, Rotation: 45}},
		},
		{
			name:   "invalid aspect ratio mode",
			layout: []models.LayoutSlotData{{Input: "source:cam", W: 100, H: 100, AspectRatioMode: "warp"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := newStubStore()
			seedComposer(t, store)
			pipe := &stubComposerPipeline{}
			svc := newComposerSvc(store, pipe)

			_, err := svc.ReplaceLayout(context.Background(), "comp-1", tt.layout)
			if err == nil {
				t.Fatal("ReplaceLayout succeeded, want validation error")
			}
			var ce *api.ComposerError
			if !errors.As(err, &ce) || ce.Code != api.ComposerErrInvalid {
				t.Errorf("error = %v, want ComposerErrInvalid", err)
			}
			if pipe.layoutCalls != 0 {
				t.Errorf("layout hot-apply attempts = %d, want 0 (rejected before push)", pipe.layoutCalls)
			}
			got, _ := store.GetComposerEntity("comp-1")
			if len(got.Layout) != 0 {
				t.Errorf("persisted layout = %+v, want empty (not persisted on validation failure)", got.Layout)
			}
		})
	}
}

// TestComposerService_ReplaceLayout_LiveRPCErrorSurfaces ensures a genuine
// RPC failure from a live composer (not the not-live sentinel) still errors
// and rolls the store back to the pre-edit state.
func TestComposerService_ReplaceLayout_LiveRPCErrorSurfaces(t *testing.T) {
	store := newStubStore()
	seedComposer(t, store)
	pipe := &stubComposerPipeline{layoutErr: errors.New("composer rejected layout: GLES error")}
	svc := newComposerSvc(store, pipe)

	_, err := svc.ReplaceLayout(context.Background(), "comp-1", validLayout())
	if err == nil {
		t.Fatal("ReplaceLayout succeeded, want RPC error to surface")
	}
	var ce *api.ComposerError
	if !errors.As(err, &ce) || ce.Code != api.ComposerErrInvalid {
		t.Errorf("error = %v, want ComposerErrInvalid", err)
	}
	got, _ := store.GetComposerEntity("comp-1")
	if len(got.Layout) != 0 {
		t.Errorf("persisted layout = %+v, want empty (rolled back on live RPC error)", got.Layout)
	}
}

// TestComposerService_SetInputEffect_LiveRPCErrorSurfaces is the effect-path
// analogue of the live-RPC-error case.
func TestComposerService_SetInputEffect_LiveRPCErrorSurfaces(t *testing.T) {
	store := newStubStore()
	seedComposer(t, store)
	pipe := &stubComposerPipeline{effectErr: errors.New("composer rejected effect")}
	svc := newComposerSvc(store, pipe)

	effect := &models.EffectData{Type: "perspective"}
	_, err := svc.SetInputEffect(context.Background(), "comp-1", "source:cam", effect)
	if err == nil {
		t.Fatal("SetInputEffect succeeded, want RPC error to surface")
	}
	got, _ := store.GetComposerEntity("comp-1")
	if got.Inputs[0].Effect != nil {
		t.Errorf("persisted effect = %+v, want nil (rolled back on live RPC error)", got.Inputs[0].Effect)
	}
}

// TestComposerService_SetInputEffect_UnknownRefRejected ensures an effect on a
// non-existent input ref is rejected before persistence and before any push.
func TestComposerService_SetInputEffect_UnknownRefRejected(t *testing.T) {
	store := newStubStore()
	seedComposer(t, store)
	pipe := &stubComposerPipeline{}
	svc := newComposerSvc(store, pipe)

	_, err := svc.SetInputEffect(context.Background(), "comp-1", "source:ghost", &models.EffectData{Type: "perspective"})
	if err == nil {
		t.Fatal("SetInputEffect succeeded, want input-not-found error")
	}
	var ce *api.ComposerError
	if !errors.As(err, &ce) || ce.Code != api.ComposerErrInputNotFound {
		t.Errorf("error = %v, want ComposerErrInputNotFound", err)
	}
	if pipe.effectCalls != 0 {
		t.Errorf("effect hot-apply attempts = %d, want 0", pipe.effectCalls)
	}
}

// TestComposerService_ReplaceLayout_SwitchOffPersistsAndRegisters ensures that
// with the master switch off, a layout edit persists and refreshes the
// in-memory registry instead of hot-applying — no process is live to push to.
func TestComposerService_ReplaceLayout_SwitchOffPersistsAndRegisters(t *testing.T) {
	store := newStubStore()
	seedComposer(t, store)
	pipe := &stubComposerPipeline{}
	svc := &composerService{
		store:  store,
		pipe:   pipe,
		psw:    &stubPipelineSwitch{cfg: streams.PipelineConfig{Enabled: false}},
		logger: logging.GetLogger("composer_svc_test"),
	}

	out, err := svc.ReplaceLayout(context.Background(), "comp-1", validLayout())
	if err != nil {
		t.Fatalf("ReplaceLayout returned error, want success: %v", err)
	}
	if len(out.Layout) != 1 {
		t.Fatalf("returned layout = %+v, want one slot", out.Layout)
	}
	if pipe.layoutCalls != 0 {
		t.Errorf("layout hot-apply attempts = %d, want 0 (switch off)", pipe.layoutCalls)
	}
	if pipe.registerCalls != 1 {
		t.Errorf("RegisterComposer calls = %d, want 1 (registry refresh)", pipe.registerCalls)
	}
	got, _ := store.GetComposerEntity("comp-1")
	if len(got.Layout) != 1 {
		t.Errorf("persisted layout = %+v, want one slot", got.Layout)
	}
}
