package events

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// EntityEvent is the uniform SSE envelope for all per-entity events.
// One wire schema replaces the per-action structs (SourceCreatedEvent,
// ComposerLayoutChangedEvent, StreamUpdatedEvent, etc.). Discriminate
// on (EntityType, Action) to decode Payload.
type EntityEvent struct {
	EntityType string `json:"entity_type" example:"source" doc:"Entity type: source | composer | stream"`
	ID         string `json:"id" example:"hdmi0" doc:"Entity identifier (empty allowed for global events)"`
	Action     string `json:"action" example:"updated" doc:"created | updated | deleted | status | metrics | consumers"`
	Payload    any    `json:"payload,omitempty" doc:"Action-specific payload (entity snapshot for lifecycle, status snapshot, metrics, or per-client consumer list)"`
	Timestamp  string `json:"timestamp" example:"2026-05-23T10:30:00Z" doc:"RFC3339 server time"`
}

// Type identifies EntityEvent on the kelindar/event bus.
func (EntityEvent) Type() uint32 { return TypeEntity }

// Action constants for the uniform envelope.
const (
	ActionCreated   = "created"
	ActionUpdated   = "updated"
	ActionDeleted   = "deleted"
	ActionStatus    = "status"
	ActionMetrics   = "metrics"
	ActionConsumers = "consumers"
)

// AnyRef is a typed cross-entity reference used by the dependency graph.
// Construct only via Entity.Ref(id) so the entity type is guaranteed to
// match a registered entity.
type AnyRef struct {
	Type string
	ID   string
}

// Registration declares an entity to the registry. The Loader is used
// by Touch to re-read an entity after an internal state change so the
// next published snapshot reflects the latest denormalized fields.
type Registration[T any] struct {
	Type        string         // canonical entity name, e.g. "source"
	RoutePrefix string         // e.g. "/api/sources" — used by auto-publish middleware
	IDOf        func(T) string // extract id from an entity snapshot
	Loader      func(context.Context, string) (T, error)
}

// Entity is a typed handle returned by Register. Services embed *Entity[T]
// so the compiler rejects any service that wasn't registered.
type Entity[T any] struct {
	bus    *Bus
	reg    *Registry
	typ    string
	prefix string
	idOf   func(T) string
	loader func(context.Context, string) (T, error)
}

// Type returns the canonical entity name.
func (e *Entity[T]) Type() string { return e.typ }

// RoutePrefix returns the HTTP route prefix this entity is mounted at.
func (e *Entity[T]) RoutePrefix() string { return e.prefix }

// Ref constructs a typed cross-entity reference; the only constructor
// for AnyRef that guarantees the entity name is registered.
func (e *Entity[T]) Ref(id string) AnyRef { return AnyRef{Type: e.typ, ID: id} }

// PublishCreated publishes an "<entity>.created" envelope carrying the
// new snapshot. Safe to call even if no SSE subscribers are attached.
func (e *Entity[T]) PublishCreated(payload T) {
	e.publish(ActionCreated, e.idOf(payload), payload)
}

// PublishUpdated publishes an "<entity>.updated" envelope with the new
// snapshot. Use after any mutation that changed an in-band field.
func (e *Entity[T]) PublishUpdated(payload T) {
	e.publish(ActionUpdated, e.idOf(payload), payload)
}

// PublishDeleted publishes an "<entity>.deleted" envelope. Payload is
// nil; subscribers should remove from their by-id maps.
func (e *Entity[T]) PublishDeleted(id string) {
	e.publish(ActionDeleted, id, nil)
}

// PublishStatus publishes an "<entity>.status" envelope carrying a
// status snapshot from a sidecar or collector.
func (e *Entity[T]) PublishStatus(id string, payload any) {
	e.publish(ActionStatus, id, payload)
}

// PublishMetrics publishes an "<entity>.metrics" envelope carrying a
// metrics snapshot (fps, bitrate, dropped frames, etc.).
func (e *Entity[T]) PublishMetrics(id string, payload any) {
	e.publish(ActionMetrics, id, payload)
}

// PublishConsumers publishes an "<entity>.consumers" envelope carrying
// a per-client consumer list (SRT consumers, WebRTC peers, RTSP readers).
func (e *Entity[T]) PublishConsumers(id string, payload any) {
	e.publish(ActionConsumers, id, payload)
}

// Touch re-reads the entity through the Loader and publishes an
// "<entity>.updated" envelope. Used by the dependency graph to refresh
// denormalized cross-entity fields (e.g., Source.Consumers when a
// Stream pointing at it is created or deleted), and by non-HTTP code
// paths that affect derived state (RTSP reader connect, sidecar status
// changes that bump consumer count, etc.). Errors from Loader are
// silently dropped — a Touch is best-effort.
func (e *Entity[T]) Touch(ctx context.Context, ids ...string) {
	for _, id := range ids {
		if id == "" {
			continue
		}
		payload, err := e.loader(ctx, id)
		if err != nil {
			// Loader miss is expected when an entity has just been
			// deleted (Touch racing with delete). Don't republish a
			// stale snapshot; the .deleted event already informed
			// subscribers.
			continue
		}
		e.publish(ActionUpdated, id, payload)
	}
}

