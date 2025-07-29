package validation

import (
	"fmt"
	"strings"
)

// NvencValidator validates NVIDIA NVENC encoders
type NvencValidator struct{}

// NewNvencValidator creates a new NVENC validator
func NewNvencValidator() *NvencValidator {
	return &NvencValidator{}
}

// CanValidate returns true if this validator can handle the given encoder name
func (v *NvencValidator) CanValidate(encoderName string) bool {
	return strings.Contains(encoderName, "nvenc")
}

// Validate tests the NVENC encoder using production settings
func (v *NvencValidator) Validate(encoderName string) (bool, error) {
	return ValidateEncoderWithSettings(v, encoderName)
}

// GetEncoderNames returns the list of NVENC encoder names
func (v *NvencValidator) GetEncoderNames() []string {
	return []string{
		"h264_nvenc",
		"hevc_nvenc",
		"av1_nvenc",
	}
}

// GetDescription returns a description of this validator
func (v *NvencValidator) GetDescription() string {
	return "NVIDIA NVENC - Hardware acceleration on NVIDIA GPUs"
}

// GetProductionSettings returns production settings for NVENC encoders
func (v *NvencValidator) GetProductionSettings(encoderName string) (*EncoderSettings, error) {
	if !v.CanValidate(encoderName) {
		return nil, fmt.Errorf("encoder %s is not supported by NVENC validator", encoderName)
	}

	return &EncoderSettings{
		GlobalArgs: []string{}, // NVENC doesn't need global args
		OutputParams: map[string]string{
			"preset": "fast",
			"cq":     "20", // Use CQ instead of QP for NVENC
		},
		VideoFilters: "", // NVENC doesn't need video filters
	}, nil
}
