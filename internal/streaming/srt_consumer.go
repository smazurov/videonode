package streaming

import (
	"bufio"
	"io"
	"log/slog"
	"net"
	"sync"
	"time"

	srt "github.com/datarhei/gosrt"
	"github.com/smazurov/videonode/internal/logging"

	"github.com/bluenviron/gortsplib/v5/pkg/description"
	"github.com/bluenviron/gortsplib/v5/pkg/format"
	"github.com/bluenviron/mediacommon/v2/pkg/codecs/h264"
	"github.com/bluenviron/mediacommon/v2/pkg/formats/mpegts"
	"github.com/bluenviron/mediacommon/v2/pkg/formats/mpegts/codecs"
)

const srtStatsInterval = 5 * time.Second

const (
	srtWriteTimeout = 10 * time.Second
	srtBufferSize   = 188 * 7 // MPEG-TS packet size * 7 for efficient writes
)

// SRTConsumer writes MPEG-TS data to an SRT connection using mediacommon's mpegts.Writer.
// It creates a Reader to subscribe to the stream and muxes decoded access units
// into MPEG-TS format for delivery over SRT.
type SRTConsumer struct {
	reader     *Reader
	conn       net.Conn
	srtConn    srt.Conn // Typed SRT connection for Stats()
	streamID   string
	consumerID string
	logger     *slog.Logger
	writer     *mpegts.Writer
	bw         *bufio.Writer
	tracks     []*mpegts.Track
	trackMap   map[*description.Media]*mpegts.Track

	// H264 SPS/PPS for prepending to IDR frames
	h264SPS      []byte
	h264PPS      []byte
	firstH264IDR bool // Track if we've sent first IDR (skip P-frames until then)

	done   chan struct{}
	closed bool
	mu     sync.Mutex

	connectedAt time.Time
	bytesSent   int64
	lastRTTMs   float64

	// Stats tracking for delta calculation
	lastRetransmits uint64
	lastDropped     uint64
}

// NewSRTConsumer creates a new SRT consumer for the given stream.
func NewSRTConsumer(stream *Stream, consumerID string, srtConn srt.Conn, logger *slog.Logger) (*SRTConsumer, error) {
	c := &SRTConsumer{
		conn:        srtConn,
		srtConn:     srtConn,
		streamID:    stream.ID(),
		consumerID:  consumerID,
		logger:      logger,
		trackMap:    make(map[*description.Media]*mpegts.Track),
		done:        make(chan struct{}),
		connectedAt: time.Now(),
	}

	// Create MPEG-TS tracks for each media in the stream
	desc := stream.Description()
	for _, medi := range desc.Medias {
		for _, forma := range medi.Formats {
			track := c.createTrack(forma)
			if track != nil {
				c.tracks = append(c.tracks, track)
				c.trackMap[medi] = track

				// Store SPS/PPS for H264
				if h264Forma, ok := forma.(*format.H264); ok {
					c.h264SPS = h264Forma.SPS
					c.h264PPS = h264Forma.PPS
					c.logger.Debug("SRT consumer got H264 params",
						logging.KeyStreamID, c.streamID,
						logging.KeySPSLen, len(c.h264SPS),
						logging.KeyPPSLen, len(c.h264PPS))
				}
			}
		}
	}

	if len(c.tracks) == 0 {
		return nil, ErrNoSupportedCodecs
	}

	// Create buffered writer for efficient SRT writes
	c.bw = bufio.NewWriterSize(srtConn, srtBufferSize)

	// Initialize mpegts.Writer
	c.writer = &mpegts.Writer{
		W:      c.bw,
		Tracks: c.tracks,
	}
	if err := c.writer.Initialize(); err != nil {
		return nil, err
	}

	// Create reader and set up callbacks
	c.reader = NewReader(stream, consumerID)
	c.setupCallbacks(desc)

	// Start stats collection goroutine
	c.startStatsCollection()

	return c, nil
}

