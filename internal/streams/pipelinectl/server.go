package pipelinectl

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

// Server is the daemon-side control plane. Each connected sidecar dials
// this server and identifies itself with a kind + ID. Sources land in
// `sidecars` (keyed by DeviceID); composers land in `composers` (keyed
// by ComposerID — which the daemon assigns per stream-id). One UDS
// serves both because the routing branches on identify.kind.
type Server struct {
	socketPath string
	logger     logging.Logger
	statusCh   chan StatusParams

	mu        sync.RWMutex
	clients   map[string]*clientConn
	composers map[string]*clientConn

	listener net.Listener
	ctx      context.Context
	cancel   context.CancelFunc
	loopDone chan struct{}
}

// clientConn is the per-connection record. LastSeen is updated by the
// status handler; the watchdog uses it to detect a wedged sidecar.
// `kind` discriminates source vs composer connections.
type clientConn struct {
	srv      *jrpc2.Server
	deviceID string
	kind     string       // "source" (default) or "composer"
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
		clients:    make(map[string]*clientConn),
		composers:  make(map[string]*clientConn),
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
		return fmt.Errorf("pipelinectl: remove stale socket: %w", err)
	}
	if dir := dirOf(s.socketPath); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("pipelinectl: mkdir %s: %w", dir, err)
		}
	}
	lst, err := net.Listen("unix", s.socketPath)
	if err != nil {
		return fmt.Errorf("pipelinectl: listen %s: %w", s.socketPath, err)
	}
	// Permit any local user in the same group to dial in.
	if err := os.Chmod(s.socketPath, 0o660); err != nil {
		_ = lst.Close()
		return fmt.Errorf("pipelinectl: chmod: %w", err)
	}
	s.listener = lst

	s.ctx, s.cancel = context.WithCancel(ctx)
	s.loopDone = make(chan struct{})
	go s.runLoop()
	s.logger.Info("pipelinectl: listening", "socket", s.socketPath)
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
		s.logger.Warn("pipelinectl: accept loop ended", "error", err)
	}
}

func (s *Server) newService() server.Service {
	return &connService{parent: s, conn: &clientConn{}}
}

// SendSetFormat dispatches a set_format command to the sidecar
// registered under deviceID. Returns an error if no such sidecar is
// connected, or if the call fails or times out.
func (s *Server) SendSetFormat(ctx context.Context, deviceID string, p SetFormatParams) (SetFormatResult, error) {
	s.mu.RLock()
	c, ok := s.clients[deviceID]
	s.mu.RUnlock()
	if !ok {
		return SetFormatResult{}, fmt.Errorf("pipelinectl: no source for device %q", deviceID)
	}
	rsp, err := c.srv.Callback(ctx, "set_format", p)
	if err != nil {
		return SetFormatResult{}, err
	}
	var out SetFormatResult
	if err := rsp.UnmarshalResult(&out); err != nil {
		return SetFormatResult{}, fmt.Errorf("pipelinectl: bad set_format result: %w", err)
	}
	return out, nil
}

// ConnectedDevices returns a snapshot of the currently-identified
// sidecar device IDs.
func (s *Server) ConnectedDevices() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]string, 0, len(s.clients))
	for id := range s.clients {
		out = append(out, id)
	}
	return out
}

// ConnectedComposers returns a snapshot of the currently-identified
// composer IDs.
func (s *Server) ConnectedComposers() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]string, 0, len(s.composers))
	for id := range s.composers {
		out = append(out, id)
	}
	return out
}

// composerConn looks up a connected composer by id. Returns the
// connection or an error if no composer is currently identified.
func (s *Server) composerConn(composerID string) (*clientConn, error) {
	s.mu.RLock()
	c, ok := s.composers[composerID]
	s.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("pipelinectl: no composer for id %q", composerID)
	}
	return c, nil
}

// callComposerVoid invokes a method on the composer that returns an
// empty result object. All daemon→composer pushes today have empty
// results; this helper threads the call/marshal/error path once.
func (s *Server) callComposerVoid(ctx context.Context, composerID, method string, p any) error {
	c, err := s.composerConn(composerID)
	if err != nil {
		return err
	}
	if _, err := c.srv.Callback(ctx, method, p); err != nil {
		return fmt.Errorf("pipelinectl: composer %s call %s: %w", composerID, method, err)
	}
	return nil
}

// SendSetCanvas pushes set_canvas to the given composer.
func (s *Server) SendSetCanvas(ctx context.Context, composerID string, p SetCanvasParams) error {
	return s.callComposerVoid(ctx, composerID, "set_canvas", p)
}

