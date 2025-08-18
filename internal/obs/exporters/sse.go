package exporters

import (
	"context"
	"log"
	"strconv"
	"time"

	"github.com/smazurov/videonode/internal/obs"
)

// OBS SSE Event Types - these should be registered with Huma SSE
type MediaMTXMetricsEvent struct {
	Type      string                   `json:"type"`
	Timestamp string                   `json:"timestamp"`
	Count     int                      `json:"count"`
	Metrics   []map[string]interface{} `json:"metrics"`
}

type OBSAlertEvent struct {
	Type      string                 `json:"type"`
	Timestamp string                 `json:"timestamp"`
	Level     string                 `json:"level"`
	Message   string                 `json:"message"`
	Details   map[string]interface{} `json:"details"`
}

type SystemMetricsEvent struct {
	Type        string `json:"type"`
	Timestamp   string `json:"timestamp"`
	LoadAverage struct {
		OneMin     float64 `json:"1m"`
		FiveMin    float64 `json:"5m"`
		FifteenMin float64 `json:"15m"`
	} `json:"load_average"`
	Network struct {
		Interface string  `json:"interface"`
		RxBytes   float64 `json:"rx_bytes"`
		TxBytes   float64 `json:"tx_bytes"`
		RxPackets float64 `json:"rx_packets"`
		TxPackets float64 `json:"tx_packets"`
	} `json:"network"`
}

type StreamMetricsEvent struct {
	Type            string `json:"type"`
	Timestamp       string `json:"timestamp"`
	StreamID        string `json:"stream_id"`
	FPS             string `json:"fps"`
	DroppedFrames   string `json:"dropped_frames"`
	DuplicateFrames string `json:"duplicate_frames"`
	ProcessingSpeed string `json:"processing_speed"`
}

// GetEventTypes returns a map of event names to their corresponding struct types
// This should be used when registering with Huma SSE
func GetEventTypes() map[string]any {
	return map[string]any{
		"mediamtx-metrics": MediaMTXMetricsEvent{},
		"obs-alert":        OBSAlertEvent{},
		"system-metrics":   SystemMetricsEvent{},
		"stream-metrics":   StreamMetricsEvent{},
	}
}

// GetEventTypesForEndpoint returns event types for a specific SSE endpoint
func GetEventTypesForEndpoint(endpoint string) map[string]any {
	switch endpoint {
	case "metrics":
		return map[string]any{
			"mediamtx-metrics": MediaMTXMetricsEvent{},
		}
	case "events":
		return map[string]any{
			"system-metrics": SystemMetricsEvent{},
			"obs-alert":      OBSAlertEvent{},
			"stream-metrics": StreamMetricsEvent{},
		}
	default:
		return map[string]any{}
	}
}

// GetEventRoutes returns the routing configuration for events
func GetEventRoutes() map[string]string {
	return map[string]string{
		"mediamtx-metrics": "metrics",
		"system-metrics":   "events",
		"obs-alert":        "events",
		"stream-metrics":   "events",
	}
}

// SSEBroadcaster defines the interface for broadcasting SSE events
type SSEBroadcaster interface {
	BroadcastEvent(eventType string, data interface{}) error
}

// SSEExporter exports observability data via Server-Sent Events
type SSEExporter struct {
	config      obs.ExporterConfig
	broadcaster SSEBroadcaster
	logLevel    string
}

// NewSSEExporter creates a new SSE exporter
func NewSSEExporter(broadcaster SSEBroadcaster) *SSEExporter {
	config := obs.ExporterConfig{
		Name:    "sse",
		Enabled: true,
		Config:  make(map[string]interface{}),
	}

	return &SSEExporter{
		config:      config,
		broadcaster: broadcaster,
		logLevel:    "info", // Default log level
	}
}

// SetLogLevel sets the logging level for observability logs
func (s *SSEExporter) SetLogLevel(level string) {
	s.logLevel = level
}

// Name returns the exporter name
func (s *SSEExporter) Name() string {
	return s.config.Name
}

// Config returns the exporter configuration
func (s *SSEExporter) Config() obs.ExporterConfig {
	return s.config
}

// Start starts the SSE exporter
func (s *SSEExporter) Start(ctx context.Context) error {
	// Nothing to start - we process immediately
	return nil
}

// Stop stops the SSE exporter
func (s *SSEExporter) Stop() error {
	// Nothing to stop - no background workers
	return nil
}

// Export processes and exports data points immediately
func (s *SSEExporter) Export(points []obs.DataPoint) error {
	if len(points) == 0 {
		return nil
	}

	// Process each point immediately without buffering
	for _, point := range points {
		switch p := point.(type) {
		case *obs.MetricPoint:
			s.processMetricImmediately(p)
		case *obs.LogEntry:
			s.processLogImmediately(p)
		}
	}

	return nil
}

