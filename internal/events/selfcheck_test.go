package events

import (
	"context"
	"strings"
	"testing"
)

type fakeProbe struct {
	routes []RouteInfo
}

func (p fakeProbe) HasRoute(method, path string) bool {
	for _, r := range p.routes {
		if strings.EqualFold(r.Method, method) && r.Path == path {
			return true
		}
	}
	return false
}

func (p fakeProbe) ListRoutes() []RouteInfo { return p.routes }

func TestSelfCheck_AllRoutesAndRegistrationsMatch(t *testing.T) {
	bus := New()
	reg := NewRegistry(bus)
	Register(reg, Registration[fakeSource]{
		Type:        "source",
		RoutePrefix: "/api/sources",
		IDOf:        func(s fakeSource) string { return s.ID },
		Loader:      func(_ context.Context, id string) (fakeSource, error) { return fakeSource{ID: id}, nil },
	})

	probe := fakeProbe{routes: []RouteInfo{
		{Method: "GET", Path: "/api/sources"},
		{Method: "POST", Path: "/api/sources"},
		{Method: "GET", Path: "/api/sources/{id}"},
		{Method: "PATCH", Path: "/api/sources/{id}"},
		{Method: "DELETE", Path: "/api/sources/{id}"},
	}}

	if err := reg.SelfCheck(context.Background(), probe); err != nil {
		t.Errorf("SelfCheck failed unexpectedly: %v", err)
	}
}

func TestSelfCheck_MissingRoute(t *testing.T) {
	bus := New()
	reg := NewRegistry(bus)
	Register(reg, Registration[fakeSource]{
		Type:        "source",
		RoutePrefix: "/api/sources",
		IDOf:        func(s fakeSource) string { return s.ID },
		Loader:      func(_ context.Context, id string) (fakeSource, error) { return fakeSource{ID: id}, nil },
	})

	probe := fakeProbe{routes: []RouteInfo{
		{Method: "GET", Path: "/api/sources"}, // GET only, no POST
	}}

	err := reg.SelfCheck(context.Background(), probe)
	if err == nil {
		t.Fatal("SelfCheck should have failed: source is registered but has no POST route")
	}
	if !strings.Contains(err.Error(), "POST /api/sources route exists") {
		t.Errorf("error message should name the missing POST route; got: %v", err)
	}
}

func TestSelfCheck_RouteWithoutRegistration(t *testing.T) {
	bus := New()
	reg := NewRegistry(bus)
	// Note: source is NOT registered.

	probe := fakeProbe{routes: []RouteInfo{
		{Method: "POST", Path: "/api/widgets"},
		{Method: "PATCH", Path: "/api/widgets/{id}"},
	}}

	err := reg.SelfCheck(context.Background(), probe)
	if err == nil {
		t.Fatal("SelfCheck should have failed: /api/widgets is a CRUD route with no registration")
	}
	if !strings.Contains(err.Error(), "events.Register") {
		t.Errorf("error message should point at events.Register; got: %v", err)
	}
}

func TestSelfCheck_SkipsNonEntityRoutes(t *testing.T) {
	bus := New()
	reg := NewRegistry(bus)

	probe := fakeProbe{routes: []RouteInfo{
		{Method: "POST", Path: "/api/devices/scan"},
		{Method: "DELETE", Path: "/api/events/something"},
		{Method: "POST", Path: "/api/pipeline/restart"},
		{Method: "PATCH", Path: "/api/update/apply"},
	}}

	if err := reg.SelfCheck(context.Background(), probe); err != nil {
		t.Errorf("SelfCheck should ignore known non-entity routes; got: %v", err)
	}
}
