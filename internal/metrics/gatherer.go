package metrics

import (
	"encoding/json"
	"strings"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

// GetFFmpegMetricsFromRegistry extracts FFmpeg metrics from Prometheus registry.
func GetFFmpegMetricsFromRegistry() (map[string]*FFmpegStreamMetrics, error) {
	families, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		return nil, err
	}

	result := make(map[string]*FFmpegStreamMetrics)

	for _, mf := range families {
		name := mf.GetName()
		if !strings.HasPrefix(name, "videonode_ffmpeg_") {
			continue
		}

		for _, m := range mf.GetMetric() {
			streamID := getLabelValue(m.GetLabel(), "stream_id")
			if streamID == "" {
				continue
			}

			if result[streamID] == nil {
				result[streamID] = &FFmpegStreamMetrics{}
			}

			value := m.GetGauge().GetValue()
			switch name {
			case "videonode_ffmpeg_fps":
				result[streamID].FPS = value
			case "videonode_ffmpeg_dropped_frames_total":
				result[streamID].DroppedFrames = value
			case "videonode_ffmpeg_duplicate_frames_total":
				result[streamID].DuplicateFrames = value
			case "videonode_ffmpeg_processing_speed":
				result[streamID].Speed = value
			}
		}
	}

	return result, nil
}

// StreamEgressMetrics holds aggregated egress byte/packet counters
// across all protocols (WebRTC + SRT) for a single stream.
type StreamEgressMetrics struct {
	BytesOut   float64
	PacketsOut float64
}

// GetStreamEgressMetrics aggregates WebRTC and SRT egress counters
// from the Prometheus registry, keyed by stream_id.
func GetStreamEgressMetrics() (map[string]*StreamEgressMetrics, error) {
	families, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		return nil, err
	}

	result := make(map[string]*StreamEgressMetrics)

	for _, mf := range families {
		name := mf.GetName()
		switch name {
		case "videonode_webrtc_stream_bytes_total",
			"videonode_srt_stream_bytes_total",
			"videonode_webrtc_stream_packets_total",
			"videonode_srt_stream_packets_total":
		default:
			continue
		}

		for _, m := range mf.GetMetric() {
			streamID := getLabelValue(m.GetLabel(), "stream_id")
			if streamID == "" {
				continue
			}
			if result[streamID] == nil {
				result[streamID] = &StreamEgressMetrics{}
			}
			value := m.GetCounter().GetValue()
			switch name {
			case "videonode_webrtc_stream_bytes_total",
				"videonode_srt_stream_bytes_total":
				result[streamID].BytesOut += value
			case "videonode_webrtc_stream_packets_total",
				"videonode_srt_stream_packets_total":
				result[streamID].PacketsOut += value
			}
		}
	}

	return result, nil
}

// GetStreamMetricsFromRegistry gathers the registry once and returns both
// per-stream FFmpeg metrics and egress counters, keyed by stream_id.
func GetStreamMetricsFromRegistry() (map[string]*FFmpegStreamMetrics, map[string]*StreamEgressMetrics, error) {
	families, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		return nil, nil, err
	}

	ffmpeg := make(map[string]*FFmpegStreamMetrics)
	egress := make(map[string]*StreamEgressMetrics)

	for _, mf := range families {
		name := mf.GetName()
		switch {
		case strings.HasPrefix(name, "videonode_ffmpeg_"):
			for _, m := range mf.GetMetric() {
				streamID := getLabelValue(m.GetLabel(), "stream_id")
				if streamID == "" {
					continue
				}
				if ffmpeg[streamID] == nil {
					ffmpeg[streamID] = &FFmpegStreamMetrics{}
				}
				value := m.GetGauge().GetValue()
				switch name {
				case "videonode_ffmpeg_fps":
					ffmpeg[streamID].FPS = value
				case "videonode_ffmpeg_dropped_frames_total":
					ffmpeg[streamID].DroppedFrames = value
				case "videonode_ffmpeg_duplicate_frames_total":
					ffmpeg[streamID].DuplicateFrames = value
				case "videonode_ffmpeg_processing_speed":
					ffmpeg[streamID].Speed = value
				}
			}
		case name == "videonode_webrtc_stream_bytes_total",
			name == "videonode_srt_stream_bytes_total",
			name == "videonode_webrtc_stream_packets_total",
			name == "videonode_srt_stream_packets_total":
			for _, m := range mf.GetMetric() {
				streamID := getLabelValue(m.GetLabel(), "stream_id")
				if streamID == "" {
					continue
				}
				if egress[streamID] == nil {
					egress[streamID] = &StreamEgressMetrics{}
				}
				value := m.GetCounter().GetValue()
				switch name {
				case "videonode_webrtc_stream_bytes_total",
					"videonode_srt_stream_bytes_total":
					egress[streamID].BytesOut += value
				case "videonode_webrtc_stream_packets_total",
					"videonode_srt_stream_packets_total":
					egress[streamID].PacketsOut += value
				}
			}
		}
	}

	return ffmpeg, egress, nil
}