func (e *Entity[T]) publish(action, id string, payload any) {
	if e.bus == nil {
		return
	}
	e.bus.Publish(EntityEvent{
		EntityType: e.typ,
		ID:         id,
		Action:     action,
		Payload:    payload,
		Timestamp:  time.Now().UTC().Format(time.RFC3339),
	})
	// Lifecycle events trigger dependency fan-out so cross-entity
	// rollups (Source.Consumers when a Stream changes, etc.) refresh
	// in the same dispatch scope. Status/metrics/consumers events are
	// per-entity by design — no fan-out.
	if e.reg != nil && isLifecycleAction(action) {
		e.reg.DispatchDependencies(context.Background(), e.typ, action, payload)
	}
}

func isLifecycleAction(action string) bool {
	switch action {
	case ActionCreated, ActionUpdated, ActionDeleted:
		return true
	}
	return false
}

// registryEntry is the type-erased view the Registry holds for lookup
// (by entity type or route prefix). Auto-publish middleware uses
// publishRaw to dispatch a parsed response body without knowing T.
type registryEntry struct {
	typ        string
	prefix     string
	publishRaw func(action, id string, payload any)
	touchRaw   func(ctx context.Context, id string)
}

// Registry holds all registered entities. One Registry is constructed
// in main.go alongside the Bus. Tests can construct independent
// Registries to avoid cross-test contamination.
type Registry struct {
	bus    *Bus
	mu     sync.RWMutex
	byType map[string]registryEntry
	deps   []dependency // see dependencies.go
}

// NewRegistry constructs a Registry bound to the given Bus.
func NewRegistry(bus *Bus) *Registry {
	return &Registry{
		bus:    bus,
		byType: make(map[string]registryEntry),
	}
}

// Register declares an entity and returns its typed handle. Calling
// Register twice with the same Type returns an error-shaped *Entity[T]
// (handle is still usable; the duplicate is logged via panic to fail
// fast at boot).
func Register[T any](r *Registry, opts Registration[T]) *Entity[T] {
	if r == nil {
		panic("events.Register: nil Registry")
	}
	if opts.Type == "" {
		panic("events.Register: Registration.Type is required")
	}
	if opts.IDOf == nil {
		panic(fmt.Sprintf("events.Register(%q): Registration.IDOf is required", opts.Type))
	}
	if opts.Loader == nil {
		panic(fmt.Sprintf("events.Register(%q): Registration.Loader is required", opts.Type))
	}
	e := &Entity[T]{
		bus:    r.bus,
		reg:    r,
		typ:    opts.Type,
		prefix: opts.RoutePrefix,
		idOf:   opts.IDOf,
		loader: opts.Loader,
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.byType[opts.Type]; exists {
		panic(fmt.Sprintf("events.Register: entity %q is already registered", opts.Type))
	}
	r.byType[opts.Type] = registryEntry{
		typ:        opts.Type,
		prefix:     opts.RoutePrefix,
		publishRaw: e.publish,
		touchRaw: func(ctx context.Context, id string) {
			e.Touch(ctx, id)
		},
	}
	return e
}

// LookupByPrefix returns the entity registration whose RoutePrefix
// matches the given request path (longest-prefix match). Empty
// RoutePrefix entries are skipped.
func (r *Registry) LookupByPrefix(path string) (typ, prefix string, ok bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	bestLen := 0
	for _, e := range r.byType {
		if e.prefix == "" {
			continue
		}
		if len(e.prefix) > bestLen && pathHasPrefix(path, e.prefix) {
			typ = e.typ
			prefix = e.prefix
			bestLen = len(e.prefix)
			ok = true
		}
	}
	return
}

// Touch re-reads `id` via the registered entity's Loader and publishes
// an updated envelope. No-op when entityType isn't registered.
func (r *Registry) Touch(ctx context.Context, entityType, id string) {
	r.mu.RLock()
	entry, ok := r.byType[entityType]
	r.mu.RUnlock()
	if !ok {
		return
	}
	entry.touchRaw(ctx, id)
}

// PublishLifecycle is the type-erased lifecycle publisher used by the
// auto-publish HTTP middleware. The middleware already parsed the
// response body; the typed Entity helpers are preferred from service
// code where T is known.
func (r *Registry) PublishLifecycle(entityType, action, id string, payload any) {
	r.mu.RLock()
	entry, ok := r.byType[entityType]
	r.mu.RUnlock()
	if !ok {
		return
	}
	entry.publishRaw(action, id, payload)
}

// RegisteredTypes returns all registered entity type names. Useful for
// SelfCheck and for the SSE handler that needs to enumerate them.
func (r *Registry) RegisteredTypes() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]string, 0, len(r.byType))
	for t := range r.byType {
		out = append(out, t)
	}
	return out
}

// pathHasPrefix matches a request path against a registered route
// prefix. Treats `/api/sources` as matching `/api/sources`,
// `/api/sources/`, and `/api/sources/<anything>` but not
// `/api/sources_alt`.
func pathHasPrefix(path, prefix string) bool {
	if len(path) < len(prefix) {
		return false
	}
	if path[:len(prefix)] != prefix {
		return false
	}
	if len(path) == len(prefix) {
		return true
	}
	return path[len(prefix)] == '/'
}
