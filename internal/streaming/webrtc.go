package streaming

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/bluenviron/gortsplib/v5/pkg/description"
	"github.com/bluenviron/gortsplib/v5/pkg/format"
	"github.com/bluenviron/gortsplib/v5/pkg/format/rtph264"
	"github.com/bluenviron/gortsplib/v5/pkg/format/rtph265"
	"github.com/pion/rtp"
	pion "github.com/pion/webrtc/v4"
	"github.com/smazurov/videonode/internal/logging"
)

// WebRTCConfig holds configuration for WebRTC connections.
type WebRTCConfig struct {
	ICEServers []pion.ICEServer
}

// WebRTCManager manages WebRTC peer connections.
type WebRTCManager struct {
	streams     StreamProvider
	config      WebRTCConfig
	peers       map[string]*webrtcPeer
	streamPeers map[string]map[string]bool // streamID -> set of peerIDs
	mu          sync.RWMutex
	logger      logging.Logger
}

// webrtcPeer holds state for a single WebRTC peer connection.
type webrtcPeer struct {
	pc           *pion.PeerConnection
	reader       *Reader
	streamID     string
	peerID       string
	tracks       map[*description.Media]*pion.TrackLocalStaticRTP
	h264Encoders map[*description.Media]*rtph264.Encoder
	h265Encoders map[*description.Media]*rtph265.Encoder
	h265Params   map[*description.Media][][]byte
	connectedAt  time.Time
	bytesSent    atomic.Int64
}

// NewWebRTCManager creates a new WebRTC manager.
func NewWebRTCManager(streams StreamProvider, config WebRTCConfig, logger logging.Logger) *WebRTCManager {
	return &WebRTCManager{
		streams:     streams,
		config:      config,
		peers:       make(map[string]*webrtcPeer),
		streamPeers: make(map[string]map[string]bool),
		logger:      logger,
	}
}

// generatePeerID returns a unique peer ID, retrying on the rare hex collision.
func (m *WebRTCManager) generatePeerID() string {
	m.mu.Lock()
	defer m.mu.Unlock()

	for range 100 {
		id := generateClientID()
		if _, exists := m.peers[id]; !exists {
			return id
		}
	}
	return generateClientID()
}

