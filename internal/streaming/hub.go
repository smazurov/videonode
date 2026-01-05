package streaming

import (
	"encoding/hex"
	"errors"
	"log/slog"
	"strings"
	"sync"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/rtsp"
	"github.com/AlexxIT/go2rtc/pkg/webrtc"
	"github.com/pion/rtp"
)

// ErrStreamNotFound is returned when a requested stream doesn't exist.
var ErrStreamNotFound = errors.New("stream not found")

// h264ProfileFromFmtp extracts the H264 profile byte from an fmtp line.
// Returns the profile_idc byte (e.g., 0x64 for High, 0x42 for Baseline).
// Checks profile-level-id first, falls back to sprop-parameter-sets SPS.
func h264ProfileFromFmtp(fmtp string) byte {
	// Try profile-level-id first (format: PPCCLL where PP is profile_idc)
	if idx := strings.Index(fmtp, "profile-level-id="); idx >= 0 {
		plid := fmtp[idx+17:]
		if end := strings.IndexAny(plid, ";, "); end > 0 {
			plid = plid[:end]
		}
		if len(plid) >= 2 {
			if b, err := hex.DecodeString(plid[:2]); err == nil && len(b) > 0 {
				return b[0]
			}
		}
	}

	// Fall back to SPS from sprop-parameter-sets
	profile, _ := core.DecodeH264(fmtp)
	switch profile {
	case "Baseline":
		return 0x42
	case "Main":
		return 0x4D
	case "Extended":
		return 0x58
	case "High":
		return 0x64
	}
	return 0
}

// Hub manages stream producers and routes consumers to them.
// Producers are RTSP connections from FFmpeg (ANNOUNCE).
// Consumers are RTSP clients (DESCRIBE) or WebRTC peers.
type Hub struct {
	producers          map[string]*rtsp.Conn
	mu                 sync.RWMutex
	logger             *slog.Logger
	onProducerReplaced func(streamID string)
}

// NewHub creates a new stream hub.
func NewHub(logger *slog.Logger) *Hub {
	return &Hub{
		producers: make(map[string]*rtsp.Conn),
		logger:    logger,
	}
}

// SetOnProducerReplaced sets the callback invoked when a producer is replaced or removed.
// This allows notifying WebRTC consumers to reconnect.
func (h *Hub) SetOnProducerReplaced(callback func(streamID string)) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.onProducerReplaced = callback
}

// AddProducer registers an RTSP producer (FFmpeg pushing via ANNOUNCE).
func (h *Hub) AddProducer(streamID string, conn *rtsp.Conn) {
	h.mu.Lock()

	// Close existing producer if any
	var callback func(streamID string)
	if existing, ok := h.producers[streamID]; ok {
		h.logger.Info("Replacing existing producer", "stream_id", streamID)
		_ = existing.Stop()
		callback = h.onProducerReplaced
	}

	h.producers[streamID] = conn
	h.logger.Info("Producer added", "stream_id", streamID)
	h.mu.Unlock()

	// Notify about producer replacement (after unlock to avoid deadlock)
	if callback != nil {
		go callback(streamID)
	}
}

// RemoveProducer removes a producer from the hub.
func (h *Hub) RemoveProducer(streamID string) {
	h.mu.Lock()

	var callback func(streamID string)
	if conn, ok := h.producers[streamID]; ok {
		_ = conn.Stop()
		delete(h.producers, streamID)
		callback = h.onProducerReplaced
		h.logger.Info("Producer removed", "stream_id", streamID)
	}
	h.mu.Unlock()

	// Notify about producer removal (after unlock to avoid deadlock)
	if callback != nil {
		go callback(streamID)
	}
}

// GetProducer returns the producer for a stream ID.
func (h *Hub) GetProducer(streamID string) *rtsp.Conn {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.producers[streamID]
}

// HasProducer checks if a producer exists for the given stream ID.
func (h *Hub) HasProducer(streamID string) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	_, ok := h.producers[streamID]
	return ok
}

