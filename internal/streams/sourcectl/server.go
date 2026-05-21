package sourcectl

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/creachadair/jrpc2"
	"github.com/creachadair/jrpc2/channel"
	"github.com/creachadair/jrpc2/handler"
	"github.com/creachadair/jrpc2/server"

	"github.com/smazurov/videonode/internal/logging"
)

// DefaultSocketPath is the well-known control socket location used by the
// daemon. /tmp matches the data-plane socket convention
// (see streams.SocketPathFor) and avoids the root-owned-/run problem;
// override at construction time if /run/videonode is provisioned.
const DefaultSocketPath = "/tmp/videonode-control.sock"

// HeartbeatInterval is the expected cadence of `status` notifications
// from each sidecar. The watchdog disconnects any connection that goes
// silent for more than HeartbeatTimeout = 3*HeartbeatInterval.
const (
	HeartbeatInterval = 1 * time.Second
	HeartbeatTimeout  = 3 * HeartbeatInterval
)

// Server is the daemon-side control plane. Each connected
// videonode-source dials this server and identifies itself with a stable
// device ID; the server then exposes per-device command dispatch
// (SendSetFormat) and a fan-out channel of status notifications.
type Server struct {
	socketPath string
	logger     logging.Logger
	statusCh   chan StatusParams

	mu       sync.RWMutex
	sidecars map[string]*sidecarConn

	listener net.Listener
	ctx      context.Context
	cancel   context.CancelFunc
	loopDone chan struct{}
}

// sidecarConn is the per-connection record. LastSeen is updated by the
// status handler; the watchdog uses it to detect a wedged sidecar.
type sidecarConn struct {
	srv      *jrpc2.Server
	deviceID string
	lastSeen atomic.Int64 // unix nanos
	closeFn  context.CancelFunc
}

// New constructs an unstarted server. Call Start to bind the socket and
// begin accepting sidecar connections.
func New(socketPath string, logger logging.Logger) *Server {
	if socketPath == "" {
		socketPath = DefaultSocketPath
	}
	if logger == nil {
		logger = logging.GetLogger("sourcectl")
	}
	return &Server{
		socketPath: socketPath,
		logger:     logger,
		statusCh:   make(chan StatusParams, 64),
		sidecars:   make(map[string]*sidecarConn),
	}
}

// SocketPath returns the path the server binds (or will bind).
func (s *Server) SocketPath() string { return s.socketPath }

// StatusFeed returns the read-only channel of status notifications from
// any connected sidecar. The channel is buffered (64 slots). On overflow
// the oldest pending event is dropped silently — heartbeat catches up.
func (s *Server) StatusFeed() <-chan StatusParams { return s.statusCh }

// Start binds the UDS, sets permissions, and begins accepting sidecar
// connections in a background goroutine. The control socket is removed
// first (after a safety check that it really is a socket node).
func (s *Server) Start(ctx context.Context) error {
	if err := removeStaleSocket(s.socketPath); err != nil {
		return fmt.Errorf("sourcectl: remove stale socket: %w", err)
	}
	if dir := dirOf(s.socketPath); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("sourcectl: mkdir %s: %w", dir, err)
		}
	}
	lst, err := net.Listen("unix", s.socketPath)
	if err != nil {
		return fmt.Errorf("sourcectl: listen %s: %w", s.socketPath, err)
	}
	// Permit any local user in the same group to dial in.
	if err := os.Chmod(s.socketPath, 0o660); err != nil {
		_ = lst.Close()
		return fmt.Errorf("sourcectl: chmod: %w", err)
	}
	s.listener = lst

	s.ctx, s.cancel = context.WithCancel(ctx)
	s.loopDone = make(chan struct{})
	go s.runLoop()
	s.logger.Info("sourcectl: listening", "socket", s.socketPath)
	return nil
}

// Stop closes the listener, terminates all connections, and waits for
// the accept loop to exit. Idempotent.
func (s *Server) Stop() error {
	if s.cancel != nil {
		s.cancel()
	}
	if s.listener != nil {
		_ = s.listener.Close()
	}
	if s.loopDone != nil {
		<-s.loopDone
	}
	return nil
}

func (s *Server) runLoop() {
	defer close(s.loopDone)
	acc := server.NetAccepter(s.listener, channel.Line)
	opts := &server.LoopOptions{
		ServerOptions: &jrpc2.ServerOptions{
			AllowPush: true, // enables srv.Callback / srv.Notify for set_format
		},
	}
	if err := server.Loop(s.ctx, acc, s.newService, opts); err != nil &&
		!errors.Is(err, net.ErrClosed) {
		s.logger.Warn("sourcectl: accept loop ended", "error", err)
	}
}

func (s *Server) newService() server.Service {
	return &connService{parent: s, conn: &sidecarConn{}}
}

// SendSetFormat dispatches a set_format command to the sidecar
// registered under deviceID. Returns an error if no such sidecar is
// connected, or if the call fails or times out.
func (s *Server) SendSetFormat(ctx context.Context, deviceID string, p SetFormatParams) (SetFormatResult, error) {
	s.mu.RLock()
	c, ok := s.sidecars[deviceID]
	s.mu.RUnlock()
	if !ok {
		return SetFormatResult{}, fmt.Errorf("sourcectl: no sidecar for device %q", deviceID)
	}
	rsp, err := c.srv.Callback(ctx, "set_format", p)
	if err != nil {
		return SetFormatResult{}, err
	}
	var out SetFormatResult
	if err := rsp.UnmarshalResult(&out); err != nil {
		return SetFormatResult{}, fmt.Errorf("sourcectl: bad set_format result: %w", err)
	}
	return out, nil
}

