package exporters

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/smazurov/videonode/internal/obs"
)

// DynamicMetric stores information about a metric with dynamic labels
type DynamicMetric struct {
	Name        string
	Help        string
	Type        prometheus.ValueType
	Value       float64
	LabelNames  []string
	LabelValues []string
	Timestamp   time.Time
}

// DynamicCollector implements prometheus.Collector for dynamic metrics
type DynamicCollector struct {
	mutex      sync.RWMutex
	metrics    map[string]*DynamicMetric // key is metric_name + sorted label pairs
	maxMetrics int                       // Ring buffer size
}

// NewDynamicCollector creates a new dynamic collector
func NewDynamicCollector() *DynamicCollector {
	return &DynamicCollector{
		metrics:    make(map[string]*DynamicMetric),
		maxMetrics: 1000, // Ring buffer limit
	}
}

// Describe implements prometheus.Collector
// We return nothing here since our metrics are dynamic
func (d *DynamicCollector) Describe(ch chan<- *prometheus.Desc) {
	// Dynamic metrics don't pre-declare their descriptors
}

// Collect implements prometheus.Collector
func (d *DynamicCollector) Collect(ch chan<- prometheus.Metric) {
	d.mutex.RLock()
	defer d.mutex.RUnlock()

	for _, metric := range d.metrics {
		desc := prometheus.NewDesc(
			metric.Name,
			metric.Help,
			metric.LabelNames,
			nil, // ConstLabels
		)

		m, err := prometheus.NewConstMetric(
			desc,
			metric.Type,
			metric.Value,
			metric.LabelValues...,
		)
		if err != nil {
			// Log error but continue with other metrics
			continue
		}

		ch <- m
	}
}

// UpdateMetric updates or adds a metric
func (d *DynamicCollector) UpdateMetric(name string, value float64, labels map[string]string, metricType prometheus.ValueType) {
	d.mutex.Lock()
	defer d.mutex.Unlock()

	// Create a unique key for this metric using only stable labels
	key := d.createMetricKey(name, labels)

	// Extract label names and values in sorted order
	labelNames, labelValues := d.extractLabels(labels)

	// Check if we need to cleanup old metrics (ring buffer behavior)
	if len(d.metrics) >= d.maxMetrics {
		d.cleanupOldMetrics()
	}

	d.metrics[key] = &DynamicMetric{
		Name:        name,
		Help:        fmt.Sprintf("Observability metric: %s", name),
		Type:        metricType,
		Value:       value,
		LabelNames:  labelNames,
		LabelValues: labelValues,
		Timestamp:   time.Now(),
	}
}

// cleanupOldMetrics removes oldest metrics to maintain ring buffer
func (d *DynamicCollector) cleanupOldMetrics() {
	if len(d.metrics) < d.maxMetrics {
		return
	}

	// Remove oldest 20% of metrics
	targetSize := int(float64(d.maxMetrics) * 0.8)
	toRemove := len(d.metrics) - targetSize

	// Sort by timestamp and remove oldest
	type metricEntry struct {
		key       string
		timestamp time.Time
	}

	var entries []metricEntry
	for key, metric := range d.metrics {
		entries = append(entries, metricEntry{key, metric.Timestamp})
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].timestamp.Before(entries[j].timestamp)
	})

	// Remove oldest entries
	for i := 0; i < toRemove && i < len(entries); i++ {
		delete(d.metrics, entries[i].key)
	}
}

// createMetricKey creates a unique key for a metric with its stable labels only
func (d *DynamicCollector) createMetricKey(name string, labels map[string]string) string {
	if len(labels) == 0 {
		return name
	}

	// Only use stable labels for metric identity
	stableLabels := []string{
		"stream_id", "collector", "service", "instance", "name",
		"interface", "host", "endpoint", "path", "prometheus_endpoint",
		"device", // Added for MPP and other device-specific metrics
	}

	var pairs []string
	for _, key := range stableLabels {
		if value, exists := labels[key]; exists {
			pairs = append(pairs, fmt.Sprintf("%s=%s", key, value))
		}
	}

	if len(pairs) == 0 {
		return name
	}

	sort.Strings(pairs)
	return fmt.Sprintf("%s{%s}", name, strings.Join(pairs, ","))
}

// extractLabels extracts label names and values in sorted order
func (d *DynamicCollector) extractLabels(labels map[string]string) ([]string, []string) {
	if len(labels) == 0 {
		return []string{}, []string{}
	}

	var names []string
	for name := range labels {
		names = append(names, name)
	}
	sort.Strings(names)

	values := make([]string, len(names))
	for i, name := range names {
		values[i] = labels[name]
	}

	return names, values
}

// PromExporter exports observability data in Prometheus format using dynamic collector
type PromExporter struct {
	config    obs.ExporterConfig
	registry  *prometheus.Registry
	collector *DynamicCollector
	handler   http.Handler
	buffer    []obs.DataPoint
	bufferMux sync.Mutex
}

