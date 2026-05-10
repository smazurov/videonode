package validation

import (
	"fmt"
	"slices"

	"github.com/smazurov/videonode/internal/types"
)

// GenericValidator validates unknown encoder types with basic parameters.
type GenericValidator struct{}

// NewGenericValidator creates a new generic validator.
func NewGenericValidator() *GenericValidator {
	return &GenericValidator{}
}

// CanValidate returns true for any encoder (this is the fallback validator).
func (v *GenericValidator) CanValidate(_ string) bool {
	return true
}

// Validate tests unknown encoder types using production settings.
func (v *GenericValidator) Validate(encoderName string) (bool, error) {
	return ValidateEncoderWithSettings(v, encoderName)
}

// GetEncoderNames returns common software encoder names that this validator handles.
func (v *GenericValidator) GetEncoderNames() []string {
	return []string{
		"libx264",
		"libx265",
		"libvpx",
		"libvpx-vp9",
		"mpeg4",
		"libxvid",
	}
}

// GetDescription returns a description of this validator.
func (v *GenericValidator) GetDescription() string {
	return "Generic validator - Software encoder fallback and validation for unknown encoder types"
}

// GetBackendName returns the canonical backend tag for software encoders.
func (v *GenericValidator) GetBackendName() string { return "sw" }

// ValidateDecoders is a no-op (SW decoders always available).
func (v *GenericValidator) ValidateDecoders(_ Logger) (working, failed []string) {
	return nil, nil
}

// ValidateFilters is a no-op for the SW backend.
func (v *GenericValidator) ValidateFilters(_ Logger) (working, failed []string) {
	return nil, nil
}

// getVideoFilters returns the appropriate video filter for the input format.
func getVideoFilters(inputFormat string) string {
	switch inputFormat {
	case "", "testsrc":
		return ""
	case "mjpeg", "yuyv422", "bgr24", "rgb24", "nv24", "nv16":
		return "format=yuv420p"
	default:
		return ""
	}
}

// GetProductionSettings returns production settings for software encoders.
func (v *GenericValidator) GetProductionSettings(encoderName string, inputFormat string) (*EncoderSettings, error) {
	switch encoderName {
	case "libx264":
		return &EncoderSettings{
			OutputParams: map[string]string{"crf": "18", "preset": "ultrafast"},
			VideoFilters: getVideoFilters(inputFormat),
		}, nil

	case "libx265":
		return &EncoderSettings{
			OutputParams: map[string]string{"crf": "20", "preset": "ultrafast"},
			VideoFilters: getVideoFilters(inputFormat),
		}, nil

	default:
		return &EncoderSettings{
			OutputParams: map[string]string{"b:v": "1M"},
			VideoFilters: getVideoFilters(inputFormat),
		}, nil
	}
}

// GetQualityParams translates quality settings to encoder parameters for software encoders.
func (v *GenericValidator) GetQualityParams(encoderName string, params *types.QualityParams) (EncoderParams, error) {
	result := make(EncoderParams)

	switch encoderName {
	case "libx264", "libx265":
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
				result["bufsize"] = fmt.Sprintf("%.1fM", *params.MaxBitrate*2)
			}

		case types.RateControlCRF:
			switch {
			case params.Quality != nil:
				result["crf"] = fmt.Sprintf("%d", *params.Quality)
			case encoderName == "libx264":
				result["crf"] = "23"
			default:
				result["crf"] = "28"
			}

		case types.RateControlCQP:
			if params.Quality != nil {
				result["qp"] = fmt.Sprintf("%d", *params.Quality)
			}

		default:
			return nil, fmt.Errorf("unsupported rate control mode %s for %s", params.Mode, encoderName)
		}

		if params.Preset != nil {
			validPresets := []string{"ultrafast", "superfast", "veryfast", "faster", "fast", "medium", "slow", "slower", "veryslow"}
			if slices.Contains(validPresets, *params.Preset) {
				result["preset"] = *params.Preset
			}
		} else {
			result["preset"] = "ultrafast"
		}

		if params.BFrames != nil {
			result["bf"] = fmt.Sprintf("%d", *params.BFrames)
		}

		if params.KeyframeInterval != nil {
			result["g"] = fmt.Sprintf("%d", *params.KeyframeInterval)
		}

	case "libvpx", "libvpx-vp9":
		switch params.Mode {
		case types.RateControlCBR:
			if params.TargetBitrate != nil {
				bitrate := fmt.Sprintf("%.1fM", *params.TargetBitrate)
				result["b:v"] = bitrate
				result["minrate"] = bitrate
				result["maxrate"] = bitrate
			}

		case types.RateControlVBR, types.RateControlCRF:
			if params.Quality != nil {
				result["crf"] = fmt.Sprintf("%d", *params.Quality)
			}
			if params.TargetBitrate != nil {
				result["b:v"] = fmt.Sprintf("%.1fM", *params.TargetBitrate)
			}

		default:
			return nil, fmt.Errorf("unsupported rate control mode %s for %s", params.Mode, encoderName)
		}

		if params.KeyframeInterval != nil {
			result["g"] = fmt.Sprintf("%d", *params.KeyframeInterval)
		}

	default:
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

		if params.KeyframeInterval != nil {
			result["g"] = fmt.Sprintf("%d", *params.KeyframeInterval)
		}
	}

	return result, nil
}
