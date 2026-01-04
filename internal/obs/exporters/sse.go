package exporters

import (
	"context"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/smazurov/videonode/internal/events"
	"github.com/smazurov/videonode/internal/obs"
)

// OBS SSE Event Type exports for SSE endpoint registration
// The actual event types are now defined in internal/events package

// GetEventTypes returns a map of event names to their corresponding struct types
// This should be used when registering with Huma SSE.
func GetEventTypes() map[string]any {
	return map[string]any{
		"stream-metrics": events.StreamMetricsEvent{},
	}
}

// GetEventTypesForEndpoint returns event types for a specific SSE endpoint.
func GetEventTypesForEndpoint(endpoint string) map[string]any {
	if endpoint == "events" {
		return map[string]any{
			"stream-metrics": events.StreamMetricsEvent{},
		}
	}
	return map[string]any{}
}

// GetEventRoutes returns the routing configuration for events.
func GetEventRoutes() map[string]string {
	return map[string]string{
		"stream-metrics": "events",
	}
}

// EventPublisher interface for publishing events (allows mocking in tests).
type EventPublisher interface {
	Publish(ev events.Event)
}

// StreamMetricsAccumulator accumulates FFmpeg metrics for a stream.
type StreamMetricsAccumulator struct {
	StreamID        string
	FPS             string
	DroppedFrames   string
	DuplicateFrames string
	ProcessingSpeed string
	LastUpdate      time.Time
}

// SSEExporter exports observability data via Server-Sent Events.
type SSEExporter struct {
	config        obs.ExporterConfig
	logger        *slog.Logger
	eventBus      EventPublisher
	logLevel      string
	streamMetrics map[string]*StreamMetricsAccumulator // stream_id -> accumulated metrics
	streamMutex   sync.RWMutex
}

// NewSSEExporter creates a new SSE exporter.
func NewSSEExporter(eventBus EventPublisher) *SSEExporter {
	config := obs.ExporterConfig{
		Name:    "sse",
		Enabled: true,
		Config:  make(map[string]any),
	}

	return &SSEExporter{
		config:        config,
		logger:        slog.With("component", "sse_exporter"),
		eventBus:      eventBus,
		logLevel:      "info", // Default log level
		streamMetrics: make(map[string]*StreamMetricsAccumulator),
	}
}

// SetLogLevel sets the logging level for observability logs.
func (s *SSEExporter) SetLogLevel(level string) {
	s.logLevel = level
}

// Name returns the exporter name.
func (s *SSEExporter) Name() string {
	return s.config.Name
}

// Config returns the exporter configuration.
func (s *SSEExporter) Config() obs.ExporterConfig {
	return s.config
}

// Start starts the SSE exporter.
func (s *SSEExporter) Start(_ context.Context) error {
	// Nothing to start - we process immediately
	return nil
}

// Stop stops the SSE exporter.
func (s *SSEExporter) Stop() error {
	// Nothing to stop - no background workers
	return nil
}

// Export processes and exports data points immediately.
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

// processMetricImmediately processes a single metric point immediately.
func (s *SSEExporter) processMetricImmediately(metric *obs.MetricPoint) {
	// Only handle FFmpeg metrics for SSE
	if strings.HasPrefix(metric.Name, "ffmpeg_") {
		s.accumulateStreamMetric(metric)
	}
	// Other metrics are ignored (handled by Prometheus exporter)
}

// processLogImmediately processes a single log entry immediately.
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
	s.logger.Info("SSE log entry", "level", logLevel, "timestamp", timestamp, "source", source, "message", message)
}

// shouldLog determines if a log entry should be logged based on the configured level.
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

// accumulateStreamMetric accumulates FFmpeg metrics and sends combined stream event.
func (s *SSEExporter) accumulateStreamMetric(metric *obs.MetricPoint) {
	streamID := metric.LabelsMap["stream_id"]
	if streamID == "" {
		return // No stream_id, can't accumulate
	}

	s.streamMutex.Lock()
	defer s.streamMutex.Unlock()

	// Get or create accumulator for this stream
	accumulator, exists := s.streamMetrics[streamID]
	if !exists {
		accumulator = &StreamMetricsAccumulator{
			StreamID: streamID,
		}
		s.streamMetrics[streamID] = accumulator
	}

	// Update specific metric value
	switch metric.Name {
	case "ffmpeg_fps":
		accumulator.FPS = strconv.FormatFloat(metric.Value, 'f', 2, 64)
	case "ffmpeg_dropped_frames_total":
		accumulator.DroppedFrames = strconv.FormatFloat(metric.Value, 'f', 0, 64)
	case "ffmpeg_duplicate_frames_total":
		accumulator.DuplicateFrames = strconv.FormatFloat(metric.Value, 'f', 0, 64)
	case "ffmpeg_processing_speed":
		accumulator.ProcessingSpeed = strconv.FormatFloat(metric.Value, 'f', 3, 64)
	}

	accumulator.LastUpdate = time.Now()

	// Send combined stream metrics event
	s.sendStreamMetricsEvent(accumulator)
}

// sendStreamMetricsEvent sends a combined stream metrics event.
func (s *SSEExporter) sendStreamMetricsEvent(accumulator *StreamMetricsAccumulator) {
	s.eventBus.Publish(events.StreamMetricsEvent{
		EventType:       "stream_metrics",
		Timestamp:       accumulator.LastUpdate.Format(time.RFC3339),
		StreamID:        accumulator.StreamID,
		FPS:             accumulator.FPS,
		DroppedFrames:   accumulator.DroppedFrames,
		DuplicateFrames: accumulator.DuplicateFrames,
		ProcessingSpeed: accumulator.ProcessingSpeed,
	})
}

// Stats returns statistics about the exporter.
func (s *SSEExporter) Stats() map[string]any {
	s.streamMutex.RLock()
	streamCount := len(s.streamMetrics)
	s.streamMutex.RUnlock()

	return map[string]any{
		"name":         s.config.Name,
		"enabled":      s.config.Enabled,
		"mode":         "immediate", // No buffering
		"stream_count": streamCount,
	}
}
