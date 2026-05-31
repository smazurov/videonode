package validation

import (
	"fmt"
	"os"
	"strings"

	"github.com/smazurov/videonode/internal/types"
)

// RkmppValidator validates Rockchip MPP encoders.
type RkmppValidator struct{}

// NewRkmppValidator creates a new RKMPP validator.
func NewRkmppValidator() *RkmppValidator {
	return &RkmppValidator{}
}

// CanValidate returns true if this validator can handle the given encoder name.
func (v *RkmppValidator) CanValidate(encoderName string) bool {
	return strings.Contains(encoderName, "rkmpp")
}

// Validate tests the RKMPP encoder using production settings.
func (v *RkmppValidator) Validate(encoderName string) (bool, error) {
	return ValidateEncoderWithSettings(v, encoderName)
}

// GetEncoderNames returns the list of RKMPP encoder names.
func (v *RkmppValidator) GetEncoderNames() []string {
	return []string{
		"h264_rkmpp",
		"hevc_rkmpp",
		"vp8_rkmpp",
		"mjpeg_rkmpp",
	}
}

// GetDescription returns a description of this validator.
func (v *RkmppValidator) GetDescription() string {
	return "RKMPP (Rockchip Media Process Platform) - Hardware acceleration for Rockchip SoCs"
}

// GetBackendName returns the canonical backend tag.
func (v *RkmppValidator) GetBackendName() string { return "rkmpp" }

// rkmppDeviceAvailable reports whether the Rockchip MPP service is present.
func rkmppDeviceAvailable() bool {
	_, err := os.Stat("/proc/mpp_service/load")
	return err == nil
}

// ValidateDecoders probes RKMPP HW decode for each canvas-relevant codec.
func (v *RkmppValidator) ValidateDecoders(logger Logger) (working, failed []string) {
	if !rkmppDeviceAvailable() {
		return nil, nil
	}
	for _, codec := range []string{"h264", "hevc", "mjpeg"} {
		// rkmpp's MJPEG decoder rejects yuvj422p; only 4:2:0 is accepted.
		pixFmt := ""
		if codec == "mjpeg" {
			pixFmt = "yuvj420p"
		}
		probe := buildDecoderProbe("rkmpp", "drm_prime", codec, pixFmt)
		ok, stderr := runProbe(probe)
		if ok {
			working = append(working, codec)
			continue
		}
		failed = append(failed, codec)
		if logger != nil && stderr != "" {
			logger.Printf("rkmpp decode %s: %s", codec, stderr)
		}
	}
	return working, failed
}

// ValidateFilters probes each rkrga filter the canvas builder emits.
func (v *RkmppValidator) ValidateFilters(logger Logger) (working, failed []string) {
	if !rkmppDeviceAvailable() {
		return nil, nil
	}
	probes := []Probe{
		rkmppScaleProbe(),
		rkmppScaleBGRAProbe(),
		rkmppTransposeProbe(),
		rkmppOverlayProbe(),
	}
	for _, p := range probes {
		ok, stderr := runProbe(p)
		if ok {
			working = append(working, p.Name)
			continue
		}
		failed = append(failed, p.Name)
		if logger != nil && stderr != "" {
			logger.Printf("rkmpp filter %s: %s", p.Name, stderr)
		}
	}
	return working, failed
}

// rkmppFilterProbe pipes MJPEG through rkmpp hwaccel decode into the given filter chain.
func rkmppFilterProbe(name, chain string, extraInputs ...string) Probe {
	args := make([]string, 0, 16+len(extraInputs)+9)
	args = append(args,
		"-hide_banner", "-nostats", "-loglevel", "error",
		"-init_hw_device", "rkmpp=hw", "-filter_hw_device", "hw",
		"-hwaccel", "rkmpp", "-hwaccel_output_format", "drm_prime",
		"-f", "mjpeg", "-i", "-",
	)
	args = append(args, extraInputs...)
	args = append(args,
		"-filter_complex", chain,
		"-map", "[v]", "-frames:v", "5", "-f", "null", "-",
	)
	return Probe{
		Name:         name,
		Args:         args,
		ProducerArgs: mjpegProducerArgs("yuvj420p"),
	}
}

func rkmppScaleProbe() Probe {
	return rkmppFilterProbe(
		"scale_rkrga",
		"[0:v]scale_rkrga=160:120:format=nv12,hwdownload,format=nv12[v]",
	)
}

