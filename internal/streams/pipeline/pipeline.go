package pipeline

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/smazurov/videonode/internal/events"
	"github.com/smazurov/videonode/internal/logging"
	"github.com/smazurov/videonode/internal/process"
)

// Config holds the daemon-wide knobs the Pipeline needs at construction
// time: paths to the three native binaries and the DRM render node.
type Config struct {
	VNSourceBin   string
	VNComposerBin string
	VNSinkBin     string
	DRMDevice     string

	// DeviceResolver maps an opaque device id (USB bus-port etc.) to a
	// canonical /dev/videoN path. Returns empty string when the device
	// can't be resolved; ApplySource surfaces that as an error. Optional
	// when every Source uses TestMode.
	DeviceResolver func(deviceID string) string

	// EventBus, when non-nil, receives StageStateChangedEvent on every
	// pool state transition.
	EventBus *events.Bus
}

// Pipeline is the stage assembler. One Pipeline owns the runtime state
// for all sources/composers/streams: registries, per-entity Stage
// instances, and the process.Pool that supervises every spawned binary.
//
// Public surface:
//   - ApplySource(s) / DeleteSource(id)
//   - ApplyComposer(c) / DeleteComposer(id)
//   - ApplyStream(s) / DeleteStream(id)
//
// Each Apply is idempotent — calling twice with the same spec is a no-op.
// Apply calls for the same id serialize via per-entity locks; calls for
// different ids run in parallel.
type Pipeline struct {
	cfg       Config
	pool      process.Pool
	logger    logging.Logger
	sources   *SourceRegistry
	composers *ComposerRegistry

	mu     sync.Mutex
	stages map[string]Stage // pool key → stage

	entityLocksMu sync.Mutex
	entityLocks   map[string]*sync.Mutex
}

// New constructs a Pipeline. Logger is optional (defaults to a
// no-attr slog logger).
func New(cfg Config, logger logging.Logger) *Pipeline {
	if logger == nil {
		logger = logging.GetLogger("pipeline")
	}
	p := &Pipeline{
		cfg:         cfg,
		logger:      logger,
		sources:     NewSourceRegistry(),
		composers:   NewComposerRegistry(),
		stages:      make(map[string]Stage),
		entityLocks: make(map[string]*sync.Mutex),
	}
	p.pool = process.NewPool(&process.PoolOptions{
		Logger:           logger,
		CommandProvider:  p.commandFor,
		ConfigureProcess: p.configureProcess,
		OnStateChange:    p.onStateChange,
	})
	return p
}

// ApplySource creates or updates the per-source `videonode-source`
// process. Validates that exactly one of Device or TestMode is set.
func (p *Pipeline) ApplySource(s Source) error {
	if s.ID == "" {
		return errors.New("pipeline: source.ID is required")
	}
	if s.TestMode && s.Device != "" {
		return fmt.Errorf("pipeline: source %s has both Device and TestMode set", s.ID)
	}
	if !s.TestMode && s.Device == "" {
		return fmt.Errorf("pipeline: source %s requires one of Device or TestMode", s.ID)
	}

	mu := p.entityLock("source:" + s.ID)
	mu.Lock()
	defer mu.Unlock()

	if err := p.ensureUdsDir(); err != nil {
		return fmt.Errorf("pipeline: mkdir uds dir: %w", err)
	}

	var devicePath string
	if !s.TestMode {
		if p.cfg.DeviceResolver == nil {
			return errors.New("pipeline: Config.DeviceResolver is nil")
		}
		devicePath = p.cfg.DeviceResolver(s.Device)
		if devicePath == "" {
			return fmt.Errorf("pipeline: device %q did not resolve to a path", s.Device)
		}
	}

	p.sources.Put(s)

	stage := &ProducerStage{
		SourceID:   s.ID,
		DevicePath: devicePath,
		TestMode:   s.TestMode,
		BinaryPath: p.cfg.VNSourceBin,
		GrpcUds:    GrpcSocketPathFor("source", s.ID),
	}
	p.replaceStage(stage)
	return p.restartStage(stage)
}

