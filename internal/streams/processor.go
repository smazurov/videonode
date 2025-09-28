package streams

import (
	"fmt"

	"github.com/smazurov/videonode/internal/ffmpeg"
	"github.com/smazurov/videonode/internal/logging"
	"github.com/smazurov/videonode/internal/types"
)

// EncoderSelector is a function that selects the best encoder for a given codec
type EncoderSelector func(codec string, inputFormat string, qualityParams *types.QualityParams, encoderOverride string) *ffmpeg.Params

// DeviceResolver is a function that resolves a device ID to a device path
type DeviceResolver func(deviceID string) string

// SocketCreator is a function that creates a monitoring socket path for a stream
type SocketCreator func(streamID string) string

// GetSocketPath returns the socket path for a stream
func GetSocketPath(streamID string) string {
	return fmt.Sprintf("/tmp/ffmpeg-progress-%s.sock", streamID)
}

// Processor handles runtime data injection for streams
type Processor struct {
	repository      Repository
	encoderSelector EncoderSelector
	deviceResolver  DeviceResolver
}

// NewProcessor creates a new stream processor
func NewProcessor(repo Repository) *Processor {
	return &Processor{
		repository: repo,
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

// SetEncoderSelector sets the encoder selection function
func (p *Processor) SetEncoderSelector(selector EncoderSelector) {
	p.encoderSelector = selector
}

// SetDeviceResolver sets the device resolution function
func (p *Processor) SetDeviceResolver(resolver DeviceResolver) {
	p.deviceResolver = resolver
}

// applyStreamSettingsToFFmpegParams applies common stream settings to FFmpeg params
func (p *Processor) applyStreamSettingsToFFmpegParams(ffmpegParams *ffmpeg.Params, stream *StreamConfig, devicePath string, socketPath string) {
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
	ffmpegParams.IsTestSource = stream.TestMode
}

// ProcessStream processes a single stream and injects runtime data
func (p *Processor) ProcessStream(streamID string) (*ProcessedStream, error) {
	stream, exists := p.repository.GetStream(streamID)
	if !exists {
		return nil, fmt.Errorf("stream %s not found", streamID)
	}

	// If custom command is set, use it directly
	if stream.CustomFFmpegCommand != "" {
		return &ProcessedStream{
			StreamID:      streamID,
			FFmpegCommand: stream.CustomFFmpegCommand,
			// Other fields will be empty/default when using custom command
		}, nil
	}

	// Resolve device path (skip if in test mode)
	var devicePath string
	if !stream.TestMode {
		devicePath = p.deviceResolver(stream.Device)
		if devicePath == "" {
			return nil, fmt.Errorf("device %s not found", stream.Device)
		}
	}

	// Create socket path
	socketPath := GetSocketPath(streamID)

	// Select encoder and get settings
	var ffmpegParams *ffmpeg.Params

	if stream.FFmpeg.Codec != "" {
		// Use encoder selector to get optimal encoder and all params
		// Pass "testsrc" as input format when in test mode to get appropriate filters
		inputFormat := stream.FFmpeg.InputFormat
		if stream.TestMode {
			inputFormat = "testsrc"
		}

		ffmpegParams = p.encoderSelector(
			stream.FFmpeg.Codec,
			inputFormat,
			stream.FFmpeg.QualityParams,
			"", // No encoder override for normal ProcessStream
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
	}

	// Apply common stream settings to FFmpeg params
	p.applyStreamSettingsToFFmpegParams(ffmpegParams, &stream, devicePath, socketPath)

	// Build FFmpeg command using the new Params struct
	ffmpegCmd := ffmpeg.BuildCommand(ffmpegParams)

	// Convert Options to string slice for ProcessedStream (for compatibility)
	var options []string
	for _, opt := range stream.FFmpeg.Options {
		options = append(options, string(opt))
	}

	return &ProcessedStream{
		StreamID:      streamID,
		FFmpegCommand: ffmpegCmd,
		DevicePath:    devicePath,
		Encoder:       ffmpegParams.Encoder,
		GlobalArgs:    ffmpegParams.GlobalArgs,
		VideoFilters:  ffmpegParams.VideoFilters,
		SocketPath:    socketPath,
		InputFormat:   stream.FFmpeg.InputFormat,
		Resolution:    stream.FFmpeg.Resolution,
		FPS:           stream.FFmpeg.FPS,
		Bitrate:       ffmpegParams.Bitrate,
		AudioDevice:   stream.FFmpeg.AudioDevice,
		Preset:        ffmpegParams.Preset,
		Options:       options,
	}, nil
}

// ProcessStreamWithEncoder processes a single stream with an optional encoder override
func (p *Processor) ProcessStreamWithEncoder(streamID string, encoderOverride string) (*ProcessedStream, error) {
	stream, exists := p.repository.GetStream(streamID)
	if !exists {
		return nil, fmt.Errorf("stream %s not found", streamID)
	}

	// If custom command is set, use it directly
	if stream.CustomFFmpegCommand != "" {
		return &ProcessedStream{
			StreamID:      streamID,
			FFmpegCommand: stream.CustomFFmpegCommand,
			// Other fields will be empty/default when using custom command
		}, nil
	}

	// Resolve device path (skip if in test mode)
	var devicePath string
	if !stream.TestMode {
		devicePath = p.deviceResolver(stream.Device)
		if devicePath == "" {
			return nil, fmt.Errorf("device %s not found", stream.Device)
		}
	}

	// Create socket path
	socketPath := GetSocketPath(streamID)

	// Select encoder and get settings
	var ffmpegParams *ffmpeg.Params

	if stream.FFmpeg.Codec != "" {
		// Use encoder selector to get optimal encoder and all params
		// If encoderOverride is provided, selector will use it directly with proper settings
		// Pass "testsrc" as input format when in test mode to get appropriate filters
		inputFormat := stream.FFmpeg.InputFormat
		if stream.TestMode {
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

	// Convert Options to string slice for ProcessedStream (for compatibility)
	var options []string
	for _, opt := range stream.FFmpeg.Options {
		options = append(options, string(opt))
	}

	return &ProcessedStream{
		StreamID:      streamID,
		FFmpegCommand: ffmpegCmd,
		DevicePath:    devicePath,
		Encoder:       ffmpegParams.Encoder,
		GlobalArgs:    ffmpegParams.GlobalArgs,
		VideoFilters:  ffmpegParams.VideoFilters,
		SocketPath:    socketPath,
		InputFormat:   stream.FFmpeg.InputFormat,
		Resolution:    stream.FFmpeg.Resolution,
		FPS:           stream.FFmpeg.FPS,
		Bitrate:       ffmpegParams.Bitrate,
		AudioDevice:   stream.FFmpeg.AudioDevice,
		Preset:        ffmpegParams.Preset,
		Options:       options,
	}, nil
}

// ProcessAllStreams processes all enabled streams
func (p *Processor) ProcessAllStreams() ([]*ProcessedStream, error) {
	var processed []*ProcessedStream

	for streamID, stream := range p.repository.GetEnabledStreams() {
		if !stream.Enabled {
			continue
		}

		ps, err := p.ProcessStream(streamID)
		if err != nil {
			logger := logging.GetLogger("streams")
			logger.Error("Error processing stream", "stream_id", streamID, "error", err)
			continue // Skip failed streams
		}

		processed = append(processed, ps)
	}

	return processed, nil
}