// ConnectedDevices returns a snapshot of the currently-identified
// sidecar device IDs.
func (s *Server) ConnectedDevices() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]string, 0, len(s.sidecars))
	for id := range s.sidecars {
		out = append(out, id)
	}
	return out
}

// ============================================================
// Per-connection service.
// ============================================================

type connService struct {
	parent *Server
	conn   *sidecarConn
}

// Assigner returns the per-connection RPC method dispatch table.
func (cs *connService) Assigner() (jrpc2.Assigner, error) {
	return handler.Map{
		"identify": handler.New(cs.handleIdentify),
		"status":   handler.New(cs.handleStatus),
	}, nil
}

// Finish runs after the per-connection server exits. It deregisters the
// device and cancels the heartbeat watchdog.
func (cs *connService) Finish(_ jrpc2.Assigner, _ jrpc2.ServerStatus) {
	if cs.conn.closeFn != nil {
		cs.conn.closeFn()
	}
	if cs.conn.deviceID != "" {
		cs.parent.mu.Lock()
		if existing, ok := cs.parent.sidecars[cs.conn.deviceID]; ok && existing == cs.conn {
			delete(cs.parent.sidecars, cs.conn.deviceID)
		}
		cs.parent.mu.Unlock()
		cs.parent.logger.Info("sourcectl: sidecar disconnected", "device_id", cs.conn.deviceID)
	}
}

// handleIdentify accepts the first message of every connection, binding
// the connection to its stable device ID. Subsequent identify calls on
// the same connection are rejected. If the same device ID was already
// registered on a different connection, the older connection is
// disconnected (last-write-wins — typically a sidecar respawn).
func (cs *connService) handleIdentify(ctx context.Context, p IdentifyParams) (struct{}, error) {
	if p.DeviceID == "" {
		return struct{}{}, errors.New("device_id required")
	}
	if cs.conn.deviceID != "" {
		return struct{}{}, fmt.Errorf("already identified as %q", cs.conn.deviceID)
	}
	srv := jrpc2.ServerFromContext(ctx)
	cs.conn.srv = srv
	cs.conn.deviceID = p.DeviceID
	cs.conn.lastSeen.Store(time.Now().UnixNano())

	cs.parent.mu.Lock()
	if old, ok := cs.parent.sidecars[p.DeviceID]; ok {
		cs.parent.logger.Warn("sourcectl: evicting prior sidecar for device",
			"device_id", p.DeviceID, "old_pid", old.deviceID)
		old.srv.Stop()
	}
	cs.parent.sidecars[p.DeviceID] = cs.conn
	cs.parent.mu.Unlock()

	// Start the per-connection heartbeat watchdog.
	watchCtx, cancel := context.WithCancel(cs.parent.ctx)
	cs.conn.closeFn = cancel
	go cs.watchHeartbeat(watchCtx)

	cs.parent.logger.Info("sourcectl: sidecar identified",
		"device_id", p.DeviceID, "pid", p.PID, "version", p.Version)
	return struct{}{}, nil
}

// handleStatus consumes a status notification and forwards it to the
// fan-out channel.
func (cs *connService) handleStatus(_ context.Context, p StatusParams) (struct{}, error) {
	if cs.conn.deviceID == "" {
		// Pre-identify status — drop silently.
		return struct{}{}, nil
	}
	cs.conn.lastSeen.Store(time.Now().UnixNano())
	// Defensive: prefer the connection-bound device ID over whatever the
	// sidecar self-reports, in case of mismatch.
	if p.DeviceID == "" {
		p.DeviceID = cs.conn.deviceID
	}
	select {
	case cs.parent.statusCh <- p:
	default:
		cs.parent.logger.Warn("sourcectl: status fan-out buffer full",
			"device_id", p.DeviceID)
	}
	return struct{}{}, nil
}

// watchHeartbeat closes the connection if no status notification has
// arrived for HeartbeatTimeout. Runs until the parent context is
// cancelled or until the connection is closed.
func (cs *connService) watchHeartbeat(ctx context.Context) {
	t := time.NewTicker(HeartbeatInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			last := cs.conn.lastSeen.Load()
			if time.Since(time.Unix(0, last)) > HeartbeatTimeout {
				cs.parent.logger.Warn("sourcectl: sidecar heartbeat lost",
					"device_id", cs.conn.deviceID)
				cs.conn.srv.Stop()
				return
			}
		}
	}
}

// ============================================================
// Helpers.
// ============================================================

func removeStaleSocket(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	// Only remove if it's actually a socket. Don't blow away a real file.
	if info.Mode()&os.ModeSocket == 0 {
		return fmt.Errorf("path %s exists but is not a socket (mode=%s)", path, info.Mode())
	}
	return os.Remove(path)
}

func dirOf(p string) string {
	for i := len(p) - 1; i >= 0; i-- {
		if p[i] == '/' {
			return p[:i]
		}
	}
	return ""
}
