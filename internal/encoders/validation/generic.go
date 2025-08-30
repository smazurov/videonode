package validation

import (
	"fmt"

	"github.com/smazurov/videonode/internal/types"
)

// GenericValidator provides fallback validation for unknown encoder types

// GenericValidator validates unknown encoder types with basic parameters
type GenericValidator struct{}

// NewGenericValidator creates a new generic validator
func NewGenericValidator() *GenericValidator {
	return &GenericValidator{}
}

// CanValidate returns true for any encoder (this is the fallback validator)
func (v *GenericValidator) CanValidate(encoderName string) bool {
	// Generic validator can handle any encoder as a fallback
	return true
}

// Validate tests unknown encoder types using production settings
func (v *GenericValidator) Validate(encoderName string) (bool, error) {
	return ValidateEncoderWithSettings(v, encoderName)
}

// GetEncoderNames returns common software encoder names that this validator handles
func (v *GenericValidator) GetEncoderNames() []string {
	// Generic validator handles common software encoders as fallback
	return []string{
		"libx264",    // x264 software encoder (H.264)
		"libx265",    // x265 software encoder (H.265/HEVC)
		"libvpx",     // VP8 software encoder
		"libvpx-vp9", // VP9 software encoder
		"mpeg4",      // MPEG-4 software encoder
		"libxvid",    // Xvid software encoder
	}
}

// GetDescription returns a description of this validator
func (v *GenericValidator) GetDescription() string {
	return "Generic validator - Software encoder fallback and validation for unknown encoder types"
}

// GetProductionSettings returns production settings for software encoders
func (v *GenericValidator) GetProductionSettings(encoderName string, inputFormat string) (*EncoderSettings, error) {
	// Provide encoder-specific settings for known software encoders
	switch encoderName {
	case "libx264":
		videoFilters := ""
		// Convert MJPEG/YUYV/RGB/NV24/NV16 to yuv420p for x264
		if inputFormat == "mjpeg" || inputFormat == "yuyv422" ||
			inputFormat == "bgr24" || inputFormat == "rgb24" ||
			inputFormat == "nv24" || inputFormat == "nv16" {
			videoFilters = "format=yuv420p"
		}
		return &EncoderSettings{
			GlobalArgs: []string{},
			OutputParams: map[string]string{
				"crf":    "18",        // High quality constant rate factor
				"preset": "ultrafast", // Fast encoding for streaming
			},
			VideoFilters: videoFilters,
		}, nil

	case "libx265":
		videoFilters := ""
		// Convert MJPEG/YUYV/RGB/NV24/NV16 to yuv420p for x265
		if inputFormat == "mjpeg" || inputFormat == "yuyv422" ||
			inputFormat == "bgr24" || inputFormat == "rgb24" ||
			inputFormat == "nv24" || inputFormat == "nv16" {
			videoFilters = "format=yuv420p"
		}
		return &EncoderSettings{
			GlobalArgs: []string{},
			OutputParams: map[string]string{
				"crf":    "20",        // Slightly higher CRF for H.265 efficiency
				"preset": "ultrafast", // Fast encoding
			},
			VideoFilters: videoFilters,
		}, nil

	default:
		// Fallback for any unknown encoders
		videoFilters := ""
		if inputFormat == "mjpeg" || inputFormat == "yuyv422" ||
			inputFormat == "bgr24" || inputFormat == "rgb24" ||
			inputFormat == "nv24" || inputFormat == "nv16" {
			videoFilters = "format=yuv420p"
		}
		return &EncoderSettings{
			GlobalArgs: []string{},
			OutputParams: map[string]string{
				"b:v": "1M", // Generic bitrate setting
			},
			VideoFilters: videoFilters,
		}, nil
	}
}