// NewPromExporter creates a new Prometheus exporter with dynamic metrics support
func NewPromExporter() *PromExporter {
	registry := prometheus.NewRegistry()
	collector := NewDynamicCollector()

	// Register our dynamic collector
	registry.MustRegister(collector)

	config := obs.ExporterConfig{
		Name:          "prometheus",
		Enabled:       true,
		BufferSize:    10000,
		FlushInterval: 30 * time.Second,
		Config:        make(map[string]interface{}),
	}

	return &PromExporter{
		config:    config,
		registry:  registry,
		collector: collector,
		handler:   promhttp.HandlerFor(registry, promhttp.HandlerOpts{}),
		buffer:    make([]obs.DataPoint, 0, config.BufferSize),
	}
}

// Name returns the exporter name
func (p *PromExporter) Name() string {
	return p.config.Name
}

// Config returns the exporter configuration
func (p *PromExporter) Config() obs.ExporterConfig {
	return p.config
}

// Start starts the Prometheus exporter
func (p *PromExporter) Start(ctx context.Context) error {
	// Prometheus exporter doesn't need a background process
	return nil
}

// Stop stops the Prometheus exporter
func (p *PromExporter) Stop() error {
	// Nothing to stop
	return nil
}

// Export processes and exports data points
func (p *PromExporter) Export(points []obs.DataPoint) error {
	if len(points) == 0 {
		return nil
	}

	// Buffer points for batch processing
	p.bufferMux.Lock()
	defer p.bufferMux.Unlock()

	for _, point := range points {
		if len(p.buffer) >= p.config.BufferSize {
			// Buffer full, process it
			p.processBuffer()
		}
		p.buffer = append(p.buffer, point)
	}

	return nil
}

// processBuffer processes buffered data points
func (p *PromExporter) processBuffer() {
	if len(p.buffer) == 0 {
		return
	}

	for _, point := range p.buffer {
		// Only process metric points for Prometheus export
		metricPoint, ok := point.(*obs.MetricPoint)
		if !ok {
			continue
		}

		// Filter out Prometheus retransmission to prevent loops
		if p.isPrometheusRetransmission(metricPoint) {
			continue
		}

		// Determine metric type
		metricType := p.determineMetricType(metricPoint.Name)

		// Update the metric in our dynamic collector
		p.collector.UpdateMetric(
			p.sanitizeMetricName(metricPoint.Name),
			metricPoint.Value,
			metricPoint.Labels(),
			metricType,
		)
	}

	// Clear buffer
	p.buffer = p.buffer[:0]
}

// determineMetricType determines the Prometheus metric type based on the name
func (p *PromExporter) determineMetricType(name string) prometheus.ValueType {
	lowerName := strings.ToLower(name)

	// Check if it's a counter
	counterPatterns := []string{
		"_total",
		"_count",
		"_errors",
		"_requests",
		"_bytes_received",
		"_bytes_sent",
		"_packets",
	}

	for _, pattern := range counterPatterns {
		if strings.Contains(lowerName, pattern) {
			return prometheus.CounterValue
		}
	}

	// Default to gauge
	return prometheus.GaugeValue
}

// sanitizeMetricName ensures the metric name is valid for Prometheus
func (p *PromExporter) sanitizeMetricName(name string) string {
	// Replace invalid characters with underscores
	result := strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' {
			return r
		}
		return '_'
	}, name)

	// Ensure it starts with a letter or underscore
	if len(result) > 0 && result[0] >= '0' && result[0] <= '9' {
		result = "_" + result
	}

	// Add obs prefix to avoid conflicts
	if !strings.HasPrefix(result, "obs_") {
		result = "obs_" + result
	}

	return result
}

// GetHandler returns the HTTP handler for serving Prometheus metrics
func (p *PromExporter) GetHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Process any buffered data before serving
		p.bufferMux.Lock()
		p.processBuffer()
		p.bufferMux.Unlock()

		// Serve metrics
		p.handler.ServeHTTP(w, r)
	})
}

// ForceFlush processes all buffered data immediately
func (p *PromExporter) ForceFlush() {
	p.bufferMux.Lock()
	defer p.bufferMux.Unlock()
	p.processBuffer()
}

// isPrometheusRetransmission checks if a metric comes from a Prometheus scraper
// to prevent retransmission loops
func (p *PromExporter) isPrometheusRetransmission(metric *obs.MetricPoint) bool {
	labels := metric.Labels()

	// Check for Prometheus collector labels
	if collectorType, exists := labels["collector_type"]; exists && collectorType == "prometheus" {
		return true
	}

	// Check for prometheus_endpoint label (added by Prometheus collector)
	if _, exists := labels["prometheus_endpoint"]; exists {
		return true
	}

	// Check for prometheus_type label
	if _, exists := labels["prometheus_type"]; exists {
		return true
	}

	// Check if source indicates this came from prometheus_collector
	if source, exists := labels["source"]; exists && source == "prometheus_collector" {
		return true
	}

	return false
}

// Stats returns statistics about the exporter
func (p *PromExporter) Stats() map[string]interface{} {
	p.bufferMux.Lock()
	bufferSize := len(p.buffer)
	p.bufferMux.Unlock()

	return map[string]interface{}{
		"name":            p.config.Name,
		"enabled":         p.config.Enabled,
		"metrics_count":   len(p.collector.metrics),
		"buffer_size":     bufferSize,
		"buffer_capacity": p.config.BufferSize,
	}
}
