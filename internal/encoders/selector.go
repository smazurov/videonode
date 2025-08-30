package encoders

import (
	"fmt"
	"log"

	"github.com/smazurov/videonode/internal/encoders/validation"
	"github.com/smazurov/videonode/internal/types"
	valmanager "github.com/smazurov/videonode/internal/validation"
)

// Selector interface defines the contract for encoder selection strategies
type Selector interface {
	// SelectEncoder chooses the best encoder for the given codec type and input format
	SelectEncoder(codecType CodecType, inputFormat string, qualityParams *types.QualityParams) (string, *validation.EncoderSettings, error)
	// ValidateEncoder checks if an encoder is valid for use
	ValidateEncoder(encoder string) error
}

// DefaultSelector implements Selector using validation data and registry
type DefaultSelector struct {
	validationManager *valmanager.Manager
	registry          *validation.ValidatorRegistry
}

// NewDefaultSelector creates a new DefaultSelector
func NewDefaultSelector(validationManager *valmanager.Manager) *DefaultSelector {
	return &DefaultSelector{
		validationManager: validationManager,
		registry:          CreateValidatorRegistry(),
	}
}

// SelectEncoder chooses the best available encoder for the given codec type and input format
func (s *DefaultSelector) SelectEncoder(codecType CodecType, inputFormat string, qualityParams *types.QualityParams) (string, *validation.EncoderSettings, error) {
	// Get validation results
	validationResults := s.validationManager.GetValidation()
	if validationResults == nil {
		// No validation data, fall back to software encoder
		return s.getFallbackEncoder(codecType), nil, nil
	}

	// Get working encoders for the codec type
	var workingEncoders []string
	switch codecType {
	case CodecH264:
		workingEncoders = validationResults.H264.Working
	case CodecH265:
		workingEncoders = validationResults.H265.Working
	default:
		return "", nil, fmt.Errorf("unsupported codec type: %v", codecType)
	}

	// If no working encoders, use fallback
	if len(workingEncoders) == 0 {
		encoder := s.getFallbackEncoder(codecType)
		log.Printf("No validated encoders for %s, using fallback: %s", codecType, encoder)
		return encoder, nil, nil
	}

	// Get available validators in priority order
	availableValidators := s.registry.GetAvailableValidators()

	// Find the first working encoder based on validator priority
	for _, validator := range availableValidators {
		encoderList := s.registry.GetCompiledEncoders(validator)

		// Check each encoder from this validator
		for _, encoder := range encoderList {
			for _, working := range workingEncoders {
				if encoder == working {
					// Found a working encoder, get its settings with quality params
					settings := s.getEncoderSettingsFromValidator(validator, encoder, inputFormat, qualityParams)
					log.Printf("Selected %s encoder %s with priority", codecType, encoder)
					return encoder, settings, nil
				}
			}
		}
	}

	// If somehow we didn't find anything (shouldn't happen), use first working encoder
	encoder := workingEncoders[0]
	settings := s.getSettingsForEncoder(encoder, inputFormat, qualityParams)
	return encoder, settings, nil
}

// ValidateEncoder checks if an encoder is in the validated working list
func (s *DefaultSelector) ValidateEncoder(encoder string) error {
	if s.validationManager.IsEncoderWorking(encoder) {
		return nil
	}

	// Check if validation data exists
	if s.validationManager.GetValidation() == nil {
		// No validation data, allow with warning
		log.Printf("Warning: No encoder validation data found, allowing encoder %s", encoder)
		return nil
	}

	return fmt.Errorf("encoder %s is not in the validated working list", encoder)
}

// getFallbackEncoder returns a software encoder fallback
func (s *DefaultSelector) getFallbackEncoder(codecType CodecType) string {
	switch codecType {
	case CodecH264:
		return "libx264"
	case CodecH265:
		return "libx265"
	default:
		return "libx264"
	}
}

