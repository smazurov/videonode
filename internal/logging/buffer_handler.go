package logging

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"
)

// LogCallback is called when a new log entry is written.
// Used to publish log events without creating import cycles.
type LogCallback func(entry LogEntry)

// BufferHandler is a slog.Handler that writes to a ring buffer
// and optionally calls a callback for each log entry.
type BufferHandler struct {
	buffer   *RingBuffer
	level    slog.Level
	attrs    []slog.Attr
	groups   []string
	callback LogCallback
}

// NewBufferHandler creates a handler that writes to the given ring buffer.
func NewBufferHandler(buffer *RingBuffer, level slog.Level, callback LogCallback) *BufferHandler {
	return &BufferHandler{
		buffer:   buffer,
		level:    level,
		callback: callback,
	}
}

// Enabled implements slog.Handler.
func (h *BufferHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.level
}

// Handle implements slog.Handler.
func (h *BufferHandler) Handle(_ context.Context, r slog.Record) error {
	attrs := make(map[string]any)
	module := "app"

	// Process handler-level attrs (from WithAttrs)
	for _, a := range h.attrs {
		if a.Key == "module" {
			module = a.Value.String()
		} else {
			flattenAttr(attrs, h.groups, a)
		}
	}

	// Process record-level attrs
	r.Attrs(func(a slog.Attr) bool {
		if a.Key == "module" {
			module = a.Value.String()
		} else {
			flattenAttr(attrs, h.groups, a)
		}
		return true
	})

	entry := LogEntry{
		Timestamp:  r.Time,
		Level:      levelToString(r.Level),
		Module:     module,
		Message:    r.Message,
		Attributes: attrs,
	}

	h.buffer.Write(entry)

	if h.callback != nil {
		h.callback(entry)
	}

	return nil
}

// flattenAttr extracts a slog.Attr into a flat map with dot-notation keys for groups.
func flattenAttr(attrs map[string]any, groups []string, a slog.Attr) {
	key := a.Key
	if len(groups) > 0 {
		key = strings.Join(groups, ".") + "." + key
	}

	switch a.Value.Kind() {
	case slog.KindGroup:
		// Recursively flatten group attributes
		for _, ga := range a.Value.Group() {
			flattenAttr(attrs, append(groups, a.Key), ga)
		}
	case slog.KindTime:
		attrs[key] = a.Value.Time().Format(time.RFC3339Nano)
	case slog.KindDuration:
		attrs[key] = a.Value.Duration().String()
	case slog.KindAny:
		// Handle error type specially
		if err, ok := a.Value.Any().(error); ok {
			attrs[key] = err.Error()
		} else {
			attrs[key] = a.Value.Any()
		}
	default:
		attrs[key] = a.Value.Any()
	}
}

// WithAttrs implements slog.Handler.
func (h *BufferHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	newAttrs := make([]slog.Attr, len(h.attrs)+len(attrs))
	copy(newAttrs, h.attrs)
	copy(newAttrs[len(h.attrs):], attrs)

	return &BufferHandler{
		buffer:   h.buffer,
		level:    h.level,
		attrs:    newAttrs,
		groups:   h.groups,
		callback: h.callback,
	}
}

// WithGroup implements slog.Handler.
func (h *BufferHandler) WithGroup(name string) slog.Handler {
	newGroups := make([]string, len(h.groups)+1)
	copy(newGroups, h.groups)
	newGroups[len(h.groups)] = name

	return &BufferHandler{
		buffer:   h.buffer,
		level:    h.level,
		attrs:    h.attrs,
		groups:   newGroups,
		callback: h.callback,
	}
}

// levelToString converts slog.Level to a lowercase string.
func levelToString(level slog.Level) string {
	switch {
	case level >= slog.LevelError:
		return "error"
	case level >= slog.LevelWarn:
		return "warn"
	case level >= slog.LevelInfo:
		return "info"
	default:
		return "debug"
	}
}

// FormatLogLine formats a LogEntry as a display string for the UI.
func FormatLogLine(entry LogEntry) string {
	var sb strings.Builder
	sb.WriteString(entry.Timestamp.Format(time.RFC3339Nano))
	sb.WriteString(" [")
	sb.WriteString(strings.ToUpper(entry.Level))
	sb.WriteString("] [")
	sb.WriteString(entry.Module)
	sb.WriteString("] ")
	sb.WriteString(entry.Message)

	// Append attributes in key=value format
	if len(entry.Attributes) > 0 {
		keys := make([]string, 0, len(entry.Attributes))
		for k := range entry.Attributes {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			sb.WriteString(" ")
			sb.WriteString(k)
			sb.WriteString("=")
			sb.WriteString(fmt.Sprint(entry.Attributes[k]))
		}
	}

	return sb.String()
}
