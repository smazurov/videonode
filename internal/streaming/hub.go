package streaming

import (
	"errors"
	"log/slog"
	"sync"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/rtsp"
	"github.com/AlexxIT/go2rtc/pkg/webrtc"
	"github.com/pion/rtp"
)

// ErrStreamNotFound is returned when a requested stream doesn't exist.
var ErrStreamNotFound = errors.New("stream not found")

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

		// Find matching codec by name
		var consumerCodec *core.Codec
		for _, c := range matchedMedia.Codecs {
			if c.Name == receiver.Codec.Name {
				consumerCodec = c
				break
			}
		}

		if consumerCodec == nil {
			h.logger.Warn("No matching codec", "stream_id", streamID, "codec", receiver.Codec.Name)
			continue
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
