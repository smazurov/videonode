package ffmpeg

import (
	"fmt"
	"strings"
)

type inputDecodeMode int

const (
	decodeHW inputDecodeMode = iota
	decodeSW
	decodeTestSrc
)

// V4L2 input formats accepted by HW decoders via -hwaccel; raw formats take the SW path.
var hwDecodableFormats = map[string]struct{}{
	"h264":  {},
	"hevc":  {},
	"mjpeg": {},
	"vp8":   {},
	"vp9":   {},
	"av1":   {},
}

func resolveDecodeMode(backend string, in CompositeInput, _ HWCapabilities) inputDecodeMode {
	if in.OverlayText != "" {
		return decodeTestSrc
	}
	if in.Perspective != nil {
		return decodeSW
	}
	if backend == "sw" || backend == "" {
		return decodeSW
	}
	if _, ok := hwDecodableFormats[in.InputFormat]; !ok {
		return decodeSW
	}
	return decodeHW
}

// canKeepEncodeOnHW reports whether the encode chain can stay on HW through slot-scale.
func canKeepEncodeOnHW(backend string, in CompositeInput, caps HWCapabilities) bool {
	if !caps.HasScale(backend) {
		return false
	}
	if in.Rotation != 0 && !caps.HasTranspose(backend) {
		return false
	}
	return true
}

// perInputDecodeArgs returns input-position args (preceding -i); nil for SW/test inputs.
func perInputDecodeArgs(backend string, mode inputDecodeMode) []string {
	if mode != decodeHW {
		return nil
	}
	switch backend {
	case "rkmpp":
		return []string{"-hwaccel", "rkmpp", "-hwaccel_output_format", "drm_prime"}
	case "vaapi":
		return []string{"-hwaccel", "vaapi", "-hwaccel_output_format", "vaapi"}
	}
	return nil
}

// extraGlobalHWArgs supplies -init_hw_device for rkmpp; VAAPI uses validator-supplied -vaapi_device.
func extraGlobalHWArgs(backend string, existing []string) []string {
	if backend != "rkmpp" {
		return nil
	}
	for i := 0; i+1 < len(existing); i++ {
		if existing[i] == "-init_hw_device" {
			return nil
		}
	}
	return []string{"-init_hw_device", "rkmpp=hw", "-filter_hw_device", "hw"}
}

func useHWOverlay(backend string, caps HWCapabilities) bool {
	return caps.HasOverlay(backend)
}

// perInputEncodeChainBGRA emits the per-input chain for the BGRA-overlay-on-YUV path.
// Output is always a HW BGRA surface at slot dims; overlay_rkrga/_vaapi composes onto YUV canvas.
func perInputEncodeChainBGRA(backend string, mode inputDecodeMode, in CompositeInput, caps HWCapabilities, canvasFPS, padColor string) []string {
	hwBGRA := hwScaleFilter(backend, in.Width, in.Height, "bgra")

	switch {
	case mode == decodeHW && canKeepEncodeOnHW(backend, in, caps):
		var fs []string
		if rot := hwTransposeForBackend(backend, in.Rotation); rot != "" {
			fs = append(fs, rot)
		}
		fs = append(fs, hwBGRA)
		return fs

	case mode == decodeHW:
		// Missing HW transpose cap: keep decode on GPU, run SW chain, re-upload as BGRA.
		swSteps := swEncodeChain(in, canvasFPS, padColor)
		fs := make([]string, 0, 4+len(swSteps))
		fs = append(fs, "hwdownload", "format=nv12")
		fs = append(fs, swSteps...)
		fs = append(fs, "format=bgra", "hwupload")
		return fs

	case mode == decodeSW && in.Perspective == nil && in.OverlayText == "" && in.CropW == 0 && in.CropH == 0:
		// Uploadable raw V4L2: skip SW chain; one RGA pass does upload + scale-to-BGRA.
		fs := make([]string, 0, 4)
		fs = append(fs, "format=nv12", "hwupload")
		if rot := hwTransposeForBackend(backend, in.Rotation); rot != "" {
			fs = append(fs, rot)
		}
		fs = append(fs, hwBGRA)
		return fs

	default:
		fs := swEncodeChain(in, canvasFPS, padColor)
		fs = append(fs, "format=bgra", "hwupload")
		return fs
	}
}

// perInputEncodeChain emits the encode-side filter list for one input.
// On decodeHW with a missing filter cap, decode stays on GPU and SW filters run on hwdownloaded NV12.
func perInputEncodeChain(backend string, mode inputDecodeMode, in CompositeInput, caps HWCapabilities, canvasFPS, padColor string) []string {
	switch mode {
	case decodeHW:
		if canKeepEncodeOnHW(backend, in, caps) {
			var fs []string
			if rot := hwTransposeForBackend(backend, in.Rotation); rot != "" {
				fs = append(fs, rot)
			}
			fs = append(fs, hwScaleFilter(backend, in.Width, in.Height, "nv12"))
			return fs
		}
		fs := []string{"hwdownload", "format=nv12"}
		return append(fs, swEncodeChain(in, canvasFPS, padColor)...)
	default:
		return swEncodeChain(in, canvasFPS, padColor)
	}
}

