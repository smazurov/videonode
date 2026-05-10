package ffmpeg

import (
	"fmt"
	"strings"
)

// hasHWOutputFormat reports whether GlobalArgs contains -hwaccel_output_format.
func hasHWOutputFormat(args []string) bool {
	for i := 0; i+1 < len(args); i++ {
		if args[i] == "-hwaccel_output_format" {
			return true
		}
	}
	return false
}

// swTransposeFilter returns the SW transpose filter for rotation; "" for 0/invalid.
func swTransposeFilter(rotation int) string {
	switch rotation {
	case 90:
		return "transpose=1"
	case 180:
		return "transpose=1,transpose=1"
	case 270:
		return "transpose=2"
	}
	return ""
}

// hwTransposeFilter returns the HW transpose filter for an encoder; "" if unknown.
func hwTransposeFilter(encoder string, rotation int) string {
	return hwTransposeForBackend(backendForEncoder(encoder), rotation)
}

// hwTransposeForBackend returns the HW transpose filter for backend; "" if unknown.
func hwTransposeForBackend(backend string, rotation int) string {
	var tmpl string
	switch backend {
	case "vaapi":
		tmpl = "transpose_vaapi=dir=%s"
	case "rkmpp":
		tmpl = "vpp_rkrga=transpose=%s"
	default:
		return ""
	}
	var dir string
	switch rotation {
	case 90:
		dir = "clock"
	case 180:
		dir = "reversal"
	case 270:
		dir = "cclock"
	default:
		return ""
	}
	return fmt.Sprintf(tmpl, dir)
}

// backendForEncoder maps an encoder name to "rkmpp", "vaapi", or "sw".
func backendForEncoder(encoder string) string {
	switch {
	case strings.HasSuffix(encoder, "_rkmpp"):
		return "rkmpp"
	case strings.HasSuffix(encoder, "_vaapi"):
		return "vaapi"
	default:
		return "sw"
	}
}

// hwScaleFilter returns the HW scale filter for backend; format defaults to nv12; sw ignores format.
func hwScaleFilter(backend string, w, h int, format string) string {
	if format == "" {
		format = "nv12"
	}
	switch backend {
	case "rkmpp":
		return fmt.Sprintf("scale_rkrga=%d:%d:format=%s", w, h, format)
	case "vaapi":
		return fmt.Sprintf("scale_vaapi=w=%d:h=%d:format=%s", w, h, format)
	default:
		return fmt.Sprintf("scale=%d:%d", w, h)
	}
}

// hwOverlayFilter returns the HW overlay filter for backend at (x, y); sw returns plain overlay.
func hwOverlayFilter(backend string, x, y int) string {
	switch backend {
	case "rkmpp":
		return fmt.Sprintf("overlay_rkrga=x=%d:y=%d", x, y)
	case "vaapi":
		return fmt.Sprintf("overlay_vaapi=x=%d:y=%d", x, y)
	default:
		return fmt.Sprintf("overlay=%d:%d", x, y)
	}
}
