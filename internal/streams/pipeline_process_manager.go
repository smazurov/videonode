package streams

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/smazurov/videonode/internal/encoders"
	"github.com/smazurov/videonode/internal/encoders/validation"
	"github.com/smazurov/videonode/internal/ffmpeg"
	"github.com/smazurov/videonode/internal/logging"
	"github.com/smazurov/videonode/internal/recording"
	"github.com/smazurov/videonode/internal/streams/pipeline"
	"github.com/smazurov/videonode/internal/streams/pipelinectl"
	"github.com/smazurov/videonode/internal/types"
)

// buildEncoderPreviewCommand returns the same shell command the
// Pipeline's EncoderStage would emit for this spec, without spawning
// anything. Used by GetFFmpegCommand / the /api/streams/{id}/ffmpeg
// debug endpoint to surface the current expected argv.
//
// Honors the same CustomEncoderArgs / inline-composer / producer-direct
// routing the live Apply path uses, so the preview matches reality.
func buildEncoderPreviewCommand(
	spec StreamSpec,
	rtspHost string,
	deviceResolver func(string) string,
	provider types.ValidationProvider,
) (string, error) {
	ps := specToPipelineStream(spec, rtspHost)
	r := resolveEncoder(ps.Encoder.Codec, provider)
	ps.Encoder.EncoderName = r.Name
	ps.Encoder.GlobalArgs = r.GlobalArgs
	ps.Encoder.VideoFilters = r.VideoFilters
	if spec.Canvas != nil {
		for i := range ps.Inputs {
			if src, ok := lookupStoreDeviceFromResolver(spec.Canvas.SourceStreams, i, deviceResolver); ok {
				ps.Inputs[i].Device = src
			}
		}
	} else if deviceResolver != nil && len(ps.Inputs) > 0 && ps.Inputs[0].Device == "" {
		ps.Inputs[0].Device = deviceResolver(spec.Device)
	}

	var video pipeline.FrameSource
	switch {
	case pipeline.NeedsComposer(ps):
		w, h := composerCanvasDims(ps)
		composerID := pipeline.ComposerIDFor(spec.ID)
		video = pipeline.InlineComposerFrameSource{
			ComposerBin: "videonode-composer",
			DRMDevice:   "/dev/dri/renderD128",
			GrpcUds:     pipeline.GrpcSocketPathFor("composer", composerID),
			ComposerID:  composerID,
			Width:       w, Height: h, Fps: 30,
		}
	case len(ps.Inputs) == 1:
		video = pipeline.ProducerFrameSource{
			Socket: pipeline.SCMSocketPathFor(ps.Inputs[0].Device),
		}
	default:
		return "", fmt.Errorf("buildEncoderPreviewCommand: stream %s has no inputs", spec.ID)
	}

	enc := &pipeline.EncoderStage{
		StreamID_: spec.ID,
		Media: pipeline.MediaSource{
			Video: video,
			Audio: pipeline.ALSADirectAudio{Config: ps.Audio},
		},
		Cfg:               ps.Encoder,
		Publish:           ps.Publish,
		CustomEncoderArgs: ps.CustomEncoderArgs,
		VNSinkBin:         "vn-sink",
	}
	argv, _, err := enc.Command()
	if err != nil {
		return "", err
	}
	if len(argv) >= 3 && argv[0] == "/bin/sh" {
		return argv[2], nil
	}
	return strings.Join(argv, " "), nil
}

// lookupStoreDeviceFromResolver is a tiny helper for resolving the
// i-th canvas source's device. Returns ("", false) when out-of-bounds.
func lookupStoreDeviceFromResolver(
	sources []string,
	i int,
	resolver func(string) string,
) (string, bool) {
	if i < 0 || i >= len(sources) || resolver == nil {
		return "", false
	}
	return sources[i], true
}

