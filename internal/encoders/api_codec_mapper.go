package encoders

import (
	"fmt"
	"strings"

	"github.com/smazurov/videonode/internal/encoders/validation"
	"github.com/smazurov/videonode/internal/types"
)

// EncoderConfig represents a complete encoder configuration.
type EncoderConfig struct {
	EncoderName string
	Settings    *validation.EncoderSettings
}

// MapAPICodec maps API codec types to the best available FFmpeg encoder.
func MapAPICodec(apiCodec string, provider types.ValidationProvider) (*EncoderConfig, error) {
	// Load validated encoders from provider
	results := provider.GetValidation()
	if results == nil {
		return nil, fmt.Errorf("no validation data available")
	}

	registry := CreateValidatorRegistry()
	availableValidators := registry.GetAvailableValidators()

	// Collect all working encoders from validation results
	allWorkingEncoders := make([]string, 0, len(results.H264.Working)+len(results.H265.Working))
	allWorkingEncoders = append(allWorkingEncoders, results.H264.Working...)
	allWorkingEncoders = append(allWorkingEncoders, results.H265.Working...)

	// Create a set of working encoders for quick lookup
	workingSet := make(map[string]bool)
	for _, encoder := range allWorkingEncoders {
		workingSet[encoder] = true
	}

	// Find the highest priority encoder that matches the API codec and is validated as working
	for _, validator := range availableValidators {
		encoderNames := validator.GetEncoderNames()
		for _, encoderName := range encoderNames {
			if workingSet[encoderName] && matchesAPICodec(encoderName, apiCodec) {
				// Pass empty input format for mapper purposes
				settings, err := validator.GetProductionSettings(encoderName, "")
				if err != nil {
					continue // Try next encoder
				}

				return &EncoderConfig{
					EncoderName: encoderName,
					Settings:    settings,
				}, nil
			}
		}
	}

	return nil, fmt.Errorf("no working encoder found for API codec: %s", apiCodec)
}

// matchesAPICodec checks if an encoder name matches the requested API codec.
func matchesAPICodec(encoderName, apiCodec string) bool {
	switch apiCodec {
	case "h264":
		return strings.Contains(encoderName, "h264") || strings.Contains(encoderName, "x264")
	case "h265":
		return strings.Contains(encoderName, "hevc") || strings.Contains(encoderName, "h265") || strings.Contains(encoderName, "x265")
	default:
		return false
	}
}