// CreateConsumer creates a WebRTC consumer for a stream. It returns the
// generated peer ID (used as the WHEP session ID) and the SDP answer.
func (m *WebRTCManager) CreateConsumer(streamID, offer string) (peerID, sdp string, err error) {
	stream := m.streams.EnsureStreamReady(streamID, 3*time.Second)
	if stream == nil {
		return "", "", ErrStreamNotFound
	}

	peerID = m.generatePeerID()

	// Create WebRTC API with optimized settings
	api, err := NewWebRTCAPI(streamID, peerID)
	if err != nil {
		return "", "", err
	}

	pc, err := api.NewPeerConnection(pion.Configuration{
		ICEServers: m.config.ICEServers,
	})
	if err != nil {
		return "", "", err
	}

	// Set up peer state
	peer := &webrtcPeer{
		pc:           pc,
		streamID:     streamID,
		peerID:       peerID,
		tracks:       make(map[*description.Media]*pion.TrackLocalStaticRTP),
		h264Encoders: make(map[*description.Media]*rtph264.Encoder),
		h265Encoders: make(map[*description.Media]*rtph265.Encoder),
		h265Params:   make(map[*description.Media][][]byte),
		connectedAt:  time.Now(),
	}

	// Add tracks for each media in the stream
	desc := stream.Description()
	audioIdx := 0
	for _, medi := range desc.Medias {
		for _, forma := range medi.Formats {
			track, trackErr := m.createTrack(forma, audioIdx)
			if trackErr != nil {
				m.logger.Warn("Failed to create track", logging.KeyStreamID, streamID, logging.KeyError, trackErr)
				continue
			}
			if track == nil {
				continue
			}
			if _, ok := forma.(*format.Opus); ok {
				audioIdx++
			}

			sender, addErr := pc.AddTrack(track)
			if addErr != nil {
				m.logger.Warn("Failed to add track", logging.KeyStreamID, streamID, logging.KeyError, addErr)
				continue
			}

			peer.tracks[medi] = track

			// Create H264 encoder for re-packetizing NAL units for WebRTC
			if _, ok := forma.(*format.H264); ok {
				encoder := &rtph264.Encoder{
					PayloadType:    96,
					PayloadMaxSize: 1188, // 1200 - 12 (RTP header)
				}
				if err := encoder.Init(); err != nil {
					m.logger.Warn("Failed to init H264 encoder", logging.KeyStreamID, streamID, logging.KeyError, err)
					continue
				}
				peer.h264Encoders[medi] = encoder
			}

			// Create H265 encoder for re-packetizing access units for WebRTC.
			// Raw RTP passthrough produced oversized packets (loss) and never
			// carried in-band VPS/SPS/PPS, so subscribers joining mid-stream
			// could not decode; re-packetizing fixes both.
			if h265, ok := forma.(*format.H265); ok {
				encoder := &rtph265.Encoder{
					PayloadType:    96,
					PayloadMaxSize: 1188,
				}
				if err := encoder.Init(); err != nil {
					m.logger.Warn("Failed to init H265 encoder", logging.KeyStreamID, streamID, logging.KeyError, err)
					continue
				}
				peer.h265Encoders[medi] = encoder
				peer.h265Params[medi] = h265ParamSets(h265)
			}

			// Start RTCP reader goroutine
			go func(s *pion.RTPSender) {
				for {
					_, _, readErr := s.ReadRTCP()
					if readErr != nil {
						return
					}
				}
			}(sender)
		}
	}

	// Set remote description (offer)
	if err := pc.SetRemoteDescription(pion.SessionDescription{
		Type: pion.SDPTypeOffer,
		SDP:  offer,
	}); err != nil {
		_ = pc.Close()
		return "", "", err
	}

	// Create answer
	answer, err := pc.CreateAnswer(nil)
	if err != nil {
		_ = pc.Close()
		return "", "", err
	}

	// Create channel to wait for ICE gathering
	gatherComplete := make(chan struct{})
	var gatherOnce sync.Once
	pc.OnICEGatheringStateChange(func(state pion.ICEGatheringState) {
		if state == pion.ICEGatheringStateComplete {
			gatherOnce.Do(func() { close(gatherComplete) })
		}
	})

	// Set local description
	if err := pc.SetLocalDescription(answer); err != nil {
		_ = pc.Close()
		return "", "", err
	}

	// Check if gathering already complete (race condition)
	if pc.ICEGatheringState() == pion.ICEGatheringStateComplete {
		gatherOnce.Do(func() { close(gatherComplete) })
	}

	// Wait for ICE gathering to complete
	<-gatherComplete

	// Create reader and set up RTP forwarding
	peer.reader = NewReader(stream, peerID)
	for medi, track := range peer.tracks {
		currentMedia := medi
		currentTrack := track

		// Check if this media has an H264 encoder (requires re-packetization)
		if encoder, ok := peer.h264Encoders[currentMedia]; ok {
			// Use OnUnit for H264 - re-encode NAL units to proper WebRTC RTP packets
			peer.reader.OnUnit(currentMedia, func(pts, _ int64, au [][]byte) error {
				if len(au) == 0 {
					return nil
				}

				// Encode NAL units to RTP packets
				packets, err := encoder.Encode(au)
				if err != nil {
					return err
				}

				// pts is already in the 90kHz RTP clock (PacketPTS returns
				// clock-rate units, not nanoseconds).
				rtpTimestamp := uint32(pts)

				for _, pkt := range packets {
					pkt.Timestamp = rtpTimestamp
					size := pkt.MarshalSize()
					IncrementPacketsSent(streamID, size)
					peer.bytesSent.Add(int64(size))
					_ = currentTrack.WriteRTP(pkt)
				}
				return nil
			})
		} else if encoder, has := peer.h265Encoders[currentMedia]; has {
			params := peer.h265Params[currentMedia]
			// Re-encode H265 access units to WebRTC RTP, prepending parameter
			// sets to keyframes so mid-stream subscribers can decode.
			peer.reader.OnUnit(currentMedia, func(pts, _ int64, au [][]byte) error {
				if len(au) == 0 {
					return nil
				}

				packets, err := encoder.Encode(prependH265ParamSets(au, params))
				if err != nil {
					return err
				}

				rtpTimestamp := uint32(pts)

				for _, pkt := range packets {
					pkt.Timestamp = rtpTimestamp
					size := pkt.MarshalSize()
					IncrementPacketsSent(streamID, size)
					peer.bytesSent.Add(int64(size))
					_ = currentTrack.WriteRTP(pkt)
				}
				return nil
			})
		} else {
			// For non-H264 formats, forward RTP packets directly
			peer.reader.OnRTP(currentMedia, func(pkt *rtp.Packet) {
				size := pkt.MarshalSize()
				IncrementPacketsSent(streamID, size)
				peer.bytesSent.Add(int64(size))
				_ = currentTrack.WriteRTP(pkt)
			})
		}
	}

	// Register peer
	m.mu.Lock()
	m.peers[peerID] = peer
	if m.streamPeers[streamID] == nil {
		m.streamPeers[streamID] = make(map[string]bool)
	}
	m.streamPeers[streamID][peerID] = true
	streamPeerCount := len(m.streamPeers[streamID])
	m.mu.Unlock()

	SetActivePeers(streamID, streamPeerCount)
	m.logger.Info("WebRTC client connected", logging.KeyStreamID, streamID, logging.KeyPeerID, peerID, logging.KeyStreamPeers, streamPeerCount)

	// Handle connection state changes
	pc.OnConnectionStateChange(func(state pion.PeerConnectionState) {
		switch state {
		case pion.PeerConnectionStateDisconnected,
			pion.PeerConnectionStateFailed,
			pion.PeerConnectionStateClosed:
			m.closePeer(peerID, streamID, state.String())
		}
	})

	// Return the local description with all ICE candidates
	return peerID, pc.LocalDescription().SDP, nil
}