// DeleteSource stops the source's `videonode-source` process and drops
// the registry entry. No-op for unknown ids. Callers are responsible
// for guaranteeing no composer or stream references this source — the
// service layer enforces that constraint.
func (p *Pipeline) DeleteSource(id string) error {
	if id == "" {
		return nil
	}
	mu := p.entityLock("source:" + id)
	mu.Lock()
	defer mu.Unlock()

	p.sources.Delete(id)
	poolID := SourcePoolKey(id)
	if err := p.pool.Stop(poolID); err != nil {
		p.logger.Warn("DeleteSource: pool.Stop failed", "id", poolID, "error", err)
	}
	p.mu.Lock()
	delete(p.stages, poolID)
	p.mu.Unlock()
	return nil
}

// ApplyComposer creates or updates the per-composer `videonode-composer`
// process. Per-frame layout/inputs/effects are pushed via the gRPC
// control plane post-spawn; this only manages the process lifecycle.
func (p *Pipeline) ApplyComposer(c Composer) error {
	if c.ID == "" {
		return errors.New("pipeline: composer.ID is required")
	}

	mu := p.entityLock("composer:" + c.ID)
	mu.Lock()
	defer mu.Unlock()

	if err := p.ensureUdsDir(); err != nil {
		return fmt.Errorf("pipeline: mkdir uds dir: %w", err)
	}

	p.composers.Put(c)

	stage := &ComposerStage{
		ComposerID: c.ID,
		BinaryPath: p.cfg.VNComposerBin,
		DRMDevice:  p.cfg.DRMDevice,
		CanvasFPS:  30,
		GrpcUds:    GrpcSocketPathFor("composer", c.ID),
	}
	p.replaceStage(stage)
	return p.restartStage(stage)
}

// DeleteComposer stops the composer process and drops the registry
// entry. No-op for unknown ids.
func (p *Pipeline) DeleteComposer(id string) error {
	if id == "" {
		return nil
	}
	mu := p.entityLock("composer:" + id)
	mu.Lock()
	defer mu.Unlock()

	p.composers.Delete(id)
	poolID := ComposerPoolKey(id)
	if err := p.pool.Stop(poolID); err != nil {
		p.logger.Warn("DeleteComposer: pool.Stop failed", "id", poolID, "error", err)
	}
	p.mu.Lock()
	delete(p.stages, poolID)
	p.mu.Unlock()
	return nil
}

// ApplyStream creates or updates the per-stream encoder process. The
// Upstream string is resolved against the source/composer registries:
//   - "source:<id>"   → ProducerFrameSource against the source's SCM
//   - "composer:<id>" → ComposerFrameSource against the composer's
//     SCM-out
//
// Dangling references (the named source/composer isn't registered) are
// an error.
func (p *Pipeline) ApplyStream(s Stream) error {
	if s.ID == "" {
		return errors.New("pipeline: stream.ID is required")
	}
	if s.Upstream == "" {
		return fmt.Errorf("pipeline: stream %s requires Upstream", s.ID)
	}

	mu := p.entityLock("stream:" + s.ID)
	mu.Lock()
	defer mu.Unlock()

	enc, err := p.buildEncoder(s)
	if err != nil {
		return fmt.Errorf("pipeline: build encoder %s: %w", s.ID, err)
	}
	p.replaceStage(enc)
	if err := p.restartStage(enc); err != nil {
		return fmt.Errorf("pipeline: start encoder %s: %w", enc.ID(), err)
	}
	return nil
}