// pipelineProcessManager implements the StreamProcessManager interface
// using internal/streams/pipeline as the supervisor. Replaces the
// legacy streamProcessManager + processor + canvasProcessor +
// producerManager stack with a thin translation layer that:
//   - converts StreamSpec → pipeline.Stream at the boundary
//   - delegates lifecycle to Pipeline.Apply / Pipeline.Delete
//   - exposes runtime state via Pipeline.Snapshot / Producers / Pool
//
// Built when ServiceOptions.Native is non-nil; otherwise NewStreamService
// falls back to the legacy streamProcessManager (testing harnesses, R
// scenarios that don't spawn binaries).
type pipelineProcessManager struct {
	pipe          *pipeline.Pipeline
	store         Store
	controlServer *pipelinectl.Manager
	rtspHost      string // resolved host:port for the RTSP publish URL
	// validation provides probe results so the encoder name resolves to
	// what the host actually supports (libx264 on a dev box, h264_rkmpp
	// on RK3588). Built from store via NewValidationService.
	validation types.ValidationProvider
	logger     logging.Logger
}

// NewPipelineProcessManager constructs a StreamProcessManager backed by
// internal/streams/pipeline. The pipeline is configured from
// opts.Native + opts.ControlServer + the daemon's device resolver.
//
// The rtspPort parameter accepts the same shape as
// ServiceOptions.RTSPPort — empty, ":8654", "host:port" — and is
// normalized via resolveRTSPHost() (the helper that powers the legacy
// GPU compose path's ffmpeg sink URL).
func NewPipelineProcessManager(
	pipe *pipeline.Pipeline,
	store Store,
	controlServer *pipelinectl.Manager,
	rtspPort string,
) StreamProcessManager {
	return &pipelineProcessManager{
		pipe:          pipe,
		store:         store,
		controlServer: controlServer,
		rtspHost:      resolveRTSPHost(rtspPort),
		validation:    NewValidationService(store),
		logger:        logging.GetLogger("pipeline_pm"),
	}
}

// resolvedEncoder bundles the encoder name with the extra plumbing
// some HW backends need (vaapi: -vaapi_device + format=nv12,hwupload;
// rkmpp: hwaccel flags). The pipeline forwards all three to
// ffmpeg.Params so the encoder isn't picked without the args it needs.
type resolvedEncoder struct {
	Name         string
	GlobalArgs   []string
	VideoFilters string
}

// resolveEncoder returns the ffmpeg encoder + supporting args for a
// logical codec ("h264"/"h265") using a layered fallback:
//  1. Preloaded validation data — yields the encoder name plus the
//     backend-specific GlobalArgs / VideoFilters needed to make it run
//     (the validator knows what -vaapi_device etc. each backend wants).
//  2. Probe ffmpeg's compiled encoders (`ffmpeg -encoders`). Reflects
//     what's actually installed without prior validation. Returns only
//     the encoder name — no supporting args, since the probe doesn't
//     know what setup each backend needs. Picks libx264/libx265 in
//     preference to vaapi (which needs setup we don't emit) so the
//     stream actually starts on bare-default hosts.
//  3. Hard-coded software fallback.
func resolveEncoder(codec string, provider types.ValidationProvider) resolvedEncoder {
	if codec == "" {
		codec = "h264"
	}
	if provider != nil {
		if cfg, err := encoders.MapAPICodec(codec, provider); err == nil && cfg != nil {
			r := resolvedEncoder{Name: cfg.EncoderName}
			if cfg.Settings != nil {
				r.GlobalArgs = append([]string(nil), cfg.Settings.GlobalArgs...)
				r.VideoFilters = cfg.Settings.VideoFilters
			}
			return r
		}
	}
	return resolvedEncoder{Name: validation.AutodetectEncoder(codec)}
}

