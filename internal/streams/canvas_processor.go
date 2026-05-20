package streams

import (
	"fmt"

	"github.com/smazurov/videonode/internal/ffmpeg"
	"github.com/smazurov/videonode/internal/logging"
	"github.com/smazurov/videonode/internal/types"
)

// canvasProcessor generates the FFmpeg command for a canvas stream composing 1–4 source streams.
type canvasProcessor struct {
	store            Store
	encoderSelector  encoderSelector
	deviceResolver   deviceResolver
	getStreamState   func(streamID string) (*Stream, bool)
	isCrashed        func(streamID string) bool
	defaultVisionFPS int // 0 = no throttle
	logger           logging.Logger
	// producerMgr is set by NewStreamProcessManager and is non-nil whenever
	// the GPU compose path is reachable. processStreamGPU reads the sink-side
	// SCM socket path from the manager (the producer process is launched
	// independently by streamProcessManager.Start).
	producerMgr *ProducerManager
	// native is the resolved binary-availability config. When CanvasReady()
	// returns true, canvases auto-route through the GPU compose path.
	native *NativePipelineConfig
}

func newCanvasProcessor(store Store) *canvasProcessor {
	return &canvasProcessor{
		store:  store,
		logger: logging.GetLogger("canvas_processor"),
		encoderSelector: func(codec string, _ string, _ *types.QualityParams, encoderOverride string) *ffmpeg.Params {
			params := &ffmpeg.Params{}
			switch {
			case encoderOverride != "":
				params.Encoder = encoderOverride
			case codec == "h265":
				params.Encoder = "libx265"
			default:
				params.Encoder = "libx264"
			}
			return params
		},
		deviceResolver: func(deviceID string) string { return deviceID },
		getStreamState: func(_ string) (*Stream, bool) { return nil, false },
	}
}

