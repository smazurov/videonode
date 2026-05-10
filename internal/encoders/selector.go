package encoders

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/smazurov/videonode/internal/encoders/validation"
	"github.com/smazurov/videonode/internal/ffmpeg"
	"github.com/smazurov/videonode/internal/logging"
	"github.com/smazurov/videonode/internal/types"
	valmanager "github.com/smazurov/videonode/internal/validation"
)

// Selector interface defines the contract for encoder selection strategies.
type Selector interface {
	SelectEncoder(codecType CodecType, inputFormat string, qualityParams *types.QualityParams, encoderOverride string) (*ffmpeg.Params, error)
	ValidateEncoder(encoder string) error
}

// DefaultSelector implements Selector using validation data and registry.
type DefaultSelector struct {
	logger            logging.Logger
	validationManager *valmanager.Manager
	registry          *validation.ValidatorRegistry
}

// NewDefaultSelector creates a new DefaultSelector.
func NewDefaultSelector(validationManager *valmanager.Manager) *DefaultSelector {
	return &DefaultSelector{
		logger:            logging.GetLogger("streams"),
		validationManager: validationManager,
		registry:          CreateValidatorRegistry(),
	}
}

// SelectEncoder chooses the best available encoder for the given codec type and input format.
func (s *DefaultSelector) SelectEncoder(codecType CodecType, inputFormat string, qualityParams *types.QualityParams, encoderOverride string) (*ffmpeg.Params, error) {
	params, err := s.selectEncoderInner(codecType, inputFormat, qualityParams, encoderOverride)
	if err != nil {
		return nil, err
	}
	if params != nil {
		params.HWBackend = hwBackendForEncoder(params.Encoder)
	}
	return params, nil
}

func (s *DefaultSelector) selectEncoderInner(codecType CodecType, inputFormat string, qualityParams *types.QualityParams, encoderOverride string) (*ffmpeg.Params, error) {
	params := &ffmpeg.Params{}

	if encoderOverride != "" {
		params.Encoder = encoderOverride
		settings := s.getSettingsForEncoder(encoderOverride, inputFormat, qualityParams)
		if settings != nil {
			s.convertSettingsToParams(params, settings, qualityParams)
		} else {
			s.populateQualityParams(params, qualityParams, s.isHardwareEncoder(encoderOverride))
		}
		return params, nil
	}

	validationResults := s.validationManager.GetValidation()
	if validationResults == nil {
		params.Encoder = s.getFallbackEncoder(codecType)
		s.populateQualityParams(params, qualityParams, false)
		return params, nil
	}

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
		params.Encoder = s.getFallbackEncoder(codecType)
		s.logger.Warn("No validated encoders, using fallback", "codec_type", codecType, "encoder", params.Encoder)
		s.populateQualityParams(params, qualityParams, false)
		return params, nil
	}

	availableValidators := s.registry.GetAvailableValidators()

	for _, validator := range availableValidators {
		encoderList := s.registry.GetCompiledEncoders(validator)

		for _, encoder := range encoderList {
			if slices.Contains(workingEncoders, encoder) {
				params.Encoder = encoder
				s.logger.Info("Selected encoder with priority", "codec_type", codecType, "encoder", encoder)

				settings := s.getEncoderSettingsFromValidator(validator, encoder, inputFormat, qualityParams)
				if settings != nil {
					s.convertSettingsToParams(params, settings, qualityParams)
				}
				return params, nil
			}
		}
	}

	params.Encoder = workingEncoders[0]
	settings := s.getSettingsForEncoder(workingEncoders[0], inputFormat, qualityParams)
	if settings != nil {
		s.convertSettingsToParams(params, settings, qualityParams)
	}
	return params, nil
}

// hwBackendForEncoder maps an encoder name to "rkmpp", "vaapi", or "sw".
func hwBackendForEncoder(encoder string) string {
	switch {
	case strings.HasSuffix(encoder, "_rkmpp"):
		return "rkmpp"
	case strings.HasSuffix(encoder, "_vaapi"):
		return "vaapi"
	default:
		return "sw"
	}
}

// ValidateEncoder checks if an encoder is in the validated working list.
func (s *DefaultSelector) ValidateEncoder(encoder string) error {
	if s.validationManager.IsEncoderWorking(encoder) {
		return nil
	}

	if s.validationManager.GetValidation() == nil {
		s.logger.Warn("No encoder validation data found, allowing encoder", "encoder", encoder)
		return nil
	}

	return fmt.Errorf("encoder %s is not in the validated working list", encoder)
}

// getFallbackEncoder returns a software encoder fallback.
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

