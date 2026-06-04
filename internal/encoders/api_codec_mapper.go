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

// MapAPICodec maps a logical codec ("h264", "h265") and input pixel
// format to the best validated FFmpeg encoder + its production settings.
func MapAPICodec(apiCodec, inputFormat string, provider types.ValidationProvider) (*EncoderConfig, error) {
	results := provider.GetValidation()
	if results == nil {
		return nil, fmt.Errorf("no validation data available")
	}

	registry := CreateValidatorRegistry()
	availableValidators := registry.GetAvailableValidators()

	allWorkingEncoders := make([]string, 0, len(results.H264.Working)+len(results.H265.Working))
	allWorkingEncoders = append(allWorkingEncoders, results.H264.Working...)
	allWorkingEncoders = append(allWorkingEncoders, results.H265.Working...)

	workingSet := make(map[string]bool)
	for _, encoder := range allWorkingEncoders {
		workingSet[encoder] = true
	}

	for _, validator := range availableValidators {
		encoderNames := validator.GetEncoderNames()
		for _, encoderName := range encoderNames {
			if workingSet[encoderName] && matchesAPICodec(encoderName, apiCodec) {
				settings, err := validator.GetProductionSettings(encoderName, inputFormat)
				if err != nil {
					continue
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
