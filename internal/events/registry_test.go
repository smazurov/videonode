package events

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type fakeSource struct {
	ID        string
	Consumers []string
}

func TestRegistry_LifecyclePublishesEntityEvent(t *testing.T) {
	bus := New()
	reg := NewRegistry(bus)

	store := map[string]fakeSource{
		"hdmi0": {ID: "hdmi0", Consumers: []string{}},
	}
	srcEntity := Register(reg, Registration[fakeSource]{
		Type:        "source",
		RoutePrefix: "/api/sources",
		IDOf:        func(s fakeSource) string { return s.ID },
		Loader: func(_ context.Context, id string) (fakeSource, error) {
			s, ok := store[id]
			if !ok {
				return fakeSource{}, fmt.Errorf("not found")
			}
			return s, nil
		},
	})

	var got EntityEvent
	var gotMu sync.Mutex
	unsub := Subscribe(bus, func(e EntityEvent) {
		gotMu.Lock()
		got = e
		gotMu.Unlock()
	})
	defer unsub()

	srcEntity.PublishCreated(store["hdmi0"])

	// kelindar/event delivery is async — give it a moment.
	deadline := time.Now().Add(200 * time.Millisecond)
	for time.Now().Before(deadline) {
		gotMu.Lock()
		ok := got.EntityType != ""
		gotMu.Unlock()
		if ok {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}

	gotMu.Lock()
	defer gotMu.Unlock()
	if got.EntityType != "source" {
		t.Fatalf("entity_type = %q, want %q", got.EntityType, "source")
	}
	if got.Action != ActionCreated {
		t.Errorf("action = %q, want %q", got.Action, ActionCreated)
	}
	if got.ID != "hdmi0" {
		t.Errorf("id = %q, want %q", got.ID, "hdmi0")
	}
	if got.Timestamp == "" {
		t.Error("timestamp is empty")
	}
}

func TestRegistry_LookupByPrefix(t *testing.T) {
	bus := New()
	reg := NewRegistry(bus)
	Register(reg, Registration[fakeSource]{
		Type:        "source",
		RoutePrefix: "/api/sources",
		IDOf:        func(s fakeSource) string { return s.ID },
		Loader: func(_ context.Context, id string) (fakeSource, error) {
			return fakeSource{ID: id}, nil
		},
	})

	tests := []struct {
		path    string
		wantTyp string
		wantOK  bool
	}{
		{"/api/sources", "source", true},
		{"/api/sources/hdmi0", "source", true},
		{"/api/sources/hdmi0/snapshot", "source", true},
		{"/api/sources_alt", "", false},
		{"/api/composers/foo", "", false},
		{"/api/streams", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			typ, _, ok := reg.LookupByPrefix(tt.path)
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}
			if typ != tt.wantTyp {
				t.Errorf("type = %q, want %q", typ, tt.wantTyp)
			}
		})
	}
}

type fakeStream struct {
	ID       string
	Upstream string // "source:hdmi0"
}

func (s fakeStream) EntityID() string { return s.ID }

func TestRegistry_DependencyFanOutTouchesReferencedEntity(t *testing.T) {
	bus := New()
	reg := NewRegistry(bus)

	srcStore := map[string]fakeSource{"hdmi0": {ID: "hdmi0"}}
	streamStore := map[string]fakeStream{}
	var loaderHits atomic.Int32

	srcEntity := Register(reg, Registration[fakeSource]{
		Type:        "source",
		RoutePrefix: "/api/sources",
		IDOf:        func(s fakeSource) string { return s.ID },
		Loader: func(_ context.Context, id string) (fakeSource, error) {
			loaderHits.Add(1)
			s, ok := srcStore[id]
			if !ok {
				return fakeSource{}, fmt.Errorf("not found")
			}
			return s, nil
		},
	})
	streamEntity := Register(reg, Registration[fakeStream]{
		Type:        "stream",
		RoutePrefix: "/api/streams",
		IDOf:        func(s fakeStream) string { return s.ID },
		Loader: func(_ context.Context, id string) (fakeStream, error) {
			s, ok := streamStore[id]
			if !ok {
				return fakeStream{}, fmt.Errorf("not found")
			}
			return s, nil
		},
	})

	OnLifecycle(streamEntity, []string{ActionCreated, ActionUpdated, ActionDeleted}, func(s fakeStream) []AnyRef {
		if s.Upstream == "source:hdmi0" {
			return []AnyRef{srcEntity.Ref("hdmi0")}
		}
		return nil
	})

	var events []EntityEvent
	var evMu sync.Mutex
	unsub := Subscribe(bus, func(e EntityEvent) {
		evMu.Lock()
		events = append(events, e)
		evMu.Unlock()
	})
	defer unsub()

	streamStore["s1"] = fakeStream{ID: "s1", Upstream: "source:hdmi0"}
	streamEntity.PublishCreated(streamStore["s1"])

	// Wait for async dispatch.
	deadline := time.Now().Add(300 * time.Millisecond)
	for time.Now().Before(deadline) {
		evMu.Lock()
		count := len(events)
		evMu.Unlock()
		if count >= 2 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}

	evMu.Lock()
	defer evMu.Unlock()
	if len(events) < 2 {
		t.Fatalf("got %d events, want >=2 (stream.created + source.updated). events=%+v", len(events), events)
	}
	if loaderHits.Load() < 1 {
		t.Errorf("expected Source Loader to be invoked by Touch; loaderHits=%d", loaderHits.Load())
	}

	// One of the events must be the source.updated triggered by the
	// dependency fan-out.
	sawSourceUpdated := false
	for _, e := range events {
		if e.EntityType == "source" && e.Action == ActionUpdated && e.ID == "hdmi0" {
			sawSourceUpdated = true
		}
	}
	if !sawSourceUpdated {
		t.Errorf("did not observe source.updated event; events=%+v", events)
	}
}

