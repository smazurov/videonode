package validation

import (
	"fmt"
	"strings"

	"github.com/smazurov/videonode/internal/types"
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
		"vp8_rkmpp",
		"mjpeg_rkmpp",
	}
}

// GetDescription returns a description of this validator
func (v *RkmppValidator) GetDescription() string {
	return "RKMPP (Rockchip Media Process Platform) - Hardware acceleration for Rockchip SoCs"
}

// GetProductionSettings returns production settings for RKMPP encoders
func (v *RkmppValidator) GetProductionSettings(encoderName string, inputFormat string) (*EncoderSettings, error) {
	if !v.CanValidate(encoderName) {
		return nil, fmt.Errorf("encoder %s is not supported by RKMPP validator", encoderName)
	}

	// Determine if it's MJPEG encoder
	if strings.Contains(encoderName, "mjpeg") {
		return &EncoderSettings{
			GlobalArgs:   []string{},
			OutputParams: map[string]string{},
			VideoFilters: "format=nv12", // MJPEG encoder needs nv12
		}, nil
	}

	// H264/H265 encoder settings
	settings := &EncoderSettings{
		GlobalArgs: []string{},
		OutputParams: map[string]string{
			"rc_mode": "VBR",
			"g":       "20",
			"bf":      "0",
		},
	}

	// RKMPP is more flexible than VAAPI - it accepts many formats directly
	// Use hardware decode for compressed formats, format conversion for raw formats
	switch inputFormat {
	case "mjpeg":
		// Use hardware MJPEG decode for best performance
		// This keeps everything in hardware
		settings.GlobalArgs = append(settings.GlobalArgs, "-hwaccel", "rkmpp", "-hwaccel_output_format", "drm_prime")
		settings.VideoFilters = "" // No filter needed with HW decode
	case "h264":
		// For H264 input, use hardware decode for best performance
		// This keeps everything in hardware
		settings.GlobalArgs = append(settings.GlobalArgs, "-hwaccel", "rkmpp", "-hwaccel_output_format", "drm_prime")
		settings.VideoFilters = "" // No filter needed with HW decode
	case "yuyv422", "yuvj422":
		// Use RGA hardware scaler for format conversion (much faster than CPU)
		// Need to initialize hardware device and upload frames to hardware memory
		settings.GlobalArgs = append(settings.GlobalArgs, "-init_hw_device", "rkmpp=hw", "-filter_hw_device", "hw")
		settings.VideoFilters = "hwupload,scale_rkrga=format=nv12:afbc=0"
	case "bgr24", "rgb24", "nv24", "nv16":
		// RKMPP encoders support these formats directly according to ffmpeg-rockchip wiki!
		// No conversion needed for modern RKMPP
		settings.VideoFilters = ""
		// Could optionally use RGA for AFBC compression to save bandwidth:
		// settings.GlobalArgs = append(settings.GlobalArgs, "-init_hw_device", "rkmpp=hw", "-filter_hw_device", "hw")
		// settings.VideoFilters = "hwupload,scale_rkrga=format=nv12:afbc=1"
	case "":
		// Empty input format means validation mode or unknown format
		settings.VideoFilters = ""
	default:
		// Unknown format - convert to nv12 which RKMPP handles well
		settings.VideoFilters = "format=nv12"
	}

	return settings, nil
}

// GetQualityParams translates quality settings to RKMPP encoder parameters
func (v *RkmppValidator) GetQualityParams(encoderName string, params *types.QualityParams) (EncoderParams, error) {
	if !v.CanValidate(encoderName) {
		return nil, fmt.Errorf("encoder %s is not supported by RKMPP validator", encoderName)
	}

	result := make(EncoderParams)

	// RKMPP rate control modes:
	// 0 = VBR, 1 = CBR, 2 = CQP, 3 = AVBR
	switch params.Mode {
	case types.RateControlCBR:
		result["rc_mode"] = "CBR"
		if params.TargetBitrate != nil {
			result["b:v"] = fmt.Sprintf("%.1fM", *params.TargetBitrate)
		}
		// RKMPP doesn't use bufsize for CBR

	case types.RateControlVBR:
		result["rc_mode"] = "VBR"
		if params.TargetBitrate != nil {
			result["b:v"] = fmt.Sprintf("%.1fM", *params.TargetBitrate)
		}
		if params.MinBitrate != nil {
			result["minrate"] = fmt.Sprintf("%.1fM", *params.MinBitrate)
		}
		if params.MaxBitrate != nil {
			result["maxrate"] = fmt.Sprintf("%.1fM", *params.MaxBitrate)
		}

	case types.RateControlCQP:
		result["rc_mode"] = "CQP"
		if params.Quality != nil {
			// RKMPP uses qp_init for constant QP mode
			result["qp_init"] = fmt.Sprintf("%d", *params.Quality)
		}

	case types.RateControlCRF:
		// RKMPP doesn't support CRF, but we can use CQP as an alternative
		result["rc_mode"] = "CQP"
		if params.Quality != nil {
			// Map CRF values to QP (both use 0-51 range)
			result["qp_init"] = fmt.Sprintf("%d", *params.Quality)
		}

	default:
		return nil, fmt.Errorf("unsupported rate control mode %s for RKMPP", params.Mode)
	}

	// Add common parameters
	if params.KeyframeInterval != nil {
		result["g"] = fmt.Sprintf("%d", *params.KeyframeInterval)
	}

	// RKMPP doesn't use B-frames parameter directly in the same way
	// It's controlled via encoder profile settings

	return result, nil
}
