package services

import (
	"slices"
	"testing"

	"github.com/smazurov/videonode/internal/logging"
	"github.com/smazurov/videonode/internal/process"
	"github.com/smazurov/videonode/internal/streams"
	"github.com/smazurov/videonode/internal/streams/pipeline"
)

// stubSourcePipeline is a manual mock of the sourcePipeline seam. It records
// the stream IDs handed to RebuildStreamEncoder so the color-matrix rebuild
// trigger can be asserted without a real supervised pipeline.
type stubSourcePipeline struct {
	rebuilt []string
}

func (p *stubSourcePipeline) ApplySource(_ pipeline.Source) error { return nil }

func (p *stubSourcePipeline) UpdateSourceFormat(_ string, _ pipeline.SourceFormat) error {
	return nil
}
func (p *stubSourcePipeline) DeleteSource(_ string) error { return nil }
func (p *stubSourcePipeline) RebuildStreamEncoder(s pipeline.Stream) error {
	p.rebuilt = append(p.rebuilt, s.ID)
	return nil
}
func (p *stubSourcePipeline) SourceLiveness(_ string) string    { return "" }
func (p *stubSourcePipeline) SourceConsumerCount(_ string) int  { return 0 }
func (p *stubSourcePipeline) SourceColorMatrix(_ string) string { return "" }
func (p *stubSourcePipeline) SourceDetectedFormat(_ string) (w, h, fps uint32, ok bool) {
	return 0, 0, 0, false
}
func (p *stubSourcePipeline) Pool() process.Pool { return nil }

func newSourceSvc(store streams.EntityStore, pipe sourcePipeline, enabled bool) *sourceService {
	return &sourceService{
		store:  store,
		pipe:   pipe,
		psw:    &stubPipelineSwitch{cfg: streams.PipelineConfig{Enabled: enabled}},
		logger: logging.GetLogger("source_svc_test"),
	}
}

// TestSourceService_OnColorMatrixResolved rebuilds exactly the encoders of
// streams that consume the resolved source — the wire that fixes the startup
// race where ApplyStream froze the encoder color tag on the height default
// before the source's first status frame reported its real matrix.
func TestSourceService_OnColorMatrixResolved(t *testing.T) {
	seed := func() *stubEntityStore {
		store := newStubStore()
		_ = store.AddPipelineStream(pipeline.Stream{ID: "s-cam", Upstream: "source:cam"})
		_ = store.AddPipelineStream(pipeline.Stream{ID: "s-cam-2", Upstream: "source:cam"})
		_ = store.AddPipelineStream(pipeline.Stream{ID: "s-other", Upstream: "source:webcam"})
		_ = store.AddPipelineStream(pipeline.Stream{ID: "s-comp", Upstream: "composer:wall"})
		return store
	}

	tests := []struct {
		name     string
		sourceID string
		enabled  bool
		want     []string
	}{
		{"rebuilds dependents", "cam", true, []string{"s-cam", "s-cam-2"}},
		{"single dependent", "webcam", true, []string{"s-other"}},
		{"no dependents", "ghost", true, nil},
		{"switch off skips rebuild", "cam", false, nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pipe := &stubSourcePipeline{}
			svc := newSourceSvc(seed(), pipe, tt.enabled)

			svc.onColorMatrixResolved(tt.sourceID)

			got := pipe.rebuilt
			slices.Sort(got)
			want := slices.Clone(tt.want)
			slices.Sort(want)
			if !slices.Equal(got, want) {
				t.Errorf("rebuilt = %v, want %v", got, want)
			}
		})
	}
}

// TestSourceService_OnColorMatrixResolved_NilPipe is a persistence-only
// service (no pipeline wired); the trigger must be inert, not panic.
func TestSourceService_OnColorMatrixResolved_NilPipe(t *testing.T) {
	svc := newSourceSvc(seed4(t), nil, true)
	svc.onColorMatrixResolved("cam")
}

func seed4(t *testing.T) *stubEntityStore {
	t.Helper()
	store := newStubStore()
	if err := store.AddPipelineStream(pipeline.Stream{ID: "s-cam", Upstream: "source:cam"}); err != nil {
		t.Fatalf("seed stream: %v", err)
	}
	return store
}