// TestRegistry_DeleteFansOutToReferencedEntity covers the bug where
// PublishDeleted shipped a nil payload, so the dependency resolver
// received a zero-value T (e.g. a fakeStream with Upstream="") and
// returned no refs — the source carrying the deleted stream in its
// Consumers list never refreshed over SSE.
//
// The test publishes a stream-created so the source learns about it,
// then publishes a stream-deleted, and expects the source to refresh
// (one final source.updated event after the delete).
func TestRegistry_DeleteFansOutToReferencedEntity(t *testing.T) {
	bus := New()
	reg := NewRegistry(bus)

	srcStore := map[string]fakeSource{"hdmi0": {ID: "hdmi0"}}
	streamStore := map[string]fakeStream{
		"s1": {ID: "s1", Upstream: "source:hdmi0"},
	}

	srcEntity := Register(reg, Registration[fakeSource]{
		Type:        "source",
		RoutePrefix: "/api/sources",
		IDOf:        func(s fakeSource) string { return s.ID },
		Loader: func(_ context.Context, id string) (fakeSource, error) {
			s, ok := srcStore[id]
			if !ok {
				return fakeSource{}, fmt.Errorf("not found")
			}
			return s, nil
		},
	})
	streamEntity := Register(reg, Registration[fakeStream]{
		Type:        "stream",
		RoutePrefix: "/api/streams",
		IDOf:        func(s fakeStream) string { return s.ID },
		Loader: func(_ context.Context, id string) (fakeStream, error) {
			s, ok := streamStore[id]
			if !ok {
				return fakeStream{}, fmt.Errorf("not found")
			}
			return s, nil
		},
	})

	OnLifecycle(streamEntity, []string{ActionCreated, ActionUpdated, ActionDeleted}, func(s fakeStream) []AnyRef {
		if s.Upstream == "source:hdmi0" {
			return []AnyRef{srcEntity.Ref("hdmi0")}
		}
		return nil
	})

	var events []EntityEvent
	var evMu sync.Mutex
	unsub := Subscribe(bus, func(e EntityEvent) {
		evMu.Lock()
		events = append(events, e)
		evMu.Unlock()
	})
	defer unsub()

	streamEntity.PublishCreated(streamStore["s1"])
	// Simulate the service's delete: snapshot, remove from store,
	// then publish the deletion WITH the snapshot so the dep engine
	// can still resolve the upstream source.
	prev := streamStore["s1"]
	delete(streamStore, "s1")
	streamEntity.PublishDeletedWith(prev)

	// Wait for both publishes + both fan-outs to drain.
	deadline := time.Now().Add(300 * time.Millisecond)
	for time.Now().Before(deadline) {
		evMu.Lock()
		count := countSourceUpdatesFor(events, "hdmi0")
		evMu.Unlock()
		if count >= 2 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}

	evMu.Lock()
	defer evMu.Unlock()

	sawCreated := false
	sawDeleted := false
	sourceUpdates := 0
	for _, e := range events {
		switch {
		case e.EntityType == "stream" && e.Action == ActionCreated:
			sawCreated = true
		case e.EntityType == "stream" && e.Action == ActionDeleted:
			sawDeleted = true
		case e.EntityType == "source" && e.Action == ActionUpdated && e.ID == "hdmi0":
			sourceUpdates++
		}
	}
	if !sawCreated || !sawDeleted {
		t.Fatalf("missing stream lifecycle events; sawCreated=%v sawDeleted=%v events=%+v",
			sawCreated, sawDeleted, events)
	}
	if sourceUpdates < 2 {
		t.Errorf("source 'hdmi0' should refresh after BOTH stream-created and stream-deleted, "+
			"but only saw %d source.updated events. PublishDeleted is dropping the payload, "+
			"so the dependency resolver gets a zero-value T and returns no refs. "+
			"Fix: add PublishDeletedWith(prev T) (or change PublishDeleted to take T) "+
			"and have services snapshot-then-delete so the resolver sees the upstream. events=%+v",
			sourceUpdates, events)
	}
}

