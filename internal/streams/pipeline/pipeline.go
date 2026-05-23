package pipeline

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"sync"
	"time"

	"github.com/smazurov/videonode/internal/events"
	"github.com/smazurov/videonode/internal/logging"
	"github.com/smazurov/videonode/internal/process"
)

// Config holds the daemon-wide knobs the Pipeline needs at construction
// time: paths to the three native binaries and the DRM render node the
// composer should target.
type Config struct {
	VNSourceBin   string
	VNComposerBin string
	VNSinkBin     string
	DRMDevice     string

	// DeviceResolver maps an opaque device id (USB bus-port etc.) to a
	// canonical /dev/videoN path. Returns empty string when the device
	// can't be resolved; Pipeline.Apply surfaces that as an error.
	DeviceResolver func(deviceID string) string

	// EventBus, when non-nil, receives StageStateChangedEvent on every
	// pool state transition. Nil = no events emitted (test path).
	EventBus *events.Bus
}

// Pipeline is the stage assembler. One Pipeline owns the runtime
// state for all streams: the ProducerRegistry (refcounted producers),
// per-stream Composer + Encoder stage instances, and the process.Pool
// that supervises every spawned binary.
//
// Public surface: Apply(stream) brings the runtime to match the spec;
// Delete(streamID) tears down everything owned by that stream and
// releases its producer claims. Both are idempotent — Apply twice with
// the same spec is a no-op; Delete on an unknown stream is a no-op.
type Pipeline struct {
	cfg       Config
	pool      process.Pool
	logger    logging.Logger
	producers *ProducerRegistry

	mu     sync.Mutex
	stages map[string]Stage // pool key → stage (for CommandProvider + Configurer lookup)
	// per-stream owned stage IDs (composer, encoder). Used by Delete()
	// to find what to stop for a given stream.
	owned map[string][]string
	// per-stream mutex serializing Apply/Delete calls for the same
	// stream. Different streams proceed in parallel. Prevents the
	// replaceStage race where two concurrent Applies for the same id
	// see IsRunning=false during a 10s Stop window and both spawn.
	streamLocksMu sync.Mutex
	streamLocks   map[string]*sync.Mutex
}

// New constructs a Pipeline. The pool is constructed internally so the
// Pipeline can wire CommandProvider / ConfigureProcess against its
// stage map. Logger is optional (defaults to a no-attr slog logger).
func New(cfg Config, logger logging.Logger) *Pipeline {
	if logger == nil {
		logger = logging.GetLogger("pipeline")
	}
	p := &Pipeline{
		cfg:         cfg,
		logger:      logger,
		producers:   NewProducerRegistry(),
		stages:      make(map[string]Stage),
		owned:       make(map[string][]string),
		streamLocks: make(map[string]*sync.Mutex),
	}
	p.pool = process.NewPool(&process.PoolOptions{
		Logger:           logger,
		CommandProvider:  p.commandFor,
		ConfigureProcess: p.configureProcess,
		OnStateChange:    p.onStateChange,
	})
	return p
}

// onStateChange forwards pool state transitions to the event bus as
// StageStateChangedEvent (when a bus is configured). Kind/StreamID
// come from the registered stage; lookup is best-effort — a stage that
// was deleted between state-change and lookup just emits with empty
// kind/stream_id.
func (p *Pipeline) onStateChange(id string, oldState, newState process.State, err error) {
	if p.cfg.EventBus == nil {
		return
	}
	p.mu.Lock()
	stage, ok := p.stages[id]
	p.mu.Unlock()
	ev := events.StageStateChangedEvent{
		StageID:   id,
		OldState:  string(oldState),
		NewState:  string(newState),
		Timestamp: time.Now().Format(time.RFC3339),
	}
	if ok {
		ev.StageKind = stage.Kind().String()
		ev.StreamID = stage.StreamID()
	}
	if err != nil {
		ev.Error = err.Error()
	}
	if newState == process.StateRunning {
		ev.PID = p.pool.GetStatus(id).PID
	}
	p.cfg.EventBus.Publish(ev)
}

// streamLock returns the per-stream mutex, creating one on first use.
// Two concurrent Apply (or Apply + Delete) calls for the same stream
// serialize on this lock; different streams run in parallel.
func (p *Pipeline) streamLock(streamID string) *sync.Mutex {
	p.streamLocksMu.Lock()
	defer p.streamLocksMu.Unlock()
	if mu, ok := p.streamLocks[streamID]; ok {
		return mu
	}
	mu := &sync.Mutex{}
	p.streamLocks[streamID] = mu
	return mu
}

