// Package pipelinectl is the daemon-side control plane for the native
// processes the daemon supervises — videonode-source (kind "source")
// and videonode-composer (kind "composer"). The daemon dials each
// process over a per-instance Unix domain socket the daemon allocated
// before spawn; the gRPC service surface lives in the C++ binary
// (composer/src/{source,render}/{source,composer}_service.cpp). See
// proto/control/*.proto for the wire schema.
//
// Public API preserved across the JSON-RPC → gRPC migration:
//
//	Manager.Send{SetFormat, SetCanvas, SetSource, ClearSource, SetLayout,
//	              SetEffects, SetSourceState}, StatusFeed, ConnectedDevices,
//	              ConnectedComposers, Start, Stop
//
// New entry points added for the architecture flip (daemon-as-client):
//
//	Manager.RegisterSource(ctx, deviceID, udsPath)
//	Manager.RegisterComposer(ctx, composerID, udsPath)
//	Manager.Unregister(id)
//	Manager.Snapshot(ctx, deviceID) — NV12 bytes + dims for the snapshot
//	                                  REST endpoint (replaces the legacy
//	                                  internal/streams/snapshot_native.go
//	                                  dma-buf consumer).
package pipelinectl

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/smazurov/videonode/internal/logging"
	pb "github.com/smazurov/videonode/internal/streams/pipelinectl/pb"
)

// DefaultSocketPath was the well-known daemon control socket location
// under the legacy JSON-RPC model. Retained as an empty-default sentinel
// for Manager.New() callers; the new model uses per-instance UDS paths
// the daemon allocates per spawn (see process_manager.go).
const DefaultSocketPath = ""

// HeartbeatInterval / HeartbeatTimeout are retained for compat with the
// existing tests; the gRPC keepalive mechanism on the StreamStatus
// stream replaces the explicit watchdog.
const (
	HeartbeatInterval = 1 * time.Second
	HeartbeatTimeout  = 3 * HeartbeatInterval
)

// Manager is the daemon-side gRPC client manager. It holds one
// connection per spawned native binary (videonode-source or
// videonode-composer). RegisterSource / RegisterComposer dial the
// per-instance UDS the daemon allocated and call Describe() to seed the
// identity; for sources we additionally open a long-lived StreamStatus
// server-streaming RPC that pumps the legacy `status` notifications
// onto the manager's status channel.
type Manager struct {
	socketPath string // retained for API compat; unused in the gRPC path
	logger     logging.Logger
	statusCh   chan StatusParams

	mu           sync.RWMutex
	sources      map[string]*nativeConn // key: device_id
	composers    map[string]*nativeConn // key: composer_id
	statusClosed bool                   // true once Stop() has closed statusCh

	ctx    context.Context
	cancel context.CancelFunc
}

// nativeConn owns a single gRPC channel + the typed clients for one
// native binary. For source-kind entries, statusCancel cancels the
// long-running StreamStatus goroutine.
type nativeConn struct {
	id         string
	kind       string // "source" or "composer"
	udsPath    string
	cc         *grpc.ClientConn
	srcClient  pb.SourceClient   // nil for composer
	compClient pb.ComposerClient // nil for source

	// Identity captured at Describe() time.
	pid             uint32
	version         string
	protocolVersion uint32

	// Stream lifecycle (sources only).
	statusCancel context.CancelFunc
	streamDone   chan struct{}
}

// New constructs an unstarted Manager. The socketPath argument is
// retained for API compat with the legacy server-mode signature; it is
// ignored in the gRPC client path (per-instance UDS paths flow through
// RegisterSource / RegisterComposer instead).
func New(socketPath string, logger logging.Logger) *Manager {
	if logger == nil {
		logger = logging.GetLogger("pipelinectl")
	}
	return &Manager{
		socketPath: socketPath,
		logger:     logger,
		statusCh:   make(chan StatusParams, 64),
		sources:    make(map[string]*nativeConn),
		composers:  make(map[string]*nativeConn),
	}
}

// SocketPath returns the legacy socket-path field. Unused in the gRPC
// path; retained for back-compat with any caller that reads it.
func (m *Manager) SocketPath() string { return m.socketPath }

