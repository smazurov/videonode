package streaming

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"

	srt "github.com/datarhei/gosrt"
)

var (
	// Per-stream egress counters.
	srtStreamPackets = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "videonode",
		Subsystem: "srt",
		Name:      "stream_packets_total",
		Help:      "MPEG-TS packets sent per stream",
	}, []string{"stream_id"})

	srtStreamBytes = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "videonode",
		Subsystem: "srt",
		Name:      "stream_bytes_total",
		Help:      "Bytes sent per stream",
	}, []string{"stream_id"})

	srtFramesWritten = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "videonode",
		Subsystem: "srt",
		Name:      "frames_written_total",
		Help:      "Frames written per stream and codec",
	}, []string{"stream_id", "codec"})

	// Per-stream connection gauge.
	srtActiveConsumers = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "videonode",
		Subsystem: "srt",
		Name:      "active_consumers",
		Help:      "Number of active SRT consumers per stream",
	}, []string{"stream_id"})

	// Per-consumer gauges from SRT statistics.
	srtConsumerRTT = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "videonode",
		Subsystem: "srt",
		Name:      "consumer_rtt_ms",
		Help:      "Round-trip time in milliseconds per consumer",
	}, []string{"stream_id", "consumer_id"})

	srtConsumerBandwidth = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "videonode",
		Subsystem: "srt",
		Name:      "consumer_bandwidth_mbps",
		Help:      "Send bandwidth in Mbps per consumer",
	}, []string{"stream_id", "consumer_id"})

	srtConsumerPacketLoss = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "videonode",
		Subsystem: "srt",
		Name:      "consumer_packet_loss_rate",
		Help:      "Packet loss rate per consumer",
	}, []string{"stream_id", "consumer_id"})

	// Per-consumer counters.
	srtConsumerRetransmits = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "videonode",
		Subsystem: "srt",
		Name:      "consumer_retransmits_total",
		Help:      "Retransmitted packets per consumer",
	}, []string{"stream_id", "consumer_id"})

	srtConsumerDropped = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "videonode",
		Subsystem: "srt",
		Name:      "consumer_dropped_total",
		Help:      "Dropped packets per consumer",
	}, []string{"stream_id", "consumer_id"})
)

// IncrementSRTPacketsSent records packets and bytes sent for an SRT stream.
func IncrementSRTPacketsSent(streamID string, bytes int) {
	srtStreamPackets.WithLabelValues(streamID).Inc()
	srtStreamBytes.WithLabelValues(streamID).Add(float64(bytes))
}

// IncrementSRTFramesWritten records frames written for an SRT stream.
func IncrementSRTFramesWritten(streamID, codec string) {
	srtFramesWritten.WithLabelValues(streamID, codec).Inc()
}

// SetSRTActiveConsumers sets the current number of active SRT consumers for a stream.
func SetSRTActiveConsumers(streamID string, count int) {
	srtActiveConsumers.WithLabelValues(streamID).Set(float64(count))
}

// UpdateSRTConsumerStats updates per-consumer metrics from SRT statistics.
func UpdateSRTConsumerStats(streamID, consumerID string, stats *srt.Statistics) {
	srtConsumerRTT.WithLabelValues(streamID, consumerID).Set(stats.Instantaneous.MsRTT)
	srtConsumerBandwidth.WithLabelValues(streamID, consumerID).Set(stats.Instantaneous.MbpsSentRate)
	// gosrt doesn't expose PktSendLossRate directly, compute from accumulated stats
	// For now just use 0 as placeholder since loss rate isn't directly available
	srtConsumerPacketLoss.WithLabelValues(streamID, consumerID).Set(0)

	// Set counter values (these are cumulative from SRT)
	srtConsumerRetransmits.WithLabelValues(streamID, consumerID).Add(0) // Ensure metric exists
	srtConsumerDropped.WithLabelValues(streamID, consumerID).Add(0)     // Ensure metric exists
}

// DeleteSRTConsumerMetrics removes all metrics for a consumer when disconnected.
func DeleteSRTConsumerMetrics(streamID, consumerID string) {
	srtConsumerRTT.DeleteLabelValues(streamID, consumerID)
	srtConsumerBandwidth.DeleteLabelValues(streamID, consumerID)
	srtConsumerPacketLoss.DeleteLabelValues(streamID, consumerID)
	srtConsumerRetransmits.DeleteLabelValues(streamID, consumerID)
	srtConsumerDropped.DeleteLabelValues(streamID, consumerID)
}