// createTrack creates a WebRTC track for the given format; audioIdx becomes
// the MSID for Opus tracks so the browser can identify which canvas audio
// device a track corresponds to (ignored for video formats).
func (m *WebRTCManager) createTrack(forma format.Format, audioIdx int) (*pion.TrackLocalStaticRTP, error) {
	switch f := forma.(type) {
	case *format.H264:
		// Build fmtp line with H264 profile parameters for browser codec negotiation
		// Use profile-level-id that matches registered codecs in webrtc_api.go
		// The actual profile from SPS (e.g., 640c34) may have constraint flags that
		// don't match browser-supported profiles, so we normalize to standard profiles
		fmtp := "level-asymmetry-allowed=1;packetization-mode=1"
		if len(f.SPS) >= 4 {
			profileIdc := f.SPS[1]
			levelIdc := f.SPS[3]
			// Map to standard profile-level-ids registered in MediaEngine
			// Strip constraint flags (byte 2) to match browser capabilities
			profileLevelID := fmt.Sprintf("%02x00%02x", profileIdc, levelIdc)
			fmtp += ";profile-level-id=" + profileLevelID
			m.logger.Info("H264 track created", logging.KeyProfileIDC, profileIdc, logging.KeyLevelIDC, levelIdc, logging.KeyFMTP, fmtp)
		} else {
			// Fallback to baseline profile
			fmtp += ";profile-level-id=42001f"
			m.logger.Warn("No SPS available, using baseline profile")
		}
		return pion.NewTrackLocalStaticRTP(
			pion.RTPCodecCapability{
				MimeType:    pion.MimeTypeH264,
				ClockRate:   90000,
				SDPFmtpLine: fmtp,
			},
			"video",
			"videonode-h264",
		)
	case *format.H265:
		return pion.NewTrackLocalStaticRTP(
			pion.RTPCodecCapability{
				MimeType:  pion.MimeTypeH265,
				ClockRate: 90000,
			},
			"video",
			"videonode-hevc",
		)
	case *format.Opus:
		return pion.NewTrackLocalStaticRTP(
			pion.RTPCodecCapability{
				MimeType:  pion.MimeTypeOpus,
				ClockRate: 48000,
				Channels:  2,
			},
			fmt.Sprintf("audio-%d", audioIdx),
			"videonode-audio",
		)
	case *format.MPEG4Audio:
		// AAC is not well supported in WebRTC browsers
		return nil, nil
	default:
		return nil, nil
	}
}

// h265ParamSets returns the VPS/SPS/PPS NAL units carried out-of-band in the
// stream's SDP, in decode order, skipping any that are absent.
func h265ParamSets(f *format.H265) [][]byte {
	var params [][]byte
	for _, p := range [][]byte{f.VPS, f.SPS, f.PPS} {
		if len(p) > 0 {
			params = append(params, p)
		}
	}
	return params
}

// prependH265ParamSets prepends VPS/SPS/PPS to an access unit that contains an
// IRAP (keyframe) slice but lacks in-band parameter sets, so a subscriber that
// joins mid-stream can configure its decoder. Non-keyframe AUs pass through.
func prependH265ParamSets(au, params [][]byte) [][]byte {
	if len(params) == 0 {
		return au
	}

	var hasIRAP, hasParams bool
	for _, nalu := range au {
		if len(nalu) == 0 {
			continue
		}
		switch naluType := (nalu[0] >> 1) & 0x3f; {
		case naluType >= 16 && naluType <= 23:
			hasIRAP = true
		case naluType >= 32 && naluType <= 34:
			hasParams = true
		}
	}
	if !hasIRAP || hasParams {
		return au
	}

	return append(append(make([][]byte, 0, len(params)+len(au)), params...), au...)
}

