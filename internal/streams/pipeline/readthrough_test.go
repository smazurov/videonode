package pipeline

import "testing"

// fakeEntityStore is the pipeline's read-through source of truth in tests:
// the same role the TOML store plays in production. Tests seed it instead
// of poking a registry, mirroring how the service layer persists before the
// pipeline ever reads.
type fakeEntityStore struct {
	sources   map[string]Source
	composers map[string]Composer
}

func newFakeStore() *fakeEntityStore {
	return &fakeEntityStore{
		sources:   map[string]Source{},
		composers: map[string]Composer{},
	}
}

func (f *fakeEntityStore) GetSourceEntity(id string) (Source, bool) {
	s, ok := f.sources[id]
	return s, ok
}

func (f *fakeEntityStore) GetComposerEntity(id string) (Composer, bool) {
	c, ok := f.composers[id]
	return c, ok
}

// resolveUpstream must read a source's format from the store, not a cached
// copy — it feeds the encoder's frame dimensions, so a stale read silently
// mis-sizes the output.
func TestResolveUpstream_ReadsSourceFromStore(t *testing.T) {
	store := newFakeStore()
	store.sources["cam"] = Source{ID: "cam", TestMode: true, Format: &SourceFormat{Width: 1280, Height: 720, FPS: 60}}
	p := New(Config{EntityStore: store, RTSPPort: ":8554"}, nil)

	fs, err := p.resolveUpstream("source:cam")
	if err != nil {
		t.Fatalf("resolveUpstream: %v", err)
	}
	pfs, ok := fs.(ProducerFrameSource)
	if !ok {
		t.Fatalf("want ProducerFrameSource, got %T", fs)
	}
	if pfs.Width != 1280 || pfs.Height != 720 || pfs.Fps != 60 {
		t.Errorf("resolveUpstream dims = %dx%d@%d, want 1280x720@60 from store", pfs.Width, pfs.Height, pfs.Fps)
	}
}

// Regression for the registry-drift bug: an edit that lands only in the
// store is immediately visible to the pipeline with no re-apply, because
// the pipeline reads through to the store instead of a cached copy. Before
// the collapse the pipeline held a registry copy that hot-edits never
// updated, so a later read (e.g. on restart) resurrected the pre-edit spec.
func TestReadThrough_StoreEditVisibleWithoutReapply(t *testing.T) {
	store := newFakeStore()
	store.composers["main"] = Composer{ID: "main", Canvas: CanvasDims{W: 1920, H: 1080}}
	p := New(Config{EntityStore: store, RTSPPort: ":8554"}, nil)

	// Simulate a hot layout/canvas edit: only the store changes — no
	// ApplyComposer, no registry refresh (there is no registry).
	store.composers["main"] = Composer{ID: "main", Canvas: CanvasDims{W: 1280, H: 720}}

	fs, err := p.resolveUpstream("composer:main")
	if err != nil {
		t.Fatalf("resolveUpstream: %v", err)
	}
	cfs := fs.(ComposerFrameSource)
	if cfs.Width != 1280 || cfs.Height != 720 {
		t.Errorf("pipeline saw stale canvas %dx%d; store edit to 1280x720 not visible", cfs.Width, cfs.Height)
	}
}

// Same invariant for composer canvas dimensions.
func TestResolveUpstream_ReadsComposerFromStore(t *testing.T) {
	store := newFakeStore()
	store.composers["main"] = Composer{ID: "main", Canvas: CanvasDims{W: 1280, H: 720}}
	p := New(Config{EntityStore: store, RTSPPort: ":8554"}, nil)

	fs, err := p.resolveUpstream("composer:main")
	if err != nil {
		t.Fatalf("resolveUpstream: %v", err)
	}
	cfs, ok := fs.(ComposerFrameSource)
	if !ok {
		t.Fatalf("want ComposerFrameSource, got %T", fs)
	}
	if cfs.Width != 1280 || cfs.Height != 720 {
		t.Errorf("resolveUpstream canvas = %dx%d, want 1280x720 from store", cfs.Width, cfs.Height)
	}
}
