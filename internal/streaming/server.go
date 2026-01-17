package streaming

import (
	"errors"
	"net"
	"sync"

	"github.com/AlexxIT/go2rtc/pkg/rtsp"
	"github.com/smazurov/videonode/internal/logging"
)

// Server handles RTSP connections from FFmpeg (producers) and clients (consumers).
type Server struct {
	hub      *Hub
	listener net.Listener
	logger   logging.Logger
	wg       sync.WaitGroup
	closed   bool
	mu       sync.Mutex
}

// NewServer creates a new streaming server.
func NewServer(hub *Hub, logger logging.Logger) *Server {
	return &Server{
		hub:    hub,
		logger: logger,
	}
}

// Start begins listening for RTSP connections on the specified address.
func (s *Server) Start(addr string) error {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}

	s.mu.Lock()
	s.listener = ln
	s.closed = false
	s.mu.Unlock()

	s.logger.Info("RTSP server started", "addr", addr)

	go s.acceptLoop()

	return nil
}

// acceptLoop accepts incoming connections.
func (s *Server) acceptLoop() {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			s.mu.Lock()
			closed := s.closed
			s.mu.Unlock()

			if closed {
				return // Server is shutting down
			}
			s.logger.Error("Failed to accept connection", "error", err)
			continue
		}

		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			s.handleConn(conn)
		}()
	}
}

// handleConn processes an incoming RTSP connection.
func (s *Server) handleConn(conn net.Conn) {
	rtspConn := rtsp.NewServer(conn)
	var streamID string

	// Listen for RTSP method events
	rtspConn.Listen(func(msg any) {
		switch msg {
		case rtsp.MethodAnnounce:
			// FFmpeg pushing stream via ANNOUNCE
			if rtspConn.URL != nil && len(rtspConn.URL.Path) > 1 {
				streamID = rtspConn.URL.Path[1:] // Strip leading /
				s.hub.AddProducer(streamID, rtspConn)
				s.logger.Info("RTSP producer connected",
					"stream_id", streamID,
					"remote", conn.RemoteAddr())
			}

		case rtsp.MethodDescribe:
			// Client requesting stream via DESCRIBE
			if rtspConn.URL != nil && len(rtspConn.URL.Path) > 1 {
				clientStreamID := rtspConn.URL.Path[1:]
				if err := s.hub.WireConsumer(clientStreamID, rtspConn); err != nil {
					s.logger.Warn("Failed to wire RTSP consumer",
						"stream_id", clientStreamID,
						"error", err)
				} else {
					s.logger.Info("RTSP consumer connected",
						"stream_id", clientStreamID,
						"remote", conn.RemoteAddr())
				}
			}
		}
	})

	// Run RTSP state machine (OPTIONS, ANNOUNCE/DESCRIBE, SETUP, PLAY/RECORD)
	if err := rtspConn.Accept(); err != nil {
		if !errors.Is(err, net.ErrClosed) {
			s.logger.Debug("RTSP accept error", "error", err)
		}
		return
	}

	// Handle data transfer (blocks until connection closes)
	if err := rtspConn.Handle(); err != nil {
		if !errors.Is(err, net.ErrClosed) {
			s.logger.Debug("RTSP handle error", "error", err)
		}
	}

	// Clean up producer on disconnect
	if streamID != "" {
		s.hub.RemoveProducer(streamID)
		s.logger.Info("RTSP producer disconnected", "stream_id", streamID)
	}
}

// Stop gracefully shuts down the server.
func (s *Server) Stop() error {
	s.mu.Lock()
	s.closed = true
	s.mu.Unlock()

	if s.listener != nil {
		if err := s.listener.Close(); err != nil {
			return err
		}
	}

	// Wait for all connections to finish
	s.wg.Wait()

	// Clean up hub
	s.hub.Stop()

	s.logger.Info("RTSP server stopped")
	return nil
}

// Hub returns the server's stream hub.
func (s *Server) Hub() *Hub {
	return s.hub
}
