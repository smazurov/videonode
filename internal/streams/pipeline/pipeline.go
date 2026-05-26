package pipeline

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/smazurov/videonode/internal/events"
	"github.com/smazurov/videonode/internal/logging"
	"github.com/smazurov/videonode/internal/metrics/collectors"
	"github.com/smazurov/videonode/internal/process"
	"github.com/smazurov/videonode/internal/snapshots"
	"github.com/smazurov/videonode/internal/streams/pipelinectl"
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

	// ControlServer, when non-nil, is used by ApplyComposer to register
	// the spawned videonode-composer over gRPC and push its initial
	// SetCanvas / SetSource / SetLayout / SetEffects RPCs. Without this
	// the composer starts but its canvas stays uninitialized and
	// downstream encoders see only black frames.
	ControlServer *pipelinectl.Manager

	// Registry, when non-nil, is used to Touch entities on pool state
	// transitions so their SSE snapshot refreshes with the new status.
	Registry *events.Registry

	// EncoderResolver maps a logical codec ("h264", "h265") and the
	// upstream pixel format ("nv12", "bgra", "") to the best validated
	// ffmpeg encoder + its HW-specific plumbing. Called by buildEncoder
	// when assembling an EncoderStage. When nil the pipeline falls back
	// to libx264/libx265.
	EncoderResolver func(codec, inputPixFmt string) (EncoderResolution, error)
}

// EncoderResolution is the output of Config.EncoderResolver: the
// concrete ffmpeg encoder name plus any backend-specific args the
// encoder needs (e.g. -vaapi_device, hwupload filter chain).
type EncoderResolution struct {
	EncoderName  string
	GlobalArgs   []string
	VideoFilters string
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

	mu         sync.Mutex
	stages     map[string]Stage                       // pool key → stage
	collectors map[string]*collectors.FFmpegCollector // stream id → progress collector

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
		collectors:  make(map[string]*collectors.FFmpegCollector),
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
	// Drop any prior control-plane registration so the next config push
	// re-dials the freshly-spawned UDS instead of holding a dead handle.
	if p.cfg.ControlServer != nil {
		p.cfg.ControlServer.Unregister(s.ID)
	}
	if err := p.restartStage(stage); err != nil {
		return err
	}
	// Initial gRPC registration + (when the spec carries one) SetFormat
	// push. Without registration the daemon can't reach the source over
	// gRPC at all, so SetFormat / Snapshot / status would all silently
	// fail. Mirrors the composer-side pushComposerConfig pattern.
	if p.cfg.ControlServer != nil {
		go p.registerAndConfigureSource(s, stage.GrpcUds)
	}
	return nil
}

// registerAndConfigureSource dials the source's gRPC UDS with a 30 s
// deadline (100 ms retry) so the source process has time to bind, then
// pushes the operator-selected V4L2 format if the spec carries one. Runs
// in its own goroutine; never blocks ApplySource.
func (p *Pipeline) registerAndConfigureSource(s Source, udsPath string) {
	sourceID := s.ID
	tag := []any{"source_id", sourceID, "uds", udsPath}

	const dialDeadline = 30 * time.Second
	const callTimeout = 5 * time.Second
	mgr := p.cfg.ControlServer

	deadline := time.Now().Add(dialDeadline)
	var lastErr error
	for {
		if time.Now().After(deadline) {
			p.logger.Warn("registerAndConfigureSource: register never succeeded",
				append(tag, "error", lastErr)...)
			return
		}
		regCtx, regCancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		err := mgr.RegisterSource(regCtx, sourceID, udsPath)
		regCancel()
		if err == nil {
			break
		}
		lastErr = err
		time.Sleep(100 * time.Millisecond)
	}

	if s.Format == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), callTimeout)
	defer cancel()
	if _, err := mgr.SendSetFormat(ctx, sourceID, pipelinectl.SetFormatParams{
		FourCC: s.Format.FourCC,
		W:      s.Format.Width,
		H:      s.Format.Height,
		FPS:    s.Format.FPS,
	}); err != nil {
		p.logger.Warn("registerAndConfigureSource: initial SetFormat failed",
			append(tag, "error", err)...)
		return
	}
	p.logger.Info("source initial format pushed",
		append(tag, "fourcc", s.Format.FourCC,
			"w", s.Format.Width, "h", s.Format.Height, "fps", s.Format.FPS)...)
}

