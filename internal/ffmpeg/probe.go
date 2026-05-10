package ffmpeg

import (
	"slices"
	"strings"

	"github.com/smazurov/videonode/internal/types"
)

// HWCapabilities reports validated hardware filters for the local FFmpeg + driver pair.
type HWCapabilities struct {
	OverlayRKRGA   bool
	ScaleRKRGA     bool
	ScaleRKRGABGRA bool // gates the BGRA-overlay path
	VppRKRGA       bool

	OverlayVAAPI   bool
	ScaleVAAPI     bool
	ScaleVAAPIBGRA bool // gates the BGRA-overlay path
	TransposeVAAPI bool

	// AMD VCN hevc_vaapi pads non-16-aligned dims with uninitialised memory (green band).
	HevcVaapiNeedsAlignedHeight bool
	HevcRkmppNeedsAlignedHeight bool
}

// HasScale reports whether the HW scale filter is validated for backend.
func (c HWCapabilities) HasScale(backend string) bool {
	switch backend {
	case "rkmpp":
		return c.ScaleRKRGA
	case "vaapi":
		return c.ScaleVAAPI
	}
	return false
}

// HasTranspose reports whether the HW transpose filter is validated for backend.
func (c HWCapabilities) HasTranspose(backend string) bool {
	switch backend {
	case "rkmpp":
		return c.VppRKRGA
	case "vaapi":
		return c.TransposeVAAPI
	}
	return false
}

// HasOverlay reports whether the HW overlay filter is validated for backend.
func (c HWCapabilities) HasOverlay(backend string) bool {
	switch backend {
	case "rkmpp":
		return c.OverlayRKRGA
	case "vaapi":
		return c.OverlayVAAPI
	}
	return false
}

// HasOverlayBGRA reports whether the backend supports BGRA-overlay-on-YUV (overlay + scale_*=bgra).
func (c HWCapabilities) HasOverlayBGRA(backend string) bool {
	switch backend {
	case "rkmpp":
		return c.OverlayRKRGA && c.ScaleRKRGABGRA
	case "vaapi":
		return c.OverlayVAAPI && c.ScaleVAAPIBGRA
	}
	return false
}

// NeedsAlignedHevcHeight reports whether HEVC SW input must be pre-padded to /16 before hwupload.
func (c HWCapabilities) NeedsAlignedHevcHeight(backend, encoder string) bool {
	if !strings.Contains(encoder, "hevc") {
		return false
	}
	switch backend {
	case "vaapi":
		return c.HevcVaapiNeedsAlignedHeight
	case "rkmpp":
		return c.HevcRkmppNeedsAlignedHeight
	}
	return false
}

// CapabilitiesFromBackends builds HWCapabilities from validator results; nil → all-SW zero value.
func CapabilitiesFromBackends(backends map[string]types.BackendValidation) HWCapabilities {
	caps := HWCapabilities{}
	if rk, ok := backends["rkmpp"]; ok {
		w := rk.Filters.Working
		caps.OverlayRKRGA = slices.Contains(w, "overlay_rkrga")
		caps.ScaleRKRGA = slices.Contains(w, "scale_rkrga")
		caps.ScaleRKRGABGRA = slices.Contains(w, "scale_rkrga_bgra")
		caps.VppRKRGA = slices.Contains(w, "vpp_rkrga")
		// "hevc_alignment" in failed[] = alignment probe ran and detected a green-band leak.
		caps.HevcRkmppNeedsAlignedHeight = slices.Contains(rk.Filters.Failed, "hevc_alignment")
	}
	if va, ok := backends["vaapi"]; ok {
		w := va.Filters.Working
		caps.OverlayVAAPI = slices.Contains(w, "overlay_vaapi")
		caps.ScaleVAAPI = slices.Contains(w, "scale_vaapi")
		caps.ScaleVAAPIBGRA = slices.Contains(w, "scale_vaapi_bgra")
		caps.TransposeVAAPI = slices.Contains(w, "transpose_vaapi")
		caps.HevcVaapiNeedsAlignedHeight = slices.Contains(va.Filters.Failed, "hevc_alignment")
	}
	return caps
}
