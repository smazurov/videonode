package validation

import (
	"fmt"
	"strings"
)

// QsvValidator validates Intel QSV encoders
type QsvValidator struct{}

// NewQsvValidator creates a new QSV validator
func NewQsvValidator() *QsvValidator {
	return &QsvValidator{}
}

// CanValidate returns true if this validator can handle the given encoder name
func (v *QsvValidator) CanValidate(encoderName string) bool {
	return strings.Contains(encoderName, "qsv")
}

// Validate tests the QSV encoder using production settings
func (v *QsvValidator) Validate(encoderName string) (bool, error) {
	return ValidateEncoderWithSettings(v, encoderName)
}

// GetEncoderNames returns the list of QSV encoder names
func (v *QsvValidator) GetEncoderNames() []string {
	return []string{
		"h264_qsv",
		"hevc_qsv",
		"mpeg2_qsv",
		"vp9_qsv",
		"av1_qsv",
	}
}

// GetDescription returns a description of this validator
func (v *QsvValidator) GetDescription() string {
	return "Intel Quick Sync Video (QSV) - Hardware acceleration on Intel CPUs/GPUs"
}

// GetProductionSettings returns production settings for QSV encoders
func (v *QsvValidator) GetProductionSettings(encoderName string) (*EncoderSettings, error) {
	if !v.CanValidate(encoderName) {
		return nil, fmt.Errorf("encoder %s is not supported by QSV validator", encoderName)
	}

	return &EncoderSettings{
		GlobalArgs: []string{"-init_hw_device", "qsv=hw", "-filter_hw_device", "hw"},
		OutputParams: map[string]string{
			"preset":         "medium",
			"global_quality": "20", // Use global_quality for QSV
		},
		VideoFilters: "hwupload=extra_hw_frames=64,format=qsv",
	}, nil
}