// getSettingsForEncoder retrieves settings for a specific encoder (override path: skips compile check).
func (s *DefaultSelector) getSettingsForEncoder(encoderName string, inputFormat string, qualityParams *types.QualityParams) *validation.EncoderSettings {
	availableValidators := s.registry.GetAllValidators()

	for _, validator := range availableValidators {
		if validator.CanValidate(encoderName) {
			return s.getEncoderSettingsFromValidator(validator, encoderName, inputFormat, qualityParams)
		}
	}

	return nil
}

// getEncoderSettingsFromValidator retrieves settings for a specific encoder from a validator.
func (s *DefaultSelector) getEncoderSettingsFromValidator(validator validation.EncoderValidator, encoderName string, inputFormat string, qualityParams *types.QualityParams) *validation.EncoderSettings {
	settings, err := validator.GetProductionSettings(encoderName, inputFormat)
	if err != nil {
		s.logger.Warn("Failed to get production settings", "encoder", encoderName, "error", err)
		return nil
	}

	if qualityParams != nil {
		qualityEncoderParams, qualityErr := validator.GetQualityParams(encoderName, qualityParams)
		if qualityErr != nil {
			s.logger.Warn("Failed to get quality params", "encoder", encoderName, "error", qualityErr)
			return settings
		}

		if settings.OutputParams == nil {
			settings.OutputParams = make(map[string]string)
		}

		// Quality params override existing OutputParams.
		mergedParams := make(map[string]string)
		maps.Copy(mergedParams, settings.OutputParams)
		maps.Copy(mergedParams, qualityEncoderParams)

		settings.OutputParams = mergedParams
	}

	return settings
}

// populateQualityParams populates FFmpeg params from quality parameters.
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
			params.CRF = 23
		}

	case types.RateControlCQP:
		if qualityParams.Quality != nil {
			params.QP = *qualityParams.Quality
		}
	}

	if qualityParams.KeyframeInterval != nil {
		params.GOP = *qualityParams.KeyframeInterval
	}
	if qualityParams.BFrames != nil {
		params.BFrames = *qualityParams.BFrames
	} else {
		params.BFrames = 0 // WebRTC default
	}
	if qualityParams.Preset != nil && !isHardware {
		params.Preset = *qualityParams.Preset
	}
}

// convertSettingsToParams converts EncoderSettings to FFmpegParams.
func (s *DefaultSelector) convertSettingsToParams(params *ffmpeg.Params, settings *validation.EncoderSettings, qualityParams *types.QualityParams) {
	params.GlobalArgs = settings.GlobalArgs
	params.VideoFilters = settings.VideoFilters
	params.HWBackend = hwBackendForEncoder(params.Encoder)

	isHardware := s.isHardwareEncoder(params.Encoder)

	for key, value := range settings.OutputParams {
		switch key {
		case "b:v":
			// handled via qualityParams
		case "rc_mode":
			params.RCMode = value
		case "qp", "qp_init":
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

	// Quality params take precedence over OutputParams.
	s.populateQualityParams(params, qualityParams, isHardware)
}

// isHardwareEncoder checks if an encoder is hardware-accelerated.
func (s *DefaultSelector) isHardwareEncoder(encoder string) bool {
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

// PrioritySelector extends DefaultSelector with custom priority logic.
type PrioritySelector struct {
	*DefaultSelector
	priorities map[string]int // encoder name -> priority (lower is better)
}

// NewPrioritySelector creates a new PrioritySelector.
func NewPrioritySelector(validationManager *valmanager.Manager, priorities map[string]int) *PrioritySelector {
	return &PrioritySelector{
		DefaultSelector: NewDefaultSelector(validationManager),
		priorities:      priorities,
	}
}

// SelectEncoder chooses encoder based on custom priorities.
func (s *PrioritySelector) SelectEncoder(codecType CodecType, inputFormat string, qualityParams *types.QualityParams, encoderOverride string) (*ffmpeg.Params, error) {
	if encoderOverride != "" {
		return s.DefaultSelector.SelectEncoder(codecType, inputFormat, qualityParams, encoderOverride)
	}

	validationResults := s.validationManager.GetValidation()
	if validationResults == nil {
		return s.DefaultSelector.SelectEncoder(codecType, inputFormat, qualityParams, "")
	}

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

	bestEncoder := ""
	bestPriority := int(^uint(0) >> 1)

	for _, encoder := range workingEncoders {
		priority, hasPriority := s.priorities[encoder]
		if !hasPriority {
			priority = 1000
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
		params.HWBackend = hwBackendForEncoder(params.Encoder)

		s.logger.Info("Selected encoder based on priority", "codec_type", codecType, "encoder", bestEncoder, "priority", bestPriority)
		return params, nil
	}

	return s.DefaultSelector.SelectEncoder(codecType, inputFormat, qualityParams, "")
}
