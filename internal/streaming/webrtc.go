package streaming

import (
	"log/slog"
	"sync"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/webrtc"
	pion "github.com/pion/webrtc/v4"
)

// WebRTCConfig holds configuration for WebRTC connections.
type WebRTCConfig struct {
	// ICEServers for STUN/TURN (empty for LAN-only)
	ICEServers []pion.ICEServer
}

// WebRTCManager manages WebRTC peer connections.
type WebRTCManager struct {
	hub         *Hub
	config      WebRTCConfig
	peers       map[string]*webrtc.Conn
	streamPeers map[string]map[string]bool // streamID -> set of peerIDs
	mu          sync.RWMutex
	logger      *slog.Logger
}

// NewWebRTCManager creates a new WebRTC manager.
func NewWebRTCManager(hub *Hub, config WebRTCConfig, logger *slog.Logger) *WebRTCManager {
	return &WebRTCManager{
		hub:         hub,
		config:      config,
		peers:       make(map[string]*webrtc.Conn),
		streamPeers: make(map[string]map[string]bool),
		logger:      logger,
	}
}

// CreateConsumer creates a WebRTC consumer for a stream.
// Takes an SDP offer from the browser and returns an SDP answer.
func (m *WebRTCManager) CreateConsumer(streamID, offer string) (string, error) {
	// Create WebRTC API with optimized NACK buffer for high-bitrate streams
	api, err := NewWebRTCAPI(streamID)
	if err != nil {
		return "", err
	}

	pc, err := api.NewPeerConnection(pion.Configuration{
		ICEServers: m.config.ICEServers,
	})
	if err != nil {
		return "", err
	}

	conn := webrtc.NewConn(pc)
	conn.Mode = core.ModePassiveConsumer

	if err := conn.SetOffer(offer); err != nil {
		_ = pc.Close()
		return "", err
	}

	if err := m.hub.WireConsumer(streamID, conn); err != nil {
		_ = pc.Close()
		return "", err
	}

	answer, err := conn.GetCompleteAnswer(nil, nil)
	if err != nil {
		_ = pc.Close()
		return "", err
	}

	peerID := core.RandString(8, 10)
	m.mu.Lock()
	m.peers[peerID] = conn
	// Track stream -> peer mapping for bulk close on stream restart
	if m.streamPeers[streamID] == nil {
		m.streamPeers[streamID] = make(map[string]bool)
	}
	m.streamPeers[streamID][peerID] = true
	peerCount := len(m.peers)
	m.mu.Unlock()

	SetActivePeers(peerCount)
	m.logger.Debug("WebRTC consumer created", "stream_id", streamID, "peer_id", peerID, "total_peers", peerCount)

	// Handle connection state changes and cleanup
	conn.Listen(func(msg any) {
		if state, ok := msg.(pion.PeerConnectionState); ok {
			switch state {
			case pion.PeerConnectionStateConnected:
				// Start RTCP reader goroutines for each sender
				// This is required for RTCP interceptors to receive NACK/PLI from browser
				for _, sender := range pc.GetSenders() {
					go func(s *pion.RTPSender) {
						m.logger.Debug("Starting RTCP reader for sender", "stream_id", streamID)
						for {
							_, _, readErr := s.ReadRTCP()
							if readErr != nil {
								return // Connection closed
							}
						}
					}(sender)
				}
			case pion.PeerConnectionStateDisconnected,
				pion.PeerConnectionStateFailed,
				pion.PeerConnectionStateClosed:
				// Stop closes senders, removing them from producer's receivers
				_ = conn.Stop()
				m.mu.Lock()
				delete(m.peers, peerID)
				if m.streamPeers[streamID] != nil {
					delete(m.streamPeers[streamID], peerID)
					if len(m.streamPeers[streamID]) == 0 {
						delete(m.streamPeers, streamID)
					}
				}
				remainingPeers := len(m.peers)
				m.mu.Unlock()
				SetActivePeers(remainingPeers)
				m.logger.Debug("WebRTC consumer disconnected", "peer_id", peerID, "stream_id", streamID, "state", state.String(), "remaining_peers", remainingPeers)
			}
		}
	})

	return answer, nil
}

// Stop closes all peer connections.
func (m *WebRTCManager) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for id, conn := range m.peers {
		_ = conn.Stop()
		delete(m.peers, id)
	}
}

// PeerCount returns the number of active WebRTC peers.
func (m *WebRTCManager) PeerCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.peers)
}

// CloseStreamConsumers closes all WebRTC peers for a given stream.
// Called when stream producer is replaced to trigger client reconnection.
func (m *WebRTCManager) CloseStreamConsumers(streamID string) {
	m.mu.Lock()
	peerIDs, exists := m.streamPeers[streamID]
	if !exists || len(peerIDs) == 0 {
		m.mu.Unlock()
		m.logger.Debug("No WebRTC consumers to close", "stream_id", streamID)
		return
	}

	// Copy peer IDs to avoid holding lock during close
	toClose := make([]string, 0, len(peerIDs))
	for peerID := range peerIDs {
		toClose = append(toClose, peerID)
	}
	m.mu.Unlock()

	m.logger.Info("Closing WebRTC consumers for stream restart", "stream_id", streamID, "peer_count", len(toClose))

	for _, peerID := range toClose {
		m.mu.RLock()
		conn, ok := m.peers[peerID]
		m.mu.RUnlock()

		if ok && conn != nil {
			m.logger.Debug("Closing WebRTC peer", "peer_id", peerID, "stream_id", streamID)
			_ = conn.Stop()
		}
	}
}
