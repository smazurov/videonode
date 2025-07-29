package validation

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
func (v *GenericValidator) GetProductionSettings(encoderName string) (*EncoderSettings, error) {
	// Provide encoder-specific settings for known software encoders
	switch encoderName {
	case "libx264":
		return &EncoderSettings{
			GlobalArgs: []string{},
			OutputParams: map[string]string{
				"crf":    "18",        // High quality constant rate factor
				"preset": "ultrafast", // Fast encoding for streaming
			},
			VideoFilters: "",
		}, nil

	case "libx265":
		return &EncoderSettings{
			GlobalArgs: []string{},
			OutputParams: map[string]string{
				"crf":    "20",        // Slightly higher CRF for H.265 efficiency
				"preset": "ultrafast", // Fast encoding
			},
			VideoFilters: "",
		}, nil

	case "libvpx":
		return &EncoderSettings{
			GlobalArgs: []string{},
			OutputParams: map[string]string{
				"b:v":      "1M", // VP8 works better with bitrate
				"cpu-used": "8",  // Fastest VP8 encoding
			},
			VideoFilters: "",
		}, nil

	case "libvpx-vp9":
		return &EncoderSettings{
			GlobalArgs: []string{},
			OutputParams: map[string]string{
				"b:v":      "1M", // VP9 bitrate
				"cpu-used": "8",  // Fastest VP9 encoding
			},
			VideoFilters: "",
		}, nil

	case "mpeg4":
		return &EncoderSettings{
			GlobalArgs: []string{},
			OutputParams: map[string]string{
				"b:v": "1M", // MPEG-4 bitrate
				"q":   "5",  // Good quality
			},
			VideoFilters: "",
		}, nil

	case "libxvid":
		return &EncoderSettings{
			GlobalArgs: []string{},
			OutputParams: map[string]string{
				"b:v": "1M", // Xvid bitrate
				"q":   "5",  // Good quality
			},
			VideoFilters: "",
		}, nil

	default:
		// Fallback for any unknown encoders
		return &EncoderSettings{
			GlobalArgs: []string{},
			OutputParams: map[string]string{
				"b:v": "1M", // Generic bitrate setting
			},
			VideoFilters: "",
		}, nil
	}
}