// processMetricImmediately processes a single metric point immediately
func (s *SSEExporter) processMetricImmediately(metric *obs.MetricPoint) {
	// Handle different metric types
	switch metric.Name {
	case "system_metrics":
		s.sendSystemMetricsEvent(metric)
	case "ffmpeg_stream_metrics":
		// Send stream metrics immediately
		labels := metric.Labels()
		event := StreamMetricsEvent{
			Type:            "stream_metrics",
			Timestamp:       metric.Timestamp().Format(time.RFC3339),
			StreamID:        labels["stream_id"],
			FPS:             labels["fps"],
			DroppedFrames:   labels["dropped_frames"],
			DuplicateFrames: labels["duplicate_frames"],
			ProcessingSpeed: labels["processing_speed"],
		}
		s.broadcaster.BroadcastEvent("stream-metrics", event)
	default:
		// Send other metrics as MediaMTX metrics individually
		metricsEvent := MediaMTXMetricsEvent{
			Type:      "mediamtx_metrics",
			Timestamp: time.Now().Format(time.RFC3339),
			Count:     1,
			Metrics:   s.formatMetricsForSSE([]*obs.MetricPoint{metric}),
		}
		s.broadcaster.BroadcastEvent("mediamtx-metrics", metricsEvent)
	}
}

// processLogImmediately processes a single log entry immediately
func (s *SSEExporter) processLogImmediately(logEntry *obs.LogEntry) {
	// Check if we should log this entry based on configured level
	if !s.shouldLog(logEntry.Level) {
		return
	}

	// Format log entry for standard logging
	logLevel := string(logEntry.Level)
	source := logEntry.Source
	message := logEntry.Message
	timestamp := logEntry.Timestamp().Format(time.RFC3339)

	// Log the entry
	log.Printf("[%s] %s [%s]: %s", logLevel, timestamp, source, message)
}

// shouldLog determines if a log entry should be logged based on the configured level
func (s *SSEExporter) shouldLog(entryLevel obs.LogLevel) bool {
	// Define log level hierarchy (lower number = higher priority)
	levels := map[string]int{
		"debug": 0,
		"info":  1,
		"warn":  2,
		"error": 3,
	}

	entryLevelStr := string(entryLevel)
	configuredLevel := levels[s.logLevel]
	entryLevelNum, exists := levels[entryLevelStr]

	// If entry level doesn't exist, log it (safety)
	if !exists {
		return true
	}

	// Log if entry level is >= configured level
	return entryLevelNum >= configuredLevel
}

// sendSystemMetricsEvent sends a structured system metrics event
func (s *SSEExporter) sendSystemMetricsEvent(metric *obs.MetricPoint) {
	labels := metric.Labels()

	// Parse load average values
	load1m, _ := strconv.ParseFloat(labels["load_1m"], 64)
	load5m, _ := strconv.ParseFloat(labels["load_5m"], 64)
	load15m, _ := strconv.ParseFloat(labels["load_15m"], 64)

	// Parse network values
	rxBytes, _ := strconv.ParseFloat(labels["net_rx_bytes"], 64)
	txBytes, _ := strconv.ParseFloat(labels["net_tx_bytes"], 64)
	rxPackets, _ := strconv.ParseFloat(labels["net_rx_packets"], 64)
	txPackets, _ := strconv.ParseFloat(labels["net_tx_packets"], 64)

	event := SystemMetricsEvent{
		Type:      "system_metrics",
		Timestamp: metric.Timestamp().Format(time.RFC3339),
	}

	event.LoadAverage.OneMin = load1m
	event.LoadAverage.FiveMin = load5m
	event.LoadAverage.FifteenMin = load15m

	event.Network.Interface = labels["net_interface"]
	event.Network.RxBytes = rxBytes
	event.Network.TxBytes = txBytes
	event.Network.RxPackets = rxPackets
	event.Network.TxPackets = txPackets

	s.broadcaster.BroadcastEvent("system-metrics", event)
}

// formatMetricsForSSE formats metrics for SSE transmission
func (s *SSEExporter) formatMetricsForSSE(metrics []*obs.MetricPoint) []map[string]interface{} {
	var result []map[string]interface{}

	for _, metric := range metrics {
		item := map[string]interface{}{
			"name":      metric.Name,
			"value":     metric.Value,
			"unit":      metric.Unit,
			"labels":    metric.Labels(),
			"timestamp": metric.Timestamp().Format(time.RFC3339),
		}
		result = append(result, item)
	}

	return result
}

// SendAlert sends an alert via SSE
func (s *SSEExporter) SendAlert(level obs.LogLevel, message string, details map[string]interface{}) error {
	alertEvent := OBSAlertEvent{
		Type:      "alert",
		Level:     string(level),
		Message:   message,
		Details:   details,
		Timestamp: time.Now().Format(time.RFC3339),
	}

	return s.broadcaster.BroadcastEvent("obs-alert", alertEvent)
}

// Stats returns statistics about the exporter
func (s *SSEExporter) Stats() map[string]interface{} {
	return map[string]interface{}{
		"name":    s.config.Name,
		"enabled": s.config.Enabled,
		"mode":    "immediate", // No buffering
	}
}
