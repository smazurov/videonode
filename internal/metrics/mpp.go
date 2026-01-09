package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	mppDeviceLoad = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "videonode",
		Subsystem: "mpp",
		Name:      "device_load",
		Help:      "Rockchip MPP device load percentage",
	}, []string{"device"})

	mppDeviceUtilization = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "videonode",
		Subsystem: "mpp",
		Name:      "device_utilization",
		Help:      "Rockchip MPP device utilization percentage",
	}, []string{"device"})
)

// SetMPPDeviceLoad sets the load percentage for an MPP device.
func SetMPPDeviceLoad(device string, load float64) {
	mppDeviceLoad.WithLabelValues(device).Set(load)
}

// SetMPPDeviceUtilization sets the utilization percentage for an MPP device.
func SetMPPDeviceUtilization(device string, utilization float64) {
	mppDeviceUtilization.WithLabelValues(device).Set(utilization)
}

// DeleteMPPMetrics removes all metrics for a device.
func DeleteMPPMetrics(device string) {
	mppDeviceLoad.DeleteLabelValues(device)
	mppDeviceUtilization.DeleteLabelValues(device)
}