// Apply reconciles the runtime state to match the given stream spec.
// Idempotent. Serialized per-stream — two concurrent Apply calls for
// the same stream queue; calls for different streams run in parallel.
//
// On producer-spawn failure: rolls back the just-made registry claims
// before returning, so the next Apply (or a Delete) sees a consistent
// view. Errors during composer / encoder spawn are NOT rolled back —
// callers can retry Apply or Delete the stream to clean up.
//
// Flow:
//  1. Compute the unique device set from stream.Inputs and Reconcile
//     against ProducerRegistry. Spawn newly-claimed producers; stop
//     producers whose refcount dropped to zero (this stream was the
//     last holder). On spawn failure, release the new claims first.
//  2. NeedsComposer(stream) decides whether a Composer process exists.
//     If yes, ensure a ComposerStage is registered and started; build
//     a ComposerFrameSource against its --scm-out socket. If no, tear
//     down any prior composer for this stream and build a
//     ProducerFrameSource against the single input's producer socket.
//  3. Build/refresh the EncoderStage with the resolved FrameSource +
//     AudioSource + EncoderConfig + Publish targets. Restart it if its
//     argv changed since the last apply (today: always restart on
//     Apply; reconfigure-without-restart is a follow-up).
func (p *Pipeline) Apply(s Stream) error {
	if s.ID == "" {
		return errors.New("pipeline: stream.ID is required")
	}
	mu := p.streamLock(s.ID)
	mu.Lock()
	defer mu.Unlock()

	if p.cfg.DeviceResolver == nil {
		return errors.New("pipeline: Config.DeviceResolver is nil")
	}
	// Resolve device paths up front so we fail fast on unknown devices.
	devices := make([]string, 0, len(s.Inputs))
	devicePaths := make(map[string]string, len(s.Inputs))
	for _, in := range s.Inputs {
		if in.Device == "" {
			return fmt.Errorf("pipeline: stream %s input %s has no device", s.ID, in.ID)
		}
		path := p.cfg.DeviceResolver(in.Device)
		if path == "" {
			return fmt.Errorf("pipeline: device %q did not resolve to a path", in.Device)
		}
		if _, seen := devicePaths[in.Device]; !seen {
			devices = append(devices, in.Device)
			devicePaths[in.Device] = path
		}
	}

	// Step 1: Reconcile producers.
	delta := p.producers.Reconcile(s.ID, devices)
	if err := p.ensureUdsDir(); err != nil {
		// Roll back: the Reconcile already mutated the registry.
		p.producers.ReleaseAll(s.ID)
		return fmt.Errorf("pipeline: mkdir uds dir: %w", err)
	}
	if err := p.startNewProducers(delta.ToStart, devicePaths); err != nil {
		// Roll back: the partially-applied delta would leak claims (the
		// registry holds them but processes aren't running) AND the
		// devices in delta.ToStop never get released. Replay both: drop
		// the new claims, then process the original ToStop list since
		// we never reached stopReleasedProducers below.
		p.producers.Reconcile(s.ID, p.heldDevicesExcluding(s.ID, delta.ToStart))
		p.stopReleasedProducers(delta.ToStop)
		return err
	}
	p.stopReleasedProducers(delta.ToStop)

	// Step 2: Composer engage/disengage.
	// Inline-composer mode: composer is bundled into the encoder shell
	// pipe (`composer | ffmpeg`) instead of running as a separate pool
	// entry. Works around the GBM-allocated-BGRA cross-process mmap
	// kernel limitation (vn-sink reports ENOSYS on the dma-buf fd).
	// Composer's gRPC control plane still works the same — daemon
	// dials the UDS regardless of who's its parent.
	p.stopComposerIfRunning(s.ID) // no separate composer pool entry today

	// Step 3: Build / restart the encoder. Inline composer is wired
	// inside buildEncoder when NeedsComposer(s).
	enc, err := p.buildEncoderInline(s)
	if err != nil {
		return fmt.Errorf("pipeline: build encoder %s: %w", s.ID, err)
	}
	p.replaceStage(enc) // unconditional restart on Apply (reconfigure-without-restart is follow-up)
	if err := p.restartStage(enc); err != nil {
		return fmt.Errorf("pipeline: start encoder %s: %w", enc.ID(), err)
	}
	return nil
}

