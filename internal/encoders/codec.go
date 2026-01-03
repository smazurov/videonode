package encoders

import (
	"fmt"

	"github.com/smazurov/videonode/internal/encoders/validation"
	"github.com/smazurov/videonode/internal/types"
)

// SelectBestCodec selects the best available codec using the validator registry
// prioritizing hardware encoders over software ones.
func SelectBestCodec(encoderList *EncoderList) *Encoder {
	if encoderList == nil || len(encoderList.VideoEncoders) == 0 {
		return nil
	}

	registry := CreateValidatorRegistry()
	availableValidators := registry.GetAvailableValidators()

	// Create a map of available encoders for quick lookup
	availableEncoders := make(map[string]Encoder)
	for _, encoder := range encoderList.VideoEncoders {
		availableEncoders[encoder.Name] = encoder
	}

	// Try each validator in order (hardware validators first, generic last)
	for _, validator := range availableValidators {
		compiledEncoders := registry.GetCompiledEncoders(validator)
		for _, encoderName := range compiledEncoders {
			if encoder, exists := availableEncoders[encoderName]; exists {
				return &encoder
			}
		}
	}

	// Final fallback: return the first available encoder
	if len(encoderList.VideoEncoders) > 0 {
		return &encoderList.VideoEncoders[0]
	}

	return nil
}

// CodecType represents the type of codec (h264 or h265).
type CodecType string

// Codec type constants.
const (
	CodecH264 CodecType = "h264"
	CodecH265 CodecType = "h265"
)

// GetOptimalCodec returns the best available codec for encoding (backward compatibility)
//
// Deprecated: Use GetOptimalEncoderWithSettings instead.
func GetOptimalCodec() string {
	// Can't use StreamManager here for backward compatibility, just return default
	return "libx264"
}

// GetOptimalEncoderWithSettings returns the best available encoder with its settings
//
// Deprecated: Use Selector interface instead.
func GetOptimalEncoderWithSettings(codecType CodecType, provider types.ValidationProvider) (string, *validation.EncoderSettings, error) {
	// Get validation results directly from provider
	validationResults := provider.GetValidation()
	if validationResults == nil {
		return "", nil, fmt.Errorf("no validation data available")
	}

	// Get working encoders for the codec type
	var workingEncoders []string
	switch codecType {
	case CodecH264:
		workingEncoders = validationResults.H264.Working
	case CodecH265:
		workingEncoders = validationResults.H265.Working
	default:
		return "", nil, fmt.Errorf("unsupported codec type: %s", codecType)
	}

	if len(workingEncoders) == 0 {
		// Fall back to software encoder
		if codecType == CodecH264 {
			return "libx264", nil, nil
		}
		return "libx265", nil, nil
	}

	// Return the first working encoder with its settings
	registry := CreateValidatorRegistry()
	for _, encoder := range workingEncoders {
		validator := registry.FindValidator(encoder)
		if validator != nil {
			settings, err := validator.GetProductionSettings(encoder, "")
			if err == nil {
				return encoder, settings, nil
			}
		}
	}

	return workingEncoders[0], nil, nil
}
