//go:build planv2_tests

// Post-rewrite service-layer tests. The monolithic StreamService is
// split into SourceService, ComposerService, and StreamService (per B9).
// Each owns its CRUD + lifecycle. This file exercises the three through
// manual mock implementations of the post-B9 interfaces.
//
// Awaits B9. Stubs live below so the test compiles under planv2_tests.
package streams

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/smazurov/videonode/internal/streams/pipeline"
)

// SourceServicePlan, ComposerServicePlan, StreamServicePlan are the
// post-B9 service interfaces in stub form. Real ones live in
// internal/streams/{source_service,composer_service,stream_service}.go
// after B9 lands.
type SourceServicePlan interface {
	Create(ctx context.Context, s pipeline.PlanSource) (pipeline.PlanSource, error)
	Get(ctx context.Context, id string) (pipeline.PlanSource, error)
	List(ctx context.Context) ([]pipeline.PlanSource, error)
	Delete(ctx context.Context, id string) error
}

type ComposerServicePlan interface {
	Create(ctx context.Context, c pipeline.PlanComposer) (pipeline.PlanComposer, error)
	Get(ctx context.Context, id string) (pipeline.PlanComposer, error)
	List(ctx context.Context) ([]pipeline.PlanComposer, error)
	Delete(ctx context.Context, id string) error
}

type StreamServicePlan interface {
	Create(ctx context.Context, s pipeline.PlanStream) (pipeline.PlanStream, error)
	Get(ctx context.Context, id string) (pipeline.PlanStream, error)
	List(ctx context.Context) ([]pipeline.PlanStream, error)
	Delete(ctx context.Context, id string) error
}

// In-memory mock services keyed by id. Capture references between
// composers/streams and sources so Delete can refuse on referenced.
type mockSourceSvc struct {
	store      map[string]pipeline.PlanSource
	composerSv *mockComposerSvc
	streamSv   *mockStreamSvc
}

func newMockSourceSvc() *mockSourceSvc {
	return &mockSourceSvc{store: map[string]pipeline.PlanSource{}}
}

func (m *mockSourceSvc) Create(_ context.Context, s pipeline.PlanSource) (pipeline.PlanSource, error) {
	if s.ID == "" {
		return s, errors.New("source.ID required")
	}
	if _, exists := m.store[s.ID]; exists {
		return s, errors.New("source already exists: " + s.ID)
	}
	m.store[s.ID] = s
	return s, nil
}

func (m *mockSourceSvc) Get(_ context.Context, id string) (pipeline.PlanSource, error) {
	s, ok := m.store[id]
	if !ok {
		return s, errors.New("source not found: " + id)
	}
	return s, nil
}

func (m *mockSourceSvc) List(_ context.Context) ([]pipeline.PlanSource, error) {
	out := make([]pipeline.PlanSource, 0, len(m.store))
	for _, s := range m.store {
		out = append(out, s)
	}
	return out, nil
}

func (m *mockSourceSvc) Delete(_ context.Context, id string) error {
	if _, ok := m.store[id]; !ok {
		return errors.New("source not found: " + id)
	}
	// Refuse if a composer references this source.
	if m.composerSv != nil {
		for _, c := range m.composerSv.store {
			for _, in := range c.Inputs {
				if in.Ref == "source:"+id {
					return errors.New("source in use by composer: " + c.ID)
				}
			}
		}
	}
	// Refuse if a stream references this source.
	if m.streamSv != nil {
		for _, s := range m.streamSv.store {
			if s.Upstream == "source:"+id {
				return errors.New("source in use by stream: " + s.ID)
			}
		}
	}
	delete(m.store, id)
	return nil
}

type mockComposerSvc struct {
	store    map[string]pipeline.PlanComposer
	streamSv *mockStreamSvc
}

func newMockComposerSvc() *mockComposerSvc {
	return &mockComposerSvc{store: map[string]pipeline.PlanComposer{}}
}

