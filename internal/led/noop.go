package led

import "log/slog"

// noop implements Controller as a no-op for systems without LED support
type noop struct {
	logger *slog.Logger
}

// newNoop creates a new no-op LED controller
func newNoop(logger *slog.Logger) *noop {
	return &noop{
		logger: logger,
	}
}

// Set logs the request but performs no actual LED control
func (n *noop) Set(ledType string, enabled bool, pattern string) error {
	n.logger.Debug("LED control not available (no-op)",
		"led_type", ledType,
		"enabled", enabled,
		"pattern", pattern)
	return nil
}

// Available returns an empty list since no LEDs are available
func (n *noop) Available() []string {
	return []string{}
}

// Patterns returns an empty list since no patterns are available
func (n *noop) Patterns() []string {
	return []string{}
}