// createTrack creates an MPEG-TS track for the given format.
func (c *SRTConsumer) createTrack(forma format.Format) *mpegts.Track {
	switch f := forma.(type) {
	case *format.H264:
		_ = f // used for SPS/PPS extraction above
		return &mpegts.Track{
			Codec: &codecs.H264{},
		}
	case *format.H265:
		return &mpegts.Track{
			Codec: &codecs.H265{},
		}
	case *format.MPEG4Audio:
		return &mpegts.Track{
			Codec: &codecs.MPEG4Audio{
				Config: *f.Config,
			},
		}
	case *format.Opus:
		return &mpegts.Track{
			Codec: &codecs.Opus{
				ChannelCount: func() int {
					if f.ChannelCount > 0 {
						return f.ChannelCount
					}
					return 2
				}(),
			},
		}
	default:
		return nil
	}
}

// setupCallbacks configures the reader callbacks for each media track.
func (c *SRTConsumer) setupCallbacks(desc *description.Session) {
	for _, medi := range desc.Medias {
		track := c.trackMap[medi]
		if track == nil {
			continue
		}

		// Capture variables for closure
		currentMedia := medi
		currentTrack := track

		// Set up callback based on codec type
		for _, forma := range medi.Formats {
			switch forma.(type) {
			case *format.H264:
				c.reader.OnUnit(currentMedia, func(pts, dts int64, au [][]byte) error {
					return c.writeH264(currentTrack, pts, dts, au)
				})
			case *format.H265:
				c.reader.OnUnit(currentMedia, func(pts, dts int64, au [][]byte) error {
					return c.writeH265(currentTrack, pts, dts, au)
				})
			case *format.MPEG4Audio:
				c.reader.OnUnit(currentMedia, func(pts, _ int64, au [][]byte) error {
					return c.writeMPEG4Audio(currentTrack, pts, au)
				})
			case *format.Opus:
				c.reader.OnUnit(currentMedia, func(pts, _ int64, packets [][]byte) error {
					return c.writeOpus(currentTrack, pts, packets)
				})
			}
			break // Only handle first format per media
		}
	}
}

// writeH264 writes H264 access units to MPEG-TS.
// It prepends SPS/PPS before IDR frames to ensure decoders can initialize.
func (c *SRTConsumer) writeH264(track *mpegts.Track, pts, dts int64, au [][]byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return io.ErrClosedPipe
	}

	isIDR := h264.IsRandomAccess(au)

	// Skip frames until we receive the first IDR frame
	// This ensures decoder can initialize properly
	if !c.firstH264IDR {
		if !isIDR {
			return nil // Skip P/B frames before first IDR
		}
		c.firstH264IDR = true
		c.logger.Debug("SRT consumer received first IDR", logging.KeyStreamID, c.streamID)
	}

	// Prepend SPS/PPS before IDR frames
	if isIDR && len(c.h264SPS) > 0 && len(c.h264PPS) > 0 {
		// Check if SPS/PPS are already in the AU
		hasSPS := false
		hasPPS := false
		for _, nalu := range au {
			if len(nalu) > 0 {
				switch h264.NALUType(nalu[0] & 0x1F) {
				case h264.NALUTypeSPS:
					hasSPS = true
				case h264.NALUTypePPS:
					hasPPS = true
				}
			}
		}

		// Prepend SPS/PPS if not present
		if !hasSPS || !hasPPS {
			newAU := make([][]byte, 0, len(au)+2)
			if !hasSPS {
				newAU = append(newAU, c.h264SPS)
			}
			if !hasPPS {
				newAU = append(newAU, c.h264PPS)
			}
			newAU = append(newAU, au...)
			au = newAU
			c.logger.Debug("SRT consumer prepended SPS/PPS", logging.KeyStreamID, c.streamID)
		}
	}

	c.conn.SetWriteDeadline(time.Now().Add(srtWriteTimeout))

	if err := c.writer.WriteH264(track, pts, dts, au); err != nil {
		return err
	}

	// Flush the buffer after each frame
	if err := c.bw.Flush(); err != nil {
		return err
	}

	c.recordMetrics(au, "h264")
	return nil
}

