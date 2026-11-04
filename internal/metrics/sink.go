package metrics

import (
	"strconv"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	sinkAudioUp = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "videonode",
		Subsystem: "sink",
		Name:      "audio_up",
		Help:      "1 while the audio device delivers samples, 0 while vn-sink substitutes silence",
	}, []string{"stream_id", "track", "device"})

	sinkAudioXruns = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "videonode",
		Subsystem: "sink",
		Name:      "audio_xruns_total",
		Help:      "Audio capture overruns in the current encoder run",
	}, []string{"stream_id", "track", "device"})

	sinkAudioSilenceSeconds = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "videonode",
		Subsystem: "sink",
		Name:      "audio_silence_seconds_total",
		Help:      "Silence substituted for missing device audio in the current encoder run",
	}, []string{"stream_id", "track", "device"})

	sinkAudioDroppedSeconds = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "videonode",
		Subsystem: "sink",
		Name:      "audio_dropped_seconds_total",
		Help:      "Device audio dropped for arriving too late in the current encoder run",
	}, []string{"stream_id", "track", "device"})

	sinkVideoUpstreamUp = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "videonode",
		Subsystem: "sink",
		Name:      "video_upstream_up",
		Help:      "1 while vn-sink is connected to its producer socket",
	}, []string{"stream_id"})

	sinkVideoRepeatedFrames = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "videonode",
		Subsystem: "sink",
		Name:      "video_repeated_frames_total",
		Help:      "Frames repeated to cover upstream gaps in the current encoder run",
	}, []string{"stream_id"})
)

// SetSinkMetrics publishes one vn-sink status report.
func SetSinkMetrics(streamID string, video SinkVideoHealth, audio []AudioInputHealth) {
	sinkVideoUpstreamUp.WithLabelValues(streamID).Set(boolGauge(video.Upstream == "connected"))
	sinkVideoRepeatedFrames.WithLabelValues(streamID).Set(float64(video.RepeatedFrames))
	for _, a := range audio {
		labels := []string{streamID, strconv.Itoa(a.Track), a.Device}
		sinkAudioUp.WithLabelValues(labels...).Set(boolGauge(a.State == "live"))
		sinkAudioXruns.WithLabelValues(labels...).Set(float64(a.Xruns))
		sinkAudioSilenceSeconds.WithLabelValues(labels...).Set(float64(a.SilenceMS) / 1000)
		sinkAudioDroppedSeconds.WithLabelValues(labels...).Set(float64(a.DroppedMS) / 1000)
	}
}

// DeleteSinkMetrics removes every vn-sink series for a stream.
func DeleteSinkMetrics(streamID string) {
	match := prometheus.Labels{"stream_id": streamID}
	for _, g := range []*prometheus.GaugeVec{
		sinkAudioUp, sinkAudioXruns, sinkAudioSilenceSeconds, sinkAudioDroppedSeconds,
		sinkVideoUpstreamUp, sinkVideoRepeatedFrames,
	} {
		g.DeletePartialMatch(match)
	}
}

func boolGauge(b bool) float64 {
	if b {
		return 1
	}
	return 0
}