// StatusFeed returns the read-only channel of status notifications
// proxied from each registered source's StreamStatus server stream.
// The channel is buffered (64 slots). On overflow the oldest pending
// event is dropped silently — heartbeat catches up.
func (m *Manager) StatusFeed() <-chan StatusParams { return m.statusCh }

// Start initializes the manager's lifecycle context. Native processes
// register via RegisterSource / RegisterComposer after the daemon
// spawns them; nothing to bind or listen on. Kept for API compat.
func (m *Manager) Start(ctx context.Context) error {
	m.ctx, m.cancel = context.WithCancel(ctx)
	m.logger.Info("pipelinectl: manager started (daemon-as-grpc-client mode)")
	return nil
}

// Stop closes all per-process gRPC channels, cancels any pending
// StreamStatus goroutines, and closes StatusFeed so range-readers
// (main.go's fan-out goroutine) can exit. Idempotent.
func (m *Manager) Stop() error {
	if m.cancel != nil {
		m.cancel()
	}
	m.mu.Lock()
	conns := make([]*nativeConn, 0, len(m.sources)+len(m.composers))
	for _, c := range m.sources {
		conns = append(conns, c)
	}
	for _, c := range m.composers {
		conns = append(conns, c)
	}
	m.sources = make(map[string]*nativeConn)
	m.composers = make(map[string]*nativeConn)
	already := m.statusClosed
	m.statusClosed = true
	m.mu.Unlock()
	for _, c := range conns {
		m.closeConn(c)
	}
	if !already {
		close(m.statusCh)
	}
	return nil
}

// ConnectedDevices returns a snapshot of registered source device IDs.
func (m *Manager) ConnectedDevices() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]string, 0, len(m.sources))
	for id := range m.sources {
		out = append(out, id)
	}
	return out
}

// ConnectedComposers returns a snapshot of registered composer IDs.
func (m *Manager) ConnectedComposers() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]string, 0, len(m.composers))
	for id := range m.composers {
		out = append(out, id)
	}
	return out
}

// dial brings up a gRPC channel to a per-instance UDS. Unix-socket
// targets use the `unix://` scheme; insecure credentials are fine for
// process-local sockets. Keepalive pings on the StreamStatus stream
// replace the legacy 1s/3s watchdog.
func (m *Manager) dial(udsPath string) (*grpc.ClientConn, error) {
	target := "unix://" + udsPath
	kp := keepalive.ClientParameters{
		Time:                1 * time.Second,
		Timeout:             3 * time.Second,
		PermitWithoutStream: true,
	}
	cc, err := grpc.NewClient(
		target,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithKeepaliveParams(kp),
	)
	if err != nil {
		return nil, fmt.Errorf("pipelinectl: dial %s: %w", target, err)
	}
	return cc, nil
}

// RegisterSource dials the source's gRPC server, calls Describe() to
// capture identity, and starts the long-running StreamStatus goroutine.
// Returns an error if the dial or Describe fails; on stream failure
// after registration the goroutine will reconnect with backoff until
// the manager is stopped or the entry is Unregistered.
func (m *Manager) RegisterSource(ctx context.Context, deviceID, udsPath string) error {
	cc, err := m.dial(udsPath)
	if err != nil {
		return err
	}
	client := pb.NewSourceClient(cc)
	info, err := client.Describe(ctx, &emptypb.Empty{})
	if err != nil {
		_ = cc.Close()
		return fmt.Errorf("pipelinectl: describe source %s: %w", deviceID, err)
	}
	// Wire the lifecycle context + done channel BEFORE publishing to
	// m.sources, so a concurrent Unregister sees non-nil values and
	// joins the runStatusStream goroutine on closeConn. Without this
	// pre-publish, an Unregister between m.sources[id]=c and the
	// `c.statusCancel=cancel` line below would skip the join and leave
	// the goroutine running against a closed gRPC channel.
	if m.ctx == nil {
		_ = cc.Close()
		return fmt.Errorf("pipelinectl: manager not started")
	}
	streamCtx, cancel := context.WithCancel(m.ctx)
	c := &nativeConn{
		id:              deviceID,
		kind:            "source",
		udsPath:         udsPath,
		cc:              cc,
		srcClient:       client,
		pid:             info.GetPid(),
		version:         info.GetVersion(),
		protocolVersion: info.GetProtocolVersion(),
		statusCancel:    cancel,
		streamDone:      make(chan struct{}),
	}

	m.mu.Lock()
	if old, ok := m.sources[deviceID]; ok {
		m.logger.Warn("pipelinectl: evicting prior source", "device_id", deviceID)
		m.mu.Unlock()
		m.closeConn(old)
		m.mu.Lock()
	}
	m.sources[deviceID] = c
	m.mu.Unlock()

	// Open the long-lived StreamStatus and pump into m.statusCh.
	go m.runStatusStream(streamCtx, c)

	m.logger.Info("pipelinectl: source registered",
		"device_id", deviceID, "pid", info.GetPid(),
		"version", info.GetVersion(), "uds", udsPath)
	return nil
}

