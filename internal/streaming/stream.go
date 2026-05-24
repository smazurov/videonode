package streaming

import (
	"sync"
	"time"

	"github.com/bluenviron/gortsplib/v5/pkg/description"
	"github.com/bluenviron/gortsplib/v5/pkg/format"
	"github.com/pion/rtp"
	"github.com/smazurov/videonode/internal/logging"
)

// StreamProvider provides read-only access to the stream registry.
// Consumers (WebRTC, SRT) depend on this interface instead of *Server directly.
type StreamProvider interface {
	GetStream(id string) *Stream
	HasStream(id string) bool
	ListStreams() []string
	GetStreamReaderCount(id string) int
	// EnsureStreamReady returns an existing stream or kicks the lazy-start
	// hook and waits up to timeout for OnAnnounce.
	EnsureStreamReady(id string, timeout time.Duration) *Stream
}

// OnDataFunc is called when media data is received.
// For video: pts/dts are in nanosecond units (90kHz clock), au contains NAL units.
// For audio: pts is the presentation timestamp, au contains audio samples.
type OnDataFunc func(pts int64, dts int64, au [][]byte) error

// Stream represents a single media stream from a producer (FFmpeg via RTSP).
// Multiple readers (WebRTC, SRT) can subscribe to receive media data.
type Stream struct {
	id          string
	desc        *description.Session
	readers     map[*Reader]struct{}
	mu          sync.RWMutex
	logger      logging.Logger
	onNoReaders func(streamID string)
}

// NewStream creates a new stream with the given session description.
func NewStream(id string, desc *description.Session, logger logging.Logger) *Stream {
	return &Stream{
		id:      id,
		desc:    desc,
		readers: make(map[*Reader]struct{}),
		logger:  logger,
	}
}

// ID returns the stream identifier.
func (s *Stream) ID() string {
	return s.id
}

// Description returns the session description.
func (s *Stream) Description() *description.Session {
	return s.desc
}

// SetOnNoReaders sets a callback invoked when the last reader disconnects.
func (s *Stream) SetOnNoReaders(cb func(streamID string)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onNoReaders = cb
}

// AddReader adds a reader to receive media data.
func (s *Stream) AddReader(r *Reader) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.readers[r] = struct{}{}
	s.logger.Debug("Reader added to stream", "stream_id", s.id, "reader_count", len(s.readers))
}

// RemoveReader removes a reader.
func (s *Stream) RemoveReader(r *Reader) {
	s.mu.Lock()
	count := len(s.readers)
	delete(s.readers, r)
	cb := s.onNoReaders
	newCount := len(s.readers)
	s.mu.Unlock()

	s.logger.Debug("Reader removed from stream", "stream_id", s.id, "reader_count", newCount)

	if count > 0 && newCount == 0 && cb != nil {
		go cb(s.id)
	}
}

// ReaderCount returns the number of active readers.
func (s *Stream) ReaderCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.readers)
}

// WriteRTP distributes an RTP packet to all readers.
// Called by the RTSP server when packets arrive from FFmpeg.
func (s *Stream) WriteRTP(medi *description.Media, forma format.Format, pkt *rtp.Packet) {
	s.mu.RLock()
	readers := make([]*Reader, 0, len(s.readers))
	for r := range s.readers {
		readers = append(readers, r)
	}
	s.mu.RUnlock()

	for _, r := range readers {
		r.writeRTP(medi, forma, pkt)
	}
}

// WriteUnit distributes a decoded access unit to all readers.
// This is used after RTP depacketization.
// PTS and DTS are in 90kHz clock units.
func (s *Stream) WriteUnit(medi *description.Media, forma format.Format, pts int64, dts int64, au [][]byte) {
	s.mu.RLock()
	readers := make([]*Reader, 0, len(s.readers))
	for r := range s.readers {
		readers = append(readers, r)
	}
	s.mu.RUnlock()

	for _, r := range readers {
		r.writeUnit(medi, forma, pts, dts, au)
	}
}

// CloseAllReaders closes all active readers.
func (s *Stream) CloseAllReaders() {
	s.mu.Lock()
	readers := make([]*Reader, 0, len(s.readers))
	for r := range s.readers {
		readers = append(readers, r)
	}
	s.mu.Unlock()

	for _, r := range readers {
		r.Close()
	}
}

// Reader receives media data from a Stream.
// Each consumer (WebRTC peer, SRT connection) creates its own Reader.
type Reader struct {
	stream  *Stream
	id      string
	onRTP   map[*description.Media]func(*rtp.Packet)
	onUnit  map[*description.Media]OnDataFunc
	mu      sync.RWMutex
	closed  bool
	onClose func()
}

// NewReader creates a new reader attached to a stream.
func NewReader(stream *Stream, id string) *Reader {
	r := &Reader{
		stream: stream,
		id:     id,
		onRTP:  make(map[*description.Media]func(*rtp.Packet)),
		onUnit: make(map[*description.Media]OnDataFunc),
	}
	stream.AddReader(r)
	return r
}

// ID returns the reader identifier.
func (r *Reader) ID() string {
	return r.id
}

// Stream returns the parent stream.
func (r *Reader) Stream() *Stream {
	return r.stream
}

// OnRTP sets a callback for raw RTP packets from a specific media.
// Used by WebRTC for zero-copy RTP forwarding.
func (r *Reader) OnRTP(medi *description.Media, cb func(*rtp.Packet)) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.onRTP[medi] = cb
}

// OnUnit sets a callback for decoded access units from a specific media.
// Used by SRT for MPEG-TS muxing.
func (r *Reader) OnUnit(medi *description.Media, cb OnDataFunc) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.onUnit[medi] = cb
}

// SetOnClose sets a callback invoked when the reader is closed.
func (r *Reader) SetOnClose(cb func()) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.onClose = cb
}

// Close removes the reader from its stream and invokes the close callback.
func (r *Reader) Close() {
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return
	}
	r.closed = true
	cb := r.onClose
	r.mu.Unlock()

	r.stream.RemoveReader(r)
	if cb != nil {
		cb()
	}
}

// writeRTP is called by the stream to deliver RTP packets.
func (r *Reader) writeRTP(medi *description.Media, _ format.Format, pkt *rtp.Packet) {
	r.mu.RLock()
	cb := r.onRTP[medi]
	closed := r.closed
	r.mu.RUnlock()

	if closed || cb == nil {
		return
	}
	cb(pkt)
}

// writeUnit is called by the stream to deliver decoded access units.
func (r *Reader) writeUnit(medi *description.Media, _ format.Format, pts, dts int64, au [][]byte) {
	r.mu.RLock()
	cb := r.onUnit[medi]
	closed := r.closed
	r.mu.RUnlock()

	if closed || cb == nil {
		return
	}
	_ = cb(pts, dts, au)
}
