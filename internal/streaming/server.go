package streaming

import (
	"errors"
	"net"
	"sync"

	"github.com/bluenviron/gortsplib/v5"
	"github.com/bluenviron/gortsplib/v5/pkg/base"
	"github.com/bluenviron/gortsplib/v5/pkg/description"
	"github.com/bluenviron/gortsplib/v5/pkg/format"
	"github.com/bluenviron/gortsplib/v5/pkg/format/rtph264"
	"github.com/bluenviron/mediacommon/v2/pkg/codecs/h264"
	"github.com/pion/rtp"
	"github.com/smazurov/videonode/internal/logging"
)

// Server handles RTSP connections from FFmpeg (producers) and clients (consumers).
type Server struct {
	streams       map[string]*Stream
	serverStreams map[string]*gortsplib.ServerStream  // gortsplib stream per ID (for RTSP consumers)
	publishers    map[string]*gortsplib.ServerSession // producer session per ID
	server        *gortsplib.Server
	logger        logging.Logger
	mu            sync.RWMutex
	closed        bool

	onProducerReplaced  func(streamID string)
	onProducerConnected func(streamID string)
}

// NewServer creates a new streaming server.
func NewServer(logger logging.Logger) *Server {
	return &Server{
		streams:       make(map[string]*Stream),
		serverStreams: make(map[string]*gortsplib.ServerStream),
		publishers:    make(map[string]*gortsplib.ServerSession),
		logger:        logger,
	}
}

// SetOnProducerReplaced sets the callback invoked when a producer is replaced or removed.
func (s *Server) SetOnProducerReplaced(callback func(streamID string)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onProducerReplaced = callback
}

// SetOnProducerConnected sets the callback invoked when a new producer connects
// and the stream is registered and ready for consumption.
func (s *Server) SetOnProducerConnected(callback func(streamID string)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onProducerConnected = callback
}

// Start begins listening for RTSP connections on the specified address.
func (s *Server) Start(addr string) error {
	s.mu.Lock()
	s.closed = false
	s.mu.Unlock()

	s.server = &gortsplib.Server{
		Handler:     s,
		RTSPAddress: addr,
	}

	err := s.server.Start()
	if err != nil {
		return err
	}

	s.logger.Info("RTSP server started", "addr", addr)
	return nil
}

// Stop gracefully shuts down the server.
func (s *Server) Stop() error {
	s.mu.Lock()
	s.closed = true
	s.mu.Unlock()

	if s.server != nil {
		s.server.Close()
	}

	s.mu.Lock()
	for _, stream := range s.streams {
		stream.CloseAllReaders()
	}
	for _, ss := range s.serverStreams {
		ss.Close()
	}
	s.streams = make(map[string]*Stream)
	s.serverStreams = make(map[string]*gortsplib.ServerStream)
	s.publishers = make(map[string]*gortsplib.ServerSession)
	s.mu.Unlock()

	s.logger.Info("RTSP server stopped")
	return nil
}

// GetStream returns the stream for a given ID.
func (s *Server) GetStream(streamID string) *Stream {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.streams[streamID]
}

// HasStream checks if a stream exists.
func (s *Server) HasStream(streamID string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, ok := s.streams[streamID]
	return ok
}

// ListStreams returns a list of all active stream IDs.
func (s *Server) ListStreams() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	ids := make([]string, 0, len(s.streams))
	for id := range s.streams {
		ids = append(ids, id)
	}
	return ids
}

// gortsplib.ServerHandler implementation

// OnConnOpen is called when a connection is opened.
func (s *Server) OnConnOpen(ctx *gortsplib.ServerHandlerOnConnOpenCtx) {
	s.logger.Debug("RTSP connection opened", "remote", ctx.Conn.NetConn().RemoteAddr())
}

// OnConnClose is called when a connection is closed.
func (s *Server) OnConnClose(ctx *gortsplib.ServerHandlerOnConnCloseCtx) {
	s.logger.Debug("RTSP connection closed", "remote", ctx.Conn.NetConn().RemoteAddr())
}

