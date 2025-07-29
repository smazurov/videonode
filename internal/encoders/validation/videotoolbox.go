package validation

import (
	"fmt"
	"strings"
)

// VideoToolboxValidator validates macOS VideoToolbox encoders
type VideoToolboxValidator struct{}

// NewVideoToolboxValidator creates a new VideoToolbox validator
func NewVideoToolboxValidator() *VideoToolboxValidator {
	return &VideoToolboxValidator{}
}

// CanValidate returns true if this validator can handle the given encoder name
func (v *VideoToolboxValidator) CanValidate(encoderName string) bool {
	return strings.Contains(encoderName, "videotoolbox")
}

// Validate tests the VideoToolbox encoder using production settings
func (v *VideoToolboxValidator) Validate(encoderName string) (bool, error) {
	return ValidateEncoderWithSettings(v, encoderName)
}

// GetEncoderNames returns the list of VideoToolbox encoder names
func (v *VideoToolboxValidator) GetEncoderNames() []string {
	return []string{
		"h264_videotoolbox",
		"hevc_videotoolbox",
		"prores_videotoolbox",
	}
}

// GetDescription returns a description of this validator
func (v *VideoToolboxValidator) GetDescription() string {
	return "Apple VideoToolbox - Hardware acceleration on macOS"
}

// GetProductionSettings returns production settings for VideoToolbox encoders
func (v *VideoToolboxValidator) GetProductionSettings(encoderName string) (*EncoderSettings, error) {
	if !v.CanValidate(encoderName) {
		return nil, fmt.Errorf("encoder %s is not supported by VideoToolbox validator", encoderName)
	}

	return &EncoderSettings{
		GlobalArgs: []string{}, // VideoToolbox doesn't need global args
		OutputParams: map[string]string{
			"allow_sw": "1",
			"realtime": "0",
			"q:v":      "20", // Quality setting for VideoToolbox
		},
		VideoFilters: "", // VideoToolbox doesn't need video filters
	}, nil
}
