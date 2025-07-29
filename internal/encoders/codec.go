package encoders

import (
	"fmt"
	"log"
	"os"

	"github.com/smazurov/videonode/internal/encoders/validation"
)

// SelectBestCodec selects the best available codec using the validator registry
// prioritizing hardware encoders over software ones
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

// GetOptimalCodec returns the best available codec for encoding (backward compatibility)
func GetOptimalCodec() string {
	encoderName, _, err := GetOptimalEncoderWithSettings()
	if err != nil {
		log.Printf("Failed to get optimal encoder: %v", err)
		return "libx264"
	}
	return encoderName
}

// GetOptimalEncoderWithSettings returns the best available encoder with its settings
func GetOptimalEncoderWithSettings() (string, *validation.EncoderSettings, error) {
	registry := CreateValidatorRegistry()

	// First try to load validation results and get first working encoder
	if workingCodec := getValidatedCodec(); workingCodec != "" {
		log.Printf("Using validated codec: %s", workingCodec)

		// Get settings for the validated codec using the registry
		settings, err := getSettingsForEncoder(workingCodec)
		if err != nil {
			log.Printf("Failed to get settings for validated codec %s: %v", workingCodec, err)
			// Fall through to registry-based detection
		} else {
			return workingCodec, settings, nil
		}
	}

	// Use validator registry to find the best available encoder
	log.Printf("Using validator registry for encoder selection")
	availableValidators := registry.GetAvailableValidators()

	for _, validator := range availableValidators {
		compiledEncoders := registry.GetCompiledEncoders(validator)
		if len(compiledEncoders) > 0 {
			// Use the first encoder from this validator
			encoderName := compiledEncoders[0]
			log.Printf("Selected encoder from %s: %s", validator.GetDescription(), encoderName)

			settings, err := validator.GetProductionSettings(encoderName)
			if err != nil {
				log.Printf("Failed to get settings for %s: %v", encoderName, err)
				continue // Try next validator
			}

			return encoderName, settings, nil
		}
	}

	return "", nil, fmt.Errorf("no available encoders found through validator registry")
}

// getSettingsForEncoder gets production settings for a specific encoder using the validation registry
func getSettingsForEncoder(encoderName string) (*validation.EncoderSettings, error) {
	registry := CreateValidatorRegistry()
	validator := registry.FindValidator(encoderName)

	if validator == nil {
		return nil, fmt.Errorf("no validator found for encoder: %s", encoderName)
	}

	return validator.GetProductionSettings(encoderName)
}

// getValidatedCodec returns the best working codec from validation results using validator registry order
func getValidatedCodec() string {
	// Try to load validation results
	validationFile := "validated_encoders.toml"
	if _, err := os.Stat(validationFile); os.IsNotExist(err) {
		return "" // No validation file exists
	}

	results, err := LoadValidationResults(validationFile)
	if err != nil {
		log.Printf("Failed to load validation results: %v", err)
		return ""
	}

	// Use validator registry order to prioritize encoders
	registry := CreateValidatorRegistry()
	availableValidators := registry.GetAvailableValidators()

	// Collect all working encoders from validation results
	var allWorkingEncoders []string
	allWorkingEncoders = append(allWorkingEncoders, results.H264.Working...)
	allWorkingEncoders = append(allWorkingEncoders, results.H265.Working...)

	// Create a set of working encoders for quick lookup
	workingSet := make(map[string]bool)
	for _, encoder := range allWorkingEncoders {
		workingSet[encoder] = true
	}

	// Find the highest priority encoder that's also validated as working
	for _, validator := range availableValidators {
		encoderNames := validator.GetEncoderNames()
		for _, encoderName := range encoderNames {
			if workingSet[encoderName] {
				return encoderName
			}
		}
	}

	return "" // No working encoders found through validator registry
}
