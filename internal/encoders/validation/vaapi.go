package validation

import (
	"fmt"
	"strings"

	"github.com/smazurov/videonode/internal/types"
)

// VaapiValidator validates VAAPI encoders.
type VaapiValidator struct{}

// NewVaapiValidator creates a new VAAPI validator.
func NewVaapiValidator() *VaapiValidator {
	return &VaapiValidator{}
}

// CanValidate returns true if this validator can handle the given encoder name.
func (v *VaapiValidator) CanValidate(encoderName string) bool {
	return strings.Contains(encoderName, "vaapi")
}

// Validate tests the VAAPI encoder using production settings.
func (v *VaapiValidator) Validate(encoderName string) (bool, error) {
	return ValidateEncoderWithSettings(v, encoderName)
}

// GetEncoderNames returns the list of VAAPI encoder names.
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

// GetDescription returns a description of this validator.
func (v *VaapiValidator) GetDescription() string {
	return "VAAPI (Video Acceleration API) - Intel/AMD hardware acceleration on Linux"
}

// GetProductionSettings returns production settings for VAAPI encoders.
func (v *VaapiValidator) GetProductionSettings(encoderName string, inputFormat string) (*EncoderSettings, error) {
	if !v.CanValidate(encoderName) {
		return nil, fmt.Errorf("encoder %s is not supported by VAAPI validator", encoderName)
	}

	settings := &EncoderSettings{
		GlobalArgs: []string{"-vaapi_device", "/dev/dri/renderD128"},
		OutputParams: map[string]string{
			"qp": "20",
			"bf": "0", // Disable B-frames for WebRTC compatibility
		},
	}

	// Set video filters based on input format
	// VAAPI requires nv12 format for hardware encoding
	switch inputFormat {
	case "testsrc":
		// Test sources output yuv420p by default, just need hwupload for VAAPI
		// Minimal filters to avoid format negotiation issues
		settings.VideoFilters = "format=nv12,hwupload"
	case "mjpeg":
		// MJPEG decodes to yuvj422p (full-range 4:2:2)
		// Need to convert: yuvj422p -> yuvj420p -> yuv420p -> nv12 -> hwupload
		settings.VideoFilters = "format=yuvj420p,format=yuv420p,format=nv12,hwupload"
	case "h264":
		// H.264 decodes to yuvj420p (full-range 4:2:0)
		// Need to convert: yuvj420p -> yuv420p -> nv12 -> hwupload
		settings.VideoFilters = "format=yuv420p,format=nv12,hwupload"
	case "yuyv422", "yuvj422":
		// Raw YUV 4:2:2 formats
		// Need to convert: yuyv422 -> yuv422p -> yuv420p -> nv12 -> hwupload
		settings.VideoFilters = "format=yuv422p,format=yuv420p,format=nv12,hwupload"
	case "bgr24", "rgb24":
		// RGB formats need color space conversion then format conversion
		// BGR/RGB -> YUV420p -> NV12 -> hwupload
		settings.VideoFilters = "format=yuv420p,format=nv12,hwupload"
	case "nv24":
		// 4:4:4 YUV needs chroma downsampling only
		// NV24 -> NV12 -> hwupload
		settings.VideoFilters = "format=nv12,hwupload"
	case "nv16":
		// 4:2:2 YUV needs chroma downsampling only
		// NV16 -> NV12 -> hwupload
		settings.VideoFilters = "format=nv12,hwupload"
	case "":
		// Empty input format means we're in validation mode or don't know the format
		// Use the safest default filter chain
		settings.VideoFilters = "format=nv12,hwupload"
	default:
		// Unknown format - use safe conversion path
		settings.VideoFilters = "format=yuv420p,format=nv12,hwupload"
	}

	return settings, nil
}

// GetQualityParams translates quality settings to VAAPI-specific encoder parameters.
func (v *VaapiValidator) GetQualityParams(encoderName string, params *types.QualityParams) (EncoderParams, error) {
	if !v.CanValidate(encoderName) {
		return nil, fmt.Errorf("encoder %s is not supported by VAAPI validator", encoderName)
	}

	result := make(EncoderParams)

	switch params.Mode {
	case types.RateControlCBR:
		result["rc_mode"] = "CBR"
		if params.TargetBitrate != nil {
			result["b:v"] = fmt.Sprintf("%.1fM", *params.TargetBitrate)
		}
		if params.BufferSize != nil {
			result["bufsize"] = fmt.Sprintf("%.1fM", *params.BufferSize)
		}

	case types.RateControlVBR:
		result["rc_mode"] = "VBR"
		if params.TargetBitrate != nil {
			result["b:v"] = fmt.Sprintf("%.1fM", *params.TargetBitrate)
		}
		if params.MaxBitrate != nil {
			result["maxrate"] = fmt.Sprintf("%.1fM", *params.MaxBitrate)
		}
		if params.BufferSize != nil {
			result["bufsize"] = fmt.Sprintf("%.1fM", *params.BufferSize)
		}

	case types.RateControlCQP:
		result["rc_mode"] = "CQP"
		if params.Quality != nil {
			result["qp"] = fmt.Sprintf("%d", *params.Quality)
		}

	case types.RateControlCRF:
		return nil, fmt.Errorf("VAAPI does not support CRF mode, use CQP instead")

	default:
		return nil, fmt.Errorf("unsupported rate control mode %s for VAAPI", params.Mode)
	}

	// Add common parameters
	if params.BFrames != nil {
		result["bf"] = fmt.Sprintf("%d", *params.BFrames)
	}
	if params.KeyframeInterval != nil {
		result["g"] = fmt.Sprintf("%d", *params.KeyframeInterval)
	}

	return result, nil
}
