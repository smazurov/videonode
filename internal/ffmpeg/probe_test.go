package ffmpeg

import (
	"testing"

	"github.com/smazurov/videonode/internal/types"
)

func TestCapabilitiesFromBackends_Empty(t *testing.T) {
	if got := CapabilitiesFromBackends(nil); got != (HWCapabilities{}) {
		t.Errorf("nil backends → %+v, want zero", got)
	}
	if got := CapabilitiesFromBackends(map[string]types.BackendValidation{}); got != (HWCapabilities{}) {
		t.Errorf("empty backends → %+v, want zero", got)
	}
}

func TestCapabilitiesFromBackends_VAAPI_PartialDriver(t *testing.T) {
	// Mirrors the live radeonsi case: scale + transpose work, overlay does
	// not. The composite builder uses ScaleVAAPI/TransposeVAAPI on the
	// per-input chain and falls back to SW overlay.
	backends := map[string]types.BackendValidation{
		"vaapi": {
			Filters: types.CodecValidation{
				Working: []string{"scale_vaapi", "transpose_vaapi"},
				Failed:  []string{"overlay_vaapi"},
			},
		},
	}
	caps := CapabilitiesFromBackends(backends)
	want := HWCapabilities{ScaleVAAPI: true, TransposeVAAPI: true}
	if caps != want {
		t.Errorf("got %+v, want %+v", caps, want)
	}
}

func TestCapabilitiesFromBackends_RKMPP_Full(t *testing.T) {
	backends := map[string]types.BackendValidation{
		"rkmpp": {
			Filters: types.CodecValidation{
				Working: []string{"scale_rkrga", "vpp_rkrga", "overlay_rkrga"},
			},
		},
	}
	caps := CapabilitiesFromBackends(backends)
	want := HWCapabilities{
		OverlayRKRGA: true,
		ScaleRKRGA:   true,
		VppRKRGA:     true,
	}
	if caps != want {
		t.Errorf("got %+v, want %+v", caps, want)
	}
}

func TestCapabilitiesFromBackends_BothBackends(t *testing.T) {
	backends := map[string]types.BackendValidation{
		"rkmpp": {
			Filters: types.CodecValidation{Working: []string{"scale_rkrga"}},
		},
		"vaapi": {
			Filters: types.CodecValidation{Working: []string{"overlay_vaapi", "scale_vaapi"}},
		},
	}
	caps := CapabilitiesFromBackends(backends)
	want := HWCapabilities{
		ScaleRKRGA:   true,
		OverlayVAAPI: true,
		ScaleVAAPI:   true,
	}
	if caps != want {
		t.Errorf("got %+v, want %+v", caps, want)
	}
}