// GetQualityParams translates quality settings to encoder parameters for software encoders
func (v *GenericValidator) GetQualityParams(encoderName string, params *types.QualityParams) (EncoderParams, error) {
	result := make(EncoderParams)

	// Handle software encoders specifically
	switch encoderName {
	case "libx264", "libx265":
		// x264/x265 specific parameters
		switch params.Mode {
		case types.RateControlCBR:
			if params.TargetBitrate != nil {
				bitrate := fmt.Sprintf("%.1fM", *params.TargetBitrate)
				result["b:v"] = bitrate
				result["minrate"] = bitrate
				result["maxrate"] = bitrate
			}
			if params.BufferSize != nil {
				result["bufsize"] = fmt.Sprintf("%.1fM", *params.BufferSize)
			} else if params.TargetBitrate != nil {
				// Default buffer size to 2x bitrate for CBR
				result["bufsize"] = fmt.Sprintf("%.1fM", *params.TargetBitrate*2)
			}

		case types.RateControlVBR:
			if params.TargetBitrate != nil {
				result["b:v"] = fmt.Sprintf("%.1fM", *params.TargetBitrate)
			}
			if params.MaxBitrate != nil {
				result["maxrate"] = fmt.Sprintf("%.1fM", *params.MaxBitrate)
			}
			if params.BufferSize != nil {
				result["bufsize"] = fmt.Sprintf("%.1fM", *params.BufferSize)
			} else if params.MaxBitrate != nil {
				// Default buffer size to 2x max bitrate for VBR
				result["bufsize"] = fmt.Sprintf("%.1fM", *params.MaxBitrate*2)
			}

		case types.RateControlCRF:
			if params.Quality != nil {
				result["crf"] = fmt.Sprintf("%d", *params.Quality)
			} else {
				// Default CRF values
				if encoderName == "libx264" {
					result["crf"] = "23"
				} else { // libx265
					result["crf"] = "28"
				}
			}

		case types.RateControlCQP:
			if params.Quality != nil {
				result["qp"] = fmt.Sprintf("%d", *params.Quality)
			}

		default:
			return nil, fmt.Errorf("unsupported rate control mode %s for %s", params.Mode, encoderName)
		}

		// Add preset if specified
		if params.Preset != nil {
			// Validate preset value
			validPresets := []string{"ultrafast", "superfast", "veryfast", "faster", "fast", "medium", "slow", "slower", "veryslow"}
			isValid := false
			for _, p := range validPresets {
				if *params.Preset == p {
					isValid = true
					break
				}
			}
			if isValid {
				result["preset"] = *params.Preset
			}
		} else {
			// Default preset for streaming
			result["preset"] = "ultrafast"
		}

		// Add B-frames if specified
		if params.BFrames != nil {
			result["bf"] = fmt.Sprintf("%d", *params.BFrames)
		}

		// Add keyframe interval if specified
		if params.KeyframeInterval != nil {
			result["g"] = fmt.Sprintf("%d", *params.KeyframeInterval)
		}

	case "libvpx", "libvpx-vp9":
		// VP8/VP9 specific parameters
		switch params.Mode {
		case types.RateControlCBR:
			if params.TargetBitrate != nil {
				bitrate := fmt.Sprintf("%.1fM", *params.TargetBitrate)
				result["b:v"] = bitrate
				result["minrate"] = bitrate
				result["maxrate"] = bitrate
			}

		case types.RateControlVBR, types.RateControlCRF:
			// VP8/VP9 uses crf for quality-based encoding
			if params.Quality != nil {
				result["crf"] = fmt.Sprintf("%d", *params.Quality)
			}
			if params.TargetBitrate != nil {
				result["b:v"] = fmt.Sprintf("%.1fM", *params.TargetBitrate)
			}

		default:
			return nil, fmt.Errorf("unsupported rate control mode %s for %s", params.Mode, encoderName)
		}

		// Add keyframe interval if specified
		if params.KeyframeInterval != nil {
			result["g"] = fmt.Sprintf("%d", *params.KeyframeInterval)
		}

	default:
		// Generic fallback for unknown encoders
		switch params.Mode {
		case types.RateControlCBR, types.RateControlVBR:
			if params.TargetBitrate != nil {
				result["b:v"] = fmt.Sprintf("%.1fM", *params.TargetBitrate)
			}
			if params.MaxBitrate != nil {
				result["maxrate"] = fmt.Sprintf("%.1fM", *params.MaxBitrate)
			}
			if params.MinBitrate != nil {
				result["minrate"] = fmt.Sprintf("%.1fM", *params.MinBitrate)
			}

		case types.RateControlCQP:
			if params.Quality != nil {
				result["qp"] = fmt.Sprintf("%d", *params.Quality)
			}

		default:
			return nil, fmt.Errorf("unsupported rate control mode %s for generic encoder", params.Mode)
		}

		// Add common parameters
		if params.KeyframeInterval != nil {
			result["g"] = fmt.Sprintf("%d", *params.KeyframeInterval)
		}
	}

	return result, nil
}
