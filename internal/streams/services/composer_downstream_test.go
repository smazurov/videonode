package services

import (
	"context"
	"testing"

	"github.com/smazurov/videonode/internal/api/models"
	"github.com/smazurov/videonode/internal/streams/pipeline"
)

// TestComposerMutationsEnrichDownstream guards the fix for the
// composer-detail "downstream streams require a reload" bug: every method that
// returns a ComposerData feeds the composer.updated SSE payload, so each must
// carry the denormalized downstream_stream_ids. A mutation that returns the
// un-enriched shape stomps the field in the UI store until a manual reload.
func TestComposerMutationsEnrichDownstream(t *testing.T) {
	newSeeded := func(t *testing.T) *composerService {
		t.Helper()
		store := newStubStore()
		seedComposer(t, store)
		if err := store.AddPipelineStream(pipeline.Stream{
			ID:       "down-1",
			Upstream: "composer:comp-1",
		}); err != nil {
			t.Fatalf("seed stream: %v", err)
		}
		return newComposerSvc(store, &stubComposerPipeline{})
	}

	wantDownstream := func(t *testing.T, label string, c *models.ComposerData) {
		t.Helper()
		if len(c.DownstreamStreamIDs) != 1 || c.DownstreamStreamIDs[0] != "down-1" {
			t.Errorf("%s: downstream_stream_ids = %v; want [down-1]", label, c.DownstreamStreamIDs)
		}
	}

	ctx := context.Background()

	t.Run("UpdateComposer", func(t *testing.T) {
		svc := newSeeded(t)
		c, err := svc.UpdateComposer(ctx, "comp-1", models.ComposerUpdateRequestData{
			Canvas: &models.CanvasDimsData{W: 1280, H: 720},
		})
		if err != nil {
			t.Fatalf("update: %v", err)
		}
		wantDownstream(t, "UpdateComposer", c)
	})

	t.Run("ReplaceLayout", func(t *testing.T) {
		svc := newSeeded(t)
		c, err := svc.ReplaceLayout(ctx, "comp-1", []models.LayoutSlotData{
			{Input: "source:cam", X: 0, Y: 0, W: 1920, H: 1080},
		})
		if err != nil {
			t.Fatalf("replace layout: %v", err)
		}
		wantDownstream(t, "ReplaceLayout", c)
	})

	t.Run("SetInputEffect", func(t *testing.T) {
		svc := newSeeded(t)
		c, err := svc.SetInputEffect(ctx, "comp-1", "source:cam", nil)
		if err != nil {
			t.Fatalf("set effect: %v", err)
		}
		wantDownstream(t, "SetInputEffect", c)
	})
}