// writeH265 writes H265 access units to MPEG-TS.
func (c *SRTConsumer) writeH265(track *mpegts.Track, pts, dts int64, au [][]byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return io.ErrClosedPipe
	}

	c.conn.SetWriteDeadline(time.Now().Add(srtWriteTimeout))

	if err := c.writer.WriteH265(track, pts, dts, au); err != nil {
		return err
	}

	if err := c.bw.Flush(); err != nil {
		return err
	}

	c.recordMetrics(au, "h265")
	return nil
}

// writeMPEG4Audio writes AAC audio to MPEG-TS.
func (c *SRTConsumer) writeMPEG4Audio(track *mpegts.Track, pts int64, aus [][]byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return io.ErrClosedPipe
	}

	c.conn.SetWriteDeadline(time.Now().Add(srtWriteTimeout))

	if err := c.writer.WriteMPEG4Audio(track, pts, aus); err != nil {
		return err
	}

	if err := c.bw.Flush(); err != nil {
		return err
	}

	c.recordMetrics(aus, "aac")
	return nil
}

// writeOpus writes Opus audio to MPEG-TS.
func (c *SRTConsumer) writeOpus(track *mpegts.Track, pts int64, packets [][]byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return io.ErrClosedPipe
	}

	c.conn.SetWriteDeadline(time.Now().Add(srtWriteTimeout))

	if err := c.writer.WriteOpus(track, pts, packets); err != nil {
		return err
	}

	if err := c.bw.Flush(); err != nil {
		return err
	}

	c.recordMetrics(packets, "opus")
	return nil
}

// recordMetrics records bytes sent and frames written for metrics.
// All callers hold c.mu.
func (c *SRTConsumer) recordMetrics(data [][]byte, codec string) {
	var bytes int
	for _, d := range data {
		bytes += len(d)
	}
	c.bytesSent += int64(bytes)
	IncrementSRTPacketsSent(c.streamID, bytes)
	IncrementSRTFramesWritten(c.streamID, codec)
}

// Stop closes the consumer and SRT connection.
func (c *SRTConsumer) Stop() error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil
	}
	c.closed = true
	close(c.done)
	c.mu.Unlock()

	// Close the reader first to stop receiving data
	c.reader.Close()

	// Close the connection
	if err := c.conn.Close(); err != nil {
		c.logger.Debug("SRT conn close error", logging.KeyStreamID, c.streamID, logging.KeyError, err)
	}

	return nil
}

// StreamID returns the stream ID this consumer is subscribed to.
func (c *SRTConsumer) StreamID() string {
	return c.streamID
}

// BytesSent returns the number of bytes sent.
func (c *SRTConsumer) BytesSent() int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.bytesSent
}

// startStatsCollection starts a goroutine that periodically collects SRT statistics.
func (c *SRTConsumer) startStatsCollection() {
	ticker := time.NewTicker(srtStatsInterval)
	go func() {
		defer ticker.Stop()
		var stats srt.Statistics
		for {
			select {
			case <-ticker.C:
				c.srtConn.Stats(&stats)
				UpdateSRTConsumerStats(c.streamID, c.consumerID, &stats)

				// Update counters with deltas
				c.mu.Lock()
				c.lastRTTMs = stats.Instantaneous.MsRTT
				retransDelta := stats.Accumulated.PktRetrans - c.lastRetransmits
				droppedDelta := stats.Accumulated.PktSendDrop - c.lastDropped
				c.lastRetransmits = stats.Accumulated.PktRetrans
				c.lastDropped = stats.Accumulated.PktSendDrop
				c.mu.Unlock()

				if retransDelta > 0 {
					srtConsumerRetransmits.WithLabelValues(c.streamID, c.consumerID).Add(float64(retransDelta))
				}
				if droppedDelta > 0 {
					srtConsumerDropped.WithLabelValues(c.streamID, c.consumerID).Add(float64(droppedDelta))
				}
			case <-c.done:
				return
			}
		}
	}()
}

// Info returns a point-in-time view for the consumer SSE event.
func (c *SRTConsumer) Info() SRTClientInfo {
	c.mu.Lock()
	defer c.mu.Unlock()
	return SRTClientInfo{
		ID:             c.consumerID,
		ConnectedSince: c.connectedAt.UTC().Format(time.RFC3339),
		BytesSent:      c.bytesSent,
		RTTMs:          c.lastRTTMs,
	}
}
