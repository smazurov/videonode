package nats

import (
	"log/slog"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
)

// StreamClient is a NATS client for stream processes.
// It publishes metrics and logs, and receives control commands.
// Gracefully degrades when NATS is unavailable.
type StreamClient struct {
	url        string
	streamID   string
	conn       *nats.Conn
	sub        *nats.Subscription
	logger     *slog.Logger
	mu         sync.RWMutex
	onRestart  func()
	connected  bool
}

// NewStreamClient creates a new NATS client for a stream process.
func NewStreamClient(url, streamID string, logger *slog.Logger) *StreamClient {
	if logger == nil {
		logger = slog.Default()
	}

	return &StreamClient{
		url:      url,
		streamID: streamID,
		logger:   logger.With("component", "nats-client", "stream_id", streamID),
	}
}

// Connect establishes a connection to the NATS server.
// Returns nil if connection fails (graceful degradation).
func (c *StreamClient) Connect() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	opts := []nats.Option{
		nats.Name("videonode-stream-" + c.streamID),
		nats.ReconnectWait(2 * time.Second),
		nats.MaxReconnects(-1), // Infinite reconnects
		nats.DisconnectErrHandler(func(_ *nats.Conn, err error) {
			c.mu.Lock()
			c.connected = false
			c.mu.Unlock()
			if err != nil {
				c.logger.Warn("NATS disconnected", "error", err)
			} else {
				c.logger.Debug("NATS disconnected")
			}
		}),
		nats.ReconnectHandler(func(_ *nats.Conn) {
			c.mu.Lock()
			c.connected = true
			c.mu.Unlock()
			c.logger.Info("NATS reconnected")
			// Resubscribe to control commands after reconnect
			c.subscribeControlLocked()
		}),
		nats.ConnectHandler(func(_ *nats.Conn) {
			c.logger.Debug("NATS connected")
		}),
	}

	conn, err := nats.Connect(c.url, opts...)
	if err != nil {
		c.logger.Warn("Failed to connect to NATS, running in offline mode", "error", err)
		return err
	}

	c.conn = conn
	c.connected = true
	c.logger.Info("Connected to NATS", "url", c.url)

	// Subscribe to control commands
	c.subscribeControlLocked()

	return nil
}

// subscribeControlLocked subscribes to control commands (must hold lock).
func (c *StreamClient) subscribeControlLocked() {
	if c.conn == nil || c.onRestart == nil {
		return
	}

	subject := SubjectControlRestart(c.streamID)
	sub, err := c.conn.Subscribe(subject, func(msg *nats.Msg) {
		ctrl, err := UnmarshalControl(msg.Data)
		if err != nil {
			c.logger.Warn("Failed to unmarshal control message", "error", err)
			return
		}

		c.logger.Info("Received control command", "action", ctrl.Action, "reason", ctrl.Reason)

		if ctrl.Action == "restart" && c.onRestart != nil {
			c.onRestart()
		}
	})
	if err != nil {
		c.logger.Warn("Failed to subscribe to control commands", "error", err)
		return
	}

	// Unsubscribe from old subscription if exists
	if c.sub != nil {
		_ = c.sub.Unsubscribe()
	}
	c.sub = sub
}

// OnRestart sets the callback for restart commands.
func (c *StreamClient) OnRestart(fn func()) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.onRestart = fn

	// Subscribe if already connected
	if c.conn != nil && c.connected {
		c.subscribeControlLocked()
	}
}

// PublishMetrics publishes stream metrics to NATS.
// No-op if not connected (graceful degradation).
func (c *StreamClient) PublishMetrics(m MetricsMessage) {
	c.mu.RLock()
	conn := c.conn
	connected := c.connected
	c.mu.RUnlock()

	if conn == nil || !connected {
		return
	}

	data, err := m.Marshal()
	if err != nil {
		c.logger.Warn("Failed to marshal metrics", "error", err)
		return
	}

	subject := SubjectStreamMetrics(c.streamID)
	if err := conn.Publish(subject, data); err != nil {
		c.logger.Warn("Failed to publish metrics", "error", err)
	}
}

// PublishLog publishes a log message to NATS.
// No-op if not connected (graceful degradation).
func (c *StreamClient) PublishLog(m LogMessage) {
	c.mu.RLock()
	conn := c.conn
	connected := c.connected
	c.mu.RUnlock()

	if conn == nil || !connected {
		return
	}

	data, err := m.Marshal()
	if err != nil {
		c.logger.Warn("Failed to marshal log", "error", err)
		return
	}

	subject := SubjectStreamLogs(c.streamID)
	if err := conn.Publish(subject, data); err != nil {
		c.logger.Warn("Failed to publish log", "error", err)
	}
}

// PublishState publishes a state change to NATS.
// No-op if not connected (graceful degradation).
func (c *StreamClient) PublishState(m StateMessage) {
	c.mu.RLock()
	conn := c.conn
	connected := c.connected
	c.mu.RUnlock()

	if conn == nil || !connected {
		return
	}

	data, err := m.Marshal()
	if err != nil {
		c.logger.Warn("Failed to marshal state", "error", err)
		return
	}

	subject := SubjectStreamState(c.streamID)
	if err := conn.Publish(subject, data); err != nil {
		c.logger.Warn("Failed to publish state", "error", err)
	}
}

// IsConnected returns true if connected to NATS.
func (c *StreamClient) IsConnected() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.connected && c.conn != nil
}

// Close closes the NATS connection.
func (c *StreamClient) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.sub != nil {
		_ = c.sub.Unsubscribe()
		c.sub = nil
	}

	if c.conn != nil {
		c.conn.Close()
		c.conn = nil
	}

	c.connected = false
	c.logger.Debug("NATS client closed")
}

// ControlPublisher is used by the server to publish control commands.
type ControlPublisher struct {
	conn   *nats.Conn
	logger *slog.Logger
}

// NewControlPublisher creates a publisher for control commands.
func NewControlPublisher(url string, logger *slog.Logger) (*ControlPublisher, error) {
	if logger == nil {
		logger = slog.Default()
	}

	conn, err := nats.Connect(url,
		nats.Name("videonode-control"),
		nats.ReconnectWait(2*time.Second),
		nats.MaxReconnects(5),
	)
	if err != nil {
		return nil, err
	}

	return &ControlPublisher{
		conn:   conn,
		logger: logger.With("component", "nats-control"),
	}, nil
}

// Restart sends a restart command to a stream process.
func (p *ControlPublisher) Restart(streamID, reason string) error {
	msg := ControlMessage{
		Action:    "restart",
		StreamID:  streamID,
		Timestamp: time.Now().Format(time.RFC3339),
		Reason:    reason,
	}

	data, err := msg.Marshal()
	if err != nil {
		return err
	}

	subject := SubjectControlRestart(streamID)
	if err := p.conn.Publish(subject, data); err != nil {
		return err
	}

	p.logger.Info("Sent restart command", "stream_id", streamID, "reason", reason)
	return nil
}

// Close closes the control publisher connection.
func (p *ControlPublisher) Close() {
	if p.conn != nil {
		p.conn.Close()
	}
}