// closePeer cleans up a peer connection.
func (m *WebRTCManager) closePeer(peerID, streamID, reason string) {
	m.mu.Lock()
	peer, ok := m.peers[peerID]
	if !ok {
		m.mu.Unlock()
		return
	}
	delete(m.peers, peerID)

	var remainingPeers int
	if m.streamPeers[streamID] != nil {
		delete(m.streamPeers[streamID], peerID)
		remainingPeers = len(m.streamPeers[streamID])
		if remainingPeers == 0 {
			delete(m.streamPeers, streamID)
		}
	}
	m.mu.Unlock()

	// Close reader
	if peer.reader != nil {
		peer.reader.Close()
	}

	// Close peer connection
	_ = peer.pc.Close()

	SetActivePeers(streamID, remainingPeers)
	DeletePeerMetrics(streamID, peerID)
	m.logger.Info("WebRTC client disconnected", logging.KeyStreamID, streamID, logging.KeyPeerID, peerID, logging.KeyReason, reason, logging.KeyStreamPeers, remainingPeers)
}

// Stop closes all peer connections.
func (m *WebRTCManager) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for peerID, peer := range m.peers {
		if peer.reader != nil {
			peer.reader.Close()
		}
		_ = peer.pc.Close()
		delete(m.peers, peerID)
	}
	m.streamPeers = make(map[string]map[string]bool)
}

// ListStreams returns a list of all active stream IDs.
func (m *WebRTCManager) ListStreams() []string {
	return m.streams.ListStreams()
}

// PeerCount returns the number of active WebRTC peers.
func (m *WebRTCManager) PeerCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.peers)
}

// StreamPeerCount returns the number of WebRTC peers attached to a
// specific stream. Used by the per-stream consumer-count emitter so
// the UI can show RTSP + WebRTC + SRT reader totals.
func (m *WebRTCManager) StreamPeerCount(streamID string) int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.streamPeers[streamID])
}

// StreamPeerInfo returns per-peer details for a stream's WebRTC consumers.
func (m *WebRTCManager) StreamPeerInfo(streamID string) []WebRTCClientInfo {
	m.mu.RLock()
	peerIDs := m.streamPeers[streamID]
	if len(peerIDs) == 0 {
		m.mu.RUnlock()
		return nil
	}
	peers := make([]*webrtcPeer, 0, len(peerIDs))
	for pid := range peerIDs {
		if p, ok := m.peers[pid]; ok {
			peers = append(peers, p)
		}
	}
	m.mu.RUnlock()

	out := make([]WebRTCClientInfo, len(peers))
	for i, p := range peers {
		out[i] = WebRTCClientInfo{
			ID:             p.peerID,
			ConnectedSince: p.connectedAt.UTC().Format(time.RFC3339),
			BytesSent:      p.bytesSent.Load(),
			JitterMs:       PeerJitterMs(p.peerID),
		}
	}
	return out
}

// DisconnectPeer disconnects a single WebRTC peer by ID. Returns false if not found.
func (m *WebRTCManager) DisconnectPeer(peerID string) bool {
	m.mu.RLock()
	peer, ok := m.peers[peerID]
	m.mu.RUnlock()
	if !ok {
		return false
	}
	m.closePeer(peerID, peer.streamID, "api_disconnect")
	return true
}

// CloseStreamConsumers closes all WebRTC peers for a given stream.
func (m *WebRTCManager) CloseStreamConsumers(streamID string) {
	m.mu.Lock()
	peerIDs, exists := m.streamPeers[streamID]
	if !exists || len(peerIDs) == 0 {
		m.mu.Unlock()
		m.logger.Debug("No WebRTC consumers to close", logging.KeyStreamID, streamID)
		return
	}

	toClose := make([]string, 0, len(peerIDs))
	for peerID := range peerIDs {
		toClose = append(toClose, peerID)
	}
	m.mu.Unlock()

	m.logger.Info("Closing WebRTC consumers for stream restart", logging.KeyStreamID, streamID, logging.KeyStreamPeers, len(toClose))

	for _, peerID := range toClose {
		m.closePeer(peerID, streamID, "stream_restart")
	}
}
