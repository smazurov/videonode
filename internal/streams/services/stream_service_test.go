package services

import (
	"context"
	"errors"
	"testing"

	"github.com/smazurov/videonode/internal/api"
	"github.com/smazurov/videonode/internal/streams"
	"github.com/smazurov/videonode/internal/streams/pipeline"
)

// stubEntityStore is a minimal in-memory EntityStore for service tests.
// Persistence shape mirrors the production tomlStore; pipeline.Apply
// calls go through the optional Pipeline (nil here = persistence-only
// path, which is what we want to verify).
type stubEntityStore struct {
	sources   map[string]pipeline.Source
	composers map[string]pipeline.Composer
	streams   map[string]pipeline.Stream
}

func newStubStore() *stubEntityStore {
	return &stubEntityStore{
		sources:   map[string]pipeline.Source{},
		composers: map[string]pipeline.Composer{},
		streams:   map[string]pipeline.Stream{},
	}
}

func (s *stubEntityStore) ListSourceEntities() []pipeline.Source {
	out := make([]pipeline.Source, 0, len(s.sources))
	for _, v := range s.sources {
		out = append(out, v)
	}
	return out
}

func (s *stubEntityStore) GetSourceEntity(id string) (pipeline.Source, bool) {
	v, ok := s.sources[id]
	return v, ok
}

func (s *stubEntityStore) AddSourceEntity(src pipeline.Source) error {
	s.sources[src.ID] = src
	return nil
}

func (s *stubEntityStore) UpdateSourceEntity(id string, src pipeline.Source) error {
	s.sources[id] = src
	return nil
}

func (s *stubEntityStore) RemoveSourceEntity(id string) error {
	delete(s.sources, id)
	return nil
}

func (s *stubEntityStore) ListComposerEntities() []pipeline.Composer {
	out := make([]pipeline.Composer, 0, len(s.composers))
	for _, v := range s.composers {
		out = append(out, v)
	}
	return out
}

func (s *stubEntityStore) GetComposerEntity(id string) (pipeline.Composer, bool) {
	v, ok := s.composers[id]
	return v, ok
}

func (s *stubEntityStore) AddComposerEntity(c pipeline.Composer) error {
	s.composers[c.ID] = c
	return nil
}

func (s *stubEntityStore) UpdateComposerEntity(id string, c pipeline.Composer) error {
	s.composers[id] = c
	return nil
}

func (s *stubEntityStore) RemoveComposerEntity(id string) error {
	delete(s.composers, id)
	return nil
}

func (s *stubEntityStore) ListPipelineStreams() []pipeline.Stream {
	out := make([]pipeline.Stream, 0, len(s.streams))
	for _, v := range s.streams {
		out = append(out, v)
	}
	return out
}

func (s *stubEntityStore) GetPipelineStream(id string) (pipeline.Stream, bool) {
	v, ok := s.streams[id]
	return v, ok
}

func (s *stubEntityStore) AddPipelineStream(st pipeline.Stream) error {
	s.streams[st.ID] = st
	return nil
}

func (s *stubEntityStore) UpdatePipelineStream(id string, st pipeline.Stream) error {
	s.streams[id] = st
	return nil
}

func (s *stubEntityStore) RemovePipelineStream(id string) error {
	delete(s.streams, id)
	return nil
}

// stubPipelineSwitch tracks GetPipeline/SetPipeline calls so the
// PipelineSwitch round-trip can be asserted without touching disk.
type stubPipelineSwitch struct {
	cfg streams.PipelineConfig
}

func (s *stubPipelineSwitch) GetPipeline() streams.PipelineConfig { return s.cfg }
func (s *stubPipelineSwitch) SetPipeline(c streams.PipelineConfig) error {
	s.cfg = c
	return nil
}