// OnSessionOpen is called when a session is opened.
func (s *Server) OnSessionOpen(ctx *gortsplib.ServerHandlerOnSessionOpenCtx) {
	s.logger.Debug("RTSP session opened", "remote", ctx.Conn.NetConn().RemoteAddr())
}

// OnSessionClose is called when a session is closed.
// Only tears down the stream if the disconnecting session is the publisher.
func (s *Server) OnSessionClose(ctx *gortsplib.ServerHandlerOnSessionCloseCtx) {
	s.logger.Debug("RTSP session closed")

	streamID := ctx.Session.Path()
	if streamID == "" || len(streamID) <= 1 {
		return
	}
	streamID = streamID[1:] // Remove leading /

	s.mu.RLock()
	isPublisher := s.publishers[streamID] == ctx.Session
	s.mu.RUnlock()

	if isPublisher {
		s.removeStream(streamID)
	}
}

// OnDescribe handles DESCRIBE requests (clients requesting stream info).
// Returns the stored ServerStream created during OnAnnounce.
func (s *Server) OnDescribe(ctx *gortsplib.ServerHandlerOnDescribeCtx) (*base.Response, *gortsplib.ServerStream, error) {
	streamID := extractStreamID(ctx.Path)

	s.mu.RLock()
	ss := s.serverStreams[streamID]
	s.mu.RUnlock()

	if ss == nil {
		return &base.Response{StatusCode: base.StatusNotFound}, nil, nil
	}

	return &base.Response{StatusCode: base.StatusOK}, ss, nil
}

// OnAnnounce handles ANNOUNCE requests (FFmpeg pushing a stream).
// Creates both a custom Stream (for WebRTC/SRT consumers) and a gortsplib
// ServerStream (for RTSP playback consumers).
func (s *Server) OnAnnounce(ctx *gortsplib.ServerHandlerOnAnnounceCtx) (*base.Response, error) {
	streamID := extractStreamID(ctx.Path)

	s.mu.Lock()
	var callback func(string)

	// Close existing stream if any
	if existing := s.streams[streamID]; existing != nil {
		s.logger.Info("Replacing existing producer", "stream_id", streamID)
		existing.CloseAllReaders()
		callback = s.onProducerReplaced
	}

	// Close existing ServerStream if any
	if oldSS := s.serverStreams[streamID]; oldSS != nil {
		oldSS.Close()
	}

	// Create new stream
	stream := NewStream(streamID, ctx.Description, s.logger)
	s.streams[streamID] = stream

	// Create and store ServerStream for RTSP playback consumers
	ss := &gortsplib.ServerStream{
		Server: s.server,
		Desc:   ctx.Description,
	}
	if err := ss.Initialize(); err != nil {
		delete(s.streams, streamID)
		s.mu.Unlock()
		s.logger.Error("Failed to initialize server stream", "error", err)
		return &base.Response{StatusCode: base.StatusInternalServerError}, err
	}
	s.serverStreams[streamID] = ss
	s.publishers[streamID] = ctx.Session
	s.mu.Unlock()

	// Notify about producer replacement (after unlock)
	if callback != nil {
		go callback(streamID)
	}

	// Notify that stream is now registered and ready for consumption
	if s.onProducerConnected != nil {
		go s.onProducerConnected(streamID)
	}

	s.logger.Info("RTSP producer connected", "stream_id", streamID, "remote", ctx.Conn.NetConn().RemoteAddr())

	return &base.Response{StatusCode: base.StatusOK}, nil
}

// OnSetup handles SETUP requests.
// Returns nil ServerStream for publishers (gortsplib v5 requirement),
// returns the stored ServerStream for consumers.
func (s *Server) OnSetup(ctx *gortsplib.ServerHandlerOnSetupCtx) (*base.Response, *gortsplib.ServerStream, error) {
	// Publishers must receive nil ServerStream — gortsplib v5 panics otherwise
	if ctx.Session.AnnouncedDescription() != nil {
		return &base.Response{StatusCode: base.StatusOK}, nil, nil
	}

	streamID := extractStreamID(ctx.Path)

	s.mu.RLock()
	ss := s.serverStreams[streamID]
	s.mu.RUnlock()

	if ss == nil {
		return &base.Response{StatusCode: base.StatusNotFound}, nil, nil
	}

	return &base.Response{StatusCode: base.StatusOK}, ss, nil
}

