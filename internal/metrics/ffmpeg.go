// Package metrics provides Prometheus metrics for FFmpeg and MPP collectors.
package metrics

import (
	"sync"

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

	// Local cache for SSE exporter access.
	ffmpegCache   = make(map[string]*FFmpegStreamMetrics)
	ffmpegCacheMu sync.RWMutex
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
	updateCache(streamID, func(m *FFmpegStreamMetrics) { m.FPS = fps })
}

// SetFFmpegDroppedFrames sets the dropped frames count for a stream.
func SetFFmpegDroppedFrames(streamID string, count float64) {
	ffmpegDroppedFrames.WithLabelValues(streamID).Set(count)
	updateCache(streamID, func(m *FFmpegStreamMetrics) { m.DroppedFrames = count })
}

// SetFFmpegDuplicateFrames sets the duplicate frames count for a stream.
func SetFFmpegDuplicateFrames(streamID string, count float64) {
	ffmpegDuplicateFrames.WithLabelValues(streamID).Set(count)
	updateCache(streamID, func(m *FFmpegStreamMetrics) { m.DuplicateFrames = count })
}

// SetFFmpegSpeed sets the processing speed for a stream.
func SetFFmpegSpeed(streamID string, speed float64) {
	ffmpegSpeed.WithLabelValues(streamID).Set(speed)
	updateCache(streamID, func(m *FFmpegStreamMetrics) { m.Speed = speed })
}

// DeleteFFmpegMetrics removes all metrics for a stream.
func DeleteFFmpegMetrics(streamID string) {
	ffmpegFPS.DeleteLabelValues(streamID)
	ffmpegDroppedFrames.DeleteLabelValues(streamID)
	ffmpegDuplicateFrames.DeleteLabelValues(streamID)
	ffmpegSpeed.DeleteLabelValues(streamID)

	ffmpegCacheMu.Lock()
	delete(ffmpegCache, streamID)
	ffmpegCacheMu.Unlock()
}

// GetFFmpegMetrics returns current metric values for a stream.
func GetFFmpegMetrics(streamID string) *FFmpegStreamMetrics {
	ffmpegCacheMu.RLock()
	defer ffmpegCacheMu.RUnlock()
	if m, ok := ffmpegCache[streamID]; ok {
		dup := *m
		return &dup
	}
	return nil
}

// GetAllFFmpegMetrics returns metrics for all active streams.
func GetAllFFmpegMetrics() map[string]*FFmpegStreamMetrics {
	ffmpegCacheMu.RLock()
	defer ffmpegCacheMu.RUnlock()
	result := make(map[string]*FFmpegStreamMetrics, len(ffmpegCache))
	for id, m := range ffmpegCache {
		dup := *m
		result[id] = &dup
	}
	return result
}

func updateCache(streamID string, update func(*FFmpegStreamMetrics)) {
	ffmpegCacheMu.Lock()
	defer ffmpegCacheMu.Unlock()
	m, ok := ffmpegCache[streamID]
	if !ok {
		m = &FFmpegStreamMetrics{}
		ffmpegCache[streamID] = m
	}
	update(m)
}
