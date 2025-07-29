package validation

import (
	"fmt"
	"strings"
)

// AmfValidator validates AMD AMF encoders
type AmfValidator struct{}

// NewAmfValidator creates a new AMF validator
func NewAmfValidator() *AmfValidator {
	return &AmfValidator{}
}

// CanValidate returns true if this validator can handle the given encoder name
func (v *AmfValidator) CanValidate(encoderName string) bool {
	return strings.Contains(encoderName, "amf")
}

// Validate tests the AMF encoder using production settings
func (v *AmfValidator) Validate(encoderName string) (bool, error) {
	return ValidateEncoderWithSettings(v, encoderName)
}

// GetEncoderNames returns the list of AMF encoder names
func (v *AmfValidator) GetEncoderNames() []string {
	return []string{
		"h264_amf",
		"hevc_amf",
		"av1_amf",
	}
}

// GetDescription returns a description of this validator
func (v *AmfValidator) GetDescription() string {
	return "AMD AMF - Hardware acceleration on AMD GPUs"
}

// GetProductionSettings returns production settings for AMF encoders
func (v *AmfValidator) GetProductionSettings(encoderName string) (*EncoderSettings, error) {
	if !v.CanValidate(encoderName) {
		return nil, fmt.Errorf("encoder %s is not supported by AMF validator", encoderName)
	}

	return &EncoderSettings{
		GlobalArgs: []string{}, // AMF doesn't need global args
		OutputParams: map[string]string{
			"usage":   "transcoding",
			"quality": "balanced",
			"rc":      "cqp", // Constant QP rate control
			"qp":      "20",  // Quality parameter
		},
		VideoFilters: "", // AMF doesn't need video filters
	}, nil
}
