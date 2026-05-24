package pipeline

import (
	"sort"
	"sync"
)

// SourceRegistry tracks the daemon's known Sources by id. Each entry is
// the current spec; the Pipeline owns the matching ProducerStage in its
// stages map. Decoupled from the legacy device-refcount registry: every
// Source is independent, consumers refer to it by id.
type SourceRegistry struct {
	mu      sync.Mutex
	sources map[string]Source
}

// NewSourceRegistry returns an empty registry.
func NewSourceRegistry() *SourceRegistry {
	return &SourceRegistry{sources: make(map[string]Source)}
}

// Put inserts or replaces the source spec, returning the prior value if
// any (used by callers to diff field-level changes).
func (r *SourceRegistry) Put(s Source) (prior Source, existed bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	prior, existed = r.sources[s.ID]
	r.sources[s.ID] = s
	return prior, existed
}

// Delete removes a source spec; returns the removed spec, if any.
func (r *SourceRegistry) Delete(id string) (Source, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	s, ok := r.sources[id]
	if ok {
		delete(r.sources, id)
	}
	return s, ok
}

// Get returns the source spec by id.
func (r *SourceRegistry) Get(id string) (Source, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	s, ok := r.sources[id]
	return s, ok
}

// IDs returns the sorted list of source ids currently registered.
func (r *SourceRegistry) IDs() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]string, 0, len(r.sources))
	for id := range r.sources {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

// ComposerRegistry tracks the daemon's known Composers by id. Same
// shape as SourceRegistry — id-keyed, no refcount.
type ComposerRegistry struct {
	mu        sync.Mutex
	composers map[string]Composer
}

// NewComposerRegistry returns an empty registry.
func NewComposerRegistry() *ComposerRegistry {
	return &ComposerRegistry{composers: make(map[string]Composer)}
}

// Put inserts or replaces the composer spec.
func (r *ComposerRegistry) Put(c Composer) (prior Composer, existed bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	prior, existed = r.composers[c.ID]
	r.composers[c.ID] = c
	return prior, existed
}

// Delete removes a composer spec.
func (r *ComposerRegistry) Delete(id string) (Composer, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	c, ok := r.composers[id]
	if ok {
		delete(r.composers, id)
	}
	return c, ok
}

// Get returns the composer spec by id.
func (r *ComposerRegistry) Get(id string) (Composer, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	c, ok := r.composers[id]
	return c, ok
}

// IDs returns the sorted list of composer ids currently registered.
func (r *ComposerRegistry) IDs() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]string, 0, len(r.composers))
	for id := range r.composers {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}
