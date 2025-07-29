package validation

import (
	"fmt"
	"strings"
)

// RkmppValidator validates Rockchip MPP encoders
type RkmppValidator struct{}

// NewRkmppValidator creates a new RKMPP validator
func NewRkmppValidator() *RkmppValidator {
	return &RkmppValidator{}
}

// CanValidate returns true if this validator can handle the given encoder name
func (v *RkmppValidator) CanValidate(encoderName string) bool {
	return strings.Contains(encoderName, "rkmpp")
}

// Validate tests the RKMPP encoder using production settings
func (v *RkmppValidator) Validate(encoderName string) (bool, error) {
	return ValidateEncoderWithSettings(v, encoderName)
}

// GetEncoderNames returns the list of RKMPP encoder names
func (v *RkmppValidator) GetEncoderNames() []string {
	return []string{
		"h264_rkmpp",
		"hevc_rkmpp",
	}
}

// GetDescription returns a description of this validator
func (v *RkmppValidator) GetDescription() string {
	return "Rockchip MPP - Hardware acceleration on Rockchip SoCs"
}

// GetProductionSettings returns production settings for RKMPP encoders
func (v *RkmppValidator) GetProductionSettings(encoderName string) (*EncoderSettings, error) {
	if !v.CanValidate(encoderName) {
		return nil, fmt.Errorf("encoder %s is not supported by RKMPP validator", encoderName)
	}

	return &EncoderSettings{
		GlobalArgs: []string{}, // RKMPP doesn't need global args
		OutputParams: map[string]string{
			"quality_min": "10",
			"quality_max": "51",
			"crf":         "20", // Use CRF for quality control
		},
		VideoFilters: "", // RKMPP doesn't need video filters
	}, nil
}