// GetProducerMetricsFromRegistry extracts per-source producer-process metrics
// (RSS / CPU) from Prometheus registry keyed by source_id.
func GetProducerMetricsFromRegistry() (map[string]*ProducerProcessMetrics, error) {
	families, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		return nil, err
	}

	result := make(map[string]*ProducerProcessMetrics)

	for _, mf := range families {
		name := mf.GetName()
		if !strings.HasPrefix(name, "videonode_producer_") {
			continue
		}
		for _, m := range mf.GetMetric() {
			sourceID := getLabelValue(m.GetLabel(), "source_id")
			if sourceID == "" {
				continue
			}
			if result[sourceID] == nil {
				result[sourceID] = &ProducerProcessMetrics{}
			}
			value := m.GetGauge().GetValue()
			switch name {
			case "videonode_producer_rss_bytes":
				result[sourceID].RSSBytes = value
			case "videonode_producer_cpu_percent":
				result[sourceID].CPUPercent = value
			}
		}
	}

	return result, nil
}

func getLabelValue(labels []*dto.LabelPair, name string) string {
	for _, lp := range labels {
		if lp.GetName() == name {
			return lp.GetValue()
		}
	}
	return ""
}

// MetricValue represents a single metric sample with labels.
type MetricValue struct {
	Labels map[string]string `json:"labels,omitempty"`
	Value  float64           `json:"value"`
}

// MetricFamily represents a Prometheus metric family in JSON format.
type MetricFamily struct {
	Name    string        `json:"name"`
	Help    string        `json:"help"`
	Type    string        `json:"type"`
	Metrics []MetricValue `json:"metrics"`
}

// GetAllMetricsAsJSON returns all metrics as JSON-serializable structure.
func GetAllMetricsAsJSON() ([]MetricFamily, error) {
	families, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		return nil, err
	}

	result := make([]MetricFamily, 0, len(families))

	for _, mf := range families {
		family := MetricFamily{
			Name:    mf.GetName(),
			Help:    mf.GetHelp(),
			Type:    mf.GetType().String(),
			Metrics: make([]MetricValue, 0),
		}

		for _, m := range mf.GetMetric() {
			mv := MetricValue{
				Labels: make(map[string]string),
			}

			for _, lp := range m.GetLabel() {
				mv.Labels[lp.GetName()] = lp.GetValue()
			}
			if len(mv.Labels) == 0 {
				mv.Labels = nil
			}

			switch mf.GetType() {
			case dto.MetricType_GAUGE:
				mv.Value = m.GetGauge().GetValue()
			case dto.MetricType_COUNTER:
				mv.Value = m.GetCounter().GetValue()
			case dto.MetricType_UNTYPED:
				mv.Value = m.GetUntyped().GetValue()
			default:
				continue
			}

			family.Metrics = append(family.Metrics, mv)
		}

		if len(family.Metrics) > 0 {
			result = append(result, family)
		}
	}

	return result, nil
}

// WriteMetricsJSON writes all metrics as JSON to the given encoder.
func WriteMetricsJSON(enc *json.Encoder) error {
	metrics, err := GetAllMetricsAsJSON()
	if err != nil {
		return err
	}
	return enc.Encode(metrics)
}