func TestStreamService_Create_PersistsAndDefaults(t *testing.T) {
	store := newStubStore()
	if err := store.AddSourceEntity(pipeline.Source{ID: "cam-1", Device: "/dev/video0"}); err != nil {
		t.Fatalf("seed source: %v", err)
	}
	svc := NewStreamService(StreamServiceOptions{Store: store})

	in := pipeline.Stream{ID: "enc-1", Upstream: "source:cam-1"}
	got, err := svc.Create(context.Background(), in)
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	if got.ID != "enc-1" || got.Upstream != "source:cam-1" {
		t.Errorf("returned wrong stream: %+v", got)
	}
	if got.Name != "enc-1" {
		t.Errorf("Name default missing: got %q", got.Name)
	}
	if got.CreatedAt.IsZero() || got.UpdatedAt.IsZero() {
		t.Errorf("Create did not set timestamps: %+v", got)
	}

	persisted, ok := store.GetPipelineStream("enc-1")
	if !ok {
		t.Fatal("Create did not persist stream to store")
	}
	if persisted.Upstream != "source:cam-1" {
		t.Errorf("persisted stream has wrong upstream: %q", persisted.Upstream)
	}
}

func TestStreamService_Create_Validation(t *testing.T) {
	tests := []struct {
		name      string
		in        pipeline.Stream
		seedStore func(*stubEntityStore)
		wantErrAs any
	}{
		{
			name:      "missing id",
			in:        pipeline.Stream{Upstream: "source:cam-1"},
			wantErrAs: &api.StreamInvalidError{},
		},
		{
			name:      "missing upstream",
			in:        pipeline.Stream{ID: "enc-1"},
			wantErrAs: &api.StreamInvalidError{},
		},
		{
			name:      "malformed upstream",
			in:        pipeline.Stream{ID: "enc-1", Upstream: "not-a-ref"},
			wantErrAs: &api.StreamInvalidError{},
		},
		{
			name:      "unknown upstream kind",
			in:        pipeline.Stream{ID: "enc-1", Upstream: "foo:cam-1"},
			wantErrAs: &api.StreamInvalidError{},
		},
		{
			name:      "dangling source upstream",
			in:        pipeline.Stream{ID: "enc-1", Upstream: "source:does-not-exist"},
			wantErrAs: &api.StreamUpstreamMissingError{},
		},
		{
			name:      "dangling composer upstream",
			in:        pipeline.Stream{ID: "enc-1", Upstream: "composer:does-not-exist"},
			wantErrAs: &api.StreamUpstreamMissingError{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := newStubStore()
			if tt.seedStore != nil {
				tt.seedStore(store)
			}
			svc := NewStreamService(StreamServiceOptions{Store: store})
			_, err := svc.Create(context.Background(), tt.in)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !errors.As(err, &tt.wantErrAs) {
				t.Errorf("error %v is not the expected type %T", err, tt.wantErrAs)
			}
			if _, ok := store.GetPipelineStream(tt.in.ID); ok {
				t.Errorf("invalid stream should not be persisted, but %q ended up in store", tt.in.ID)
			}
		})
	}
}

func TestStreamService_Create_DuplicateRejected(t *testing.T) {
	store := newStubStore()
	_ = store.AddSourceEntity(pipeline.Source{ID: "cam-1", Device: "/dev/video0"})
	svc := NewStreamService(StreamServiceOptions{Store: store})

	in := pipeline.Stream{ID: "enc-1", Upstream: "source:cam-1"}
	if _, err := svc.Create(context.Background(), in); err != nil {
		t.Fatalf("first Create: %v", err)
	}
	_, err := svc.Create(context.Background(), in)
	if err == nil {
		t.Fatal("expected StreamExistsError on duplicate Create, got nil")
	}
	var exists *api.StreamExistsError
	if !errors.As(err, &exists) {
		t.Errorf("error %v is not StreamExistsError", err)
	}
}

