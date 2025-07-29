package exporters

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/smazurov/videonode/internal/obs"
)

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
	}
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

	// Send logs update
	if len(logs) > 0 {
		s.sendLogsUpdate(logs)
	}

	// Clear buffer
	s.buffer = s.buffer[:0]
}

// sendMetricsUpdate sends metrics data via SSE
func (s *SSEExporter) sendMetricsUpdate(metrics []*obs.MetricPoint) {
	// Group metrics by name for chart data
	chartData := s.groupMetricsForCharts(metrics)

	// Send individual chart updates
	for chartID, data := range chartData {
		event := map[string]interface{}{
			"type":      "data",
			"timestamp": time.Now().Format(time.RFC3339),
			"values":    data.Values,
			"labels":    data.Labels,
		}

		if err := s.broadcaster.BroadcastEvent(chartID, event); err != nil {
			// Log error but continue
			continue
		}
	}

	// Send general metrics update
	metricsEvent := map[string]interface{}{
		"type":      "metrics_update",
		"timestamp": time.Now().Format(time.RFC3339),
		"count":     len(metrics),
		"metrics":   s.formatMetricsForSSE(metrics),
	}

	s.broadcaster.BroadcastEvent("obs-metrics", metricsEvent)
}

// sendLogsUpdate sends logs data via SSE
func (s *SSEExporter) sendLogsUpdate(logs []*obs.LogEntry) {
	// Group logs by level
	logsByLevel := make(map[obs.LogLevel][]*obs.LogEntry)
	for _, log := range logs {
		logsByLevel[log.Level] = append(logsByLevel[log.Level], log)
	}

	// Send logs update
	logsEvent := map[string]interface{}{
		"type":      "logs_update",
		"timestamp": time.Now().Format(time.RFC3339),
		"count":     len(logs),
		"logs":      s.formatLogsForSSE(logs),
		"by_level":  s.formatLogsByLevel(logsByLevel),
	}

	s.broadcaster.BroadcastEvent("obs-logs", logsEvent)

	// Send individual log entries for real-time display
	for _, log := range logs {
		if log.Level == obs.LogLevelError || log.Level == obs.LogLevelFatal {
			logEvent := map[string]interface{}{
				"type":      "log_entry",
				"level":     string(log.Level),
				"message":   log.Message,
				"source":    log.Source,
				"timestamp": log.Timestamp().Format(time.RFC3339),
				"labels":    log.Labels(),
			}
			s.broadcaster.BroadcastEvent("obs-log-entry", logEvent)
		}
	}
}

// ChartData represents data for a chart
type ChartData struct {
	Values []float64  `json:"values"`
	Labels obs.Labels `json:"labels"`
}

// groupMetricsForCharts groups metrics into chart-friendly format
func (s *SSEExporter) groupMetricsForCharts(metrics []*obs.MetricPoint) map[string]ChartData {
	charts := make(map[string]ChartData)

	// Group by metric name and key labels
	metricGroups := make(map[string][]*obs.MetricPoint)
	for _, metric := range metrics {
		key := s.getChartKey(metric)
		metricGroups[key] = append(metricGroups[key], metric)
	}

	// Convert to chart data
	for chartID, metricList := range metricGroups {
		if len(metricList) == 0 {
			continue
		}

		// For now, just take the latest values
		// In production, you might want to aggregate or sample
		var values []float64
		var labels obs.Labels

		if len(metricList) > 0 {
			latest := metricList[len(metricList)-1]
			values = []float64{latest.Value}
			labels = latest.Labels()
		}

		charts[chartID] = ChartData{
			Values: values,
			Labels: labels,
		}
	}

	return charts
}

// getChartKey generates a chart ID for a metric
func (s *SSEExporter) getChartKey(metric *obs.MetricPoint) string {
	// Create chart ID based on metric name and key labels
	chartID := fmt.Sprintf("obs-%s", metric.Name)

	// Add important labels to make unique charts
	if streamID, ok := metric.Labels()["stream_id"]; ok {
		chartID += fmt.Sprintf("-%s", streamID)
	}
	if device, ok := metric.Labels()["device"]; ok {
		chartID += fmt.Sprintf("-%s", device)
	}
	if source, ok := metric.Labels()["source"]; ok {
		chartID += fmt.Sprintf("-%s", source)
	}

	return chartID + "-chart"
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

// SendChartConfig sends chart configuration for a metric
func (s *SSEExporter) SendChartConfig(metricName string, config map[string]interface{}) error {
	chartID := fmt.Sprintf("obs-%s-chart", metricName)

	configEvent := map[string]interface{}{
		"id":    chartID,
		"type":  "line",
		"title": fmt.Sprintf("OBS: %s", metricName),
	}

	// Merge with provided config
	for k, v := range config {
		configEvent[k] = v
	}

	return s.broadcaster.BroadcastEvent("chart-config", configEvent)
}

// SendAlert sends an alert via SSE
func (s *SSEExporter) SendAlert(level obs.LogLevel, message string, details map[string]interface{}) error {
	alertEvent := map[string]interface{}{
		"type":      "alert",
		"level":     string(level),
		"message":   message,
		"details":   details,
		"timestamp": time.Now().Format(time.RFC3339),
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