// specToPipelineStream converts a StreamSpec (canvas-or-source dichotomy)
// into a pipeline.Stream (unified Inputs+Layout+Effects shape) using
// rtspHost (resolveRTSPHost-output) as the publish target's host:port.
//
// Free function rather than method so test code can call it without
// constructing a full manager. The
// translation is lossy in two known directions:
//   - StreamSpec.TestMode is preserved on the pipeline.Stream surface
//     but the daemon currently ignores it (matches CLAUDE.md guidance:
//     test mode is a no-op until follow-up RPC + test-producer work).
//   - StreamSpec.Vision is dropped on the pipeline side — vision frames
//     come from the Producer's gRPC Snapshot RPC on the new path.
func specToPipelineStream(spec StreamSpec, rtspHost string) pipeline.Stream {
	s := pipeline.Stream{
		ID:                spec.ID,
		Name:              spec.Name,
		TestMode:          spec.TestMode,
		CustomEncoderArgs: spec.CustomFFmpegCommand,
		Audio: pipeline.AudioConfig{
			Devices: nil,
			Codec:   "",
			Bitrate: "",
			Filters: spec.FFmpeg.AudioDevice, // approximate; ALSA wrapper picks up Devices below
		},
		Encoder: pipeline.EncoderConfig{
			Codec:   spec.FFmpeg.Codec,
			Preset:  "",
			GOP:     0,
			BFrames: 0,
		},
		CreatedAt: spec.CreatedAt,
		UpdatedAt: spec.UpdatedAt,
	}

	if spec.FFmpeg.AudioDevice != "" {
		s.Audio.Devices = []string{spec.FFmpeg.AudioDevice}
		s.Audio.Filters = ""
	}
	if spec.FFmpeg.QualityParams != nil && spec.FFmpeg.QualityParams.TargetBitrate != nil {
		s.Encoder.Bitrate = fmt.Sprintf("%.0fM", *spec.FFmpeg.QualityParams.TargetBitrate)
	}

	switch {
	case spec.Canvas != nil:
		// Legacy canvas API users expect a composer regardless of input
		// count — existing smoke + UI flows wait for "composer registered"
		// the moment a canvas spec is POSTed. Honor that by forcing the
		// composer stage on; native-only streams (created through the new
		// pipeline.Stream API) opt in via picker rules (N>1 OR effects).
		s.ForceComposer = true
		// Canvas → N inputs, layout from the canvas's SourceStreams + a
		// trivial side-by-side default. The legacy ComputeCanvasLayout
		// solver derived per-source rects; this path simplifies to "place
		// each source full-width in a uniform stack" pending the
		// layout-solver port (follow-up).
		w := spec.Canvas.Width
		h := spec.Canvas.Height
		if w == 0 {
			w = 1920
		}
		if h == 0 {
			h = 1080
		}
		for i, srcID := range spec.Canvas.SourceStreams {
			// Device id resolved by the caller (it has the store) — left
			// empty here so specToPipelineStream stays pure.
			s.Inputs = append(s.Inputs, pipeline.InputRef{ID: srcID, Device: ""})
			rows := len(spec.Canvas.SourceStreams)
			if rows == 0 {
				rows = 1
			}
			slotH := h / rows
			s.Layout = append(s.Layout, pipeline.SlotPlacement{
				Slot: i,
				X:    0,
				Y:    i * slotH,
				W:    w,
				H:    slotH,
			})
		}
		s.Audio.Devices = append(s.Audio.Devices, spec.Canvas.AudioDevices...)
	default:
		// Single source → one input, identity layout, perspective effect
		// on the single input id.
		s.Inputs = []pipeline.InputRef{{ID: spec.ID, Device: spec.Device}}
	}

	if spec.Perspective != nil && len(s.Inputs) > 0 {
		s.Effects = map[string][]pipeline.Effect{
			s.Inputs[0].ID: {{
				Type:    "perspective",
				Corners: spec.Perspective.Corners,
			}},
		}
	}

	// Publish: RTSP at the daemon's configured host:port. Same default
	// as the legacy GPU compose path (resolveRTSPHost handles the
	// bind-on-all/empty cases).
	host := rtspHost
	if host == "" {
		host = "127.0.0.1:8554"
	}
	s.Publish = []pipeline.PublishTarget{{
		Type: "rtsp",
		URL:  fmt.Sprintf("rtsp://%s/%s", host, spec.ID),
	}}

	return s
}

