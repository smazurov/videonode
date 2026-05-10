// Package metrics provides Prometheus metrics for FFmpeg and MPP collectors.
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	ffmpegFPS = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "videonode",
		Subsystem: "ffmpeg",
		Name:      "fps",
		Help:      "Current FFmpeg encoding FPS",
	}, []string{"stream_id"})

	ffmpegDroppedFrames = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "videonode",
		Subsystem: "ffmpeg",
		Name:      "dropped_frames_total",
		Help:      "Total dropped frames",
	}, []string{"stream_id"})

	ffmpegDuplicateFrames = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "videonode",
		Subsystem: "ffmpeg",
		Name:      "duplicate_frames_total",
		Help:      "Total duplicate frames",
	}, []string{"stream_id"})

	ffmpegSpeed = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "videonode",
		Subsystem: "ffmpeg",
		Name:      "processing_speed",
		Help:      "FFmpeg processing speed multiplier",
	}, []string{"stream_id"})
)

// FFmpegStreamMetrics holds current metric values for a stream.
type FFmpegStreamMetrics struct {
	FPS             float64
	DroppedFrames   float64
	DuplicateFrames float64
	Speed           float64
}

// SetFFmpegFPS sets the current FPS for a stream.
func SetFFmpegFPS(streamID string, fps float64) {
	ffmpegFPS.WithLabelValues(streamID).Set(fps)
}

// SetFFmpegDroppedFrames sets the dropped frames count for a stream.
func SetFFmpegDroppedFrames(streamID string, count float64) {
	ffmpegDroppedFrames.WithLabelValues(streamID).Set(count)
}

// SetFFmpegDuplicateFrames sets the duplicate frames count for a stream.
func SetFFmpegDuplicateFrames(streamID string, count float64) {
	ffmpegDuplicateFrames.WithLabelValues(streamID).Set(count)
}

// SetFFmpegSpeed sets the processing speed for a stream.
func SetFFmpegSpeed(streamID string, speed float64) {
	ffmpegSpeed.WithLabelValues(streamID).Set(speed)
}

// DeleteFFmpegMetrics removes all metrics for a stream.
func DeleteFFmpegMetrics(streamID string) {
	ffmpegFPS.DeleteLabelValues(streamID)
	ffmpegDroppedFrames.DeleteLabelValues(streamID)
	ffmpegDuplicateFrames.DeleteLabelValues(streamID)
	ffmpegSpeed.DeleteLabelValues(streamID)
}
