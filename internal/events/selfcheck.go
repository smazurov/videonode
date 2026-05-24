package events

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
)

// SelfCheck validates that every registered entity has a working
// Loader and (when probe is non-nil) a matching HTTP route, returning
// an aggregate error naming every problem so a future contributor can
// fix the wiring at boot time instead of discovering it via a stale UI.
//
// Caller invokes SelfCheck from main.go after all services and routes
// are registered. The probe callback (typically backed by a list of
// huma.Operations) is optional; pass nil to skip the route-vs-registry
// cross-check.
func (r *Registry) SelfCheck(_ context.Context, probe RouteProbe) error {
	var errs []string

	r.mu.RLock()
	entries := make([]registryEntry, 0, len(r.byType))
	for _, e := range r.byType {
		entries = append(entries, e)
	}
	r.mu.RUnlock()
	sort.Slice(entries, func(i, j int) bool { return entries[i].typ < entries[j].typ })

	// 1. Every registered entity should have a CRUD POST route at its
	//    prefix (route registration was forgotten otherwise).
	if probe != nil {
		for _, e := range entries {
			if e.prefix == "" {
				continue
			}
			if !probe.HasRoute("POST", e.prefix) {
				errs = append(errs, fmt.Sprintf(
					"entity %q is registered (prefix=%s) but no POST %s route exists — "+
						"did you forget to register a create handler? See internal/api/%ss.go for the pattern",
					e.typ, e.prefix, e.prefix, e.typ))
			}
		}

		// 2. Inverse: routes that look like entity CRUD but have no
		//    matching registration. Adding a new entity model and
		//    forgetting events.Register lands here.
		for _, route := range probe.ListRoutes() {
			if !looksLikeEntityCRUD(route.Method, route.Path) {
				continue
			}
			if _, _, ok := r.LookupByPrefix(route.Path); !ok {
				errs = append(errs, fmt.Sprintf(
					"route %s %s looks like entity CRUD but no events.Register call covers it — "+
						"add events.Register(events.Registration[YourModel]{Type: ..., RoutePrefix: ...}) "+
						"in your service constructor or in api.NewServer",
					route.Method, route.Path))
			}
		}
	}

	if len(errs) == 0 {
		return nil
	}
	return errors.New("entity registry self-check failed:\n  - " + strings.Join(errs, "\n  - "))
}

// RouteProbe is the minimal contract SelfCheck needs from the HTTP
// layer to perform the route-vs-registry cross-check. Implemented by
// the API server using huma's operation list; tests can pass a static
// implementation.
type RouteProbe interface {
	// HasRoute returns true if the given method+path is registered.
	HasRoute(method, path string) bool
	// ListRoutes returns every registered (method, path) pair.
	ListRoutes() []RouteInfo
}

// RouteInfo is one (method, path) pair surfaced by RouteProbe.
type RouteInfo struct {
	Method string
	Path   string
}

// looksLikeEntityCRUD identifies routes that *look* like they mutate
// an entity. Used by the inverse check to flag missing registrations.
// Conservative: only flags POST/PATCH/DELETE under /api/{plural}.
func looksLikeEntityCRUD(method, path string) bool {
	switch method {
	case "POST", "PATCH", "DELETE":
	default:
		return false
	}
	if !strings.HasPrefix(path, "/api/") {
		return false
	}
	// Skip well-known non-entity routes that match the shape.
	skip := []string{
		"/api/events", "/api/logs", "/api/health", "/api/metrics",
		"/api/docs", "/api/openapi", "/api/devices", "/api/pipeline",
		"/api/processes", "/api/webrtc", "/api/recordings", "/api/encoders",
		"/api/update",
	}
	for _, s := range skip {
		if strings.HasPrefix(path, s) {
			return false
		}
	}
	return true
}