// resolveCanvasSource looks up a source stream's device id from the store.
// The translation function above calls into this when building Inputs.
func (m *pipelineProcessManager) resolveCanvasSource(srcID string) string {
	if m.store == nil {
		return ""
	}
	src, ok := m.store.GetStream(srcID)
	if !ok {
		return ""
	}
	return src.Device
}

// applySpec converts spec to a pipeline.Stream (resolving canvas
// sources via the store) and calls Pipeline.Apply. Used by Start +
// Restart. When a composer is engaged, kicks off a
// goroutine that registers the composer with pipelinectl and pushes
// the initial config (set_canvas / set_source / set_layout / etc.)
// after the composer's gRPC UDS becomes ready.
func (m *pipelineProcessManager) applySpec(spec StreamSpec) error {
	ps := specToPipelineStream(spec, m.rtspHost)
	r := resolveEncoder(ps.Encoder.Codec, m.validation)
	ps.Encoder.EncoderName = r.Name
	ps.Encoder.GlobalArgs = r.GlobalArgs
	ps.Encoder.VideoFilters = r.VideoFilters
	if spec.Canvas != nil {
		for i := range ps.Inputs {
			ps.Inputs[i].Device = m.resolveCanvasSource(ps.Inputs[i].ID)
		}
	}
	if err := m.pipe.Apply(ps); err != nil {
		return err
	}
	if pipeline.NeedsComposer(ps) && m.controlServer != nil {
		go m.orchestrateComposer(ps)
	}
	return nil
}

// orchestrateComposer waits for the composer's gRPC UDS to be ready,
// dials it via the pipelinectl manager, and pushes the initial config
// (canvas dims + slot bindings + layout + effects). Mirrors the legacy
// streamProcessManager.orchestrateComposer flow but driven from the
// translation layer instead of the Pool's OnStateChange callback.
func (m *pipelineProcessManager) orchestrateComposer(s pipeline.Stream) {
	composerID := pipeline.ComposerIDFor(s.ID)
	udsPath := pipeline.GrpcSocketPathFor("composer", composerID)
	const dialDeadline = 30 * time.Second
	const callTimeout = 5 * time.Second
	tag := []any{"stream_id", s.ID, "composer_id", composerID, "uds", udsPath}

	deadline := time.Now().Add(dialDeadline)
	var lastErr error
	for {
		if time.Now().After(deadline) {
			m.logger.Warn("orchestrateComposer: register never succeeded",
				append(tag, "error", lastErr)...)
			return
		}
		regCtx, regCancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		err := m.controlServer.RegisterComposer(regCtx, composerID, udsPath)
		regCancel()
		if err == nil {
			break
		}
		lastErr = err
		time.Sleep(100 * time.Millisecond)
	}
	m.logger.Info("orchestrateComposer: composer registered, pushing initial config", tag...)

	push := func(name string, fn func(context.Context) error) bool {
		ctx, cancel := context.WithTimeout(context.Background(), callTimeout)
		defer cancel()
		if err := fn(ctx); err != nil {
			m.logger.Warn("orchestrateComposer push failed",
				append(append([]any{}, tag...), "method", name, "error", err)...)
			return false
		}
		return true
	}

	// SetCanvas — dims + fps. Pull from layout's bounding box; fps
	// hint comes from the first input or a 30fps default.
	canvasW, canvasH := composerCanvasDims(s)
	fps := uint32(30)
	if !push("set_canvas", func(c context.Context) error {
		return m.controlServer.SendSetCanvas(c, composerID, pipelinectl.SetCanvasParams{
			W: uint32(canvasW), H: uint32(canvasH), FPS: fps,
		})
	}) {
		return
	}

	// SetSource per input (one slot per input id).
	for i, in := range s.Inputs {
		slot := slotNameFor(i)
		if !push("set_source", func(c context.Context) error {
			return m.controlServer.SendSetSource(c, composerID, pipelinectl.SetSourceParams{
				Slot:     slot,
				SourceID: in.ID,
				ScmPath:  pipeline.SCMSocketPathFor(in.Device),
				Width:    uint32(canvasW),
				Height:   uint32(canvasH),
				FPS:      fps,
			})
		}) {
			return
		}
	}

	// SetLayout from the Stream's layout (or a single full-canvas slot
	// when none was provided).
	slots := make([]pipelinectl.LayoutSlotEntry, 0, len(s.Inputs))
	if len(s.Layout) > 0 {
		for _, l := range s.Layout {
			slots = append(slots, pipelinectl.LayoutSlotEntry{
				Slot: slotNameFor(l.Slot),
				X:    int32(l.X), Y: int32(l.Y), W: int32(l.W), H: int32(l.H),
			})
		}
	} else if len(s.Inputs) > 0 {
		slots = append(slots, pipelinectl.LayoutSlotEntry{
			Slot: slotNameFor(0), X: 0, Y: 0, W: int32(canvasW), H: int32(canvasH),
		})
	}
	if !push("set_layout", func(c context.Context) error {
		return m.controlServer.SendSetLayout(c, composerID,
			pipelinectl.SetLayoutParams{Slots: slots})
	}) {
		return
	}

	// SetEffects per input that has one.
	for inputID, effects := range s.Effects {
		out := make([]pipelinectl.EffectParams, 0, len(effects))
		for _, e := range effects {
			out = append(out, pipelinectl.EffectParams{
				Type:    e.Type,
				Corners: e.Corners,
			})
		}
		if !push("set_effects", func(c context.Context) error {
			return m.controlServer.SendSetEffects(c, composerID,
				pipelinectl.SetEffectsParams{SourceID: inputID, Effects: out})
		}) {
			return
		}
	}

	// SetSourceState — mark each input "live" so composer applies any
	// configured warp instead of falling back to identity.
	for _, in := range s.Inputs {
		if !push("set_source_state", func(c context.Context) error {
			return m.controlServer.SendSetSourceState(c, composerID,
				pipelinectl.SetSourceStateParams{SourceID: in.ID, State: "live"})
		}) {
			return
		}
	}

	m.logger.Info("composer initial config pushed",
		append(tag, "canvas", fmt.Sprintf("%dx%d@%dfps", canvasW, canvasH, fps),
			"sources", len(s.Inputs))...)
}