// RegisterComposer dials the composer's gRPC server and calls
// Describe() to capture identity. Composers don't push status today,
// so no streaming goroutine is started.
func (m *Manager) RegisterComposer(ctx context.Context, composerID, udsPath string) error {
	if m.ctx == nil {
		return fmt.Errorf("pipelinectl: manager not started")
	}
	cc, err := m.dial(udsPath)
	if err != nil {
		return err
	}
	client := pb.NewComposerClient(cc)
	info, err := client.Describe(ctx, &emptypb.Empty{})
	if err != nil {
		_ = cc.Close()
		return fmt.Errorf("pipelinectl: describe composer %s: %w", composerID, err)
	}
	c := &nativeConn{
		id:              composerID,
		kind:            "composer",
		udsPath:         udsPath,
		cc:              cc,
		compClient:      client,
		pid:             info.GetPid(),
		version:         info.GetVersion(),
		protocolVersion: info.GetProtocolVersion(),
	}

	m.mu.Lock()
	if old, ok := m.composers[composerID]; ok {
		m.logger.Warn("pipelinectl: evicting prior composer", "composer_id", composerID)
		m.mu.Unlock()
		m.closeConn(old)
		m.mu.Lock()
	}
	m.composers[composerID] = c
	m.mu.Unlock()

	m.logger.Info("pipelinectl: composer registered",
		"composer_id", composerID, "pid", info.GetPid(),
		"version", info.GetVersion(), "uds", udsPath)
	return nil
}

// Unregister cancels any active stream and closes the gRPC channel for
// the given id. Looks in both source + composer registries; either or
// neither may match.
func (m *Manager) Unregister(id string) {
	m.mu.Lock()
	src, sok := m.sources[id]
	comp, cok := m.composers[id]
	if sok {
		delete(m.sources, id)
	}
	if cok {
		delete(m.composers, id)
	}
	m.mu.Unlock()
	if sok {
		m.closeConn(src)
	}
	if cok {
		m.closeConn(comp)
	}
}

func (m *Manager) closeConn(c *nativeConn) {
	if c == nil {
		return
	}
	if c.statusCancel != nil {
		c.statusCancel()
	}
	if c.streamDone != nil {
		<-c.streamDone
	}
	if c.cc != nil {
		_ = c.cc.Close()
	}
}

// runStatusStream opens the server-streaming RPC and pumps each
// received Status onto m.statusCh. On any stream error (server gone,
// network blip, etc.) it backs off and reconnects until the context is
// cancelled.
func (m *Manager) runStatusStream(ctx context.Context, c *nativeConn) {
	defer close(c.streamDone)
	backoff := 200 * time.Millisecond
	const backoffCap = 5 * time.Second
	for {
		if ctx.Err() != nil {
			return
		}
		stream, err := c.srcClient.StreamStatus(ctx, &emptypb.Empty{})
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			m.logger.Warn("pipelinectl: StreamStatus failed; will retry",
				"device_id", c.id, "error", err, "backoff", backoff)
			select {
			case <-ctx.Done():
				return
			case <-time.After(backoff):
			}
			if backoff < backoffCap {
				backoff *= 2
				if backoff > backoffCap {
					backoff = backoffCap
				}
			}
			continue
		}
		backoff = 200 * time.Millisecond
		for {
			msg, err := stream.Recv()
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				if ctx.Err() == nil {
					m.logger.Warn("pipelinectl: StreamStatus recv error",
						"device_id", c.id, "error", err)
				}
				break
			}
			params := statusFromProto(msg)
			if params.DeviceID == "" {
				params.DeviceID = c.id
			}
			// Skip if Stop closed statusCh between our last ctx check
			// and now — sending on a closed chan would panic. The flag
			// is set under m.mu in Stop.
			m.mu.RLock()
			closed := m.statusClosed
			m.mu.RUnlock()
			if closed {
				return
			}
			select {
			case m.statusCh <- params:
			default:
				m.logger.Warn("pipelinectl: status fan-out buffer full",
					"device_id", c.id)
			}
		}
	}
}

