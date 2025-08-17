package exporters

import (
	"context"
	"log"
	"strconv"
	"sync"
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

// GetEventTypes returns a map of event names to their corresponding struct types
// This should be used when registering with Huma SSE
func GetEventTypes() map[string]any {
	return map[string]any{
		"mediamtx-metrics": MediaMTXMetricsEvent{},
		"obs-alert":        OBSAlertEvent{},
		"system-metrics":   SystemMetricsEvent{},
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
	buffer      []obs.DataPoint
	bufferMux   sync.Mutex
	ctx         context.Context
	cancel      context.CancelFunc
	wg          sync.WaitGroup
	logLevel    string
}

// NewSSEExporter creates a new SSE exporter
func NewSSEExporter(broadcaster SSEBroadcaster) *SSEExporter {
	config := obs.ExporterConfig{
		Name:          "sse",
		Enabled:       true,
		BufferSize:    1000,
		FlushInterval: 2 * time.Second, // Send updates every 2 seconds
		Config:        make(map[string]interface{}),
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &SSEExporter{
		config:      config,
		broadcaster: broadcaster,
		buffer:      make([]obs.DataPoint, 0, config.BufferSize),
		ctx:         ctx,
		cancel:      cancel,
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
	s.wg.Add(1)
	go s.flushWorker()
	return nil
}

// Stop stops the SSE exporter
func (s *SSEExporter) Stop() error {
	s.cancel()
	s.wg.Wait()
	return nil
}

// Export processes and exports data points
func (s *SSEExporter) Export(points []obs.DataPoint) error {
	if len(points) == 0 {
		return nil
	}

	s.bufferMux.Lock()
	defer s.bufferMux.Unlock()

	// Add points to buffer
	for _, point := range points {
		if len(s.buffer) >= s.config.BufferSize {
			// Buffer full, send immediately
			s.sendBufferedData()
		}
		s.buffer = append(s.buffer, point)
	}

	return nil
}

// flushWorker periodically flushes buffered data
func (s *SSEExporter) flushWorker() {
	defer s.wg.Done()

	ticker := time.NewTicker(s.config.FlushInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.bufferMux.Lock()
			s.sendBufferedData()
			s.bufferMux.Unlock()

		case <-s.ctx.Done():
			// Final flush before stopping
			s.bufferMux.Lock()
			s.sendBufferedData()
			s.bufferMux.Unlock()
			return
		}
	}
}

// sendBufferedData sends all buffered data via SSE
func (s *SSEExporter) sendBufferedData() {
	if len(s.buffer) == 0 {
		return
	}

	// Group data by type
	metrics := make([]*obs.MetricPoint, 0)
	logs := make([]*obs.LogEntry, 0)

	for _, point := range s.buffer {
		switch p := point.(type) {
		case *obs.MetricPoint:
			metrics = append(metrics, p)
		case *obs.LogEntry:
			logs = append(logs, p)
		}
	}

	// Send metrics update
	if len(metrics) > 0 {
		s.sendMetricsUpdate(metrics)
	}

	// Log entries instead of broadcasting them
	if len(logs) > 0 {
		s.logEntries(logs)
	}

	// Clear buffer
	s.buffer = s.buffer[:0]
}

// sendMetricsUpdate sends metrics data via SSE
func (s *SSEExporter) sendMetricsUpdate(metrics []*obs.MetricPoint) {
	// Handle system metrics specially
	for _, metric := range metrics {
		if metric.Name == "system_metrics" {
			s.sendSystemMetricsEvent(metric)
			continue
		}
	}

	// Send general metrics update for non-system metrics
	if len(metrics) > 0 {
		metricsEvent := MediaMTXMetricsEvent{
			Type:      "mediamtx_metrics",
			Timestamp: time.Now().Format(time.RFC3339),
			Count:     len(metrics),
			Metrics:   s.formatMetricsForSSE(metrics),
		}

		s.broadcaster.BroadcastEvent("mediamtx-metrics", metricsEvent)
	}
}

// logEntries logs the entries to standard output instead of broadcasting via SSE
func (s *SSEExporter) logEntries(logs []*obs.LogEntry) {
	for _, logEntry := range logs {
		// Check if we should log this entry based on configured level
		if !s.shouldLog(logEntry.Level) {
			continue
		}

		// Format log entry for standard logging
		logLevel := string(logEntry.Level)
		source := logEntry.Source
		message := logEntry.Message
		timestamp := logEntry.Timestamp().Format(time.RFC3339)

		// Log the entry
		log.Printf("[%s] %s [%s]: %s", logLevel, timestamp, source, message)
	}
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

// formatLogsForSSE formats logs for SSE transmission
func (s *SSEExporter) formatLogsForSSE(logs []*obs.LogEntry) []map[string]interface{} {
	var result []map[string]interface{}

	for _, log := range logs {
		item := map[string]interface{}{
			"level":     string(log.Level),
			"message":   log.Message,
			"source":    log.Source,
			"labels":    log.Labels(),
			"fields":    log.Fields,
			"timestamp": log.Timestamp().Format(time.RFC3339),
		}
		result = append(result, item)
	}

	return result
}

// formatLogsByLevel formats logs grouped by level
func (s *SSEExporter) formatLogsByLevel(logsByLevel map[obs.LogLevel][]*obs.LogEntry) map[string]int {
	result := make(map[string]int)

	for level, logs := range logsByLevel {
		result[string(level)] = len(logs)
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

// ForceFlush immediately sends all buffered data
func (s *SSEExporter) ForceFlush() {
	s.bufferMux.Lock()
	defer s.bufferMux.Unlock()
	s.sendBufferedData()
}

// Stats returns statistics about the exporter
func (s *SSEExporter) Stats() map[string]interface{} {
	s.bufferMux.Lock()
	bufferSize := len(s.buffer)
	s.bufferMux.Unlock()

	return map[string]interface{}{
		"name":            s.config.Name,
		"enabled":         s.config.Enabled,
		"buffer_size":     bufferSize,
		"buffer_capacity": s.config.BufferSize,
		"flush_interval":  s.config.FlushInterval.String(),
	}
}
