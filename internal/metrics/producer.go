package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Producer-process metrics carry a source_id label so each source's RSS /
// CPU can be tracked independently (multiple sources share a single host).
var (
	producerRSSBytes = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "videonode",
		Subsystem: "producer",
		Name:      "rss_bytes",
		Help:      "Resident set size of a source producer process in bytes",
	}, []string{"source_id"})

	producerCPUPercent = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "videonode",
		Subsystem: "producer",
		Name:      "cpu_percent",
		Help:      "CPU usage of a source producer process as a percentage (0-100 per core)",
	}, []string{"source_id"})
)

// ProducerProcessMetrics holds current per-source producer process metrics.
type ProducerProcessMetrics struct {
	RSSBytes   float64
	CPUPercent float64
}

// SetProducerRSS sets resident set size (bytes) for a producer process.
func SetProducerRSS(sourceID string, rssBytes float64) {
	producerRSSBytes.WithLabelValues(sourceID).Set(rssBytes)
}

// SetProducerCPU sets CPU percent for a producer process.
func SetProducerCPU(sourceID string, cpuPercent float64) {
	producerCPUPercent.WithLabelValues(sourceID).Set(cpuPercent)
}

// DeleteProducerMetrics removes all producer-process metrics for a source.
func DeleteProducerMetrics(sourceID string) {
	producerRSSBytes.DeleteLabelValues(sourceID)
	producerCPUPercent.DeleteLabelValues(sourceID)
}
