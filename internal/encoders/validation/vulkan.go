package validation

import (
	"fmt"
	"strings"
)

// VulkanValidator validates Vulkan video encoders
type VulkanValidator struct{}

// NewVulkanValidator creates a new Vulkan validator
func NewVulkanValidator() *VulkanValidator {
	return &VulkanValidator{}
}

// CanValidate returns true if this validator can handle the given encoder name
func (v *VulkanValidator) CanValidate(encoderName string) bool {
	return strings.Contains(encoderName, "vulkan")
}

// Validate tests the Vulkan encoder using production settings
func (v *VulkanValidator) Validate(encoderName string) (bool, error) {
	return ValidateEncoderWithSettings(v, encoderName)
}

// GetEncoderNames returns the list of Vulkan encoder names
func (v *VulkanValidator) GetEncoderNames() []string {
	return []string{
		"h264_vulkan",
		"hevc_vulkan",
	}
}

// GetDescription returns a description of this validator
func (v *VulkanValidator) GetDescription() string {
	return "Vulkan Video - Cross-platform GPU acceleration via Vulkan API"
}

// GetProductionSettings returns production settings for Vulkan encoders
// Optimized for highest quality WebRTC-compatible streaming
// Note: Vulkan video encoding support varies by GPU driver. AMD RADV drivers may have issues.
// NVIDIA and Intel GPUs with Vulkan Video support should work better.
func (v *VulkanValidator) GetProductionSettings(encoderName string) (*EncoderSettings, error) {
	if !v.CanValidate(encoderName) {
		return nil, fmt.Errorf("encoder %s is not supported by Vulkan validator", encoderName)
	}

	settings := &EncoderSettings{
		GlobalArgs:   []string{"-init_hw_device", "vulkan"},
		VideoFilters: "format=nv12,hwupload",
	}

	switch encoderName {
	case "h264_vulkan":
		// WebRTC-compatible H.264 settings optimized for highest quality
		// Simplified settings due to driver compatibility issues
		settings.OutputParams = map[string]string{
			"qp": "18", // Lower QP for highest quality (range: 0-51, lower=better)
			"g":  "60", // GOP size ~2 seconds at 30fps
		}
	case "hevc_vulkan":
		// HEVC settings optimized for highest quality streaming
		// Simplified settings due to driver compatibility issues
		settings.OutputParams = map[string]string{
			"qp": "20", // HEVC needs slightly higher QP than H.264 for same quality
			"g":  "60", // GOP size
		}
	default:
		return nil, fmt.Errorf("unknown Vulkan encoder: %s", encoderName)
	}

	return settings, nil
}
