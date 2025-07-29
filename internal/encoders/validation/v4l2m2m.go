package validation

import (
	"fmt"
	"strings"
)

// V4l2m2mValidator validates V4L2 Memory-to-Memory encoders
type V4l2m2mValidator struct{}

// NewV4l2m2mValidator creates a new V4L2 M2M validator
func NewV4l2m2mValidator() *V4l2m2mValidator {
	return &V4l2m2mValidator{}
}

// CanValidate returns true if this validator can handle the given encoder name
func (v *V4l2m2mValidator) CanValidate(encoderName string) bool {
	return strings.Contains(encoderName, "v4l2m2m")
}

// Validate tests the V4L2 M2M encoder using production settings
func (v *V4l2m2mValidator) Validate(encoderName string) (bool, error) {
	return ValidateEncoderWithSettings(v, encoderName)
}

// GetEncoderNames returns the list of V4L2 M2M encoder names
func (v *V4l2m2mValidator) GetEncoderNames() []string {
	return []string{
		"h264_v4l2m2m",
		"hevc_v4l2m2m",
		"mpeg4_v4l2m2m",
		"vp8_v4l2m2m",
		"vp9_v4l2m2m",
	}
}

// GetDescription returns a description of this validator
func (v *V4l2m2mValidator) GetDescription() string {
	return "V4L2 Memory-to-Memory - Hardware acceleration on ARM/embedded devices"
}

// GetProductionSettings returns production settings for V4L2 M2M encoders
func (v *V4l2m2mValidator) GetProductionSettings(encoderName string) (*EncoderSettings, error) {
	if !v.CanValidate(encoderName) {
		return nil, fmt.Errorf("encoder %s is not supported by V4L2 M2M validator", encoderName)
	}

	return &EncoderSettings{
		GlobalArgs: []string{}, // V4L2 M2M doesn't need global args
		OutputParams: map[string]string{
			"num_output_buffers":  "32",
			"num_capture_buffers": "16",
			"b:v":                 "1M", // Set bitrate for V4L2 M2M
		},
		VideoFilters: "", // V4L2 M2M doesn't need video filters
	}, nil
}
