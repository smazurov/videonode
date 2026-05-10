package logging

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

// dedupEntry tracks suppression state for a repeated log message.
type dedupEntry struct {
	firstSeen  time.Time
	suppressed int
	lastEntry  LogEntry // most recent version for SSE updates
}

// DedupHandler wraps another slog.Handler and deduplicates repeated messages.
// The first occurrence is forwarded normally. Duplicates within the cooldown
// window update the ring buffer entry in-place and push live SSE updates
// without consuming additional ring buffer slots.
type DedupHandler struct {
	inner    slog.Handler
	mu       sync.Mutex
	seen     map[string]*dedupEntry
	cooldown time.Duration
	attrs    []slog.Attr
	groups   []string
}

// NewDedupHandler creates a handler that deduplicates repeated messages
// forwarded to inner.
func NewDedupHandler(inner slog.Handler, cooldown time.Duration) *DedupHandler {
	return &DedupHandler{
		inner:    inner,
		seen:     make(map[string]*dedupEntry),
		cooldown: cooldown,
	}
}

// Enabled implements slog.Handler.
func (h *DedupHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.inner.Enabled(ctx, level)
}

// Handle implements slog.Handler.
func (h *DedupHandler) Handle(ctx context.Context, r slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	now := r.Time
	if now.IsZero() {
		now = time.Now()
	}

	// Clean up stale entries
	for msg, entry := range h.seen {
		if now.Sub(entry.firstSeen) > 2*h.cooldown {
			delete(h.seen, msg)
		}
	}

	entry, exists := h.seen[r.Message]
	if !exists || now.Sub(entry.firstSeen) >= h.cooldown {
		// First occurrence or cooldown expired — forward normally and reset tracking
		h.seen[r.Message] = &dedupEntry{
			firstSeen: now,
		}
		return h.inner.Handle(ctx, r)
	}

	// Duplicate within cooldown — update in-place
	entry.suppressed++

	// Build the updated LogEntry for ring buffer update and SSE callback
	attrs := make(map[string]any)
	module := "app"

	for _, a := range h.attrs {
		if a.Key == "module" {
			module = a.Value.String()
		} else {
			attrs[a.Key] = a.Value.Any()
		}
	}
	r.Attrs(func(a slog.Attr) bool {
		if a.Key == "module" {
			module = a.Value.String()
		} else {
			attrs[a.Key] = a.Value.Any()
		}
		return true
	})
	attrs["suppressed"] = entry.suppressed

	updated := LogEntry{
		Timestamp:  r.Time,
		Level:      levelToString(r.Level),
		Module:     module,
		Message:    r.Message,
		Attributes: attrs,
	}
	entry.lastEntry = updated

	// Update ring buffer in-place (same package, access global)
	mutex.RLock()
	buf := logBuffer
	cb := logCallback
	mutex.RUnlock()

	if buf != nil {
		msg := r.Message
		buf.UpdateLatest(
			func(e *LogEntry) bool { return e.Message == msg },
			func(e *LogEntry) {
				if e.Attributes == nil {
					e.Attributes = make(map[string]any)
				}
				e.Attributes["suppressed"] = entry.suppressed
				e.Timestamp = r.Time
			},
		)
	}

	// Push live SSE update
	if cb != nil {
		cb(updated)
	}

	return nil
}

// WithAttrs implements slog.Handler.
func (h *DedupHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	newAttrs := make([]slog.Attr, len(h.attrs)+len(attrs))
	copy(newAttrs, h.attrs)
	copy(newAttrs[len(h.attrs):], attrs)

	return &DedupHandler{
		inner:    h.inner.WithAttrs(attrs),
		seen:     h.seen,
		cooldown: h.cooldown,
		attrs:    newAttrs,
		groups:   h.groups,
	}
}

// WithGroup implements slog.Handler.
func (h *DedupHandler) WithGroup(name string) slog.Handler {
	newGroups := make([]string, len(h.groups)+1)
	copy(newGroups, h.groups)
	newGroups[len(h.groups)] = name

	return &DedupHandler{
		inner:    h.inner.WithGroup(name),
		seen:     h.seen,
		cooldown: h.cooldown,
		attrs:    h.attrs,
		groups:   newGroups,
	}
}