// rkmppScaleBGRAProbe verifies scale_rkrga can output BGRA in one pass (BGRA-overlay path).
func rkmppScaleBGRAProbe() Probe {
	return rkmppFilterProbe(
		"scale_rkrga_bgra",
		"[0:v]scale_rkrga=160:120:format=bgra,hwdownload,format=bgra[v]",
	)
}

func rkmppTransposeProbe() Probe {
	return rkmppFilterProbe(
		"vpp_rkrga",
		"[0:v]vpp_rkrga=transpose=clock,hwdownload,format=nv12[v]",
	)
}

// rkmppOverlayProbe exercises overlay_rkrga in production shape: YUV-main + BGRA-overlay → YUV-out.
// RGA's compositor only accepts BGRA-on-YUV (supported_formats_overlay = RGB_FORMATS).
func rkmppOverlayProbe() Probe {
	return rkmppFilterProbe(
		"overlay_rkrga",
		"[1:v]format=nv12,hwupload,scale_rkrga=80:60:format=bgra[layer];"+
			"[0:v][layer]overlay_rkrga=x=10:y=10,hwdownload,format=nv12[v]",
		"-f", "lavfi", "-i", "color=size=80x60:rate=10:duration=0.3",
	)
}

// GetProductionSettings returns production settings for RKMPP encoders.
func (v *RkmppValidator) GetProductionSettings(encoderName string, inputFormat string) (*EncoderSettings, error) {
	if !v.CanValidate(encoderName) {
		return nil, fmt.Errorf("encoder %s is not supported by RKMPP validator", encoderName)
	}

	if strings.Contains(encoderName, "mjpeg") {
		return &EncoderSettings{
			GlobalArgs:   []string{},
			OutputParams: map[string]string{},
			VideoFilters: "format=nv12",
		}, nil
	}

	settings := &EncoderSettings{
		GlobalArgs: []string{},
		OutputParams: map[string]string{
			"rc_mode": "VBR",
			"g":       "20",
			"bf":      "0",
		},
	}

	switch inputFormat {
	case "testsrc":
		settings.VideoFilters = ""
	case "mjpeg", "h264":
		settings.GlobalArgs = append(settings.GlobalArgs, "-hwaccel", "rkmpp", "-hwaccel_output_format", "drm_prime")
		settings.VideoFilters = ""
	case "yuyv422", "yuvj422":
		settings.GlobalArgs = append(settings.GlobalArgs, "-init_hw_device", "rkmpp=hw", "-filter_hw_device", "hw")
		settings.VideoFilters = "hwupload,scale_rkrga=format=nv12:afbc=0"
	case "bgr24", "rgb24", "nv24", "nv16",
		"bgra", "rgba", "argb", "abgr", "bgr0", "rgb0", "0rgb", "0bgr", "":
		// rkmpp ingests these natively and converts in hardware; a software
		// `format=nv12` filter here would burn CPU on every frame (e.g. BGRA
		// composer canvas at 1080p60).
		settings.VideoFilters = ""
	default:
		settings.VideoFilters = "format=nv12"
	}

	return settings, nil
}

// GetQualityParams translates quality settings to RKMPP encoder parameters.
func (v *RkmppValidator) GetQualityParams(encoderName string, params *types.QualityParams) (EncoderParams, error) {
	if !v.CanValidate(encoderName) {
		return nil, fmt.Errorf("encoder %s is not supported by RKMPP validator", encoderName)
	}

	result := make(EncoderParams)

	switch params.Mode {
	case types.RateControlCBR:
		result["rc_mode"] = "CBR"
		if params.TargetBitrate != nil {
			result["b:v"] = fmt.Sprintf("%.1fM", *params.TargetBitrate)
		}

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

	case types.RateControlCQP, types.RateControlCRF:
		// RKMPP has no CRF; CQP carries both modes via qp_init.
		result["rc_mode"] = "CQP"
		if params.Quality != nil {
			result["qp_init"] = fmt.Sprintf("%d", *params.Quality)
		}

	default:
		return nil, fmt.Errorf("unsupported rate control mode %s for RKMPP", params.Mode)
	}

	if params.KeyframeInterval != nil {
		result["g"] = fmt.Sprintf("%d", *params.KeyframeInterval)
	}

	return result, nil
}