// buildEncoderInline picks the right FrameSource:
//   - NeedsComposer(s): InlineComposerFrameSource (composer is a child
//     of the encoder shell pipe; daemon orchestrates via gRPC UDS)
//   - else: ProducerFrameSource (vn-sink dials producer SCM directly)
func (p *Pipeline) buildEncoderInline(s Stream) (*EncoderStage, error) {
	var video FrameSource
	switch {
	case NeedsComposer(s):
		w, h := canvasDimsHint(s)
		composerID := ComposerIDFor(s.ID)
		video = InlineComposerFrameSource{
			ComposerBin: p.cfg.VNComposerBin,
			DRMDevice:   p.cfg.DRMDevice,
			GrpcUds:     GrpcSocketPathFor("composer", composerID),
			ComposerID:  composerID,
			Width:       w,
			Height:      h,
			Fps:         parseFPSHintWithDefault(s, 30),
		}
	case len(s.Inputs) == 1:
		video = ProducerFrameSource{Socket: SCMSocketPathFor(s.Inputs[0].Device)}
	default:
		return nil, errors.New("encoder build: 0 inputs and no composer — nothing to encode")
	}
	return &EncoderStage{
		StreamID_:         s.ID,
		Media:             MediaSource{Video: video, Audio: ALSADirectAudio{Config: s.Audio}},
		Cfg:               s.Encoder,
		Publish:           s.Publish,
		CustomEncoderArgs: s.CustomEncoderArgs,
		VNSinkBin:         p.cfg.VNSinkBin,
	}, nil
}

// heldDevicesExcluding returns the device set this consumer would
// reconcile to in order to drop a specific subset. Used by Apply's
// rollback path: when startNewProducers fails partway, we want to
// release the new claims without touching pre-existing ones.
func (p *Pipeline) heldDevicesExcluding(consumerID string, excluded []string) []string {
	bad := make(map[string]struct{}, len(excluded))
	for _, d := range excluded {
		bad[d] = struct{}{}
	}
	all := p.producers.Devices()
	out := make([]string, 0, len(all))
	for dev := range all {
		if _, drop := bad[dev]; drop {
			continue
		}
		// Only keep devices this consumer actually holds — Devices()
		// returns the global map, not per-consumer view.
		if slices.Contains(p.producers.ConsumersOf(dev), consumerID) {
			out = append(out, dev)
		}
	}
	return out
}

// Delete tears down all stages owned by the stream and releases its
// producer claims. Producers whose refcount drops to zero are stopped.
// Safe for unknown streamIDs (no-op). Serialized per-stream with Apply.
//
// Stops are fanned out in goroutines because pool.Stop blocks up to
// 10s per process; serial teardown of a 4-stage stream (composer +
// encoder + 2 producers) would exceed an HTTP request timeout. Mirrors
// pool.StopAll's fan-out pattern.
func (p *Pipeline) Delete(streamID string) error {
	if streamID == "" {
		return nil
	}
	mu := p.streamLock(streamID)
	mu.Lock()
	defer mu.Unlock()

	// Collect stream-owned stages and released producers up front.
	p.mu.Lock()
	owned := append([]string(nil), p.owned[streamID]...)
	delete(p.owned, streamID)
	p.mu.Unlock()

	delta := p.producers.ReleaseAll(streamID)

	// Fan out all Stop calls (owned stages + dropped producers).
	// Each Stop blocks independently; total wall time is bounded by
	// the slowest single Stop.
	stopIDs := make([]string, 0, len(owned)+len(delta.ToStop))
	stopIDs = append(stopIDs, owned...)
	for _, dev := range delta.ToStop {
		stopIDs = append(stopIDs, ProducerPoolKey(dev))
	}

	var wg sync.WaitGroup
	wg.Add(len(stopIDs))
	for _, id := range stopIDs {
		go func(id string) {
			defer wg.Done()
			if err := p.pool.Stop(id); err != nil {
				p.logger.Warn("Delete: pool.Stop failed", "id", id, "error", err)
			}
			p.mu.Lock()
			delete(p.stages, id)
			p.mu.Unlock()
		}(id)
	}
	wg.Wait()
	return nil
}

