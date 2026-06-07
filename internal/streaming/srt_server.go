package streaming

import (
	"errors"
	"log/slog"
	"sync"
	"time"

	srt "github.com/datarhei/gosrt"
	"github.com/smazurov/videonode/internal/logging"
)

// ErrNoSupportedCodecs is returned when no supported codecs are found in the stream.
var ErrNoSupportedCodecs = errors.New("no supported codecs in stream")

// ErrStreamNotFound is returned when a requested stream doesn't exist.
var ErrStreamNotFound = errors.New("stream not found")

// SRTConfig holds configuration for the SRT server.
type SRTConfig struct {
	Enabled bool   `yaml:"enabled" json:"enabled"`
	Addr    string `yaml:"addr" json:"addr"`       // Listen address (default ":6001")
	Latency int    `yaml:"latency" json:"latency"` // Peer latency the listener demands of receivers, in milliseconds. The connection negotiates the max of this and the receiver's own latency.

	// OverheadBW is the retransmit bandwidth headroom in percent (valid range
	// 10-100). Raise it on lossy links such as WiFi so SRT has room to resend
	// lost packets. Zero leaves the gosrt default (25).
	OverheadBW int `yaml:"overhead_bw" json:"overhead_bw"`

	// MaxBW caps total send bandwidth (incl. retransmissions) in bytes/s.
	// -1 = unlimited (gosrt default); 0 = relative cap derived from InputBW
	// and OverheadBW (MaxBW = InputBW * (100 + OverheadBW) / 100).
	MaxBW int64 `yaml:"max_bw" json:"max_bw"`

	// InputBW is the expected stream input rate in bytes/s, used for the
	// relative MaxBW cap when MaxBW is 0. Zero lets SRT estimate it.
	InputBW int64 `yaml:"input_bw" json:"input_bw"`

	// PayloadSize is the SRT payload size in bytes. Pinned to 1316 (7x188)
	// for MPEG-TS alignment; zero leaves the gosrt default.
	PayloadSize int `yaml:"payload_size" json:"payload_size"`

	// PeerIdleTimeout is how long, in milliseconds, the listener waits without
	// any packet from a peer before declaring the connection dead. Zero leaves
	// the gosrt default (5000); raising it lets a session ride out brief WiFi
	// stalls or roams.
	PeerIdleTimeout int `yaml:"peer_idle_timeout" json:"peer_idle_timeout"`
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

	// The listener is the sender, so PeerLatency is the knob that matters: it
	// is the minimum latency the receiver will use after negotiation.
	config.PeerLatency = time.Duration(s.config.Latency) * time.Millisecond

	if s.config.OverheadBW > 0 {
		config.OverheadBW = int64(s.config.OverheadBW)
	}
	config.MaxBW = s.config.MaxBW
	if s.config.InputBW > 0 {
		config.InputBW = s.config.InputBW
	}
	if s.config.PayloadSize > 0 {
		config.PayloadSize = uint32(s.config.PayloadSize)
	}
	if s.config.PeerIdleTimeout > 0 {
		config.PeerIdleTimeout = time.Duration(s.config.PeerIdleTimeout) * time.Millisecond
	}

	s.server = &srt.Server{
		Addr:            s.config.Addr,
		Config:          &config,
		HandleConnect:   s.handleConnect,
		HandleSubscribe: s.handleSubscribe,
	}

	s.logger.Info("Starting SRT server",
		logging.KeyAddr, s.config.Addr,
		logging.KeyLatencyMS, s.config.Latency,
		logging.KeyOverheadBW, config.OverheadBW,
		logging.KeyPayloadSize, config.PayloadSize)

	go func() {
		if err := s.server.ListenAndServe(); err != nil && !errors.Is(err, srt.ErrServerClosed) {
			s.logger.Error("SRT server error", logging.KeyError, err)
		}
	}()

	return nil
}

// handleConnect is called for each incoming SRT connection.
func (s *SRTServer) handleConnect(req srt.ConnRequest) srt.ConnType {
	streamID := req.StreamId()

	s.logger.Debug("SRT connection request",
		logging.KeyStreamID, streamID,
		logging.KeyRemote, req.RemoteAddr(),
		logging.KeyVersion, req.Version())

	// Ensure the stream exists, or kick the lazy-start hook and wait.
	if s.streams.EnsureStreamReady(streamID, 3*time.Second) == nil {
		s.logger.Warn("SRT connection rejected: stream not found",
			logging.KeyStreamID, streamID,
			logging.KeyRemote, req.RemoteAddr())
		req.Reject(srt.REJ_PEER)
		return srt.REJECT
	}

	return srt.SUBSCRIBE
}

// handleSubscribe is called when a subscriber connection is accepted.
func (s *SRTServer) handleSubscribe(conn srt.Conn) {
	streamID := conn.StreamId()
	consumerID := generateClientID()
	clientIP := hostOnly(conn.RemoteAddr().String())

	s.logger.Info("SRT subscriber connected",
		logging.KeyStreamID, streamID,
		logging.KeyConsumerID, consumerID,
		logging.KeyClientIP, clientIP,
		logging.KeyRemote, conn.RemoteAddr())

	// Get the stream (already started by handleConnect's EnsureStreamReady).
	stream := s.streams.EnsureStreamReady(streamID, 3*time.Second)
	if stream == nil {
		s.logger.Warn("SRT subscriber rejected: stream disappeared",
			logging.KeyStreamID, streamID,
			logging.KeyConsumerID, consumerID)
		_ = conn.Close()
		return
	}

	// Create consumer
	consumer, err := NewSRTConsumer(stream, consumerID, clientIP, conn, s.logger)
	if err != nil {
		s.logger.Error("Failed to create SRT consumer",
			logging.KeyStreamID, streamID,
			logging.KeyConsumerID, consumerID,
			logging.KeyError, err)
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
		logging.KeyStreamID, streamID,
		logging.KeyConsumerID, consumerID,
		logging.KeyTotalConsumers, consumerCount)

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
		logging.KeyStreamID, streamID,
		logging.KeyConsumerID, consumerID,
		logging.KeyBytesSent, consumer.BytesSent())
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
			logging.KeyStreamID, streamID)
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
