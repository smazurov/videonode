package validation

import (
	"fmt"
	"strings"
)

// VaapiValidator validates VAAPI encoders
type VaapiValidator struct{}

// NewVaapiValidator creates a new VAAPI validator
func NewVaapiValidator() *VaapiValidator {
	return &VaapiValidator{}
}

// CanValidate returns true if this validator can handle the given encoder name
func (v *VaapiValidator) CanValidate(encoderName string) bool {
	return strings.Contains(encoderName, "vaapi")
}

// Validate tests the VAAPI encoder using production settings
func (v *VaapiValidator) Validate(encoderName string) (bool, error) {
	return ValidateEncoderWithSettings(v, encoderName)
}

// GetEncoderNames returns the list of VAAPI encoder names
func (v *VaapiValidator) GetEncoderNames() []string {
	return []string{
		"h264_vaapi",
		"hevc_vaapi",
		"mpeg2_vaapi",
		"vp8_vaapi",
		"vp9_vaapi",
		"av1_vaapi",
	}
}

// GetDescription returns a description of this validator
func (v *VaapiValidator) GetDescription() string {
	return "VAAPI (Video Acceleration API) - Intel/AMD hardware acceleration on Linux"
}

// GetProductionSettings returns production settings for VAAPI encoders
func (v *VaapiValidator) GetProductionSettings(encoderName string) (*EncoderSettings, error) {
	if !v.CanValidate(encoderName) {
		return nil, fmt.Errorf("encoder %s is not supported by VAAPI validator", encoderName)
	}

	return &EncoderSettings{
		GlobalArgs: []string{"-vaapi_device", "/dev/dri/renderD128"},
		OutputParams: map[string]string{
			"qp": "20",
			"bf": "0", // Disable B-frames for WebRTC compatibility
		},
		VideoFilters: "format=nv12,hwupload",
	}, nil
}