// getSettingsForEncoder retrieves settings for a specific encoder
func (s *DefaultSelector) getSettingsForEncoder(encoderName string, inputFormat string, qualityParams *types.QualityParams) *validation.EncoderSettings {
	// Try to find settings from validators
	availableValidators := s.registry.GetAvailableValidators()

	for _, validator := range availableValidators {
		encoderList := s.registry.GetCompiledEncoders(validator)
		for _, encoder := range encoderList {
			if encoder == encoderName {
				return s.getEncoderSettingsFromValidator(validator, encoder, inputFormat, qualityParams)
			}
		}
	}

	return nil
}

// getEncoderSettingsFromValidator retrieves settings for a specific encoder from a validator
func (s *DefaultSelector) getEncoderSettingsFromValidator(validator validation.EncoderValidator, encoderName string, inputFormat string, qualityParams *types.QualityParams) *validation.EncoderSettings {
	// Get production settings from the validator
	settings, err := validator.GetProductionSettings(encoderName, inputFormat)
	if err != nil {
		log.Printf("Failed to get production settings for %s: %v", encoderName, err)
		return nil
	}

	// If quality params are provided, get quality-specific encoder params and merge
	if qualityParams != nil {
		qualityEncoderParams, err := validator.GetQualityParams(encoderName, qualityParams)
		if err != nil {
			log.Printf("Failed to get quality params for %s: %v", encoderName, err)
			// Return settings without quality params rather than failing completely
			return settings
		}

		// Merge OutputParams - quality params override existing ones
		if settings.OutputParams == nil {
			settings.OutputParams = make(map[string]string)
		}

		// First copy existing params
		mergedParams := make(map[string]string)
		for k, v := range settings.OutputParams {
			mergedParams[k] = v
		}

		// Then apply quality params (these override)
		for k, v := range qualityEncoderParams {
			mergedParams[k] = v
		}

		settings.OutputParams = mergedParams
	}

	return settings
}

// PrioritySelector extends DefaultSelector with custom priority logic
type PrioritySelector struct {
	*DefaultSelector
	priorities map[string]int // encoder name -> priority (lower is better)
}

// NewPrioritySelector creates a new PrioritySelector
func NewPrioritySelector(validationManager *valmanager.Manager, priorities map[string]int) *PrioritySelector {
	return &PrioritySelector{
		DefaultSelector: NewDefaultSelector(validationManager),
		priorities:      priorities,
	}
}

// SelectEncoder chooses encoder based on custom priorities
func (s *PrioritySelector) SelectEncoder(codecType CodecType, inputFormat string, qualityParams *types.QualityParams) (string, *validation.EncoderSettings, error) {
	// Get validation results
	validationResults := s.validationManager.GetValidation()
	if validationResults == nil {
		return s.DefaultSelector.SelectEncoder(codecType, inputFormat, qualityParams)
	}

	// Get working encoders
	var workingEncoders []string
	switch codecType {
	case CodecH264:
		workingEncoders = validationResults.H264.Working
	case CodecH265:
		workingEncoders = validationResults.H265.Working
	default:
		return "", nil, fmt.Errorf("unsupported codec type: %v", codecType)
	}

	if len(workingEncoders) == 0 {
		return s.DefaultSelector.SelectEncoder(codecType, inputFormat, qualityParams)
	}

	// Find encoder with best priority
	bestEncoder := ""
	bestPriority := int(^uint(0) >> 1) // Max int

	for _, encoder := range workingEncoders {
		priority, hasPriority := s.priorities[encoder]
		if !hasPriority {
			priority = 1000 // Default priority for unlisted encoders
		}

		if priority < bestPriority {
			bestPriority = priority
			bestEncoder = encoder
		}
	}

	if bestEncoder != "" {
		settings := s.getSettingsForEncoder(bestEncoder, inputFormat, qualityParams)
		log.Printf("Selected %s encoder %s based on priority %d", codecType, bestEncoder, bestPriority)
		return bestEncoder, settings, nil
	}

	// Fall back to default selection
	return s.DefaultSelector.SelectEncoder(codecType, inputFormat, qualityParams)
}