// Pool exposes the underlying process.Pool for callers that need
// status lookups (process-manager UI follow-up). Not used by Apply /
// Delete; only diagnostics should reach for it.
func (p *Pipeline) Pool() process.Pool { return p.pool }

// Producers exposes the ProducerRegistry for diagnostics.
func (p *Pipeline) Producers() *ProducerRegistry { return p.producers }

// ----- internal helpers -----

// commandFor is the pool's CommandProvider callback. Looks up the
// stage by pool id, asks it for argv, joins to a shell command string.
func (p *Pipeline) commandFor(id string) (string, error) {
	p.mu.Lock()
	stage, ok := p.stages[id]
	p.mu.Unlock()
	if !ok {
		return "", fmt.Errorf("pipeline: no stage registered for pool id %s", id)
	}
	argv, _, err := stage.Command()
	if err != nil {
		return "", err
	}
	return shellJoinArgv(argv), nil
}

// configureProcess is the pool's Configurer callback. Wires the
// stage's LogParser + LogAttrs into the process.Process so each stage's
// stderr lands in journald with the right module + structured fields.
//
// Does NOT call back into Pool methods that take Pool's mu — this is
// invoked from inside pool.startProcess which already holds it.
// Snapshot reads stage kind from the Pipeline's stages map, so
// Info.Kind on the pool side isn't load-bearing for /api/processes.
//
// Passes slog.Attr through With() as the typed Attr (not key+Any()) so
// non-scalar kinds (slog.Group, LogValuer) survive — Go's slog handles
// Attr-typed args specially.
func (p *Pipeline) configureProcess(id string, proc *process.Process) {
	p.mu.Lock()
	stage, ok := p.stages[id]
	p.mu.Unlock()
	if !ok {
		return
	}
	moduleLogger := logging.GetLogger(stage.Kind().String())
	attrs := stage.LogAttrs()
	if len(attrs) > 0 {
		args := make([]any, len(attrs))
		for i, a := range attrs {
			args[i] = a
		}
		moduleLogger = moduleLogger.With(args...)
	}
	proc.SetLogParser(moduleLogger, stage.LogParser())
}

func (p *Pipeline) startNewProducers(toStart []string, devicePaths map[string]string) error {
	for _, dev := range toStart {
		stage := &ProducerStage{
			DeviceID:   dev,
			DevicePath: devicePaths[dev],
			BinaryPath: p.cfg.VNSourceBin,
			GrpcUds:    GrpcSocketPathFor("source", dev),
		}
		if err := p.startStage(stage); err != nil {
			return fmt.Errorf("pipeline: start producer for %s: %w", dev, err)
		}
	}
	return nil
}

func (p *Pipeline) stopReleasedProducers(toStop []string) {
	for _, dev := range toStop {
		id := ProducerPoolKey(dev)
		if err := p.pool.Stop(id); err != nil {
			p.logger.Warn("stopReleasedProducers: pool.Stop failed", "id", id, "error", err)
		}
		p.mu.Lock()
		delete(p.stages, id)
		p.mu.Unlock()
	}
}

func (p *Pipeline) ensureComposer(s Stream) *ComposerStage {
	fps := 30
	if len(s.Layout) > 0 {
		// Layout doesn't carry FPS today — pull from EncoderConfig if
		// the user set it implicitly via "fps". For now leave at 30
		// and let SetCanvas push from the daemon override.
	}
	if s.Encoder.GOP > 0 {
		// GOP without FPS doesn't decide tick rate; fall through.
	}
	if fpsHint := parseFPSHint(s); fpsHint > 0 {
		fps = fpsHint
	}
	return &ComposerStage{
		StreamID_:  s.ID,
		BinaryPath: p.cfg.VNComposerBin,
		DRMDevice:  p.cfg.DRMDevice,
		CanvasFPS:  fps,
		GrpcUds:    GrpcSocketPathFor("composer", ComposerIDFor(s.ID)),
	}
}

func (p *Pipeline) stopComposerIfRunning(streamID string) {
	id := ComposerPoolKey(streamID)
	p.mu.Lock()
	_, exists := p.stages[id]
	p.mu.Unlock()
	if !exists {
		return
	}
	if err := p.pool.Stop(id); err != nil {
		p.logger.Warn("stopComposerIfRunning: pool.Stop failed", "id", id, "error", err)
	}
	p.mu.Lock()
	delete(p.stages, id)
	p.unbindOwnedLocked(streamID, id)
	p.mu.Unlock()
}

