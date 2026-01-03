package nats

import (
	"log/slog"
	"sync"

	"github.com/nats-io/nats.go"
	"github.com/smazurov/videonode/internal/events"
)

// Bridge subscribes to NATS subjects and forwards messages to the event bus.
type Bridge struct {
	url      string
	eventBus *events.Bus
	conn     *nats.Conn
	subs     []*nats.Subscription
	logger   *slog.Logger
	mu       sync.Mutex
}

// NewBridge creates a new NATS-to-EventBus bridge.
func NewBridge(url string, eventBus *events.Bus, logger *slog.Logger) *Bridge {
	if logger == nil {
		logger = slog.Default()
	}

	return &Bridge{
		url:      url,
		eventBus: eventBus,
		logger:   logger.With("component", "nats-bridge"),
	}
}

// Start connects to NATS and subscribes to stream subjects.
func (b *Bridge) Start() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	conn, err := nats.Connect(b.url,
		nats.Name("videonode-bridge"),
		nats.ReconnectWait(2),
		nats.MaxReconnects(-1),
		nats.DisconnectErrHandler(func(_ *nats.Conn, err error) {
			if err != nil {
				b.logger.Warn("NATS bridge disconnected", "error", err)
			}
		}),
		nats.ReconnectHandler(func(_ *nats.Conn) {
			b.logger.Info("NATS bridge reconnected")
		}),
	)
	if err != nil {
		return err
	}

	b.conn = conn
	b.logger.Info("NATS bridge connected", "url", b.url)

	// Subscribe to all stream metrics using wildcard
	metricsSub, err := conn.Subscribe(SubjectStreamsPrefix+".*.metrics", b.handleMetrics)
	if err != nil {
		conn.Close()
		return err
	}
	b.subs = append(b.subs, metricsSub)

	// Subscribe to all stream logs using wildcard
	logsSub, err := conn.Subscribe(SubjectStreamsPrefix+".*.logs", b.handleLogs)
	if err != nil {
		b.cleanup()
		return err
	}
	b.subs = append(b.subs, logsSub)

	// Subscribe to all stream state changes using wildcard
	stateSub, err := conn.Subscribe(SubjectStreamsPrefix+".*.state", b.handleState)
	if err != nil {
		b.cleanup()
		return err
	}
	b.subs = append(b.subs, stateSub)

	b.logger.Info("NATS bridge subscribed to stream subjects")
	return nil
}

// handleMetrics processes incoming metrics messages.
func (b *Bridge) handleMetrics(msg *nats.Msg) {
	m, err := UnmarshalMetrics(msg.Data)
	if err != nil {
		b.logger.Warn("Failed to unmarshal metrics", "error", err, "subject", msg.Subject)
		return
	}

	// Convert to event bus event
	event := events.StreamMetricsEvent{
		EventType:       "stream_metrics",
		Timestamp:       m.Timestamp,
		StreamID:        m.StreamID,
		FPS:             m.FPS,
		DroppedFrames:   m.DroppedFrames,
		DuplicateFrames: m.DuplicateFrames,
		ProcessingSpeed: m.ProcessingSpeed,
	}

	b.eventBus.Publish(event)
	b.logger.Debug("Published metrics event", "stream_id", m.StreamID)
}

// handleLogs processes incoming log messages.
func (b *Bridge) handleLogs(msg *nats.Msg) {
	m, err := UnmarshalLog(msg.Data)
	if err != nil {
		b.logger.Warn("Failed to unmarshal log", "error", err, "subject", msg.Subject)
		return
	}

	// Convert to OBS alert event for significant log messages
	if m.Level == "error" || m.Level == "warn" {
		event := events.OBSAlertEvent{
			EventType: "obs_alert",
			Timestamp: m.Timestamp,
			Level:     m.Level,
			Message:   m.Message,
			Details: map[string]any{
				"stream_id": m.StreamID,
				"source":    m.Source,
			},
		}
		b.eventBus.Publish(event)
		b.logger.Debug("Published alert event", "stream_id", m.StreamID, "level", m.Level)
	}
}

// handleState processes incoming state change messages.
func (b *Bridge) handleState(msg *nats.Msg) {
	m, err := UnmarshalState(msg.Data)
	if err != nil {
		b.logger.Warn("Failed to unmarshal state", "error", err, "subject", msg.Subject)
		return
	}

	// Convert to event bus event
	event := events.StreamStateChangedEvent{
		StreamID:  m.StreamID,
		Enabled:   m.Enabled,
		Timestamp: m.Timestamp,
	}

	b.eventBus.Publish(event)
	b.logger.Debug("Published state event", "stream_id", m.StreamID, "enabled", m.Enabled)
}

// cleanup unsubscribes and closes connection.
func (b *Bridge) cleanup() {
	for _, sub := range b.subs {
		_ = sub.Unsubscribe()
	}
	b.subs = nil

	if b.conn != nil {
		b.conn.Close()
		b.conn = nil
	}
}

// Stop closes the bridge connection.
func (b *Bridge) Stop() {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.cleanup()
	b.logger.Info("NATS bridge stopped")
}

// IsConnected returns true if the bridge is connected to NATS.
func (b *Bridge) IsConnected() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.conn != nil && b.conn.IsConnected()
}
