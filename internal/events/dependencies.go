package events

import (
	"context"
	"sync"
)

// OnLifecycleFunc decides which other entities should be re-read after
// `source` (an entity of type T) has gone through an action covered by
// the dependency declaration. Return nil to skip.
//
// The function MUST NOT call Touch directly — return references and
// let the dependency engine dedupe within the current request scope.
type OnLifecycleFunc[T any] func(source T) []AnyRef

// dependency is the type-erased view of a registered hook.
type dependency struct {
	sourceType string
	actions    map[string]bool
	resolve    func(payload any) []AnyRef
}

// OnLifecycle registers a hook: whenever `source` publishes a
// lifecycle event whose action is in `actions`, the engine calls
// `resolve(snapshot)` to find referenced entities and Touches each of
// them so their denormalized rollups are refreshed.
//
// Example: a Stream whose upstream is "source:hdmi0" creates a
// dependency on that source so the source's Consumers list re-publishes
// when the stream is created, retargeted, or deleted.
func OnLifecycle[T any](source *Entity[T], actions []string, resolve OnLifecycleFunc[T]) {
	if source == nil {
		panic("events.OnLifecycle: nil source entity (forgot to call Register?)")
	}
	if source.reg == nil {
		panic("events.OnLifecycle: source entity is not attached to a Registry")
	}
	actionSet := make(map[string]bool, len(actions))
	for _, a := range actions {
		actionSet[a] = true
	}
	dep := dependency{
		sourceType: source.typ,
		actions:    actionSet,
		resolve: func(payload any) []AnyRef {
			// Lifecycle payloads from Entity[T].publish are typed T.
			// For deleted events the payload is nil; the hook is
			// expected to handle nil if it cares about deletes.
			var typed T
			if payload != nil {
				if t, ok := payload.(T); ok {
					typed = t
				} else {
					return nil
				}
			}
			return resolve(typed)
		},
	}
	source.reg.mu.Lock()
	source.reg.deps = append(source.reg.deps, dep)
	source.reg.mu.Unlock()
}

// dispatchDeps invokes every hook whose (sourceType, action) matches
// and Touches each returned AnyRef. Deduplication is per-call (one
// HTTP request scope, or one sidecar status push) — caller passes a
// fresh touchSet.
func (r *Registry) dispatchDeps(ctx context.Context, sourceType, action string, payload any, set *touchSet) {
	r.mu.RLock()
	deps := append([]dependency(nil), r.deps...)
	r.mu.RUnlock()
	for _, d := range deps {
		if d.sourceType != sourceType {
			continue
		}
		if !d.actions[action] {
			continue
		}
		for _, ref := range d.resolve(payload) {
			if ref.Type == "" || ref.ID == "" {
				continue
			}
			if !set.add(ref) {
				continue
			}
			r.Touch(ctx, ref.Type, ref.ID)
		}
	}
}

// touchSet deduplicates Touch targets within a single dispatch scope
// so two hooks pointing at the same entity only republish it once.
type touchSet struct {
	mu   sync.Mutex
	seen map[AnyRef]bool
}

func newTouchSet() *touchSet { return &touchSet{seen: make(map[AnyRef]bool)} }

func (s *touchSet) add(ref AnyRef) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.seen[ref] {
		return false
	}
	s.seen[ref] = true
	return true
}

// DispatchDependencies runs the dependency graph for one lifecycle
// event. The auto-publish middleware calls this after publishing; the
// non-HTTP Touch paths (sidecar status, RTSP reader connect) call it
// too when they republish via Entity.PublishUpdated.
func (r *Registry) DispatchDependencies(ctx context.Context, sourceType, action string, payload any) {
	set := newTouchSet()
	// Avoid republishing the source itself if a hook accidentally
	// resolves back to it.
	if id, ok := extractID(payload); ok {
		set.add(AnyRef{Type: sourceType, ID: id})
	}
	r.dispatchDeps(ctx, sourceType, action, payload, set)
}

// extractID is a best-effort id reader for type-erased lifecycle
// payloads. The registry knows IDOf via the typed Entity[T], but
// dispatchDeps only sees `any`. Returning ok=false means "couldn't
// derive id" and the self-dedup is skipped (harmless — at worst, one
// extra republish).
func extractID(payload any) (string, bool) {
	type withID interface{ EntityID() string }
	if p, ok := payload.(withID); ok {
		return p.EntityID(), true
	}
	return "", false
}