func (m *mockComposerSvc) Create(_ context.Context, c pipeline.PlanComposer) (pipeline.PlanComposer, error) {
	if c.ID == "" {
		return c, errors.New("composer.ID required")
	}
	if _, exists := m.store[c.ID]; exists {
		return c, errors.New("composer already exists: " + c.ID)
	}
	m.store[c.ID] = c
	return c, nil
}

func (m *mockComposerSvc) Get(_ context.Context, id string) (pipeline.PlanComposer, error) {
	c, ok := m.store[id]
	if !ok {
		return c, errors.New("composer not found: " + id)
	}
	return c, nil
}

func (m *mockComposerSvc) List(_ context.Context) ([]pipeline.PlanComposer, error) {
	out := make([]pipeline.PlanComposer, 0, len(m.store))
	for _, c := range m.store {
		out = append(out, c)
	}
	return out, nil
}

func (m *mockComposerSvc) Delete(_ context.Context, id string) error {
	if _, ok := m.store[id]; !ok {
		return errors.New("composer not found: " + id)
	}
	if m.streamSv != nil {
		for _, s := range m.streamSv.store {
			if s.Upstream == "composer:"+id {
				return errors.New("composer in use by stream: " + s.ID)
			}
		}
	}
	delete(m.store, id)
	return nil
}

type mockStreamSvc struct {
	store map[string]pipeline.PlanStream
}

func newMockStreamSvc() *mockStreamSvc {
	return &mockStreamSvc{store: map[string]pipeline.PlanStream{}}
}

func (m *mockStreamSvc) Create(_ context.Context, s pipeline.PlanStream) (pipeline.PlanStream, error) {
	if s.ID == "" {
		return s, errors.New("stream.ID required")
	}
	if _, exists := m.store[s.ID]; exists {
		return s, errors.New("stream already exists: " + s.ID)
	}
	m.store[s.ID] = s
	return s, nil
}

func (m *mockStreamSvc) Get(_ context.Context, id string) (pipeline.PlanStream, error) {
	s, ok := m.store[id]
	if !ok {
		return s, errors.New("stream not found: " + id)
	}
	return s, nil
}

func (m *mockStreamSvc) List(_ context.Context) ([]pipeline.PlanStream, error) {
	out := make([]pipeline.PlanStream, 0, len(m.store))
	for _, s := range m.store {
		out = append(out, s)
	}
	return out, nil
}

func (m *mockStreamSvc) Delete(_ context.Context, id string) error {
	if _, ok := m.store[id]; !ok {
		return errors.New("stream not found: " + id)
	}
	delete(m.store, id)
	return nil
}

func newTriadServices() (*mockSourceSvc, *mockComposerSvc, *mockStreamSvc) {
	src := newMockSourceSvc()
	comp := newMockComposerSvc()
	str := newMockStreamSvc()
	src.composerSv = comp
	src.streamSv = str
	comp.streamSv = str
	return src, comp, str
}

