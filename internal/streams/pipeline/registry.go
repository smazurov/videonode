package pipeline

import (
	"fmt"
	"sync"
)

// ProducerRegistry tracks which streams hold each device producer.
// Replaces the legacy integer-refcount Acquire/Release API with
// consumer-set Reconcile: each Reconcile call declares the full set of
// devices a stream wants right now, and the registry computes the
// delta itself. A buggy double-Reconcile is idempotent; a missed
// release on stream Delete is one ReleaseAll() call.
//
// The registry owns no process lifecycle by itself — it returns the
// delta (devices to start, devices to stop) and the Pipeline drives
// process.Pool calls based on those. Keeping the diff math centralized
// here means every caller (Apply, RestartCanvas, ReleaseCanvas, Stop)
// stops computing their own deltas inline.
type ProducerRegistry struct {
	mu        sync.Mutex
	consumers map[string]map[string]struct{} // device → set of stream IDs holding it
}

// NewProducerRegistry returns an empty registry.
func NewProducerRegistry() *ProducerRegistry {
	return &ProducerRegistry{consumers: make(map[string]map[string]struct{})}
}

// ReconcileDelta is the result of Reconcile: which devices a Reconcile
// call newly required (need to spawn a producer for these) and which
// dropped to zero refcount (need to stop those producers).
type ReconcileDelta struct {
	ToStart []string
	ToStop  []string
}

// Reconcile sets the device set held by consumerID to exactly `devices`.
// Adds claims for devices not previously held; releases claims for
// devices in the prior set but not in the new one. Returns the delta
// against the registry-wide refcounts so the caller can decide what to
// spawn or stop.
//
// Idempotent: calling Reconcile twice with the same args after the
// first call returns an empty delta.
func (r *ProducerRegistry) Reconcile(consumerID string, devices []string) ReconcileDelta {
	if consumerID == "" {
		return ReconcileDelta{}
	}
	wanted := make(map[string]struct{}, len(devices))
	for _, d := range devices {
		if d != "" {
			wanted[d] = struct{}{}
		}
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	var d ReconcileDelta

	// Add new claims (and surface devices that go 0→1).
	for dev := range wanted {
		set, exists := r.consumers[dev]
		if !exists {
			set = make(map[string]struct{})
			r.consumers[dev] = set
		}
		wasEmpty := len(set) == 0
		if _, held := set[consumerID]; !held {
			set[consumerID] = struct{}{}
			if wasEmpty {
				d.ToStart = append(d.ToStart, dev)
			}
		}
	}

	// Drop stale claims (surface devices that go to 0).
	for dev, set := range r.consumers {
		if _, stillWant := wanted[dev]; stillWant {
			continue
		}
		if _, held := set[consumerID]; !held {
			continue
		}
		delete(set, consumerID)
		if len(set) == 0 {
			delete(r.consumers, dev)
			d.ToStop = append(d.ToStop, dev)
		}
	}

	return d
}

// ReleaseAll drops every claim held by consumerID. Returns the set of
// devices whose refcount dropped to zero (caller stops those producers).
// Safe for unknown consumerID (no-op delta).
func (r *ProducerRegistry) ReleaseAll(consumerID string) ReconcileDelta {
	r.mu.Lock()
	defer r.mu.Unlock()

	var d ReconcileDelta
	for dev, set := range r.consumers {
		if _, held := set[consumerID]; !held {
			continue
		}
		delete(set, consumerID)
		if len(set) == 0 {
			delete(r.consumers, dev)
			d.ToStop = append(d.ToStop, dev)
		}
	}
	return d
}

// ConsumersOf returns the snapshot of stream IDs currently holding the
// device. Used by the process-manager UI (and diagnostics like "if I
// stop this producer, what dies?"). Sorted for stable output.
func (r *ProducerRegistry) ConsumersOf(device string) []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	set, ok := r.consumers[device]
	if !ok {
		return nil
	}
	out := make([]string, 0, len(set))
	for id := range set {
		out = append(out, id)
	}
	return out
}

// Devices returns the snapshot of currently-claimed devices with their
// refcounts. Used for diagnostics + process-manager view.
func (r *ProducerRegistry) Devices() map[string]int {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make(map[string]int, len(r.consumers))
	for dev, set := range r.consumers {
		out[dev] = len(set)
	}
	return out
}

// Refcount returns the current refcount for the given device, or 0 if
// no consumer holds it. Convenience over Devices()[device].
func (r *ProducerRegistry) Refcount(device string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	if set, ok := r.consumers[device]; ok {
		return len(set)
	}
	return 0
}

// MustReconcile is the testing-friendly variant; panics on empty
// consumerID instead of silently returning an empty delta. Production
// callers should use Reconcile.
func (r *ProducerRegistry) MustReconcile(consumerID string, devices []string) ReconcileDelta {
	if consumerID == "" {
		panic(fmt.Sprintf("ProducerRegistry.MustReconcile: empty consumerID with devices=%v",
			devices))
	}
	return r.Reconcile(consumerID, devices)
}
