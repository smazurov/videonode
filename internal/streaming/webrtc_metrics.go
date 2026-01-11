package streaming

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// Per-stream egress counters (from producer to all consumers).
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

	// Per-stream connection gauge.
	webrtcActivePeers = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "videonode",
		Subsystem: "webrtc",
		Name:      "active_peers",
		Help:      "Number of active WebRTC peers per stream",
	}, []string{"stream_id"})

	// Per-peer RTCP feedback counters.
	webrtcPeerRTCPPackets = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "videonode",
		Subsystem: "webrtc",
		Name:      "peer_rtcp_packets_total",
		Help:      "RTCP packets received per peer",
	}, []string{"stream_id", "peer_id"})

	webrtcPeerNACKs = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "videonode",
		Subsystem: "webrtc",
		Name:      "peer_nacks_total",
		Help:      "NACK requests per peer (indicates packet loss)",
	}, []string{"stream_id", "peer_id"})

	webrtcPeerPLIs = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "videonode",
		Subsystem: "webrtc",
		Name:      "peer_plis_total",
		Help:      "PLI requests per peer (picture loss indication)",
	}, []string{"stream_id", "peer_id"})

	webrtcPeerFIRs = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "videonode",
		Subsystem: "webrtc",
		Name:      "peer_firs_total",
		Help:      "FIR requests per peer (full intra request)",
	}, []string{"stream_id", "peer_id"})

	// Per-peer jitter gauge (from RTCP Receiver Reports).
	webrtcPeerJitter = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "videonode",
		Subsystem: "webrtc",
		Name:      "peer_jitter",
		Help:      "Interarrival jitter per peer (RTP timestamp units, divide by 90000 for seconds)",
	}, []string{"stream_id", "peer_id"})
)

// IncrementRTCPPackets records RTCP packets received from a peer.
func IncrementRTCPPackets(streamID, peerID string) {
	webrtcPeerRTCPPackets.WithLabelValues(streamID, peerID).Inc()
}

// IncrementNACKs records NACK requests received from a peer.
func IncrementNACKs(streamID, peerID string, count int) {
	webrtcPeerNACKs.WithLabelValues(streamID, peerID).Add(float64(count))
}

// IncrementPLIs records PLI requests received from a peer.
func IncrementPLIs(streamID, peerID string) {
	webrtcPeerPLIs.WithLabelValues(streamID, peerID).Inc()
}

// IncrementFIRs records FIR requests received from a peer.
func IncrementFIRs(streamID, peerID string) {
	webrtcPeerFIRs.WithLabelValues(streamID, peerID).Inc()
}

// RecordJitter records interarrival jitter from RTCP Receiver Reports.
func RecordJitter(streamID, peerID string, jitter uint32) {
	webrtcPeerJitter.WithLabelValues(streamID, peerID).Set(float64(jitter))
}

// DeletePeerMetrics removes all metrics for a peer when disconnected.
func DeletePeerMetrics(streamID, peerID string) {
	webrtcPeerRTCPPackets.DeleteLabelValues(streamID, peerID)
	webrtcPeerNACKs.DeleteLabelValues(streamID, peerID)
	webrtcPeerPLIs.DeleteLabelValues(streamID, peerID)
	webrtcPeerFIRs.DeleteLabelValues(streamID, peerID)
	webrtcPeerJitter.DeleteLabelValues(streamID, peerID)
}

// IncrementPacketsSent records packets and bytes sent for a stream.
func IncrementPacketsSent(streamID string, bytes int) {
	webrtcStreamPackets.WithLabelValues(streamID).Inc()
	webrtcStreamBytes.WithLabelValues(streamID).Add(float64(bytes))
}

// SetActivePeers sets the current number of active peers for a stream.
func SetActivePeers(streamID string, count int) {
	webrtcActivePeers.WithLabelValues(streamID).Set(float64(count))
}
