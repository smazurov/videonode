package streams

import (
	"fmt"

	"github.com/smazurov/videonode/internal/ffmpeg"
	"github.com/smazurov/videonode/internal/mediamtx"
	"github.com/smazurov/videonode/internal/types"
)

// encoderSelector is a function that selects the best encoder for a given codec.
type encoderSelector func(codec string, inputFormat string, qualityParams *types.QualityParams, encoderOverride string) *ffmpeg.Params

// deviceResolver is a function that resolves a device ID to a device path.
type deviceResolver func(deviceID string) string

// getSocketPath returns the socket path for a stream.
func getSocketPath(streamID string) string {
	return fmt.Sprintf("/tmp/ffmpeg-progress-%s.sock", streamID)
}

// processor handles runtime data injection for streams.
type processor struct {
	store           Store
	encoderSelector encoderSelector
	deviceResolver  deviceResolver
	getStreamState  func(streamID string) (*Stream, bool) // Get runtime state
}

// newProcessor creates a new stream processor.
func newProcessor(repo Store) *processor {
	return &processor{
		store: repo,
		// Default implementations that do nothing
		encoderSelector: func(codec string, _ string, _ *types.QualityParams, encoderOverride string) *ffmpeg.Params {
			// Default to software encoder
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
		deviceResolver: func(deviceID string) string {
			return deviceID // Return as-is by default
		},
		getStreamState: func(_ string) (*Stream, bool) {
			return nil, false // No runtime state by default
		},
	}
}

// setEncoderSelector sets the encoder selection function.
func (p *processor) setEncoderSelector(selector encoderSelector) {
	p.encoderSelector = selector
}

// setDeviceResolver sets the device resolution function.
func (p *processor) setDeviceResolver(resolver deviceResolver) {
	p.deviceResolver = resolver
}

// setStreamStateGetter sets the function to get runtime stream state.
func (p *processor) setStreamStateGetter(getter func(streamID string) (*Stream, bool)) {
	p.getStreamState = getter
}

// applyStreamSettingsToFFmpegParams applies common stream settings to FFmpeg params.
func (p *processor) applyStreamSettingsToFFmpegParams(ffmpegParams *ffmpeg.Params, streamConfig *StreamSpec, streamID string, devicePath string, socketPath string, enabled bool) {
	// Add stream-specific settings to FFmpeg params
	ffmpegParams.DevicePath = devicePath
	ffmpegParams.InputFormat = streamConfig.FFmpeg.InputFormat
	ffmpegParams.Resolution = streamConfig.FFmpeg.Resolution
	ffmpegParams.FPS = streamConfig.FFmpeg.FPS
	ffmpegParams.AudioDevice = streamConfig.FFmpeg.AudioDevice

	// Set default audio resampling filter for sync when audio device is present
	if streamConfig.FFmpeg.AudioDevice != "" {
		ffmpegParams.AudioFilters = "aresample=async=1:min_hard_comp=0.100000:first_pts=0"
	}

	ffmpegParams.ProgressSocket = socketPath
	ffmpegParams.Options = streamConfig.FFmpeg.Options
	ffmpegParams.OutputURL = fmt.Sprintf("srt://localhost:8890?streamid=publish:%s", streamID)

	// Determine test source mode and overlay text
	switch {
	case !enabled:
		// Device offline or no signal
		ffmpegParams.IsTestSource = true
		ffmpegParams.TestOverlay = "NO SIGNAL"
	case streamConfig.TestMode:
		// Explicit test mode
		ffmpegParams.IsTestSource = true
		ffmpegParams.TestOverlay = "TEST MODE"
	default:
		// Normal device input
		ffmpegParams.IsTestSource = false
		ffmpegParams.TestOverlay = ""
	}
}

// processStream processes a single stream and injects runtime data.
func (p *processor) processStream(streamID string) (*mediamtx.ProcessedStream, error) {
	return p.processStreamWithEncoder(streamID, "")
}

// processStreamWithEncoder processes a single stream with an optional encoder override.
func (p *processor) processStreamWithEncoder(streamID string, encoderOverride string) (*mediamtx.ProcessedStream, error) {
	streamConfig, exists := p.store.GetStream(streamID)
	if !exists {
		return nil, fmt.Errorf("stream %s not found", streamID)
	}

	// Get runtime state (enabled status)
	streamState, hasState := p.getStreamState(streamID)
	enabled := false
	if hasState {
		enabled = streamState.Enabled
	}

	// Priority order:
	// 1. NO SIGNAL (device offline) - absolute precedence
	// 2. Custom command (device online + custom command set)
	// 3. Test mode (device online + no custom command + test mode enabled)
	// 4. Normal capture (device online + no custom command + test mode disabled)

	// If device is online AND custom command is set - use custom command
	// (skip if device is offline to generate NO SIGNAL pattern instead)
	if enabled && streamConfig.CustomFFmpegCommand != "" {
		return &mediamtx.ProcessedStream{
			StreamID:      streamID,
			FFmpegCommand: streamConfig.CustomFFmpegCommand,
		}, nil
	}

	// Determine if we should use test source (either TestMode or device not enabled)
	useTestSource := streamConfig.TestMode || !enabled

	// Resolve device path (skip if using test source)
	var devicePath string
	if !useTestSource {
		devicePath = p.deviceResolver(streamConfig.Device)
		if devicePath == "" {
			return nil, fmt.Errorf("device %s not found", streamConfig.Device)
		}
	}

	// Create socket path
	socketPath := getSocketPath(streamID)

	// Select encoder and get settings
	var ffmpegParams *ffmpeg.Params

	if streamConfig.FFmpeg.Codec != "" {
		// Use encoder selector to get optimal encoder and all params
		// If encoderOverride is provided, selector will use it directly with proper settings
		// Pass "testsrc" as input format when using test source to get appropriate filters
		inputFormat := streamConfig.FFmpeg.InputFormat
		if useTestSource {
			inputFormat = "testsrc"
		}

		ffmpegParams = p.encoderSelector(
			streamConfig.FFmpeg.Codec,
			inputFormat,
			streamConfig.FFmpeg.QualityParams,
			encoderOverride, // Pass encoder override to selector
		)

		// Set preset for software encoders if not already set
		if ffmpegParams.Preset == "" && (ffmpegParams.Encoder == "libx264" || ffmpegParams.Encoder == "libx265") {
			ffmpegParams.Preset = "fast"
		}
	} else {
		// Default fallback
		ffmpegParams = &ffmpeg.Params{
			Encoder: "libx264",
			Preset:  "fast",
			Bitrate: "2M",
		}

		// Apply encoder override even for default fallback
		if encoderOverride != "" {
			ffmpegParams.Encoder = encoderOverride
		}
	}

	// Apply common stream settings to FFmpeg params
	p.applyStreamSettingsToFFmpegParams(ffmpegParams, &streamConfig, streamID, devicePath, socketPath, enabled)

	// Build FFmpeg command using the new Params struct
	ffmpegCmd := ffmpeg.BuildCommand(ffmpegParams)

	return &mediamtx.ProcessedStream{
		StreamID:      streamID,
		FFmpegCommand: ffmpegCmd,
	}, nil
}