// SendSetSource binds a slot to a SCM-publishing source.
func (s *Server) SendSetSource(ctx context.Context, composerID string, p SetSourceParams) error {
	return s.callComposerVoid(ctx, composerID, "set_source", p)
}

// SendClearSource unbinds a slot.
func (s *Server) SendClearSource(ctx context.Context, composerID string, p ClearSourceParams) error {
	return s.callComposerVoid(ctx, composerID, "clear_source", p)
}

// SendSetLayout replaces the composer's whole layout.
func (s *Server) SendSetLayout(ctx context.Context, composerID string, p SetLayoutParams) error {
	return s.callComposerVoid(ctx, composerID, "set_layout", p)
}

// SendSetEffects atomically replaces the effect list for one source on
// the composer.
func (s *Server) SendSetEffects(ctx context.Context, composerID string, p SetEffectsParams) error {
	return s.callComposerVoid(ctx, composerID, "set_effects", p)
}

// SendSetSourceState pushes a per-source state update (live /
// transitioning / placeholder) to the composer.
func (s *Server) SendSetSourceState(ctx context.Context, composerID string, p SetSourceStateParams) error {
	return s.callComposerVoid(ctx, composerID, "set_source_state", p)
}

// ============================================================
// Per-connection service.
// ============================================================

type connService struct {
	parent *Server
	conn   *clientConn
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
	if cs.conn.deviceID == "" {
		return
	}
	cs.parent.mu.Lock()
	switch cs.conn.kind {
	case "composer":
		if existing, ok := cs.parent.composers[cs.conn.deviceID]; ok && existing == cs.conn {
			delete(cs.parent.composers, cs.conn.deviceID)
		}
	default: // "source"
		if existing, ok := cs.parent.clients[cs.conn.deviceID]; ok && existing == cs.conn {
			delete(cs.parent.clients, cs.conn.deviceID)
		}
	}
	cs.parent.mu.Unlock()
	cs.parent.logger.Info("pipelinectl: client disconnected",
		"id", cs.conn.deviceID, "kind", cs.conn.kind)
}

// handleIdentify accepts the first message of every connection, binding
// the connection to its stable device ID. Subsequent identify calls on
// the same connection are rejected. If the same device ID was already
// registered on a different connection, the older connection is
// disconnected (last-write-wins — typically a sidecar respawn).
//
// Kind selects which registry to land in. "" / "source" → sidecars,
// "composer" → composers. Composers don't have heartbeats — they don't
// emit `status` notifications (yet) — so we skip the watchdog for them.
func (cs *connService) handleIdentify(ctx context.Context, p IdentifyParams) (struct{}, error) {
	if p.DeviceID == "" {
		return struct{}{}, errors.New("device_id required")
	}
	if cs.conn.deviceID != "" {
		return struct{}{}, fmt.Errorf("already identified as %q", cs.conn.deviceID)
	}
	kind := p.Kind
	if kind == "" {
		kind = "source"
	}
	if kind != "source" && kind != "composer" {
		return struct{}{}, fmt.Errorf("unknown identify kind %q", kind)
	}
	srv := jrpc2.ServerFromContext(ctx)
	cs.conn.srv = srv
	cs.conn.deviceID = p.DeviceID
	cs.conn.kind = kind
	cs.conn.lastSeen.Store(time.Now().UnixNano())

	cs.parent.mu.Lock()
	if kind == "source" {
		if old, ok := cs.parent.clients[p.DeviceID]; ok {
			cs.parent.logger.Warn("pipelinectl: evicting prior source for device",
				"device_id", p.DeviceID)
			old.srv.Stop()
		}
		cs.parent.clients[p.DeviceID] = cs.conn
	} else {
		if old, ok := cs.parent.composers[p.DeviceID]; ok {
			cs.parent.logger.Warn("pipelinectl: evicting prior composer",
				"composer_id", p.DeviceID)
			old.srv.Stop()
		}
		cs.parent.composers[p.DeviceID] = cs.conn
	}
	cs.parent.mu.Unlock()

	// Sources are expected to emit status notifications; composers don't,
	// so only sources get the heartbeat watchdog.
	if kind == "source" {
		watchCtx, cancel := context.WithCancel(cs.parent.ctx)
		cs.conn.closeFn = cancel
		go cs.watchHeartbeat(watchCtx)
	}

	cs.parent.logger.Info("pipelinectl: client identified",
		"id", p.DeviceID, "kind", kind, "pid", p.PID, "version", p.Version)
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
		cs.parent.logger.Warn("pipelinectl: status fan-out buffer full",
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
				cs.parent.logger.Warn("pipelinectl: source heartbeat lost",
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