func swEncodeChain(in CompositeInput, canvasFPS, padColor string) []string {
	var fs []string

	fs = append(fs, "fps="+canvasFPS)

	if in.Perspective != nil {
		fs = append(fs, "format=yuv420p")
		fs = append(fs, perspectiveFilterString(in.Perspective))
		natW, natH := perspectiveOutputSize(in.Perspective)
		if natW > 0 && natH > 0 {
			fs = append(fs, fmt.Sprintf("scale=%d:%d", natW, natH))
		}
	}

	if f := swTransposeFilter(in.Rotation); f != "" {
		fs = append(fs, f)
	}

	if in.CropW > 0 && in.CropH > 0 {
		fs = append(fs, fmt.Sprintf("crop=%d:%d:%d:%d", in.CropW, in.CropH, in.CropX, in.CropY))
	}

	if in.OverlayText != "" {
		fs = append(fs, fmt.Sprintf(
			"drawtext=text='%s':x=(w-text_w)/2:y=(h-text_h)/2:fontsize=80:fontcolor=white:box=1:boxcolor=black@0.5:boxborderw=5",
			in.OverlayText))
	}

	color := padColor
	if color == "" {
		color = "0x000000"
	}
	fs = append(fs, fmt.Sprintf(
		"scale=%d:%d:force_original_aspect_ratio=decrease,pad=%d:%d:(ow-iw)/2:(oh-ih)/2:color=%s",
		in.Width, in.Height, in.Width, in.Height, color))

	return fs
}

// perInputVisionChain emits the vision branch ending in system-memory NV12 for the raw-frame pipe.
// Leading setpts rebases V4L2 wallclock; without it, fps filter stalls on huge absolute timestamps.
func perInputVisionChain(backend string, mode inputDecodeMode, in CompositeInput, caps HWCapabilities) []string {
	vw, vh := in.VisionWidth, in.VisionHeight
	if vw <= 0 {
		vw = 640
	}
	if vh <= 0 {
		vh = 480
	}
	fs := []string{"setpts=PTS-STARTPTS"}
	if in.VisionFPS > 0 {
		fs = append(fs, fmt.Sprintf("fps=%d", in.VisionFPS))
	}
	switch {
	case mode == decodeHW && caps.HasScale(backend):
		fs = append(fs, hwScaleFilter(backend, vw, vh, "nv12"))
		fs = append(fs, "hwdownload", "format=nv12")
	case mode == decodeHW:
		fs = append(fs, "hwdownload", "format=nv12")
		fs = append(fs, fmt.Sprintf("scale=%d:%d", vw, vh), "format=nv12")
	default:
		fs = append(fs, fmt.Sprintf("scale=%d:%d", vw, vh), "format=nv12")
	}
	return fs
}

func canvasBaseFilter(canvasMode string, w, h int, fps, color string, padForAlignment bool) string {
	if color == "" {
		color = "0x000000"
	}
	base := fmt.Sprintf("color=c=%s:s=%dx%d:r=%s", color, w, h, fps)
	if canvasMode == "hw" {
		return base + "," + uploadFilterChain(w, h, padForAlignment) + "[canvas]"
	}
	return base + "[canvas]"
}

// alignedDim rounds up to the next multiple of 16 (HEVC CTU alignment).
func alignedDim(d int) int {
	return (d + 15) &^ 15
}

// uploadFilterChain lands a SW frame on the encoder's HW device.
// PadForAlignment pads to /16 with black to keep AMD VCN HEVC from leaking green chroma.
func uploadFilterChain(w, h int, padForAlignment bool) string {
	if !padForAlignment {
		return "format=nv12,hwupload"
	}
	aw, ah := alignedDim(w), alignedDim(h)
	if aw == w && ah == h {
		return "format=nv12,hwupload"
	}
	return fmt.Sprintf("format=nv12,pad=%d:%d:0:0:black,hwupload", aw, ah)
}

func joinFilters(fs []string) string {
	return strings.Join(fs, ",")
}

// perInputEncodeTail bridges the per-input chain to the overlay surface (hwdownload or hwupload as needed).
func perInputEncodeTail(backend string, mode inputDecodeMode, in CompositeInput, caps HWCapabilities, canvasMode string) string {
	leavesOnHW := mode == decodeHW && canKeepEncodeOnHW(backend, in, caps)
	switch {
	case leavesOnHW && canvasMode == "sw":
		return "hwdownload,format=nv12"
	case !leavesOnHW && canvasMode == "hw":
		return "format=nv12,hwupload"
	}
	return ""
}

// finalOverlayTail uploads [vout] to the encoder device when SW overlay ran on a HW backend.
func finalOverlayTail(backend string, hwOverlay bool, w, h int, padForAlignment bool) string {
	if hwOverlay {
		return ""
	}
	if backend == "rkmpp" || backend == "vaapi" {
		return uploadFilterChain(w, h, padForAlignment)
	}
	return ""
}