func (p *Pipeline) buildEncoder(s Stream, composerSock string) (*EncoderStage, error) {
	var video FrameSource
	switch {
	case composerSock != "":
		w, h := canvasDimsHint(s)
		video = ComposerFrameSource{
			Socket: composerSock,
			Width:  w,
			Height: h,
			Fps:    parseFPSHintWithDefault(s, 30),
		}
	case len(s.Inputs) == 1:
		video = ProducerFrameSource{Socket: SCMSocketPathFor(s.Inputs[0].Device)}
	default:
		return nil, errors.New("encoder build: 0 inputs and no composer — nothing to encode")
	}

	return &EncoderStage{
		StreamID_:         s.ID,
		Media:             MediaSource{Video: video, Audio: ALSADirectAudio{Config: s.Audio}},
		Cfg:               s.Encoder,
		Publish:           s.Publish,
		CustomEncoderArgs: s.CustomEncoderArgs,
		VNSinkBin:         p.cfg.VNSinkBin,
	}, nil
}

func (p *Pipeline) startStage(stage Stage) error {
	id := stage.ID()
	p.mu.Lock()
	p.stages[id] = stage
	if sid := stage.StreamID(); sid != "" {
		p.bindOwnedLocked(sid, id)
	}
	p.mu.Unlock()
	if p.pool.IsRunning(id) {
		return nil
	}
	return p.pool.Start(id)
}

// replaceStage swaps the registered stage for an id (e.g. encoder
// argv changed). Caller must follow with restartStage.
func (p *Pipeline) replaceStage(stage Stage) {
	p.mu.Lock()
	p.stages[stage.ID()] = stage
	if sid := stage.StreamID(); sid != "" {
		p.bindOwnedLocked(sid, stage.ID())
	}
	p.mu.Unlock()
}

func (p *Pipeline) restartStage(stage Stage) error {
	id := stage.ID()
	if p.pool.IsRunning(id) {
		if err := p.pool.Stop(id); err != nil {
			return err
		}
	}
	return p.pool.Start(id)
}

func (p *Pipeline) bindOwnedLocked(streamID, stageID string) {
	if slices.Contains(p.owned[streamID], stageID) {
		return
	}
	p.owned[streamID] = append(p.owned[streamID], stageID)
}

func (p *Pipeline) unbindOwnedLocked(streamID, stageID string) {
	cur := p.owned[streamID]
	for i, id := range cur {
		if id == stageID {
			p.owned[streamID] = append(cur[:i], cur[i+1:]...)
			return
		}
	}
}

func (p *Pipeline) ensureUdsDir() error {
	if err := os.MkdirAll(NativeUdsDir, 0o755); err != nil {
		return err
	}
	return nil
}

// parseFPSHint extracts the canvas FPS from a stream spec. For now,
// we only consult the encoder GOP (treating GOP/2 as a frame-rate
// hint) until layout/canvas carries an explicit FPS field.
func parseFPSHint(s Stream) int {
	return 0
}

func parseFPSHintWithDefault(s Stream, def int) int {
	if v := parseFPSHint(s); v > 0 {
		return v
	}
	return def
}

// canvasDimsHint returns the canvas dimensions implied by the layout.
// Today the canvas is the bounding box of all slots; falls back to
// 1920x1080 when no layout is set.
func canvasDimsHint(s Stream) (int, int) {
	if len(s.Layout) == 0 {
		return 1920, 1080
	}
	maxX, maxY := 0, 0
	for _, l := range s.Layout {
		if r := l.X + l.W; r > maxX {
			maxX = r
		}
		if b := l.Y + l.H; b > maxY {
			maxY = b
		}
	}
	if maxX == 0 || maxY == 0 {
		return 1920, 1080
	}
	return maxX, maxY
}

// joinPaths is a small helper used internally for composing socket
// paths; lifted out so tests can swap it for deterministic output
// without poking at filepath.Join semantics.
func joinPaths(parts ...string) string {
	return filepath.Join(parts...)
}

// _ keeps strconv referenced; some upcoming follow-up plumbing uses it.
var _ = strconv.Itoa
