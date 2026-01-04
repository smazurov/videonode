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
	hub    *Hub
	config WebRTCConfig
	peers  map[string]*webrtc.Conn
	mu     sync.RWMutex
	logger *slog.Logger
}

// NewWebRTCManager creates a new WebRTC manager.
func NewWebRTCManager(hub *Hub, config WebRTCConfig, logger *slog.Logger) *WebRTCManager {
	return &WebRTCManager{
		hub:    hub,
		config: config,
		peers:  make(map[string]*webrtc.Conn),
		logger: logger,
	}
}

// CreateConsumer creates a WebRTC consumer for a stream.
// Takes an SDP offer from the browser and returns an SDP answer.
func (m *WebRTCManager) CreateConsumer(streamID, offer string) (string, error) {
	// Create WebRTC API with registered codecs
	api, err := webrtc.NewAPI()
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
		pc.Close()
		return "", err
	}

	if err := m.hub.WireConsumer(streamID, conn); err != nil {
		pc.Close()
		return "", err
	}

	answer, err := conn.GetCompleteAnswer(nil, nil)
	if err != nil {
		pc.Close()
		return "", err
	}

	peerID := core.RandString(8, 10)
	m.mu.Lock()
	m.peers[peerID] = conn
	m.mu.Unlock()

	m.logger.Debug("WebRTC consumer created", "stream_id", streamID, "peer_id", peerID)

	// Handle cleanup on disconnect
	conn.Listen(func(msg any) {
		if state, ok := msg.(pion.PeerConnectionState); ok {
			switch state {
			case pion.PeerConnectionStateDisconnected,
				pion.PeerConnectionStateFailed,
				pion.PeerConnectionStateClosed:
				m.mu.Lock()
				delete(m.peers, peerID)
				m.mu.Unlock()
				m.logger.Debug("WebRTC consumer disconnected", "peer_id", peerID, "state", state.String())
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
		conn.Close()
		delete(m.peers, id)
	}
}

// PeerCount returns the number of active WebRTC peers.
func (m *WebRTCManager) PeerCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.peers)
}