// composerCanvasDims returns the canvas dims for the composer's
// SetCanvas RPC. Uses the layout's bounding box when available;
// 1920x1080 default otherwise.
func composerCanvasDims(s pipeline.Stream) (int, int) {
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

// slotNameFor returns the conventional alphabetic slot label used by
// the existing composer protocol (slot "a" for index 0, "b" for 1, ...).
// Wraps back to numeric strings past 26 for safety.
func slotNameFor(i int) string {
	if i < 0 || i > 25 {
		return fmt.Sprintf("slot%d", i)
	}
	return string(rune('a' + i))
}

// Start applies the spec for streamID to the pipeline.
func (m *pipelineProcessManager) Start(streamID string) error {
	spec, ok := m.store.GetStream(streamID)
	if !ok {
		return fmt.Errorf("stream %s not found", streamID)
	}
	return m.applySpec(spec)
}

// Stop tears down all stages owned by the stream.
func (m *pipelineProcessManager) Stop(streamID string) error {
	return m.pipe.Delete(streamID)
}

// Restart re-applies the current spec. With per-stream serialization in
// Pipeline.Apply, this is safe to call mid-Apply for the same stream.
func (m *pipelineProcessManager) Restart(streamID string) error {
	return m.Start(streamID)
}

// GetStatus returns a snapshot of the stream's encoder stage (the
// always-present stage). Falls back to idle if not running.
func (m *pipelineProcessManager) GetStatus(streamID string) (*ProcessInfo, error) {
	encID := "encoder:" + streamID
	info := m.pipe.Pool().GetStatus(encID)
	return &ProcessInfo{
		StreamID:     streamID,
		State:        ProcessState(info.State),
		PID:          info.PID,
		StartedAt:    info.StartedAt,
		RestartCount: info.RestartCount,
		LastError:    info.LastError,
	}, nil
}

// StartAll applies every stream currently in the store.
func (m *pipelineProcessManager) StartAll() error {
	if m.store == nil {
		return nil
	}
	all := m.store.GetAllStreams()
	var errs []error
	for id, spec := range all {
		if err := m.applySpec(spec); err != nil {
			m.logger.Warn("StartAll: apply failed", "stream_id", id, "error", err)
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}

// StopAll stops every supervised process.
func (m *pipelineProcessManager) StopAll() {
	m.pipe.Pool().StopAll()
}

// IsRunning checks the encoder stage's pool state.
func (m *pipelineProcessManager) IsRunning(streamID string) bool {
	return m.pipe.Pool().IsRunning("encoder:" + streamID)
}

// IsCrashed returns true when the encoder stage's last state was Error.
// Pipeline doesn't track a separate crashed flag — derive from pool.
func (m *pipelineProcessManager) IsCrashed(streamID string) bool {
	info := m.pipe.Pool().GetStatus("encoder:" + streamID)
	return info.State == "error"
}

// CaptureSourceSnapshot pulls a raw NV12-derived JPEG snapshot from a
// source producer via the pipelinectl Snapshot RPC. Today the source-id
// maps 1:1 to a stream entry whose Device drives the producer; once
// sources become first-class (B2/B5) this lookup swaps to a source store.
func (m *pipelineProcessManager) CaptureSourceSnapshot(sourceID string) ([]byte, error) {
	if m.controlServer == nil {
		return nil, fmt.Errorf("no control server for snapshot")
	}
	spec, ok := m.store.GetStream(sourceID)
	if !ok {
		return nil, recording.ErrSourceNotFound
	}
	if spec.Device == "" {
		return nil, fmt.Errorf("source %s has no device", sourceID)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	resp, err := m.controlServer.Snapshot(ctx, spec.Device)
	if err != nil {
		return nil, err
	}
	return ffmpeg.EncodeNV12ToJPEG(resp.GetNv12(), int(resp.GetWidth()), int(resp.GetHeight()))
}

// OwnedBy reports which stream "owns" a source's device in the producer
// registry. In the unified model, ownership is shared via refcounting;
// returns the first consumer when shared, empty when unowned.
func (m *pipelineProcessManager) OwnedBy(sourceStreamID string) string {
	spec, ok := m.store.GetStream(sourceStreamID)
	if !ok || spec.Device == "" {
		return ""
	}
	consumers := m.pipe.Producers().ConsumersOf(spec.Device)
	if len(consumers) == 0 {
		return ""
	}
	// The producer is shared — pick the first consumer that's NOT this
	// stream itself, mirroring legacy "owned by canvas" semantics.
	for _, c := range consumers {
		if c != sourceStreamID {
			return c
		}
	}
	return ""
}

// CanvasOwner mirrors OwnedBy in the unified model — there is no
// canvas-as-distinct-entity, so the answer is the same.
func (m *pipelineProcessManager) CanvasOwner(sourceStreamID string) string {
	return m.OwnedBy(sourceStreamID)
}

// PushComposerPerspective routes a live perspective update to the
// composer's SetEffects RPC. Returns (delivered, error). When no
// composer is running for the stream, returns (false, nil) so the
// caller falls back to a restart.
func (m *pipelineProcessManager) PushComposerPerspective(
	streamID, sourceID string,
	persp *ffmpeg.PerspectiveConfig,
) (bool, error) {
	if m.controlServer == nil {
		return false, nil
	}
	composerID := streamID + "-composer"
	if !slices.Contains(m.controlServer.ConnectedComposers(), composerID) {
		return false, nil
	}
	effects := []pipelinectl.EffectParams{}
	if persp != nil {
		eff := pipelinectl.EffectParams{
			Type:           "perspective",
			Corners:        persp.Corners,
			SnapshotWidth:  persp.SnapshotWidth,
			SnapshotHeight: persp.SnapshotHeight,
		}
		effects = append(effects, eff)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := m.controlServer.SendSetEffects(ctx, composerID, pipelinectl.SetEffectsParams{
		SourceID: sourceID,
		Effects:  effects,
	}); err != nil {
		return false, err
	}
	return true, nil
}