// UpdateSourceFormat hot-applies a new V4L2 capture format to an already-
// registered source via the gRPC control plane. Does NOT restart the
// source process — connected SCM_RIGHTS consumers stay attached.
// Returns an error if the source isn't registered or the RPC fails;
// callers (source_service.Update) fall back to ApplySource on error.
func (p *Pipeline) UpdateSourceFormat(id string, f SourceFormat) error {
	if id == "" {
		return errors.New("pipeline: source.ID is required")
	}
	if p.cfg.ControlServer == nil {
		return errors.New("pipeline: ControlServer is nil; cannot hot-apply format")
	}
	cur, found := p.sources.Get(id)
	if !found {
		return fmt.Errorf("pipeline: source %q not registered", id)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := p.cfg.ControlServer.SendSetFormat(ctx, id, pipelinectl.SetFormatParams{
		FourCC: f.FourCC,
		W:      f.Width,
		H:      f.Height,
		FPS:    f.FPS,
	}); err != nil {
		return err
	}
	cur.Format = &f
	p.sources.Put(cur)
	return nil
}

// RegisterSource validates and populates the in-memory source registry
// without spawning a process. Used when the pipeline master switch is
// off so the registry stays current for upstream-ref resolution while
// no processes are running.
func (p *Pipeline) RegisterSource(s Source) error {
	if s.ID == "" {
		return errors.New("pipeline: source.ID is required")
	}
	if s.TestMode && s.Device != "" {
		return fmt.Errorf("pipeline: source %s has both Device and TestMode set", s.ID)
	}
	if !s.TestMode && s.Device == "" {
		return fmt.Errorf("pipeline: source %s requires one of Device or TestMode", s.ID)
	}
	p.sources.Put(s)
	return nil
}

// StopSource stops the source's process and tears down its gRPC
// registration and stage, but preserves the registry entry so the UI
// shows the entity as idle. Used by StopPipeline.
func (p *Pipeline) StopSource(id string) error {
	if id == "" {
		return nil
	}
	mu := p.entityLock("source:" + id)
	mu.Lock()
	defer mu.Unlock()

	if p.cfg.ControlServer != nil {
		p.cfg.ControlServer.Unregister(id)
	}
	poolID := SourcePoolKey(id)
	if err := p.pool.Stop(poolID); err != nil {
		p.logger.Warn("StopSource: pool.Stop failed", "id", poolID, "error", err)
	}
	p.mu.Lock()
	delete(p.stages, poolID)
	p.mu.Unlock()
	return nil
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
	if p.cfg.ControlServer != nil {
		p.cfg.ControlServer.Unregister(id)
	}
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

	canvasW, canvasH := c.Canvas.W, c.Canvas.H
	if canvasW <= 0 || canvasH <= 0 {
		canvasW, canvasH = 1920, 1080
	}
	stage := &ComposerStage{
		ComposerID: c.ID,
		BinaryPath: p.cfg.VNComposerBin,
		DRMDevice:  p.cfg.DRMDevice,
		CanvasFPS:  canvasFPSOrDefault(c.Canvas.FPS),
		CanvasW:    canvasW,
		CanvasH:    canvasH,
		GrpcUds:    GrpcSocketPathFor("composer", c.ID),
	}
	p.replaceStage(stage)
	// Drop any prior control-plane registration so the next config push
	// re-dials the freshly-spawned UDS instead of holding a dead handle.
	if p.cfg.ControlServer != nil {
		p.cfg.ControlServer.Unregister(c.ID)
	}
	if err := p.restartStage(stage); err != nil {
		return err
	}
	// Initial gRPC config push (SetCanvas/SetSource/SetLayout/SetEffects).
	// Without this the composer process runs but its canvas stays
	// uninitialized and every downstream encoder sees black frames.
	if p.cfg.ControlServer != nil {
		go p.pushComposerConfig(c, stage.GrpcUds)
	}
	return nil
}

// pushComposerConfig waits for the composer's gRPC UDS to come up,
// registers it with the control plane, then issues the initial
// SetCanvas / SetSource(per input) / SetLayout / SetEffects(per input
// with effect) RPCs. Runs in its own goroutine; bounded by a 30 s
// register deadline and a 5 s per-call timeout.
func (p *Pipeline) pushComposerConfig(c Composer, udsPath string) {
	composerID := c.ID
	tag := []any{"composer_id", composerID, "uds", udsPath}

	const dialDeadline = 30 * time.Second
	const callTimeout = 5 * time.Second
	mgr := p.cfg.ControlServer

	deadline := time.Now().Add(dialDeadline)
	var lastErr error
	for {
		if time.Now().After(deadline) {
			p.logger.Warn("pushComposerConfig: register never succeeded",
				append(tag, "error", lastErr)...)
			return
		}
		regCtx, regCancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		err := mgr.RegisterComposer(regCtx, composerID, udsPath)
		regCancel()
		if err == nil {
			break
		}
		lastErr = err
		time.Sleep(100 * time.Millisecond)
	}

	push := func(name string, fn func(context.Context) error) bool {
		ctx, cancel := context.WithTimeout(context.Background(), callTimeout)
		defer cancel()
		if err := fn(ctx); err != nil {
			p.logger.Warn("pushComposerConfig: rpc failed",
				append(append([]any{}, tag...), "method", name, "error", err)...)
			return false
		}
		return true
	}

	canvasW, canvasH := c.Canvas.W, c.Canvas.H
	if canvasW <= 0 || canvasH <= 0 {
		canvasW, canvasH = 1920, 1080
	}
	fps := uint32(canvasFPSOrDefault(c.Canvas.FPS))
	if !push("set_canvas", func(ctx context.Context) error {
		return mgr.SendSetCanvas(ctx, composerID, pipelinectl.SetCanvasParams{
			W: uint32(canvasW), H: uint32(canvasH), FPS: fps,
		})
	}) {
		return
	}

	// Bind each input to a slot. Slot label is positional ("a", "b", …)
	// to stay wire-compatible with the existing composer protocol.
	for i, in := range c.Inputs {
		slot := slotNameFor(i)
		sourceID := strings.TrimPrefix(in.Ref, "source:")
		if !push("set_source", func(ctx context.Context) error {
			return mgr.SendSetSource(ctx, composerID, pipelinectl.SetSourceParams{
				Slot:     slot,
				SourceID: sourceID,
				ScmPath:  SCMSocketPathFor(sourceID),
				Width:    uint32(canvasW),
				Height:   uint32(canvasH),
				FPS:      fps,
			})
		}) {
			return
		}
	}

	// SetLayout — map each LayoutSlot's Input ref to the matching input
	// index (slot label). When no layout is provided, fall back to a
	// single full-canvas slot referencing input 0.
	slots := make([]pipelinectl.LayoutSlotEntry, 0, len(c.Layout))
	if len(c.Layout) > 0 {
		inputIdx := make(map[string]int, len(c.Inputs))
		for i, in := range c.Inputs {
			inputIdx[in.Ref] = i
		}
		for _, l := range c.Layout {
			idx, ok := inputIdx[l.Input]
			if !ok {
				p.logger.Warn("pushComposerConfig: layout references unknown input",
					append(tag, "input", l.Input)...)
				continue
			}
			slots = append(slots, pipelinectl.LayoutSlotEntry{
				Slot: slotNameFor(idx),
				X:    int32(l.X), Y: int32(l.Y), W: int32(l.W), H: int32(l.H),
			})
		}
	} else if len(c.Inputs) > 0 {
		slots = append(slots, pipelinectl.LayoutSlotEntry{
			Slot: slotNameFor(0), X: 0, Y: 0, W: int32(canvasW), H: int32(canvasH),
		})
	}
	if !push("set_layout", func(ctx context.Context) error {
		return mgr.SendSetLayout(ctx, composerID, pipelinectl.SetLayoutParams{Slots: slots})
	}) {
		return
	}

	// SetEffects per input that carries one.
	for _, in := range c.Inputs {
		if in.Effect == nil {
			continue
		}
		sourceID := strings.TrimPrefix(in.Ref, "source:")
		effect := *in.Effect
		if !push("set_effects", func(ctx context.Context) error {
			return mgr.SendSetEffects(ctx, composerID, pipelinectl.SetEffectsParams{
				SourceID: sourceID,
				Effects: []pipelinectl.EffectParams{{
					Type:           effect.Type,
					Corners:        effect.Corners,
					SnapshotWidth:  effect.SnapshotW,
					SnapshotHeight: effect.SnapshotH,
				}},
			})
		}) {
			return
		}
	}

	p.logger.Info("composer initial config pushed",
		append(tag, "canvas", fmt.Sprintf("%dx%d@%dfps", canvasW, canvasH, fps),
			"sources", len(c.Inputs))...)
}

// slotNameFor returns the alphabetic slot label used by the composer
// control plane ("a"..."z"; "slotN" past 26 for safety).
func slotNameFor(i int) string {
	if i < 0 || i > 25 {
		return fmt.Sprintf("slot%d", i)
	}
	return string(rune('a' + i))
}

// UpdateComposerLayout hot-applies a new layout to a running composer
// via the gRPC control plane. Does NOT restart the composer process —
// callers use this for layout-only edits to avoid killing downstream
// vn-sink consumers. Returns an error if the composer isn't registered
// (e.g. spawn hasn't finished) or if any RPC fails.
func (p *Pipeline) UpdateComposerLayout(id string, layout []LayoutSlot) error {
	if id == "" {
		return errors.New("pipeline: composer.ID is required")
	}
	if p.cfg.ControlServer == nil {
		return errors.New("pipeline: ControlServer is nil; cannot hot-apply layout")
	}
	c, found := p.composers.Get(id)
	if !found {
		return fmt.Errorf("pipeline: composer %q not registered", id)
	}
	inputIdx := make(map[string]int, len(c.Inputs))
	for i, in := range c.Inputs {
		inputIdx[in.Ref] = i
	}
	slots := make([]pipelinectl.LayoutSlotEntry, 0, len(layout))
	for _, l := range layout {
		idx, ok := inputIdx[l.Input]
		if !ok {
			return fmt.Errorf("pipeline: layout references unknown input %q", l.Input)
		}
		slots = append(slots, pipelinectl.LayoutSlotEntry{
			Slot: slotNameFor(idx),
			X:    int32(l.X), Y: int32(l.Y), W: int32(l.W), H: int32(l.H),
		})
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return p.cfg.ControlServer.SendSetLayout(ctx, id, pipelinectl.SetLayoutParams{Slots: slots})
}

// UpdateComposerEffect hot-applies an effect change for one input. A
// nil effect clears the input's effect list. Does NOT restart the
// composer process.
func (p *Pipeline) UpdateComposerEffect(id, inputRef string, effect *Effect) error {
	if id == "" {
		return errors.New("pipeline: composer.ID is required")
	}
	if p.cfg.ControlServer == nil {
		return errors.New("pipeline: ControlServer is nil; cannot hot-apply effect")
	}
	if _, found := p.composers.Get(id); !found {
		return fmt.Errorf("pipeline: composer %q not registered", id)
	}
	sourceID := strings.TrimPrefix(inputRef, "source:")
	params := pipelinectl.SetEffectsParams{SourceID: sourceID}
	if effect != nil {
		params.Effects = []pipelinectl.EffectParams{{
			Type:           effect.Type,
			Corners:        effect.Corners,
			SnapshotWidth:  effect.SnapshotW,
			SnapshotHeight: effect.SnapshotH,
		}}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return p.cfg.ControlServer.SendSetEffects(ctx, id, params)
}

// RegisterComposer validates and populates the in-memory composer
// registry without spawning a process. Used when the pipeline master
// switch is off.
func (p *Pipeline) RegisterComposer(c Composer) error {
	if c.ID == "" {
		return errors.New("pipeline: composer.ID is required")
	}
	p.composers.Put(c)
	return nil
}

// StopComposer stops the composer's process and tears down its gRPC
// registration and stage, but preserves the registry entry so the UI
// shows the entity as idle. Used by StopPipeline.
func (p *Pipeline) StopComposer(id string) error {
	if id == "" {
		return nil
	}
	mu := p.entityLock("composer:" + id)
	mu.Lock()
	defer mu.Unlock()

	if p.cfg.ControlServer != nil {
		p.cfg.ControlServer.Unregister(id)
	}
	poolID := ComposerPoolKey(id)
	if err := p.pool.Stop(poolID); err != nil {
		p.logger.Warn("StopComposer: pool.Stop failed", "id", poolID, "error", err)
	}
	p.mu.Lock()
	delete(p.stages, poolID)
	p.mu.Unlock()
	return nil
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
	if p.cfg.ControlServer != nil {
		p.cfg.ControlServer.Unregister(id)
	}
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
	p.ensureCollector(s.ID)
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

	p.stopCollector(id)
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
// The concrete ffmpeg encoder is chosen by Config.EncoderResolver from
// the stream's Codec + the upstream pixel format.
func (p *Pipeline) buildEncoder(s Stream) (*EncoderStage, error) {
	video, err := p.resolveUpstream(s.Upstream)
	if err != nil {
		return nil, err
	}

	resolved := p.resolveEncoder(s.Encoder.Codec, video)

	return &EncoderStage{
		OwnerStreamID:     s.ID,
		Media:             MediaSource{Video: video, Audio: ALSADirectAudio{Config: s.Audio}},
		Cfg:               s.Encoder,
		Resolved:          resolved,
		Publish:           s.Publish,
		CustomEncoderArgs: s.CustomEncoderArgs,
		VNSinkBin:         p.cfg.VNSinkBin,
	}, nil
}

// resolveEncoder calls the configured EncoderResolver or falls back to
// libx264/libx265 when no resolver is wired.
func (p *Pipeline) resolveEncoder(codec string, video FrameSource) EncoderResolution {
	inputPixFmt := ""
	if video != nil {
		switch video.Kind() {
		case FrameKindBGRARaw:
			inputPixFmt = "bgra"
		default:
			inputPixFmt = "nv12"
		}
	}

	if p.cfg.EncoderResolver != nil {
		res, err := p.cfg.EncoderResolver(codec, inputPixFmt)
		if err != nil {
			p.logger.Warn("EncoderResolver failed, using software fallback",
				"codec", codec, "input_pix_fmt", inputPixFmt, "error", err)
		} else {
			p.logger.Info("Resolved encoder", "codec", codec, "encoder", res.EncoderName)
			return res
		}
	}

	fallback := "libx264"
	if codec == "h265" || codec == "hevc" {
		fallback = "libx265"
	}
	return EncoderResolution{EncoderName: fallback}
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
		src, found := p.sources.Get(id)
		if !found {
			return nil, fmt.Errorf("pipeline: upstream source %q not registered", id)
		}
		pfs := ProducerFrameSource{Socket: SCMSocketPathFor(id)}
		if src.Format != nil {
			pfs.Width = int(src.Format.Width)
			pfs.Height = int(src.Format.Height)
			pfs.Fps = int(src.Format.FPS)
		}
		if pfs.Width == 0 || pfs.Height == 0 {
			pfs.Width, pfs.Height = 1920, 1080
		}
		if pfs.Fps == 0 {
			pfs.Fps = 30
		}
		return pfs, nil
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
		return ComposerFrameSource{Socket: sock, Width: w, Height: h, Fps: canvasFPSOrDefault(c.Canvas.FPS)}, nil
	default:
		return nil, fmt.Errorf("pipeline: unknown upstream kind %q", kind)
	}
}

// splitUpstream parses "source:foo" / "composer:bar" into (kind, id).
func splitUpstream(s string) (kind, id string, ok bool) {
	return SplitUpstream(s)
}

// SplitUpstream parses an upstream reference like "source:foo" or
// "composer:bar" into (kind, id, true). Returns ok=false for malformed
// input.
func SplitUpstream(s string) (kind, id string, ok bool) {
	idx := strings.IndexByte(s, ':')
	if idx <= 0 || idx == len(s)-1 {
		return "", "", false
	}
	return s[:idx], s[idx+1:], true
}

// StopEncoder stops the encoder stage for a stream if it is running.
// No-op when the encoder isn't running.
func (p *Pipeline) StopEncoder(streamID string) error {
	if streamID == "" {
		return errors.New("pipeline: streamID is required")
	}
	mu := p.entityLock("stream:" + streamID)
	mu.Lock()
	defer mu.Unlock()

	id := EncoderIDFor(streamID)
	if !p.pool.IsRunning(id) {
		return nil
	}
	if err := p.pool.Stop(id); err != nil {
		return fmt.Errorf("pipeline: stop encoder %s: %w", id, err)
	}
	return nil
}

// EnsureEncoder starts the encoder stage for a stream if it isn't
// already running. Requires ApplyStream to have been called previously.
func (p *Pipeline) EnsureEncoder(streamID string) error {
	if streamID == "" {
		return errors.New("pipeline: streamID is required")
	}
	mu := p.entityLock("stream:" + streamID)
	mu.Lock()
	defer mu.Unlock()

	id := EncoderIDFor(streamID)
	if p.pool.IsRunning(id) {
		return nil
	}
	p.mu.Lock()
	_, ok := p.stages[id]
	p.mu.Unlock()
	if !ok {
		return fmt.Errorf("pipeline: no cached encoder stage for stream %s (ApplyStream not called)", streamID)
	}
	if err := p.pool.Start(id); err != nil {
		return fmt.Errorf("pipeline: start encoder %s: %w", id, err)
	}
	return nil
}

// Pool exposes the underlying process.Pool for callers that need status
// lookups. Not used by Apply/Delete; only diagnostics should reach for
// it.
func (p *Pipeline) Pool() process.Pool { return p.pool }

// SnapshotSource dials the source's gRPC Snapshot RPC and returns the
// raw NV12 frame + metadata. The daemon's snapshot cache JPEG-encodes
// on demand; the Pipeline no longer ffmpeg-encodes inline.
func (p *Pipeline) SnapshotSource(ctx context.Context, sourceID string) (snapshots.Frame, error) {
	if p.cfg.ControlServer == nil {
		return snapshots.Frame{}, fmt.Errorf("no control server for snapshot")
	}
	resp, err := p.cfg.ControlServer.Snapshot(ctx, sourceID)
	if err != nil {
		return snapshots.Frame{}, err
	}
	return snapshots.Frame{
		Bytes:      resp.GetNv12(),
		Format:     snapshots.FormatNV12,
		Width:      int(resp.GetWidth()),
		Height:     int(resp.GetHeight()),
		FrameIdx:   resp.GetFrameIdx(),
		CapturedNs: resp.GetCapturedAtNs(),
	}, nil
}

// SnapshotComposer dials the composer's gRPC Snapshot RPC and returns
// the raw BGRA canvas frame + metadata.
func (p *Pipeline) SnapshotComposer(ctx context.Context, composerID string) (snapshots.Frame, error) {
	if p.cfg.ControlServer == nil {
		return snapshots.Frame{}, fmt.Errorf("no control server for snapshot")
	}
	resp, err := p.cfg.ControlServer.ComposerSnapshot(ctx, composerID)
	if err != nil {
		return snapshots.Frame{}, err
	}
	return snapshots.Frame{
		Bytes:      resp.GetBgra(),
		Format:     snapshots.FormatBGRA,
		Width:      int(resp.GetWidth()),
		Height:     int(resp.GetHeight()),
		FrameIdx:   resp.GetFrameIdx(),
		CapturedNs: resp.GetCapturedAtNs(),
	}, nil
}

// Sources exposes the SourceRegistry for diagnostics.
func (p *Pipeline) Sources() *SourceRegistry { return p.sources }

// Composers exposes the ComposerRegistry for diagnostics.
func (p *Pipeline) Composers() *ComposerRegistry { return p.composers }

// ensureCollector starts an FFmpegCollector for the given stream if one
// isn't already running. The collector creates its Unix socket before
// returning, so it's ready to receive data when FFmpeg spawns.
func (p *Pipeline) ensureCollector(streamID string) {
	p.mu.Lock()
	_, exists := p.collectors[streamID]
	p.mu.Unlock()
	if exists {
		return
	}
	if err := p.ensureUdsDir(); err != nil {
		p.logger.Warn("ensureCollector: mkdir failed", "stream_id", streamID, "error", err)
		return
	}
	sockPath := ProgressSocketPathFor(streamID)
	c := collectors.NewFFmpegCollector(sockPath, streamID)
	if err := c.Start(context.Background()); err != nil {
		p.logger.Warn("ensureCollector: start failed", "stream_id", streamID, "error", err)
		return
	}
	p.mu.Lock()
	p.collectors[streamID] = c
	p.mu.Unlock()
}

// stopCollector stops and removes the FFmpegCollector for the given
// stream. Idempotent.
func (p *Pipeline) stopCollector(streamID string) {
	p.mu.Lock()
	c, ok := p.collectors[streamID]
	if ok {
		delete(p.collectors, streamID)
	}
	p.mu.Unlock()
	if ok {
		c.Stop()
	}
}

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

// onStateChange forwards pool state transitions to the event bus and
// touches the owning entity so its SSE snapshot refreshes with the new
// pool-derived status field.
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

	if p.cfg.Registry != nil {
		switch {
		case strings.HasPrefix(id, "producer:"):
			sourceID := strings.TrimPrefix(id, "producer:")
			p.cfg.Registry.Touch(context.Background(), "source", sourceID)
			if newState == process.StateIdle {
				p.cfg.Registry.Publish("source", events.ActionStatus, sourceID, map[string]any{
					"started_at_us": nil,
					"ts_ms":         time.Now().UnixMilli(),
				})
			}
		case strings.HasPrefix(id, "composer:"):
			p.cfg.Registry.Touch(context.Background(), "composer", strings.TrimPrefix(id, "composer:"))
		case strings.HasPrefix(id, "encoder:"):
			p.cfg.Registry.Touch(context.Background(), "stream", strings.TrimPrefix(id, "encoder:"))
		}
	}
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

// canvasFPSOrDefault returns fps when positive, otherwise the daemon
// default canvas frame rate. Centralized so every composer-spawn /
// SetCanvas / downstream-encoder path agrees on the fallback.
func canvasFPSOrDefault(fps int) int {
	if fps > 0 {
		return fps
	}
	return DefaultCanvasFPS
}
