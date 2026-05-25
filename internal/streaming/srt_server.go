package streaming

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"log/slog"
	"sync"
	"time"

	srt "github.com/datarhei/gosrt"
)

// ErrNoSupportedCodecs is returned when no supported codecs are found in the stream.
var ErrNoSupportedCodecs = errors.New("no supported codecs in stream")

// ErrStreamNotFound is returned when a requested stream doesn't exist.
var ErrStreamNotFound = errors.New("stream not found")

// SRTConfig holds configuration for the SRT server.
type SRTConfig struct {
	Enabled bool   `yaml:"enabled" json:"enabled"`
	Addr    string `yaml:"addr" json:"addr"`       // Listen address (default ":6001")
	Latency int    `yaml:"latency" json:"latency"` // SRT latency in milliseconds (default 120)
}

// SRTServer handles SRT connections and routes them to stream producers.
type SRTServer struct {
	streams   StreamProvider
	config    SRTConfig
	server    *srt.Server
	logger    *slog.Logger
	consumers map[string]map[string]*SRTConsumer // streamID -> consumerID -> consumer
	mu        sync.RWMutex
}

// NewSRTServer creates a new SRT server.
func NewSRTServer(streams StreamProvider, config SRTConfig, logger *slog.Logger) *SRTServer {
	return &SRTServer{
		streams:   streams,
		config:    config,
		logger:    logger,
		consumers: make(map[string]map[string]*SRTConsumer),
	}
}

// Start begins listening for SRT connections.
func (s *SRTServer) Start() error {
	config := srt.DefaultConfig()
	config.Latency = time.Duration(s.config.Latency) * time.Millisecond

	s.server = &srt.Server{
		Addr:            s.config.Addr,
		Config:          &config,
		HandleConnect:   s.handleConnect,
		HandleSubscribe: s.handleSubscribe,
	}

	s.logger.Info("Starting SRT server", "addr", s.config.Addr, "latency_ms", s.config.Latency)

	go func() {
		if err := s.server.ListenAndServe(); err != nil && !errors.Is(err, srt.ErrServerClosed) {
			s.logger.Error("SRT server error", "error", err)
		}
	}()

	return nil
}

// handleConnect is called for each incoming SRT connection.
func (s *SRTServer) handleConnect(req srt.ConnRequest) srt.ConnType {
	streamID := req.StreamId()

	s.logger.Debug("SRT connection request",
		"stream_id", streamID,
		"remote", req.RemoteAddr(),
		"version", req.Version())

	// Ensure the stream exists, or kick the lazy-start hook and wait.
	if s.streams.EnsureStreamReady(streamID, 3*time.Second) == nil {
		s.logger.Warn("SRT connection rejected: stream not found",
			"stream_id", streamID,
			"remote", req.RemoteAddr())
		req.Reject(srt.REJ_PEER)
		return srt.REJECT
	}

	return srt.SUBSCRIBE
}

// handleSubscribe is called when a subscriber connection is accepted.
func (s *SRTServer) handleSubscribe(conn srt.Conn) {
	streamID := conn.StreamId()
	consumerID := generateConsumerID()

	s.logger.Info("SRT subscriber connected",
		"stream_id", streamID,
		"consumer_id", consumerID,
		"remote", conn.RemoteAddr())

	// Get the stream (already started by handleConnect's EnsureStreamReady).
	stream := s.streams.EnsureStreamReady(streamID, 3*time.Second)
	if stream == nil {
		s.logger.Warn("SRT subscriber rejected: stream disappeared",
			"stream_id", streamID,
			"consumer_id", consumerID)
		_ = conn.Close()
		return
	}

	// Create consumer
	consumer, err := NewSRTConsumer(stream, consumerID, conn, s.logger)
	if err != nil {
		s.logger.Error("Failed to create SRT consumer",
			"stream_id", streamID,
			"consumer_id", consumerID,
			"error", err)
		_ = conn.Close()
		return
	}

	// Register consumer
	s.mu.Lock()
	if s.consumers[streamID] == nil {
		s.consumers[streamID] = make(map[string]*SRTConsumer)
	}
	s.consumers[streamID][consumerID] = consumer
	consumerCount := len(s.consumers[streamID])
	s.mu.Unlock()

	// Update metrics
	SetSRTActiveConsumers(streamID, consumerCount)

	s.logger.Debug("SRT consumer registered",
		"stream_id", streamID,
		"consumer_id", consumerID,
		"total_consumers", consumerCount)

	// Block until connection closes
	// The SRT connection will be read from to detect close
	buf := make([]byte, 1500)
	for {
		_, err := conn.Read(buf)
		if err != nil {
			break
		}
	}

	// Cleanup
	_ = consumer.Stop()
	s.removeConsumer(streamID, consumerID)

	s.logger.Info("SRT subscriber disconnected",
		"stream_id", streamID,
		"consumer_id", consumerID,
		"bytes_sent", consumer.BytesSent())
}