func TestSourceService_CreateGetListDelete(t *testing.T) {
	ctx := context.Background()
	src, _, _ := newTriadServices()

	if _, err := src.Create(ctx, pipeline.PlanSource{ID: "hdmi0", Device: "/dev/video0"}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	got, err := src.Get(ctx, "hdmi0")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Device != "/dev/video0" {
		t.Errorf("Get returned Device=%q", got.Device)
	}
	list, err := src.List(ctx)
	if err != nil || len(list) != 1 {
		t.Errorf("List = %v, err=%v", list, err)
	}
	if err := src.Delete(ctx, "hdmi0"); err != nil {
		t.Errorf("Delete: %v", err)
	}
	if _, err := src.Get(ctx, "hdmi0"); err == nil {
		t.Error("expected Get after Delete to error")
	}
}

func TestSourceService_DeleteRefusedWhenReferencedByComposer(t *testing.T) {
	ctx := context.Background()
	src, comp, _ := newTriadServices()
	_, _ = src.Create(ctx, pipeline.PlanSource{ID: "hdmi0", Device: "/dev/video0"})
	_, _ = comp.Create(ctx, pipeline.PlanComposer{
		ID:     "main",
		Canvas: pipeline.PlanCanvasDims{W: 1920, H: 1080},
		Inputs: []pipeline.PlanComposerInput{{Ref: "source:hdmi0"}},
	})

	err := src.Delete(ctx, "hdmi0")
	if err == nil || !strings.Contains(err.Error(), "in use by composer") {
		t.Errorf("want refusal with 'in use by composer', got %v", err)
	}
}

func TestSourceService_DeleteRefusedWhenReferencedByStream(t *testing.T) {
	ctx := context.Background()
	src, _, str := newTriadServices()
	_, _ = src.Create(ctx, pipeline.PlanSource{ID: "hdmi0", Device: "/dev/video0"})
	_, _ = str.Create(ctx, pipeline.PlanStream{ID: "solo", Upstream: "source:hdmi0"})

	err := src.Delete(ctx, "hdmi0")
	if err == nil || !strings.Contains(err.Error(), "in use by stream") {
		t.Errorf("want refusal with 'in use by stream', got %v", err)
	}
}

func TestComposerService_DeleteRefusedWhenReferencedByStream(t *testing.T) {
	ctx := context.Background()
	src, comp, str := newTriadServices()
	_, _ = src.Create(ctx, pipeline.PlanSource{ID: "hdmi0", Device: "/dev/video0"})
	_, _ = comp.Create(ctx, pipeline.PlanComposer{
		ID:     "main",
		Canvas: pipeline.PlanCanvasDims{W: 1920, H: 1080},
		Inputs: []pipeline.PlanComposerInput{{Ref: "source:hdmi0"}},
	})
	_, _ = str.Create(ctx, pipeline.PlanStream{ID: "archive", Upstream: "composer:main"})

	err := comp.Delete(ctx, "main")
	if err == nil || !strings.Contains(err.Error(), "in use by stream") {
		t.Errorf("want refusal with 'in use by stream', got %v", err)
	}
}

func TestStreamService_CreateRejectsDuplicate(t *testing.T) {
	ctx := context.Background()
	_, _, str := newTriadServices()
	_, _ = str.Create(ctx, pipeline.PlanStream{ID: "a", Upstream: "source:x"})
	_, err := str.Create(ctx, pipeline.PlanStream{ID: "a", Upstream: "source:y"})
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Errorf("want duplicate rejection, got %v", err)
	}
}

func TestStreamService_DeleteUnknownIsError(t *testing.T) {
	ctx := context.Background()
	_, _, str := newTriadServices()
	err := str.Delete(ctx, "ghost")
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Errorf("want not-found error, got %v", err)
	}
}

func TestTriad_SharedComposerByTwoStreams(t *testing.T) {
	// One composer feeding two streams — the multi-encode-one-scene
	// happy path. Both creates succeed; neither blocks the other.
	ctx := context.Background()
	src, comp, str := newTriadServices()
	_, _ = src.Create(ctx, pipeline.PlanSource{ID: "hdmi0", Device: "/dev/video0"})
	_, _ = comp.Create(ctx, pipeline.PlanComposer{
		ID:     "main",
		Canvas: pipeline.PlanCanvasDims{W: 1920, H: 1080},
		Inputs: []pipeline.PlanComposerInput{{Ref: "source:hdmi0"}},
	})
	for _, id := range []string{"archive", "low-latency"} {
		if _, err := str.Create(ctx, pipeline.PlanStream{ID: id, Upstream: "composer:main"}); err != nil {
			t.Errorf("Create stream %s: %v", id, err)
		}
	}
	list, _ := str.List(ctx)
	if len(list) != 2 {
		t.Errorf("expected 2 streams sharing composer:main, got %d", len(list))
	}
}