// DeleteStream stops the encoder process and drops the registry entry.
// No-op for unknown ids. Upstream sources/composers stay warm.
func (p *Pipeline) DeleteStream(id string) error {
	if id == "" {
		return nil
	}
	mu := p.entityLock("stream:" + id)
	mu.Lock()
	defer mu.Unlock()

	poolID := EncoderIDFor(id)
	if err := p.pool.Stop(poolID); err != nil {
		p.logger.Warn("DeleteStream: pool.Stop failed", "id", poolID, "error", err)
	}
	p.mu.Lock()
	delete(p.stages, poolID)
	p.mu.Unlock()
	return nil
}

// buildEncoder resolves the stream's Upstream string and constructs the
// matching EncoderStage. The Source/Composer must already be registered.
func (p *Pipeline) buildEncoder(s Stream) (*EncoderStage, error) {
	video, err := p.resolveUpstream(s.Upstream)
	if err != nil {
		return nil, err
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

// resolveUpstream maps a stream's Upstream reference ("source:<id>" or
// "composer:<id>") to a FrameSource that points at the right SCM socket.
func (p *Pipeline) resolveUpstream(upstream string) (FrameSource, error) {
	kind, id, ok := splitUpstream(upstream)
	if !ok {
		return nil, fmt.Errorf("pipeline: invalid upstream %q (want source:<id> or composer:<id>)", upstream)
	}
	switch kind {
	case "source":
		if _, found := p.sources.Get(id); !found {
			return nil, fmt.Errorf("pipeline: upstream source %q not registered", id)
		}
		return ProducerFrameSource{Socket: SCMSocketPathFor(id)}, nil
	case "composer":
		c, found := p.composers.Get(id)
		if !found {
			return nil, fmt.Errorf("pipeline: upstream composer %q not registered", id)
		}
		w := c.Canvas.W
		h := c.Canvas.H
		if w == 0 || h == 0 {
			w, h = 1920, 1080
		}
		sock := SCMSocketPathFor("composer-" + id)
		return ComposerFrameSource{Socket: sock, Width: w, Height: h, Fps: 30}, nil
	default:
		return nil, fmt.Errorf("pipeline: unknown upstream kind %q", kind)
	}
}

// splitUpstream parses "source:foo" / "composer:bar" into (kind, id).
func splitUpstream(s string) (kind, id string, ok bool) {
	idx := strings.IndexByte(s, ':')
	if idx <= 0 || idx == len(s)-1 {
		return "", "", false
	}
	return s[:idx], s[idx+1:], true
}

// Pool exposes the underlying process.Pool for callers that need status
// lookups. Not used by Apply/Delete; only diagnostics should reach for
// it.
func (p *Pipeline) Pool() process.Pool { return p.pool }

// Sources exposes the SourceRegistry for diagnostics.
func (p *Pipeline) Sources() *SourceRegistry { return p.sources }

// Composers exposes the ComposerRegistry for diagnostics.
func (p *Pipeline) Composers() *ComposerRegistry { return p.composers }

// ----- internal helpers -----

// entityLock returns a per-entity mutex, creating one on first use.
func (p *Pipeline) entityLock(key string) *sync.Mutex {
	p.entityLocksMu.Lock()
	defer p.entityLocksMu.Unlock()
	if mu, ok := p.entityLocks[key]; ok {
		return mu
	}
	mu := &sync.Mutex{}
	p.entityLocks[key] = mu
	return mu
}

// onStateChange forwards pool state transitions to the event bus.
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

// commandFor is the pool's CommandProvider callback.
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

// configureProcess is the pool's Configurer callback. Wires the stage's
// LogParser + LogAttrs into the process.Process so each stage's stderr
// lands in journald with the right module + structured fields.
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

// replaceStage swaps the registered stage for an id. Caller follows
// with restartStage to spawn/respawn under the new spec.
func (p *Pipeline) replaceStage(stage Stage) {
	p.mu.Lock()
	p.stages[stage.ID()] = stage
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

func (p *Pipeline) ensureUdsDir() error {
	return os.MkdirAll(NativeUdsDir, 0o755)
}
