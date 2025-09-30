package streams

import (
	"fmt"

	"github.com/smazurov/videonode/internal/ffmpeg"
	"github.com/smazurov/videonode/internal/logging"
	"github.com/smazurov/videonode/internal/mediamtx"
	"github.com/smazurov/videonode/internal/types"
)

// encoderSelector is a function that selects the best encoder for a given codec
type encoderSelector func(codec string, inputFormat string, qualityParams *types.QualityParams, encoderOverride string) *ffmpeg.Params

// deviceResolver is a function that resolves a device ID to a device path
type deviceResolver func(deviceID string) string

// getSocketPath returns the socket path for a stream
func getSocketPath(streamID string) string {
	return fmt.Sprintf("/tmp/ffmpeg-progress-%s.sock", streamID)
}

// processor handles runtime data injection for streams
type processor struct {
	store           Store
	encoderSelector encoderSelector
	deviceResolver  deviceResolver
}

// newProcessor creates a new stream processor
func newProcessor(repo Store) *processor {
	return &processor{
		store: repo,
		// Default implementations that do nothing
		encoderSelector: func(codec string, inputFormat string, qualityParams *types.QualityParams, encoderOverride string) *ffmpeg.Params {
			// Default to software encoder
			params := &ffmpeg.Params{}
			if encoderOverride != "" {
				params.Encoder = encoderOverride
			} else if codec == "h265" {
				params.Encoder = "libx265"
			} else {
				params.Encoder = "libx264"
			}
			return params
		},
		deviceResolver: func(deviceID string) string {
			return deviceID // Return as-is by default
		},
	}
}

// setEncoderSelector sets the encoder selection function
func (p *processor) setEncoderSelector(selector encoderSelector) {
	p.encoderSelector = selector
}

// setDeviceResolver sets the device resolution function
func (p *processor) setDeviceResolver(resolver deviceResolver) {
	p.deviceResolver = resolver
}

// applyStreamSettingsToFFmpegParams applies common stream settings to FFmpeg params
func (p *processor) applyStreamSettingsToFFmpegParams(ffmpegParams *ffmpeg.Params, stream *StreamSpec, devicePath string, socketPath string) {
	// Add stream-specific settings to FFmpeg params
	ffmpegParams.DevicePath = devicePath
	ffmpegParams.InputFormat = stream.FFmpeg.InputFormat
	ffmpegParams.Resolution = stream.FFmpeg.Resolution
	ffmpegParams.FPS = stream.FFmpeg.FPS
	ffmpegParams.AudioDevice = stream.FFmpeg.AudioDevice

	// Set default audio resampling filter for sync when audio device is present
	if stream.FFmpeg.AudioDevice != "" {
		ffmpegParams.AudioFilters = "aresample=async=1:min_hard_comp=0.100000:first_pts=0"
	}

	ffmpegParams.ProgressSocket = socketPath
	ffmpegParams.Options = stream.FFmpeg.Options
	ffmpegParams.OutputURL = "rtsp://localhost:8554/$MTX_PATH"

	// Determine test source mode and overlay text
	if !stream.Enabled {
		// Device offline or no signal
		ffmpegParams.IsTestSource = true
		ffmpegParams.TestOverlay = "NO SIGNAL"
	} else if stream.TestMode {
		// Explicit test mode
		ffmpegParams.IsTestSource = true
		ffmpegParams.TestOverlay = "TEST MODE"
	} else {
		// Normal device input
		ffmpegParams.IsTestSource = false
		ffmpegParams.TestOverlay = ""
	}
}

// processStream processes a single stream and injects runtime data
func (p *processor) processStream(streamID string) (*mediamtx.ProcessedStream, error) {
	return p.processStreamWithEncoder(streamID, "")
}

// processStreamWithEncoder processes a single stream with an optional encoder override
func (p *processor) processStreamWithEncoder(streamID string, encoderOverride string) (*mediamtx.ProcessedStream, error) {
	stream, exists := p.store.GetStream(streamID)
	if !exists {
		return nil, fmt.Errorf("stream %s not found", streamID)
	}

	// If custom command is set, use it directly
	if stream.CustomFFmpegCommand != "" {
		return &mediamtx.ProcessedStream{
			StreamID:      streamID,
			FFmpegCommand: stream.CustomFFmpegCommand,
			// Other fields will be empty/default when using custom command
		}, nil
	}

	// Determine if we should use test source (either TestMode or device not enabled)
	useTestSource := stream.TestMode || !stream.Enabled

	// Resolve device path (skip if using test source)
	var devicePath string
	if !useTestSource {
		devicePath = p.deviceResolver(stream.Device)
		if devicePath == "" {
			return nil, fmt.Errorf("device %s not found", stream.Device)
		}
	}

	// Create socket path
	socketPath := getSocketPath(streamID)

	// Select encoder and get settings
	var ffmpegParams *ffmpeg.Params

	if stream.FFmpeg.Codec != "" {
		// Use encoder selector to get optimal encoder and all params
		// If encoderOverride is provided, selector will use it directly with proper settings
		// Pass "testsrc" as input format when using test source to get appropriate filters
		inputFormat := stream.FFmpeg.InputFormat
		if useTestSource {
			inputFormat = "testsrc"
		}

		ffmpegParams = p.encoderSelector(
			stream.FFmpeg.Codec,
			inputFormat,
			stream.FFmpeg.QualityParams,
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
	p.applyStreamSettingsToFFmpegParams(ffmpegParams, &stream, devicePath, socketPath)

	// Build FFmpeg command using the new Params struct
	ffmpegCmd := ffmpeg.BuildCommand(ffmpegParams)

	return &mediamtx.ProcessedStream{
		StreamID:      streamID,
		FFmpegCommand: ffmpegCmd,
	}, nil
}

// processAllStreams processes all streams (both enabled and disabled)
// Disabled streams will be rendered as test patterns with "NO SIGNAL" overlay
func (p *processor) processAllStreams() ([]*mediamtx.ProcessedStream, error) {
	var processed []*mediamtx.ProcessedStream

	for streamID := range p.store.GetAllStreams() {
		ps, err := p.processStream(streamID)
		if err != nil {
			logger := logging.GetLogger("streams")
			logger.Error("Error processing stream", "stream_id", streamID, "error", err)
			continue // Skip failed streams
		}

		processed = append(processed, ps)
	}

	return processed, nil
}
