package logging

import (
	"context"
	"fmt"
	"log/slog"
	"maps"
	"sort"
	"strings"
	"sync"
	"time"
)

// dedupEntry tracks suppression state for a repeated log record.
type dedupEntry struct {
	firstSeen  time.Time
	suppressed int
	lastEntry  LogEntry // most recent version for SSE updates
}

// dedupKey builds the identity used to detect duplicates. A record is a
// duplicate only when its level, module, message, and all attributes match —
// not the message text alone, so lines that share a message but differ in
// attributes are kept distinct. The dynamic "suppressed" attribute is excluded
// so a buffer entry already carrying a count still matches its own key.
func dedupKey(level, module, message string, attrs map[string]any) string {
	keys := make([]string, 0, len(attrs))
	for k := range attrs {
		if k == "suppressed" {
			continue
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var sb strings.Builder
	sb.WriteString(level)
	sb.WriteByte('\x1f')
	sb.WriteString(module)
	sb.WriteByte('\x1f')
	sb.WriteString(message)
	for _, k := range keys {
		sb.WriteByte('\x1f')
		sb.WriteString(k)
		sb.WriteByte('=')
		fmt.Fprintf(&sb, "%v", attrs[k])
	}
	return sb.String()
}

// DedupHandler wraps another slog.Handler and deduplicates repeated records.
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
	for key, entry := range h.seen {
		if now.Sub(entry.firstSeen) > 2*h.cooldown {
			delete(h.seen, key)
		}
	}

	// Build attributes + module up front so the dedup key can fold in every key,
	// not just the message text.
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

	level := levelToString(r.Level)
	key := dedupKey(level, module, r.Message, attrs)

	entry, exists := h.seen[key]
	if !exists || now.Sub(entry.firstSeen) >= h.cooldown {
		// First occurrence or cooldown expired — forward normally and reset tracking
		h.seen[key] = &dedupEntry{
			firstSeen: now,
		}
		return h.inner.Handle(ctx, r)
	}

	// Duplicate within cooldown — update in-place
	entry.suppressed++
	attrs["suppressed"] = entry.suppressed

	updated := LogEntry{
		Timestamp:  r.Time,
		Level:      level,
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
		buf.UpdateLatest(
			func(e *LogEntry) bool { return dedupKey(e.Level, e.Module, e.Message, e.Attributes) == key },
			func(e *LogEntry) {
				// Copy-on-write: a published reference to this map may be
				// mid-marshal on another goroutine, and a concurrent map
				// write during json.Marshal panics in mapEncoder.
				next := make(map[string]any, len(e.Attributes)+1)
				maps.Copy(next, e.Attributes)
				next["suppressed"] = entry.suppressed
				e.Attributes = next
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