// OnPlay handles PLAY requests (clients starting playback).
func (s *Server) OnPlay(_ *gortsplib.ServerHandlerOnPlayCtx) (*base.Response, error) {
	return &base.Response{StatusCode: base.StatusOK}, nil
}

// OnRecord handles RECORD requests (FFmpeg starting to push data).
// Sets up RTP handlers that feed both the custom Stream (WebRTC/SRT) and
// the ServerStream (RTSP playback consumers).
func (s *Server) OnRecord(ctx *gortsplib.ServerHandlerOnRecordCtx) (*base.Response, error) {
	streamID := extractStreamID(ctx.Path)

	s.mu.RLock()
	stream := s.streams[streamID]
	ss := s.serverStreams[streamID]
	s.mu.RUnlock()

	if stream == nil {
		return &base.Response{StatusCode: base.StatusNotFound}, errors.New("stream not found")
	}

	// Set up RTP handlers for each media in the stream
	for _, medi := range stream.Description().Medias {
		for _, forma := range medi.Formats {
			switch f := forma.(type) {
			case *format.H264:
				setupH264Handler(ctx, stream, ss, medi, f, s.logger)
			case *format.H265:
				setupH265Handler(ctx, stream, ss, medi, f, s.logger)
			default:
				setupGenericHandler(ctx, stream, ss, medi, forma, s.logger)
			}
		}
	}

	s.logger.Info("RTSP recording started", "stream_id", streamID)
	return &base.Response{StatusCode: base.StatusOK}, nil
}

// setupH264Handler configures H264 RTP depacketization and distribution.
func setupH264Handler(ctx *gortsplib.ServerHandlerOnRecordCtx, stream *Stream, ss *gortsplib.ServerStream, medi *description.Media, forma *format.H264, logger logging.Logger) {
	dec, err := forma.CreateDecoder()
	if err != nil {
		logger.Error("Failed to create H264 decoder", "error", err)
		return
	}

	var dtsExtractor *h264.DTSExtractor
	var firstIDRReceived bool

	ctx.Session.OnPacketRTP(medi, forma, func(pkt *rtp.Packet) {
		// Feed RTSP playback consumers with raw RTP
		if ss != nil {
			if err := ss.WritePacketRTP(medi, pkt); err != nil {
				logger.Debug("RTSP relay write error", "error", err)
			}
		}

		pts, ok := ctx.Session.PacketPTS(medi, pkt)
		if !ok {
			return
		}

		au, err := dec.Decode(pkt)
		if err != nil {
			if !errors.Is(err, rtph264.ErrNonStartingPacketAndNoPrevious) &&
				!errors.Is(err, rtph264.ErrMorePacketsNeeded) {
				logger.Debug("H264 decode error", "error", err)
			}
			return
		}

		// Wait for first IDR frame before starting DTS extraction
		idrPresent := h264.IsRandomAccess(au)
		if !firstIDRReceived {
			if !idrPresent {
				return
			}
			firstIDRReceived = true
			dtsExtractor = &h264.DTSExtractor{}
			dtsExtractor.Initialize()
		}

		dts, err := dtsExtractor.Extract(au, pts)
		if err != nil {
			logger.Debug("DTS extraction error", "error", err)
			return
		}

		// Distribute raw RTP to WebRTC readers
		stream.WriteRTP(medi, forma, pkt)

		// Distribute decoded AU to SRT readers
		stream.WriteUnit(medi, forma, pts, dts, au)
	})
}

