package nats

import (
	"fmt"
	"log/slog"
	"time"

	"github.com/nats-io/nats-server/v2/server"
)

// ServerOptions configures the embedded NATS server.
type ServerOptions struct {
	Port   int
	Host   string
	Name   string
	Logger *slog.Logger
}

// DefaultServerOptions returns sensible defaults for the embedded server.
func DefaultServerOptions() ServerOptions {
	return ServerOptions{
		Port: 4222,
		Host: "127.0.0.1",
		Name: "videonode",
	}
}

// Server wraps an embedded NATS server.
type Server struct {
	ns     *server.Server
	opts   ServerOptions
	logger *slog.Logger
}

// NewServer creates a new embedded NATS server.
func NewServer(opts ServerOptions) *Server {
	if opts.Port == 0 {
		opts.Port = 4222
	}
	if opts.Host == "" {
		opts.Host = "127.0.0.1"
	}
	if opts.Name == "" {
		opts.Name = "videonode"
	}

	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}

	return &Server{
		opts:   opts,
		logger: logger.With("component", "nats-server"),
	}
}

// Start starts the embedded NATS server and waits for it to be ready.
func (s *Server) Start() error {
	nsOpts := &server.Options{
		Host:           s.opts.Host,
		Port:           s.opts.Port,
		ServerName:     s.opts.Name,
		NoLog:          true, // We handle logging ourselves
		NoSigs:         true, // Don't handle signals (main process does this)
		MaxControlLine: 4096,
		MaxPayload:     1024 * 1024, // 1MB
	}

	ns, err := server.NewServer(nsOpts)
	if err != nil {
		return fmt.Errorf("failed to create NATS server: %w", err)
	}

	// Start the server in a goroutine
	go ns.Start()

	// Wait for the server to be ready (with timeout)
	if !ns.ReadyForConnections(5 * time.Second) {
		ns.Shutdown()
		return fmt.Errorf("NATS server failed to start within 5 seconds")
	}

	s.ns = ns
	s.logger.Info("NATS server started", "url", s.ClientURL())

	return nil
}

// Stop gracefully shuts down the NATS server.
func (s *Server) Stop() {
	if s.ns != nil {
		s.logger.Info("Stopping NATS server")
		s.ns.Shutdown()
		s.ns.WaitForShutdown()
		s.ns = nil
	}
}

// ClientURL returns the URL clients should use to connect.
func (s *Server) ClientURL() string {
	if s.ns == nil {
		return fmt.Sprintf("nats://%s:%d", s.opts.Host, s.opts.Port)
	}
	return s.ns.ClientURL()
}

// IsRunning returns true if the server is running and accepting connections.
func (s *Server) IsRunning() bool {
	return s.ns != nil && s.ns.Running()
}

// NumClients returns the number of connected clients.
func (s *Server) NumClients() int {
	if s.ns == nil {
		return 0
	}
	return s.ns.NumClients()
}