// WireConsumer connects a consumer to a producer's tracks.
// The consumer will receive all media tracks from the producer.
func (h *Hub) WireConsumer(streamID string, cons core.Consumer) error {
	h.mu.RLock()
	prod := h.producers[streamID]
	h.mu.RUnlock()

	if prod == nil {
		return ErrStreamNotFound
	}

	consumerMedias := cons.GetMedias()
	h.logger.Debug("WireConsumer", "stream_id", streamID, "consumer_medias_count", len(consumerMedias))

	// If consumer has no medias (RTSP playback), add all producer tracks directly
	if len(consumerMedias) == 0 {
		for _, receiver := range prod.Receivers {
			// Construct media from codec (avoids deprecated receiver.Media)
			media := &core.Media{
				Kind:      core.GetKind(receiver.Codec.Name),
				Direction: core.DirectionRecvonly,
				Codecs:    []*core.Codec{receiver.Codec},
			}
			if err := cons.AddTrack(media, receiver.Codec, receiver); err != nil {
				h.logger.Warn("Failed to add track", "error", err)
			}
		}
		return nil
	}

	// Type-assert for WebRTC-specific RTP passthrough optimization
	webrtcConn, isWebRTC := cons.(*webrtc.Conn)

	// Match producer tracks to consumer medias by kind (for WebRTC)
	for _, receiver := range prod.Receivers {
		// Find matching consumer media by kind (video/audio)
		receiverKind := core.GetKind(receiver.Codec.Name)
		var matchedMedia *core.Media
		for _, m := range consumerMedias {
			if m.Kind == receiverKind && m.Direction == core.DirectionSendonly {
				matchedMedia = m
				break
			}
		}

		if matchedMedia == nil {
			continue
		}

		// Find the matching codec from the consumer's media
		// For H264, match by profile to ensure SDP reflects actual stream profile
		var consumerCodec *core.Codec
		var fallbackCodec *core.Codec

		producerProfile := byte(0)
		if receiver.Codec.Name == core.CodecH264 {
			producerProfile = h264ProfileFromFmtp(receiver.Codec.FmtpLine)
		}

		// First pass: find exact profile match or collect fallback
		for _, c := range matchedMedia.Codecs {
			if c.Name != receiver.Codec.Name {
				continue
			}

			// For non-H264 or unknown producer profile, take first match
			if receiver.Codec.Name != core.CodecH264 || producerProfile == 0 {
				consumerCodec = c
				break
			}

			// H264: check profile match
			consumerProfile := h264ProfileFromFmtp(c.FmtpLine)
			if consumerProfile == producerProfile {
				consumerCodec = c
				break
			}

			// Keep first H264 as fallback
			if fallbackCodec == nil {
				fallbackCodec = c
			}
		}

		// Use fallback if no exact profile match
		if consumerCodec == nil {
			consumerCodec = fallbackCodec
		}

		if consumerCodec == nil {
			h.logger.Warn("No matching codec", "stream_id", streamID, "codec", receiver.Codec.Name)
			continue
		}

		if receiver.Codec.Name == core.CodecH264 {
			consumerProfile := h264ProfileFromFmtp(consumerCodec.FmtpLine)
			h.logger.Debug("H264 codec selected",
				"stream_id", streamID,
				"producer_profile", producerProfile,
				"consumer_profile", consumerProfile,
				"producer_fmtp", receiver.Codec.FmtpLine)
		}

		// Track sender count for RTP passthrough optimization
		var senderCountBefore int
		if isWebRTC {
			senderCountBefore = len(webrtcConn.Senders)
		}

		if err := cons.AddTrack(matchedMedia, consumerCodec, receiver); err != nil {
			h.logger.Warn("Failed to add track", "stream_id", streamID, "error", err)
			continue
		}

		// H264 RTP passthrough: skip depay/repay when source is already RTP
		// Uses zero-copy forwarding with SPS/PPS injection
		if isWebRTC && receiver.Codec.IsRTP() &&
			receiver.Codec.Name == core.CodecH264 &&
			len(webrtcConn.Senders) > senderCountBefore {
			sender := webrtcConn.Senders[len(webrtcConn.Senders)-1]
			localTrack := webrtcConn.GetSenderTrack(matchedMedia.ID)
			payloadType := consumerCodec.PayloadType

			if localTrack != nil {
				streamHandler := newH264StreamHandler(receiver.Codec, func(packet *rtp.Packet) {
					size := packet.MarshalSize()
					webrtcConn.Send += size
					IncrementPacketsSent(streamID, size)
					_ = localTrack.WriteRTP(payloadType, packet)
				})
				sender.Handler = streamHandler.handlePacket
			}
		}
	}

	return nil
}

// ListStreams returns a list of all active stream IDs.
func (h *Hub) ListStreams() []string {
	h.mu.RLock()
	defer h.mu.RUnlock()

	streams := make([]string, 0, len(h.producers))
	for id := range h.producers {
		streams = append(streams, id)
	}
	return streams
}

// Stop closes all producers and cleans up.
func (h *Hub) Stop() {
	h.mu.Lock()
	defer h.mu.Unlock()

	for id, conn := range h.producers {
		_ = conn.Stop()
		delete(h.producers, id)
	}
	h.logger.Info("Hub stopped")
}
