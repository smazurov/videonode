package streaming

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// Per-stream packet counters.
	webrtcStreamPackets = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "videonode",
		Subsystem: "webrtc",
		Name:      "stream_packets_total",
		Help:      "RTP packets sent per stream",
	}, []string{"stream_id"})

	webrtcStreamBytes = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "videonode",
		Subsystem: "webrtc",
		Name:      "stream_bytes_total",
		Help:      "Bytes sent per stream",
	}, []string{"stream_id"})

	// RTCP counters.
	webrtcRTCPPackets = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "videonode",
		Subsystem: "webrtc",
		Name:      "rtcp_packets_total",
		Help:      "Total RTCP packets received from WebRTC peers",
	}, []string{"stream_id"})

	webrtcNACKsReceived = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "videonode",
		Subsystem: "webrtc",
		Name:      "nacks_received_total",
		Help:      "Total NACK requests received from WebRTC peers (indicates packet loss)",
	})

	webrtcPLIsReceived = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "videonode",
		Subsystem: "webrtc",
		Name:      "plis_received_total",
		Help:      "Total PLI (Picture Loss Indication) requests received from WebRTC peers",
	})

	webrtcFIRsReceived = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "videonode",
		Subsystem: "webrtc",
		Name:      "firs_received_total",
		Help:      "Total FIR (Full Intra Request) requests received from WebRTC peers",
	})

	// Connection gauges.
	webrtcActivePeers = promauto.NewGauge(prometheus.GaugeOpts{
		Namespace: "videonode",
		Subsystem: "webrtc",
		Name:      "active_peers",
		Help:      "Number of currently active WebRTC peer connections",
	})

	// Per-stream counters (with stream_id label).
	webrtcStreamNACKs = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "videonode",
		Subsystem: "webrtc",
		Name:      "stream_nacks_total",
		Help:      "NACK requests per stream",
	}, []string{"stream_id"})

	webrtcStreamPLIs = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "videonode",
		Subsystem: "webrtc",
		Name:      "stream_plis_total",
		Help:      "PLI requests per stream",
	}, []string{"stream_id"})
)

// IncrementRTCPPackets records RTCP packets received.
func IncrementRTCPPackets(streamID string) {
	webrtcRTCPPackets.WithLabelValues(streamID).Inc()
}

// IncrementNACKs records NACK requests received.
func IncrementNACKs(streamID string, count int) {
	webrtcNACKsReceived.Add(float64(count))
	webrtcStreamNACKs.WithLabelValues(streamID).Add(float64(count))
}

// IncrementPLIs records PLI requests received.
func IncrementPLIs(streamID string) {
	webrtcPLIsReceived.Inc()
	webrtcStreamPLIs.WithLabelValues(streamID).Inc()
}

// IncrementFIRs records FIR requests received.
func IncrementFIRs() {
	webrtcFIRsReceived.Inc()
}

// IncrementPacketsSent records packets and bytes sent for a stream.
func IncrementPacketsSent(streamID string, bytes int) {
	webrtcStreamPackets.WithLabelValues(streamID).Inc()
	webrtcStreamBytes.WithLabelValues(streamID).Add(float64(bytes))
}

// SetActivePeers sets the current number of active peers.
func SetActivePeers(count int) {
	webrtcActivePeers.Set(float64(count))
}
