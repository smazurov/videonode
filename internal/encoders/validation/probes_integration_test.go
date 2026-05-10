//go:build integration

package validation

import (
	"testing"
)

// silentLogger discards probe output during integration tests.
type silentLogger struct{}

func (silentLogger) Printf(_ string, _ ...any) {}

// TestVaapiBackendProbes runs every VAAPI probe on the host and prints the
// per-filter result. Skipped when /dev/dri/renderD128 is missing. Real
// hardware test — invoke via `go test -tags=integration ./internal/encoders/validation/...`.
func TestVaapiBackendProbes(t *testing.T) {
	if !vaapiDeviceAvailable() {
		t.Skipf("VAAPI device %s not present", VaapiDevicePath)
	}
	v := NewVaapiValidator()
	decW, decF := v.ValidateDecoders(silentLogger{})
	fltW, fltF := v.ValidateFilters(silentLogger{})
	t.Logf("vaapi decoders working=%v failed=%v", decW, decF)
	t.Logf("vaapi filters  working=%v failed=%v", fltW, fltF)
}

// TestRkmppBackendProbes runs every RKMPP probe on the host. Skipped when
// /proc/mpp_service/load is missing (i.e. not running on a Rockchip box).
func TestRkmppBackendProbes(t *testing.T) {
	if !rkmppDeviceAvailable() {
		t.Skip("RKMPP device not present (no /proc/mpp_service/load)")
	}
	v := NewRkmppValidator()
	decW, decF := v.ValidateDecoders(silentLogger{})
	fltW, fltF := v.ValidateFilters(silentLogger{})
	t.Logf("rkmpp decoders working=%v failed=%v", decW, decF)
	t.Logf("rkmpp filters  working=%v failed=%v", fltW, fltF)
}
