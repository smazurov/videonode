package logging

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"slices"
	"strings"

	"github.com/coreos/go-systemd/v22/journal"
)

// JournalHandler is a slog.Handler that sends logs to systemd journal.
type JournalHandler struct {
	level  slog.Level
	attrs  []slog.Attr
	groups []string
}

// NewJournalHandler creates a new journal handler.
func NewJournalHandler(level slog.Level) *JournalHandler {
	return &JournalHandler{
		level:  level,
		attrs:  make([]slog.Attr, 0),
		groups: make([]string, 0),
	}
}

// Enabled reports whether the handler handles records at the given level.
func (h *JournalHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.level
}

// Handle sends the log record to systemd journal.
func (h *JournalHandler) Handle(_ context.Context, r slog.Record) error {
	// Map slog level to journal priority
	priority := mapLevelToPriority(r.Level)

	// Build journal fields
	fields := make(map[string]string)
	fields["PRIORITY"] = fmt.Sprintf("%d", priority)
	fields["MESSAGE"] = r.Message
	fields["SYSLOG_IDENTIFIER"] = "videonode"

	// Add pre-existing attributes from WithAttrs
	for _, attr := range h.attrs {
		addAttrToFields(fields, attr, h.groups)
	}

	// Add attributes from the record
	r.Attrs(func(attr slog.Attr) bool {
		addAttrToFields(fields, attr, h.groups)
		return true
	})

	// Send to journal
	err := journal.Send(r.Message, priority, fields)
	if err != nil {
		// Fallback to stderr if journal is unavailable
		fmt.Fprintf(os.Stderr, "Failed to send to journal: %v\n", err)
		return err
	}

	return nil
}

// WithAttrs returns a new handler with additional attributes.
func (h *JournalHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	newAttrs := make([]slog.Attr, len(h.attrs)+len(attrs))
	copy(newAttrs, h.attrs)
	copy(newAttrs[len(h.attrs):], attrs)

	return &JournalHandler{
		level:  h.level,
		attrs:  newAttrs,
		groups: h.groups,
	}
}

// WithGroup returns a new handler with a group prefix.
func (h *JournalHandler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}

	newGroups := make([]string, len(h.groups)+1)
	copy(newGroups, h.groups)
	newGroups[len(h.groups)] = name

	return &JournalHandler{
		level:  h.level,
		attrs:  h.attrs,
		groups: newGroups,
	}
}

// mapLevelToPriority maps slog levels to journal priorities.
func mapLevelToPriority(level slog.Level) journal.Priority {
	switch {
	case level >= slog.LevelError:
		return journal.PriErr
	case level >= slog.LevelWarn:
		return journal.PriWarning
	case level >= slog.LevelInfo:
		return journal.PriInfo
	default:
		return journal.PriDebug
	}
}

// addAttrToFields adds an slog attribute to journal fields.
func addAttrToFields(fields map[string]string, attr slog.Attr, groups []string) {
	// Skip empty attributes
	if attr.Equal(slog.Attr{}) {
		return
	}

	// Build field key with group prefix
	key := attr.Key
	if len(groups) > 0 {
		key = strings.Join(groups, "_") + "_" + key
	}

	// Convert key to uppercase for journal convention
	key = strings.ToUpper(key)

	// Handle different value types
	switch attr.Value.Kind() {
	case slog.KindString:
		fields[key] = attr.Value.String()
	case slog.KindInt64:
		fields[key] = fmt.Sprintf("%d", attr.Value.Int64())
	case slog.KindUint64:
		fields[key] = fmt.Sprintf("%d", attr.Value.Uint64())
	case slog.KindFloat64:
		fields[key] = fmt.Sprintf("%f", attr.Value.Float64())
	case slog.KindBool:
		fields[key] = fmt.Sprintf("%t", attr.Value.Bool())
	case slog.KindDuration:
		fields[key] = attr.Value.Duration().String()
	case slog.KindTime:
		fields[key] = attr.Value.Time().Format("2006-01-02T15:04:05.000Z07:00")
	case slog.KindGroup:
		// Handle nested groups
		attrs := attr.Value.Group()
		newGroups := append(slices.Clone(groups), key)
		for _, a := range attrs {
			addAttrToFields(fields, a, newGroups)
		}
	default:
		fields[key] = attr.Value.String()
	}
}

// IsJournalAvailable checks if systemd journal is available.
func IsJournalAvailable() bool {
	return journal.Enabled()
}