// SendSetFormat dispatches a set_format command to the source registered
// under deviceID.
func (m *Manager) SendSetFormat(ctx context.Context, deviceID string, p SetFormatParams) (SetFormatResult, error) {
	m.mu.RLock()
	c, ok := m.sources[deviceID]
	m.mu.RUnlock()
	if !ok {
		return SetFormatResult{}, fmt.Errorf("pipelinectl: no source for device %q", deviceID)
	}
	resp, err := c.srcClient.SetFormat(ctx, &pb.SetFormatRequest{
		Fourcc: p.FourCC, W: p.W, H: p.H, Fps: p.FPS,
	})
	if err != nil {
		return SetFormatResult{}, fmt.Errorf("pipelinectl: set_format %s: %w", deviceID, err)
	}
	return SetFormatResult{Applied: resp.GetApplied()}, nil
}

// Snapshot pulls a raw NV12 frame from the source's broadcast loop via
// the Source.Snapshot unary RPC. Replaces the legacy SCM_RIGHTS dma-buf
// consumer; the daemon's existing ffmpeg-subprocess JPEG encoder reads
// the returned bytes.
func (m *Manager) Snapshot(ctx context.Context, deviceID string) (*pb.SnapshotResponse, error) {
	m.mu.RLock()
	c, ok := m.sources[deviceID]
	m.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("pipelinectl: no source for device %q", deviceID)
	}
	resp, err := c.srcClient.Snapshot(ctx, &pb.SnapshotRequest{})
	if err != nil {
		if st, sok := status.FromError(err); sok && st.Code() == codes.Unavailable {
			return nil, fmt.Errorf("pipelinectl: source %s has no frame yet", deviceID)
		}
		return nil, fmt.Errorf("pipelinectl: snapshot %s: %w", deviceID, err)
	}
	return resp, nil
}

// SendSetCanvas pushes set_canvas to the given composer.
func (m *Manager) SendSetCanvas(ctx context.Context, composerID string, p SetCanvasParams) error {
	return m.callComposer(ctx, composerID, func(c pb.ComposerClient) error {
		_, err := c.SetCanvas(ctx, &pb.SetCanvasRequest{W: p.W, H: p.H, Fps: p.FPS})
		return err
	})
}

// SendSetSource binds a slot to a SCM-publishing source.
func (m *Manager) SendSetSource(ctx context.Context, composerID string, p SetSourceParams) error {
	return m.callComposer(ctx, composerID, func(c pb.ComposerClient) error {
		_, err := c.SetSource(ctx, &pb.SetSourceRequest{
			Slot: p.Slot, SourceId: p.SourceID, ScmPath: p.ScmPath,
			Width: p.Width, Height: p.Height, Fps: p.FPS,
		})
		return err
	})
}

// SendClearSource unbinds a slot.
func (m *Manager) SendClearSource(ctx context.Context, composerID string, p ClearSourceParams) error {
	return m.callComposer(ctx, composerID, func(c pb.ComposerClient) error {
		_, err := c.ClearSource(ctx, &pb.ClearSourceRequest{Slot: p.Slot})
		return err
	})
}

// SendSetLayout replaces the composer's whole layout.
func (m *Manager) SendSetLayout(ctx context.Context, composerID string, p SetLayoutParams) error {
	slots := make([]*pb.LayoutSlot, 0, len(p.Slots))
	for _, s := range p.Slots {
		slots = append(slots, &pb.LayoutSlot{
			Slot: s.Slot, X: s.X, Y: s.Y, W: s.W, H: s.H,
		})
	}
	return m.callComposer(ctx, composerID, func(c pb.ComposerClient) error {
		_, err := c.SetLayout(ctx, &pb.SetLayoutRequest{Slots: slots})
		return err
	})
}

