package encoders

import (
	"fmt"
	"log"
	"strings"

	"github.com/smazurov/videonode/internal/encoders/validation"
	"github.com/smazurov/videonode/internal/ffmpeg"
	"github.com/smazurov/videonode/internal/types"
	valmanager "github.com/smazurov/videonode/internal/validation"
)

// Selector interface defines the contract for encoder selection strategies
type Selector interface {
	// SelectEncoder chooses the best encoder for the given codec type and input format
	// Returns FFmpegParams with all encoding parameters populated
	// If encoderOverride is provided, uses that encoder directly with proper settings
	SelectEncoder(codecType CodecType, inputFormat string, qualityParams *types.QualityParams, encoderOverride string) (*ffmpeg.Params, error)
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
func (s *DefaultSelector) SelectEncoder(codecType CodecType, inputFormat string, qualityParams *types.QualityParams, encoderOverride string) (*ffmpeg.Params, error) {
	params := &ffmpeg.Params{}

	// If encoder override is provided, use it directly with proper settings
	if encoderOverride != "" {
		params.Encoder = encoderOverride
		settings := s.getSettingsForEncoder(encoderOverride, inputFormat, qualityParams)
		if settings != nil {
			s.convertSettingsToParams(params, settings, qualityParams)
		} else {
			// No specific settings found, just populate quality params
			s.populateQualityParams(params, qualityParams, s.isHardwareEncoder(encoderOverride))
		}
		return params, nil
	}

	// Get validation results for auto-selection
	validationResults := s.validationManager.GetValidation()
	if validationResults == nil {
		// No validation data, fall back to software encoder
		params.Encoder = s.getFallbackEncoder(codecType)
		s.populateQualityParams(params, qualityParams, false)
		return params, nil
	}

	// Get working encoders for the codec type
	var workingEncoders []string
	switch codecType {
	case CodecH264:
		workingEncoders = validationResults.H264.Working
	case CodecH265:
		workingEncoders = validationResults.H265.Working
	default:
		return nil, fmt.Errorf("unsupported codec type: %v", codecType)
	}

	// If no working encoders, use fallback
	if len(workingEncoders) == 0 {
		params.Encoder = s.getFallbackEncoder(codecType)
		log.Printf("No validated encoders for %s, using fallback: %s", codecType, params.Encoder)
		s.populateQualityParams(params, qualityParams, false)
		return params, nil
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
					// Found a working encoder
					params.Encoder = encoder
					log.Printf("Selected %s encoder %s with priority", codecType, encoder)

					// Get settings from validator and convert to params
					settings := s.getEncoderSettingsFromValidator(validator, encoder, inputFormat, qualityParams)
					if settings != nil {
						s.convertSettingsToParams(params, settings, qualityParams)
					}
					return params, nil
				}
			}
		}
	}

	// If somehow we didn't find anything (shouldn't happen), use first working encoder
	params.Encoder = workingEncoders[0]
	settings := s.getSettingsForEncoder(workingEncoders[0], inputFormat, qualityParams)
	if settings != nil {
		s.convertSettingsToParams(params, settings, qualityParams)
	}
	return params, nil
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
	// Use GetAllValidators for encoder overrides - don't check if compiled
	availableValidators := s.registry.GetAllValidators()

	for _, validator := range availableValidators {
		// Just check if validator can handle this encoder
		if validator.CanValidate(encoderName) {
			return s.getEncoderSettingsFromValidator(validator, encoderName, inputFormat, qualityParams)
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

// populateQualityParams populates FFmpeg params from quality parameters
func (s *DefaultSelector) populateQualityParams(params *ffmpeg.Params, qualityParams *types.QualityParams, isHardware bool) {
	if qualityParams == nil {
		return
	}

	switch qualityParams.Mode {
	case types.RateControlCBR:
		if qualityParams.TargetBitrate != nil {
			params.Bitrate = fmt.Sprintf("%.1fM", *qualityParams.TargetBitrate)
			if !isHardware {
				params.MinRate = params.Bitrate
				params.MaxRate = params.Bitrate
			}
		}
		if qualityParams.BufferSize != nil {
			params.BufferSize = fmt.Sprintf("%.1fM", *qualityParams.BufferSize)
		} else if qualityParams.TargetBitrate != nil && !isHardware {
			params.BufferSize = fmt.Sprintf("%.1fM", *qualityParams.TargetBitrate*2)
		}

	case types.RateControlVBR:
		if qualityParams.TargetBitrate != nil {
			params.Bitrate = fmt.Sprintf("%.1fM", *qualityParams.TargetBitrate)
		}
		if qualityParams.MinBitrate != nil {
			params.MinRate = fmt.Sprintf("%.1fM", *qualityParams.MinBitrate)
		}
		if qualityParams.MaxBitrate != nil {
			params.MaxRate = fmt.Sprintf("%.1fM", *qualityParams.MaxBitrate)
		}
		if qualityParams.BufferSize != nil {
			params.BufferSize = fmt.Sprintf("%.1fM", *qualityParams.BufferSize)
		}

	case types.RateControlCRF:
		if qualityParams.Quality != nil {
			params.CRF = *qualityParams.Quality
		} else if !isHardware {
			params.CRF = 23 // Default CRF for software encoders
		}

	case types.RateControlCQP:
		if qualityParams.Quality != nil {
			params.QP = *qualityParams.Quality
		}
	}

	// Common parameters
	if qualityParams.KeyframeInterval != nil {
		params.GOP = *qualityParams.KeyframeInterval
	}
	if qualityParams.BFrames != nil {
		params.BFrames = *qualityParams.BFrames
	} else {
		params.BFrames = 0 // Default to 0 for WebRTC compatibility
	}
	if qualityParams.Preset != nil && !isHardware {
		params.Preset = *qualityParams.Preset
	}
}

// convertSettingsToParams converts EncoderSettings to FFmpegParams
func (s *DefaultSelector) convertSettingsToParams(params *ffmpeg.Params, settings *validation.EncoderSettings, qualityParams *types.QualityParams) {
	// Set global args and video filters
	params.GlobalArgs = settings.GlobalArgs
	params.VideoFilters = settings.VideoFilters

	// Determine if hardware encoder
	isHardware := s.isHardwareEncoder(params.Encoder)

	// Process output params but skip b:v since we handle it via qualityParams
	for key, value := range settings.OutputParams {
		switch key {
		case "b:v":
			// Skip - handled via qualityParams
		case "rc_mode":
			params.RCMode = value
		case "qp", "qp_init":
			// Convert to int if needed
			if params.QP == 0 {
				fmt.Sscanf(value, "%d", &params.QP)
			}
		case "crf":
			if params.CRF == 0 {
				fmt.Sscanf(value, "%d", &params.CRF)
			}
		case "preset":
			if params.Preset == "" {
				params.Preset = value
			}
		case "g":
			if params.GOP == 0 {
				fmt.Sscanf(value, "%d", &params.GOP)
			}
		case "bf":
			if params.BFrames == -1 {
				fmt.Sscanf(value, "%d", &params.BFrames)
			}
		case "minrate":
			if params.MinRate == "" {
				params.MinRate = value
			}
		case "maxrate":
			if params.MaxRate == "" {
				params.MaxRate = value
			}
		case "bufsize":
			if params.BufferSize == "" {
				params.BufferSize = value
			}
		}
	}

	// Now populate from quality params, which takes precedence
	s.populateQualityParams(params, qualityParams, isHardware)
}

// isHardwareEncoder checks if an encoder is hardware-accelerated
func (s *DefaultSelector) isHardwareEncoder(encoder string) bool {
	// List of known hardware encoder prefixes/suffixes
	hardwareEncoders := []string{
		"_vaapi", "_nvenc", "_qsv", "_amf", "_videotoolbox", "_v4l2m2m", "_mmal", "_omx", "_rkmpp",
	}

	for _, hw := range hardwareEncoders {
		if strings.Contains(encoder, hw) {
			return true
		}
	}

	return false
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
func (s *PrioritySelector) SelectEncoder(codecType CodecType, inputFormat string, qualityParams *types.QualityParams, encoderOverride string) (*ffmpeg.Params, error) {
	// If encoder override provided, delegate to default selector
	if encoderOverride != "" {
		return s.DefaultSelector.SelectEncoder(codecType, inputFormat, qualityParams, encoderOverride)
	}

	// Get validation results
	validationResults := s.validationManager.GetValidation()
	if validationResults == nil {
		return s.DefaultSelector.SelectEncoder(codecType, inputFormat, qualityParams, "")
	}

	// Get working encoders
	var workingEncoders []string
	switch codecType {
	case CodecH264:
		workingEncoders = validationResults.H264.Working
	case CodecH265:
		workingEncoders = validationResults.H265.Working
	default:
		return nil, fmt.Errorf("unsupported codec type: %v", codecType)
	}

	if len(workingEncoders) == 0 {
		return s.DefaultSelector.SelectEncoder(codecType, inputFormat, qualityParams, "")
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
		params := &ffmpeg.Params{
			Encoder: bestEncoder,
		}

		settings := s.getSettingsForEncoder(bestEncoder, inputFormat, qualityParams)
		if settings != nil {
			s.convertSettingsToParams(params, settings, qualityParams)
		}

		log.Printf("Selected %s encoder %s based on priority %d", codecType, bestEncoder, bestPriority)
		return params, nil
	}

	// Fall back to default selection
	return s.DefaultSelector.SelectEncoder(codecType, inputFormat, qualityParams, "")
}
