package streaming

import (
	"errors"
	"log/slog"
	"sync"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/rtsp"
)

// ErrStreamNotFound is returned when a requested stream doesn't exist.
var ErrStreamNotFound = errors.New("stream not found")

// Hub manages stream producers and routes consumers to them.
// Producers are RTSP connections from FFmpeg (ANNOUNCE).
// Consumers are RTSP clients (DESCRIBE) or WebRTC peers.
type Hub struct {
	producers map[string]*rtsp.Conn
	mu        sync.RWMutex
	logger    *slog.Logger
}

// NewHub creates a new stream hub.
func NewHub(logger *slog.Logger) *Hub {
	return &Hub{
		producers: make(map[string]*rtsp.Conn),
		logger:    logger,
	}
}

// AddProducer registers an RTSP producer (FFmpeg pushing via ANNOUNCE).
func (h *Hub) AddProducer(streamID string, conn *rtsp.Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()

	// Close existing producer if any
	if existing, ok := h.producers[streamID]; ok {
		h.logger.Info("Replacing existing producer", "stream_id", streamID)
		existing.Stop()
	}

	h.producers[streamID] = conn
	h.logger.Info("Producer added", "stream_id", streamID)
}

// RemoveProducer removes a producer from the hub.
func (h *Hub) RemoveProducer(streamID string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if conn, ok := h.producers[streamID]; ok {
		conn.Stop()
		delete(h.producers, streamID)
		h.logger.Info("Producer removed", "stream_id", streamID)
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

	// If consumer has no medias (RTSP playback), add all producer tracks directly
	if len(consumerMedias) == 0 {
		for _, receiver := range prod.Receivers {
			if err := cons.AddTrack(receiver.Media, receiver.Codec, receiver); err != nil {
				h.logger.Warn("Failed to add track", "error", err)
			}
		}
		return nil
	}

	// Match producer tracks to consumer medias by kind (for WebRTC)
	for _, receiver := range prod.Receivers {
		// Find matching consumer media by kind (video/audio)
		var matchedMedia *core.Media
		for _, m := range consumerMedias {
			if m.Kind == receiver.Media.Kind && m.Direction == core.DirectionSendonly {
				matchedMedia = m
				break
			}
		}

		if matchedMedia == nil {
			continue
		}

		// Find the matching codec from the consumer's media
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

		if err := cons.AddTrack(matchedMedia, consumerCodec, receiver); err != nil {
			h.logger.Warn("Failed to add track", "stream_id", streamID, "error", err)
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
		conn.Stop()
		delete(h.producers, id)
	}
	h.logger.Info("Hub stopped")
}