func TestStreamService_Update_PatchAppliedAndPersisted(t *testing.T) {
	store := newStubStore()
	_ = store.AddSourceEntity(pipeline.Source{ID: "cam-1", Device: "/dev/video0"})
	_ = store.AddSourceEntity(pipeline.Source{ID: "cam-2", Device: "/dev/video1"})
	svc := NewStreamService(StreamServiceOptions{Store: store})

	if _, err := svc.Create(context.Background(), pipeline.Stream{ID: "enc-1", Upstream: "source:cam-1"}); err != nil {
		t.Fatalf("seed Create: %v", err)
	}

	got, err := svc.Update(context.Background(), "enc-1", func(st *pipeline.Stream) error {
		st.Name = "renamed"
		st.Upstream = "source:cam-2"
		return nil
	})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if got.Name != "renamed" || got.Upstream != "source:cam-2" {
		t.Errorf("Update returned wrong stream: %+v", got)
	}

	persisted, _ := store.GetPipelineStream("enc-1")
	if persisted.Name != "renamed" || persisted.Upstream != "source:cam-2" {
		t.Errorf("Update did not persist patch: %+v", persisted)
	}
}

func TestStreamService_Update_RejectsDanglingUpstream(t *testing.T) {
	store := newStubStore()
	_ = store.AddSourceEntity(pipeline.Source{ID: "cam-1", Device: "/dev/video0"})
	svc := NewStreamService(StreamServiceOptions{Store: store})
	_, _ = svc.Create(context.Background(), pipeline.Stream{ID: "enc-1", Upstream: "source:cam-1"})

	_, err := svc.Update(context.Background(), "enc-1", func(st *pipeline.Stream) error {
		st.Upstream = "source:not-here"
		return nil
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var missing *api.StreamUpstreamMissingError
	if !errors.As(err, &missing) {
		t.Errorf("error %v is not StreamUpstreamMissingError", err)
	}
	persisted, _ := store.GetPipelineStream("enc-1")
	if persisted.Upstream != "source:cam-1" {
		t.Errorf("failed Update should not mutate persisted state, got upstream %q", persisted.Upstream)
	}
}

func TestStreamService_Delete_RemovesFromStore(t *testing.T) {
	store := newStubStore()
	_ = store.AddSourceEntity(pipeline.Source{ID: "cam-1", Device: "/dev/video0"})
	svc := NewStreamService(StreamServiceOptions{Store: store})
	_, _ = svc.Create(context.Background(), pipeline.Stream{ID: "enc-1", Upstream: "source:cam-1"})

	if err := svc.Delete(context.Background(), "enc-1"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, ok := store.GetPipelineStream("enc-1"); ok {
		t.Error("Delete left stream in the store")
	}

	err := svc.Delete(context.Background(), "enc-1")
	var notFound *api.StreamNotFoundError
	if !errors.As(err, &notFound) {
		t.Errorf("second Delete should report not-found, got %v", err)
	}
}

func TestStreamService_PipelineSwitch_RoundTrip(t *testing.T) {
	store := newStubStore()
	psw := &stubPipelineSwitch{cfg: streams.PipelineConfig{Enabled: true}}
	svc := NewStreamService(StreamServiceOptions{Store: store, PipelineSwitch: psw})

	if !svc.PipelineEnabled() {
		t.Error("expected pipeline enabled at construction")
	}

	wasEnabled, err := svc.StopPipeline(context.Background())
	if err != nil {
		t.Fatalf("StopPipeline: %v", err)
	}
	if !wasEnabled {
		t.Error("StopPipeline should report previous state was enabled")
	}
	if svc.PipelineEnabled() {
		t.Error("PipelineEnabled should be false after StopPipeline")
	}

	wasDisabled, err := svc.StartPipeline(context.Background())
	if err != nil {
		t.Fatalf("StartPipeline: %v", err)
	}
	if !wasDisabled {
		t.Error("StartPipeline should report transition from disabled (true)")
	}
	if !svc.PipelineEnabled() {
		t.Error("PipelineEnabled should be true after StartPipeline")
	}
}
