package metrics

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestMPPMetrics(t *testing.T) {
	device := "rkvenc-test"

	// Set metrics
	SetMPPDeviceLoad(device, 45.5)
	SetMPPDeviceUtilization(device, 78.2)

	// Verify values via prometheus testutil
	loadVal := testutil.ToFloat64(mppDeviceLoad.WithLabelValues(device))
	if loadVal != 45.5 {
		t.Errorf("mppDeviceLoad = %v, want 45.5", loadVal)
	}

	utilVal := testutil.ToFloat64(mppDeviceUtilization.WithLabelValues(device))
	if utilVal != 78.2 {
		t.Errorf("mppDeviceUtilization = %v, want 78.2", utilVal)
	}

	// Delete metrics
	DeleteMPPMetrics(device)

	// Delete non-existent should not panic
	DeleteMPPMetrics("non-existent-device")
}