// processStream builds the composite FFmpeg command for the given canvas stream.
func (cp *canvasProcessor) processStream(canvasID string) (*ProcessedStream, error) {
	canvasSpec, exists := cp.store.GetStream(canvasID)
	if !exists {
		return nil, fmt.Errorf("canvas stream %s not found", canvasID)
	}
	if canvasSpec.Canvas == nil {
		return nil, fmt.Errorf("stream %s has no canvas config", canvasID)
	}
	canvas := canvasSpec.Canvas
	if len(canvas.SourceStreams) == 0 || len(canvas.SourceStreams) > 4 {
		return nil, fmt.Errorf("canvas %s must reference 1–4 source streams, got %d",
			canvasID, len(canvas.SourceStreams))
	}

	sourceSpecs := make(map[string]*StreamSpec, len(canvas.SourceStreams))
	for _, sourceID := range canvas.SourceStreams {
		if src, ok := cp.store.GetStream(sourceID); ok {
			spec := src
			sourceSpecs[sourceID] = &spec
		}
	}

	// GPU compose path: videonode-source sidecar + videonode-composer + ffmpeg
	// pushed to the daemon's local RTSP. Auto-engaged when the native
	// binaries are installed; falls back to the legacy filter graph
	// otherwise.
	if cp.native.CanvasReady() {
		return cp.processStreamGPU(canvasID, canvas, sourceSpecs)
	}

	layout := ComputeCanvasLayout(canvas, sourceSpecs)
	if len(layout.Slots) == 0 {
		return nil, fmt.Errorf("canvas %s: no slots for %d sources at %dx%d",
			canvasID, len(canvas.SourceStreams), canvas.Width, canvas.Height)
	}

	canvasState, _ := cp.getStreamState(canvasID)
	inputsEnabled := make(map[string]bool)
	if canvasState != nil && canvasState.InputsEnabled != nil {
		inputsEnabled = canvasState.InputsEnabled
	}

	isCrashed := cp.isCrashed != nil && cp.isCrashed(canvasID)

	inputs := make([]ffmpeg.CompositeInput, 0, len(canvas.SourceStreams))
	for i, sourceID := range canvas.SourceStreams {
		slot := layout.Slots[i]
		// Content rect (not slot rect): HW scalers ignore force_original_aspect_ratio.
		ci := ffmpeg.CompositeInput{
			X:      slot.ContentX,
			Y:      slot.ContentY,
			Width:  slot.ContentW,
			Height: slot.ContentH,
		}

		src := sourceSpecs[sourceID]
		if src == nil {
			ci.OverlayText = "NO SIGNAL: " + sourceID
			cp.logger.Warn("Canvas source stream missing",
				"canvas_id", canvasID, "source_id", sourceID)
			inputs = append(inputs, ci)
			continue
		}
		if src.Canvas != nil {
			return nil, fmt.Errorf("canvas %s cannot nest canvas source %s",
				canvasID, sourceID)
		}

		ci.InputFormat = src.FFmpeg.InputFormat
		ci.Resolution = src.FFmpeg.Resolution
		ci.FPS = src.FFmpeg.FPS
		ci.Rotation = slot.RotationApplied

		ci.VisionEnabled = true
		vw, vh := visionDimensions(src.Vision, src.FFmpeg.Resolution)
		ci.VisionWidth = vw
		ci.VisionHeight = vh
		ci.VisionFPS = resolveVisionFPS(src.Vision, cp.defaultVisionFPS)
		if src.Perspective != nil {
			ci.Perspective = src.Perspective
		}

		switch {
		case isCrashed:
			ci.OverlayText = "CRASH"
		case src.TestMode:
			ci.OverlayText = "TEST MODE"
		case !inputsEnabled[sourceID]:
			ci.OverlayText = "NO SIGNAL"
		default:
			devicePath := cp.deviceResolver(src.Device)
			if devicePath == "" {
				ci.OverlayText = "NO SIGNAL"
				cp.logger.Warn("Canvas source device not found",
					"canvas_id", canvasID, "source_id", sourceID, "device", src.Device)
			} else {
				ci.DevicePath = devicePath
			}
		}

		inputs = append(inputs, ci)
	}

	ffmpegParams := cp.encoderSelector(
		canvasSpec.FFmpeg.Codec,
		"testsrc",
		canvasSpec.FFmpeg.QualityParams,
		"",
	)

	if ffmpegParams.Preset == "" && (ffmpegParams.Encoder == "libx264" || ffmpegParams.Encoder == "libx265") {
		ffmpegParams.Preset = "fast"
	}

	cp2 := &ffmpeg.CompositeParams{
		Width:    canvas.Width,
		Height:   canvas.Height,
		FPS:      canvas.FPS,
		KeyColor: canvas.KeyColor,
		Inputs:   inputs,

		AudioDevices: canvas.AudioDevices,

		Encoder:      ffmpegParams.Encoder,
		GlobalArgs:   ffmpegParams.GlobalArgs,
		VideoFilters: ffmpegParams.VideoFilters,
		HWBackend:    ffmpegParams.HWBackend,
		HWCaps:       cp.hwCaps(),

		Bitrate:    ffmpegParams.Bitrate,
		MinRate:    ffmpegParams.MinRate,
		MaxRate:    ffmpegParams.MaxRate,
		BufferSize: ffmpegParams.BufferSize,
		CRF:        ffmpegParams.CRF,
		QP:         ffmpegParams.QP,
		RCMode:     ffmpegParams.RCMode,

		Preset:  ffmpegParams.Preset,
		GOP:     ffmpegParams.GOP,
		BFrames: ffmpegParams.BFrames,

		ProgressSocket: getSocketPath(canvasID),
		OutputURL:      fmt.Sprintf("rtsp://127.0.0.1:8554/%s", canvasID),

		Options: canvasSpec.FFmpeg.Options,
	}

	cmd := ffmpeg.BuildCompositeCommand(cp2)

	return &ProcessedStream{
		StreamID:      canvasID,
		FFmpegCommand: cmd,
	}, nil
}

// hwCaps returns HW filter capabilities from persisted validation results.
func (cp *canvasProcessor) hwCaps() ffmpeg.HWCapabilities {
	vr := cp.store.GetValidation()
	if vr == nil {
		return ffmpeg.HWCapabilities{}
	}
	return ffmpeg.CapabilitiesFromBackends(vr.Backends)
}