// SendSetEffects atomically replaces the effect list for one source.
func (m *Manager) SendSetEffects(ctx context.Context, composerID string, p SetEffectsParams) error {
	effects := make([]*pb.Effect, 0, len(p.Effects))
	for _, e := range p.Effects {
		out := &pb.Effect{Type: e.Type}
		if e.Type == "perspective" {
			corners := make([]int32, 0, 8)
			for i := range 4 {
				corners = append(corners, int32(e.Corners[i][0]), int32(e.Corners[i][1]))
			}
			out.Perspective = &pb.PerspectiveEffectParams{
				Corners:   corners,
				SnapshotW: int32(e.SnapshotWidth),
				SnapshotH: int32(e.SnapshotHeight),
			}
		}
		effects = append(effects, out)
	}
	return m.callComposer(ctx, composerID, func(c pb.ComposerClient) error {
		_, err := c.SetEffects(ctx, &pb.SetEffectsRequest{SourceId: p.SourceID, Effects: effects})
		return err
	})
}

// SendSetSourceState pushes a per-source state update to the composer.
func (m *Manager) SendSetSourceState(ctx context.Context, composerID string, p SetSourceStateParams) error {
	return m.callComposer(ctx, composerID, func(c pb.ComposerClient) error {
		_, err := c.SetSourceState(ctx, &pb.SetSourceStateRequest{SourceId: p.SourceID, State: p.State})
		return err
	})
}

func (m *Manager) callComposer(_ context.Context, composerID string, fn func(pb.ComposerClient) error) error {
	m.mu.RLock()
	c, ok := m.composers[composerID]
	m.mu.RUnlock()
	if !ok {
		return fmt.Errorf("pipelinectl: no composer for id %q", composerID)
	}
	if err := fn(c.compClient); err != nil {
		return fmt.Errorf("pipelinectl: composer %s: %w", composerID, err)
	}
	return nil
}

// statusFromProto converts the gRPC Status message into the legacy
// StatusParams shape consumed by internal/events/types.go and the SSE
// fan-out path. The conversion is mechanical: field-for-field copy.
func statusFromProto(s *pb.Status) StatusParams {
	if s == nil {
		return StatusParams{}
	}
	out := StatusParams{
		DeviceID:    s.GetDeviceId(),
		TimestampMs: s.GetTsMs(),
		Health:      s.GetHealth(),
	}
	if d := s.GetDevice(); d != nil {
		out.Device = SourceDeviceInfo{Path: d.GetPath(), Multiplanar: d.GetMultiplanar()}
	}
	if sig := s.GetSignal(); sig != nil {
		out.Signal = SourceSignalInfo{
			HasDvTimings: sig.GetHasDvTimings(),
			CablePresent: sig.GetCablePresent(),
			SignalLocked: sig.GetSignalLocked(),
			DvTimings:    sig.GetDvTimings(),
		}
	}
	if f := s.GetFormat(); f != nil {
		out.Format = SourceFormatInfo{
			FourCC:  f.GetFourcc(),
			W:       f.GetW(),
			H:       f.GetH(),
			FPS:     f.GetFps(),
			Buffers: f.GetBuffers(),
			Mode:    f.GetMode(),
		}
	}
	if b := s.GetBroadcast(); b != nil {
		out.Broadcast = SourceBroadcastInfo{
			TargetFPS:         b.GetTargetFps(),
			RealFrames:        b.GetRealFrames(),
			PlaceholderFrames: b.GetPlaceholderFrames(),
			LastSeq:           b.GetLastSeq(),
		}
	}
	if cs := s.GetConsumers(); cs != nil {
		out.Consumers.Count = int(cs.GetCount())
		out.Consumers.Live = consumerEntriesFromProto(cs.GetLive())
		out.Consumers.Evicted = consumerEntriesFromProto(cs.GetEvicted())
	}
	return out
}

func consumerEntriesFromProto(in []*pb.ConsumerEntry) []SourceConsumerEntry {
	if len(in) == 0 {
		return nil
	}
	out := make([]SourceConsumerEntry, 0, len(in))
	for _, e := range in {
		out = append(out, SourceConsumerEntry{
			FD:             int(e.GetFd()),
			FramesSent:     e.GetFramesSent(),
			FramesDropped:  e.GetFramesDropped(),
			EvictedAtFrame: e.GetEvictedAtFrame(),
		})
	}
	return out
}

// Compile-time assertion so we trip a build error if the package's
// errors-package import is dropped accidentally.
var _ = errors.New
