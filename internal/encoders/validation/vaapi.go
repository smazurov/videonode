package validation

import (
	"fmt"
	"os"
	"strings"

	"github.com/smazurov/videonode/internal/types"
)

// VaapiDevicePath matches the validator's GlobalArgs so probes hit the production device.
const VaapiDevicePath = "/dev/dri/renderD128"

func vaapiDeviceAvailable() bool {
	_, err := os.Stat(VaapiDevicePath)
	return err == nil
}

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

// GetBackendName returns the canonical backend tag.
func (v *VaapiValidator) GetBackendName() string { return "vaapi" }

// ValidateDecoders probes VAAPI HW decode for each canvas-relevant codec.
func (v *VaapiValidator) ValidateDecoders(logger Logger) (working, failed []string) {
	if !vaapiDeviceAvailable() {
		return nil, nil
	}
	for _, codec := range []string{"h264", "hevc", "mjpeg"} {
		// 4:2:2 mjpeg catches radeonsi's VPP gap.
		pixFmt := ""
		if codec == "mjpeg" {
			pixFmt = "yuvj422p"
		}
		probe := buildDecoderProbe("vaapi", "vaapi", codec, pixFmt)
		probe.Args = append([]string{"-vaapi_device", VaapiDevicePath}, probe.Args...)
		ok, stderr := runProbe(probe)
		if ok {
			working = append(working, codec)
			continue
		}
		failed = append(failed, codec)
		if logger != nil && stderr != "" {
			logger.Printf("vaapi decode %s: %s", codec, stderr)
		}
	}
	return working, failed
}

// ValidateFilters probes each VAAPI VPP filter; catches drivers (radeonsi) lacking VPP at init.
func (v *VaapiValidator) ValidateFilters(logger Logger) (working, failed []string) {
	if !vaapiDeviceAvailable() {
		return nil, nil
	}
	probes := []Probe{
		vaapiScaleProbe(),
		vaapiScaleBGRAProbe(),
		vaapiTransposeProbe(),
		vaapiOverlayProbe(),
	}
	for _, p := range probes {
		ok, stderr := runProbe(p)
		if ok {
			working = append(working, p.Name)
			continue
		}
		failed = append(failed, p.Name)
		if logger != nil && stderr != "" {
			logger.Printf("vaapi filter %s: %s", p.Name, stderr)
		}
	}

	if isEncoderCompiled("hevc_vaapi") {
		ok, stderr := runHevcAlignmentProbe(vaapiHevcAlignmentBugProbe())
		if ok {
			working = append(working, "hevc_alignment")
		} else {
			failed = append(failed, "hevc_alignment")
			if logger != nil && stderr != "" {
				logger.Printf("vaapi hevc_alignment: %s", stderr)
			}
		}
	}

	return working, failed
}

// vaapiHevcAlignmentBugProbe encodes red at 320x72 (h%16=8) and decodes the bottom row to rgb24.
func vaapiHevcAlignmentBugProbe() Probe {
	return Probe{
		Name: "hevc_alignment",
		ProducerArgs: []string{
			"-hide_banner", "-nostats", "-loglevel", "error",
			"-vaapi_device", VaapiDevicePath,
			"-f", "lavfi", "-i", "color=c=red:s=320x72:r=10:duration=0.3",
			"-vf", "format=nv12,hwupload",
			"-c:v", "hevc_vaapi", "-bf", "0",
			"-f", "hevc", "-",
		},
		Args: []string{
			"-hide_banner", "-nostats", "-loglevel", "error",
			"-f", "hevc", "-i", "-",
			"-vf", "crop=1:1:160:71",
			"-frames:v", "1", "-pix_fmt", "rgb24", "-f", "rawvideo", "-",
		},
	}
}

// mjpegProducerArgs returns producer args emitting MJPEG with the given chroma to stdout.
// VAAPI uses yuvj422p (catches radeonsi VPP gap); rkmpp uses yuvj420p (its decoder rejects 4:2:2).
func mjpegProducerArgs(pixFmt string) []string {
	return []string{
		"-hide_banner", "-nostats", "-loglevel", "error",
		"-f", "lavfi", "-i", "testsrc2=size=320x240:rate=10:duration=0.3",
		"-c:v", "mjpeg", "-pix_fmt", pixFmt, "-f", "mjpeg", "-",
	}
}

// vaapiFilterProbe pipes 4:2:2 MJPEG through VAAPI hwaccel decode into the given filter chain.
// Chain must end at [v] with hwdownload+format=nv12.
func vaapiFilterProbe(name, chain string, extraInputs ...string) Probe {
	args := make([]string, 0, 14+len(extraInputs)+9)
	args = append(args,
		"-hide_banner", "-nostats", "-loglevel", "error",
		"-vaapi_device", VaapiDevicePath,
		"-hwaccel", "vaapi", "-hwaccel_output_format", "vaapi",
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
		ProducerArgs: mjpegProducerArgs("yuvj422p"),
	}
}

func vaapiScaleProbe() Probe {
	return vaapiFilterProbe(
		"scale_vaapi",
		"[0:v]scale_vaapi=w=160:h=120:format=nv12,hwdownload,format=nv12[v]",
	)
}

// vaapiScaleBGRAProbe verifies scale_vaapi can output BGRA in one pass (BGRA-overlay path).
func vaapiScaleBGRAProbe() Probe {
	return vaapiFilterProbe(
		"scale_vaapi_bgra",
		"[0:v]scale_vaapi=w=160:h=120:format=bgra,hwdownload,format=bgra[v]",
	)
}

func vaapiTransposeProbe() Probe {
	return vaapiFilterProbe(
		"transpose_vaapi",
		"[0:v]transpose_vaapi=dir=clock,hwdownload,format=nv12[v]",
	)
}

// vaapiOverlayProbe exercises overlay_vaapi in the unified BGRA-on-YUV → YUV shape.
func vaapiOverlayProbe() Probe {
	return vaapiFilterProbe(
		"overlay_vaapi",
		"[1:v]format=nv12,hwupload,scale_vaapi=w=80:h=60:format=bgra[layer];"+
			"[0:v][layer]overlay_vaapi=x=10:y=10,hwdownload,format=nv12[v]",
		"-f", "lavfi", "-i", "color=size=80x60:rate=10:duration=0.3",
	)
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
			"bf": "0", // disable B-frames for WebRTC
		},
	}

	switch inputFormat {
	case "testsrc":
		settings.VideoFilters = "format=nv12,hwupload"
	case "mjpeg", "h264", "yuvj422":
		settings.GlobalArgs = append(settings.GlobalArgs, "-hwaccel", "vaapi", "-hwaccel_output_format", "vaapi")
		settings.VideoFilters = "scale_vaapi=format=nv12"
	case "yuyv422", "bgr24", "rgb24":
		settings.VideoFilters = "format=yuv420p,format=nv12,hwupload"
	case "nv24", "nv16", "":
		settings.VideoFilters = "format=nv12,hwupload"
	default:
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

	if params.BFrames != nil {
		result["bf"] = fmt.Sprintf("%d", *params.BFrames)
	}
	if params.KeyframeInterval != nil {
		result["g"] = fmt.Sprintf("%d", *params.KeyframeInterval)
	}

	return result, nil
}
