package streams

import (
	"fmt"
	"log/slog"

	"github.com/smazurov/videonode/internal/devices"
	"github.com/smazurov/videonode/internal/encoders"
	"github.com/smazurov/videonode/internal/ffmpeg"
	"github.com/smazurov/videonode/internal/types"
	valManager "github.com/smazurov/videonode/internal/validation"
)

// formatResolution creates a resolution string from width and height values.
func formatResolution(width, height *int) string {
	if width != nil && height != nil && *width > 0 && *height > 0 {
		return fmt.Sprintf("%dx%d", *width, *height)
	}
	return ""
}

// formatFPS creates a framerate string from a framerate value.
func formatFPS(framerate *int) string {
	if framerate != nil && *framerate > 0 {
		return fmt.Sprintf("%d", *framerate)
	}
	return ""
}

// getStreamSafe retrieves a stream from memory in a thread-safe manner.
func (s *service) getStreamSafe(streamID string) (*Stream, bool) {
	s.streamsMutex.RLock()
	defer s.streamsMutex.RUnlock()
	stream, exists := s.streams[streamID]
	return stream, exists
}

// parseCodecType converts a codec string to a CodecType.
func parseCodecType(codec string) encoders.CodecType {
	if codec == "h265" {
		return encoders.CodecH265
	}
	return encoders.CodecH264
}

// copyStream creates a copy of a stream to prevent external mutation.
func copyStream(stream *Stream) *Stream {
	if stream == nil {
		return nil
	}
	streamCopy := *stream
	return &streamCopy
}

// makeEncoderSelector creates an encoder selector from options or default.
func makeEncoderSelector(logger *slog.Logger, opts *ServiceOptions, repo Store) encoders.Selector {
	if opts != nil && opts.EncoderSelector != nil {
		return opts.EncoderSelector
	}

	// Create default encoder selector with validation manager
	validationService := NewValidationService(repo)
	vm := valManager.NewManager(validationService)
	if err := vm.LoadValidation(); err != nil {
		logger.Error("Failed to load validation data", "error", err)
	}
	return encoders.NewDefaultSelector(vm)
}

// makeEncoderSelectorFunc creates the encoder selector function for the processor.
func makeEncoderSelectorFunc(encoderSelector encoders.Selector, logger *slog.Logger) func(string, string, *types.QualityParams, string) *ffmpeg.Params {
	return func(codec string, inputFormat string, qualityParams *types.QualityParams, encoderOverride string) *ffmpeg.Params {
		// Convert codec string to CodecType
		codecType := parseCodecType(codec)

		// Select optimal encoder (or use override)
		params, err := encoderSelector.SelectEncoder(codecType, inputFormat, qualityParams, encoderOverride)
		if err != nil {
			logger.Error("Failed to select encoder", "error", err)
			// Return defaults
			defaultParams := &ffmpeg.Params{}
			switch {
			case encoderOverride != "":
				defaultParams.Encoder = encoderOverride
			case codecType == encoders.CodecH265:
				defaultParams.Encoder = "libx265"
			default:
				defaultParams.Encoder = "libx264"
			}
			return defaultParams
		}

		return params
	}
}

// makeDeviceResolver creates the device resolver function for the processor.
func makeDeviceResolver(logger *slog.Logger) func(string) string {
	return func(deviceID string) string {
		devicePath, err := devices.ResolveDevicePath(deviceID)
		if err != nil {
			logger.Warn("Device resolution failed", "device_id", deviceID, "error", err)
			return ""
		}
		return devicePath
	}
}

// buildQualityParams creates quality parameters from bitrate.
func buildQualityParams(bitrate *float64) *types.QualityParams {
	if bitrate != nil && *bitrate > 0 {
		return &types.QualityParams{
			Mode:          types.RateControlCBR,
			TargetBitrate: bitrate,
		}
	}
	return nil
}

// validateCodec validates that codec is either h264 or h265.
func validateCodec(codec string) error {
	if codec != "h264" && codec != "h265" {
		return fmt.Errorf("invalid codec: %s (must be h264 or h265)", codec)
	}
	return nil
}

// buildFFmpegOptions converts string options to FFmpeg OptionType or returns defaults.
func buildFFmpegOptions(options []string) []ffmpeg.OptionType {
	if len(options) > 0 {
		ffmpegOptions := make([]ffmpeg.OptionType, 0, len(options))
		for _, opt := range options {
			ffmpegOptions = append(ffmpegOptions, ffmpeg.OptionType(opt))
		}
		return ffmpegOptions
	}
	return ffmpeg.GetDefaultOptions()
}
