package streams

import (
	"fmt"
	"maps"

	"github.com/smazurov/videonode/internal/devices"
	"github.com/smazurov/videonode/internal/encoders"
	"github.com/smazurov/videonode/internal/ffmpeg"
	"github.com/smazurov/videonode/internal/logging"
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

// copyStream creates a copy of a stream to prevent external mutation.
func copyStream(stream *Stream) *Stream {
	if stream == nil {
		return nil
	}
	streamCopy := *stream
	if stream.InputsEnabled != nil {
		streamCopy.InputsEnabled = make(map[string]bool, len(stream.InputsEnabled))
		maps.Copy(streamCopy.InputsEnabled, stream.InputsEnabled)
	}
	return &streamCopy
}

// makeEncoderSelector creates an encoder selector from options or default.
func makeEncoderSelector(logger logging.Logger, opts *ServiceOptions, repo Store) encoders.Selector {
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

// makeDeviceResolver creates the device resolver function for the processor.
func makeDeviceResolver(logger logging.Logger) func(string) string {
	return MakeDeviceResolver(logger)
}

// MakeDeviceResolver is the exported variant used by the pipeline
// package's Pipeline.Config (which lives outside the streams package).
// Returns a function mapping opaque device ids to canonical /dev/videoN
// paths, or "" when resolution fails.
func MakeDeviceResolver(logger logging.Logger) func(string) string {
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