// setupH265Handler configures H265 RTP depacketization.
func setupH265Handler(ctx *gortsplib.ServerHandlerOnRecordCtx, stream *Stream, ss *gortsplib.ServerStream, medi *description.Media, forma *format.H265, logger logging.Logger) {
	dec, err := forma.CreateDecoder()
	if err != nil {
		logger.Error("Failed to create H265 decoder", "error", err)
		return
	}

	ctx.Session.OnPacketRTP(medi, forma, func(pkt *rtp.Packet) {
		if ss != nil {
			if err := ss.WritePacketRTP(medi, pkt); err != nil {
				logger.Debug("RTSP relay write error", "error", err)
			}
		}

		pts, ok := ctx.Session.PacketPTS(medi, pkt)
		if !ok {
			return
		}

		au, err := dec.Decode(pkt)
		if err != nil {
			return
		}

		// For H265, DTS = PTS for now (simplified)
		stream.WriteRTP(medi, forma, pkt)
		stream.WriteUnit(medi, forma, pts, pts, au)
	})
}

// setupGenericHandler handles other formats (audio, etc.).
func setupGenericHandler(ctx *gortsplib.ServerHandlerOnRecordCtx, stream *Stream, ss *gortsplib.ServerStream, medi *description.Media, forma format.Format, logger logging.Logger) {
	ctx.Session.OnPacketRTP(medi, forma, func(pkt *rtp.Packet) {
		if ss != nil {
			if err := ss.WritePacketRTP(medi, pkt); err != nil {
				logger.Debug("RTSP relay write error", "error", err)
			}
		}

		pts, ok := ctx.Session.PacketPTS(medi, pkt)
		if !ok {
			return
		}

		stream.WriteRTP(medi, forma, pkt)
		stream.WriteUnit(medi, forma, pts, pts, nil)
	})
}

// extractStreamID extracts the stream ID from the RTSP path.
func extractStreamID(path string) string {
	if len(path) > 1 && path[0] == '/' {
		return path[1:]
	}
	return path
}

// removeStream removes a stream when the producer disconnects.
func (s *Server) removeStream(streamID string) {
	s.mu.Lock()
	stream := s.streams[streamID]
	var callback func(string)
	if stream != nil {
		stream.CloseAllReaders()
		delete(s.streams, streamID)
		callback = s.onProducerReplaced
	}
	if ss := s.serverStreams[streamID]; ss != nil {
		ss.Close()
		delete(s.serverStreams, streamID)
	}
	delete(s.publishers, streamID)
	s.mu.Unlock()

	if callback != nil {
		go callback(streamID)
	}

	s.logger.Info("RTSP producer disconnected", "stream_id", streamID)
}

// OnPacketsLost handles packet loss notifications.
func (s *Server) OnPacketsLost(ctx *gortsplib.ServerHandlerOnPacketsLostCtx) {
	s.logger.Debug("RTSP packets lost", "count", ctx.Lost)
}

// OnDecodeError handles decode errors.
func (s *Server) OnDecodeError(ctx *gortsplib.ServerHandlerOnDecodeErrorCtx) {
	if !errors.Is(ctx.Error, net.ErrClosed) {
		s.logger.Debug("RTSP decode error", "error", ctx.Error)
	}
}

// OnStreamWriteError handles stream write errors.
func (s *Server) OnStreamWriteError(ctx *gortsplib.ServerHandlerOnStreamWriteErrorCtx) {
	s.logger.Debug("RTSP stream write error", "error", ctx.Error)
}

// CloseStreamConsumers closes all consumers for a specific stream.
func (s *Server) CloseStreamConsumers(streamID string) {
	s.mu.RLock()
	stream := s.streams[streamID]
	s.mu.RUnlock()

	if stream != nil {
		stream.CloseAllReaders()
	}
}

// GetStreamReaderCount returns the number of readers for a stream.
func (s *Server) GetStreamReaderCount(streamID string) int {
	s.mu.RLock()
	stream := s.streams[streamID]
	s.mu.RUnlock()

	if stream == nil {
		return 0
	}
	return stream.ReaderCount()
}