// removeConsumer removes a consumer from the tracking map.
func (s *SRTServer) removeConsumer(streamID, consumerID string) {
	s.mu.Lock()
	var remainingCount int
	if consumers, ok := s.consumers[streamID]; ok {
		delete(consumers, consumerID)
		remainingCount = len(consumers)
		if remainingCount == 0 {
			delete(s.consumers, streamID)
		}
	}
	s.mu.Unlock()

	// Update metrics
	SetSRTActiveConsumers(streamID, remainingCount)
	DeleteSRTConsumerMetrics(streamID, consumerID)
}

// CloseStreamConsumers closes all SRT consumers for a given stream.
func (s *SRTServer) CloseStreamConsumers(streamID string) {
	s.mu.Lock()
	consumers := s.consumers[streamID]
	toClose := make([]*SRTConsumer, 0, len(consumers))
	for _, consumer := range consumers {
		toClose = append(toClose, consumer)
	}
	s.mu.Unlock()

	for _, consumer := range toClose {
		s.logger.Debug("Closing SRT consumer due to producer replacement",
			"stream_id", streamID)
		_ = consumer.Stop()
	}
}

// Stop gracefully shuts down the SRT server.
func (s *SRTServer) Stop() {
	if s.server != nil {
		s.server.Shutdown()
	}

	// Close all consumers
	s.mu.Lock()
	for streamID, consumers := range s.consumers {
		for consumerID, consumer := range consumers {
			_ = consumer.Stop()
			delete(consumers, consumerID)
		}
		delete(s.consumers, streamID)
	}
	s.mu.Unlock()

	s.logger.Info("SRT server stopped")
}

// ConsumerCount returns the total number of active SRT consumers.
func (s *SRTServer) ConsumerCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	count := 0
	for _, consumers := range s.consumers {
		count += len(consumers)
	}
	return count
}

// StreamConsumerCount returns the number of SRT consumers for a specific stream.
func (s *SRTServer) StreamConsumerCount(streamID string) int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if consumers, ok := s.consumers[streamID]; ok {
		return len(consumers)
	}
	return 0
}

// StreamConsumerInfo returns per-consumer details for a stream's SRT consumers.
func (s *SRTServer) StreamConsumerInfo(streamID string) []SRTClientInfo {
	s.mu.RLock()
	consumers := s.consumers[streamID]
	if len(consumers) == 0 {
		s.mu.RUnlock()
		return nil
	}
	refs := make([]*SRTConsumer, 0, len(consumers))
	for _, c := range consumers {
		refs = append(refs, c)
	}
	s.mu.RUnlock()

	out := make([]SRTClientInfo, len(refs))
	for i, c := range refs {
		out[i] = c.Info()
	}
	return out
}

// DisconnectConsumer disconnects a single SRT consumer by ID. Returns false if not found.
func (s *SRTServer) DisconnectConsumer(consumerID string) bool {
	s.mu.RLock()
	var found *SRTConsumer
	for _, consumers := range s.consumers {
		if c, ok := consumers[consumerID]; ok {
			found = c
			break
		}
	}
	s.mu.RUnlock()
	if found == nil {
		return false
	}
	_ = found.Stop()
	return true
}

// generateConsumerID creates a unique consumer ID.
func generateConsumerID() string {
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		slog.Error("Failed to generate random bytes for consumer ID", "error", err)
	}
	return hex.EncodeToString(b)
}