// TestRegistry_UpdateFansOutToPreviousAndCurrentReferences covers the
// retarget bug: when a stream's upstream changes from "source:A" to
// "source:B", only B was being touched — A's denormalized Consumers
// list still named the stream that no longer pointed at it.
//
// The fix is a separate publish API that hands both the previous and
// the new snapshot to the dep engine so OnLifecycle hooks can resolve
// refs on both sides.
func TestRegistry_UpdateFansOutToPreviousAndCurrentReferences(t *testing.T) {
	bus := New()
	reg := NewRegistry(bus)

	srcStore := map[string]fakeSource{
		"sourceA": {ID: "sourceA"},
		"sourceB": {ID: "sourceB"},
	}
	streamStore := map[string]fakeStream{
		"s1": {ID: "s1", Upstream: "source:sourceA"},
	}

	srcEntity := Register(reg, Registration[fakeSource]{
		Type:        "source",
		RoutePrefix: "/api/sources",
		IDOf:        func(s fakeSource) string { return s.ID },
		Loader: func(_ context.Context, id string) (fakeSource, error) {
			s, ok := srcStore[id]
			if !ok {
				return fakeSource{}, fmt.Errorf("not found")
			}
			return s, nil
		},
	})
	streamEntity := Register(reg, Registration[fakeStream]{
		Type:        "stream",
		RoutePrefix: "/api/streams",
		IDOf:        func(s fakeStream) string { return s.ID },
		Loader: func(_ context.Context, id string) (fakeStream, error) {
			s, ok := streamStore[id]
			if !ok {
				return fakeStream{}, fmt.Errorf("not found")
			}
			return s, nil
		},
	})

	OnLifecycle(streamEntity, []string{ActionCreated, ActionUpdated, ActionDeleted}, func(s fakeStream) []AnyRef {
		const prefix = "source:"
		if !strings.HasPrefix(s.Upstream, prefix) {
			return nil
		}
		return []AnyRef{srcEntity.Ref(s.Upstream[len(prefix):])}
	})

	var events []EntityEvent
	var evMu sync.Mutex
	unsub := Subscribe(bus, func(e EntityEvent) {
		evMu.Lock()
		events = append(events, e)
		evMu.Unlock()
	})
	defer unsub()

	// Initial create — populates source A's Consumers.
	streamEntity.PublishCreated(streamStore["s1"])

	// Retarget: snapshot the previous shape, mutate to new upstream,
	// publish update WITH both snapshots so dep engine touches BOTH
	// the old (A) and new (B) source.
	prev := streamStore["s1"]
	next := prev
	next.Upstream = "source:sourceB"
	streamStore["s1"] = next
	streamEntity.PublishUpdatedWith(prev, next)

	deadline := time.Now().Add(300 * time.Millisecond)
	for time.Now().Before(deadline) {
		evMu.Lock()
		a := countSourceUpdatesFor(events, "sourceA")
		b := countSourceUpdatesFor(events, "sourceB")
		evMu.Unlock()
		if a >= 2 && b >= 1 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}

	evMu.Lock()
	defer evMu.Unlock()
	a := countSourceUpdatesFor(events, "sourceA")
	b := countSourceUpdatesFor(events, "sourceB")
	if a < 2 {
		t.Errorf("source A should refresh on create AND on retarget-away, but saw %d source.updated for sourceA. "+
			"PublishUpdatedWith is dropping prev so the dep engine can't touch the old upstream. events=%+v",
			a, events)
	}
	if b < 1 {
		t.Errorf("source B should refresh on retarget-to, but saw %d source.updated for sourceB. events=%+v",
			b, events)
	}
}

func countSourceUpdatesFor(events []EntityEvent, id string) int {
	n := 0
	for _, e := range events {
		if e.EntityType == "source" && e.Action == ActionUpdated && e.ID == id {
			n++
		}
	}
	return n
}

func TestRegistry_DuplicateRegistrationPanics(t *testing.T) {
	bus := New()
	reg := NewRegistry(bus)
	Register(reg, Registration[fakeSource]{
		Type:        "source",
		RoutePrefix: "/api/sources",
		IDOf:        func(s fakeSource) string { return s.ID },
		Loader:      func(_ context.Context, id string) (fakeSource, error) { return fakeSource{ID: id}, nil },
	})
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic on duplicate Register, got none")
		}
	}()
	Register(reg, Registration[fakeSource]{
		Type:        "source",
		RoutePrefix: "/api/sources",
		IDOf:        func(s fakeSource) string { return s.ID },
		Loader:      func(_ context.Context, id string) (fakeSource, error) { return fakeSource{ID: id}, nil },
	})
}
