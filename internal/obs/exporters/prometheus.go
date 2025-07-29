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

// PrometheusExporter exports observability data in Prometheus format
type PrometheusExporter struct {
	config     obs.ExporterConfig
	registry   *prometheus.Registry
	metrics    map[string]prometheus.Collector
	metricsMux sync.RWMutex
	handler    http.Handler
	buffer     []obs.DataPoint
	bufferMux  sync.Mutex
}

// NewPrometheusExporter creates a new Prometheus exporter
func NewPrometheusExporter(registry *prometheus.Registry) *PrometheusExporter {
	if registry == nil {
		registry = prometheus.NewRegistry()
	}

	config := obs.ExporterConfig{
		Name:          "prometheus",
		Enabled:       true,
		BufferSize:    10000,
		FlushInterval: 30 * time.Second,
		Config:        make(map[string]interface{}),
	}

	return &PrometheusExporter{
		config:   config,
		registry: registry,
		metrics:  make(map[string]prometheus.Collector),
		handler:  promhttp.HandlerFor(registry, promhttp.HandlerOpts{}),
		buffer:   make([]obs.DataPoint, 0, config.BufferSize),
	}
}

// Name returns the exporter name
func (p *PrometheusExporter) Name() string {
	return p.config.Name
}

// Config returns the exporter configuration
func (p *PrometheusExporter) Config() obs.ExporterConfig {
	return p.config
}

// Start starts the Prometheus exporter
func (p *PrometheusExporter) Start(ctx context.Context) error {
	// Prometheus exporter doesn't need a background process - it serves metrics on demand
	return nil
}

// Stop stops the Prometheus exporter
func (p *PrometheusExporter) Stop() error {
	// Clean up metrics
	p.metricsMux.Lock()
	defer p.metricsMux.Unlock()

	for name, metric := range p.metrics {
		p.registry.Unregister(metric)
		delete(p.metrics, name)
	}

	return nil
}

// Export processes and exports data points
func (p *PrometheusExporter) Export(points []obs.DataPoint) error {
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
func (p *PrometheusExporter) processBuffer() {
	if len(p.buffer) == 0 {
		return
	}

	// Group metrics by name and labels
	metricGroups := p.groupMetrics(p.buffer)

	// Update Prometheus metrics
	for metricKey, points := range metricGroups {
		p.updatePrometheusMetric(metricKey, points)
	}

	// Clear buffer
	p.buffer = p.buffer[:0]
}

// metricKey represents a unique metric identifier
type metricKey struct {
	name   string
	labels string // sorted label string for consistent grouping
}

// groupMetrics groups data points by metric name and labels
func (p *PrometheusExporter) groupMetrics(points []obs.DataPoint) map[metricKey][]obs.DataPoint {
	groups := make(map[metricKey][]obs.DataPoint)

	for _, point := range points {
		// Only process metric points for Prometheus export
		metricPoint, ok := point.(*obs.MetricPoint)
		if !ok {
			continue
		}

		key := metricKey{
			name:   metricPoint.Name,
			labels: p.labelsToString(metricPoint.Labels()),
		}

		groups[key] = append(groups[key], point)
	}

	return groups
}

// labelsToString converts labels to a consistent string representation
func (p *PrometheusExporter) labelsToString(labels obs.Labels) string {
	if len(labels) == 0 {
		return ""
	}

	var pairs []string
	for k, v := range labels {
		pairs = append(pairs, fmt.Sprintf("%s=%s", k, v))
	}
	sort.Strings(pairs)
	return strings.Join(pairs, ",")
}

// updatePrometheusMetric updates or creates a Prometheus metric
func (p *PrometheusExporter) updatePrometheusMetric(key metricKey, points []obs.DataPoint) {
	if len(points) == 0 {
		return
	}

	// Get the latest point for current value
	latestPoint := points[len(points)-1].(*obs.MetricPoint)

	p.metricsMux.Lock()
	defer p.metricsMux.Unlock()

	// Check if we already have this metric
	metricID := fmt.Sprintf("%s_%s", key.name, key.labels)
	existingMetric, exists := p.metrics[metricID]

	if !exists {
		// Create new metric
		metric := p.createPrometheusMetric(latestPoint)
		if metric != nil {
			p.registry.MustRegister(metric)
			p.metrics[metricID] = metric
		}
	} else {
		// Update existing metric
		p.updateExistingMetric(existingMetric, latestPoint)
	}
}

// createPrometheusMetric creates a new Prometheus metric based on the data point
func (p *PrometheusExporter) createPrometheusMetric(point *obs.MetricPoint) prometheus.Collector {
	name := p.sanitizeMetricName(point.Name)
	help := fmt.Sprintf("Observability metric: %s", point.Name)

	// Extract label names and values
	labelNames, labelValues := p.extractLabels(point.Labels())

	// Determine metric type based on name patterns
	if p.isCounterMetric(point.Name) {
		// Create CounterVec
		counter := prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: name,
			Help: help,
		}, labelNames)

		// Set initial value
		counter.WithLabelValues(labelValues...).Add(point.Value)
		return counter
	} else {
		// Create GaugeVec (default)
		gauge := prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: name,
			Help: help,
		}, labelNames)

		// Set value
		gauge.WithLabelValues(labelValues...).Set(point.Value)
		return gauge
	}
}

// updateExistingMetric updates an existing Prometheus metric
func (p *PrometheusExporter) updateExistingMetric(metric prometheus.Collector, point *obs.MetricPoint) {
	// Extract label values
	_, labelValues := p.extractLabels(point.Labels())

	switch m := metric.(type) {
	case *prometheus.CounterVec:
		// For counters, we need to calculate the delta
		// This is simplified - in production, you'd want to track previous values
		m.WithLabelValues(labelValues...).Add(0) // Add 0 to ensure the metric exists

	case *prometheus.GaugeVec:
		m.WithLabelValues(labelValues...).Set(point.Value)
	}
}

// isCounterMetric determines if a metric should be a counter based on its name
func (p *PrometheusExporter) isCounterMetric(name string) bool {
	counterPatterns := []string{
		"_total",
		"_count",
		"_errors",
		"_requests",
		"_bytes_received",
		"_bytes_sent",
		"_packets",
	}

	lowerName := strings.ToLower(name)
	for _, pattern := range counterPatterns {
		if strings.Contains(lowerName, pattern) {
			return true
		}
	}

	return false
}

// sanitizeMetricName ensures the metric name is valid for Prometheus
func (p *PrometheusExporter) sanitizeMetricName(name string) string {
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

// extractLabels extracts label names and values from the labels map
func (p *PrometheusExporter) extractLabels(labels obs.Labels) ([]string, []string) {
	if len(labels) == 0 {
		return []string{}, []string{}
	}

	var names []string
	var values []string

	// Sort label names for consistency
	for name := range labels {
		names = append(names, name)
	}
	sort.Strings(names)

	// Get values in the same order
	for _, name := range names {
		values = append(values, labels[name])
	}

	return names, values
}

// GetHandler returns the HTTP handler for serving Prometheus metrics
func (p *PrometheusExporter) GetHandler() http.Handler {
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
func (p *PrometheusExporter) ForceFlush() {
	p.bufferMux.Lock()
	defer p.bufferMux.Unlock()
	p.processBuffer()
}

// GetMetricsText returns the current metrics in Prometheus text format
func (p *PrometheusExporter) GetMetricsText() (string, error) {
	// This would require implementing a custom text formatter
	// For now, return a placeholder
	return "# Prometheus metrics from obs package\n", nil
}

// Stats returns statistics about the exporter
func (p *PrometheusExporter) Stats() map[string]interface{} {
	p.metricsMux.RLock()
	defer p.metricsMux.RUnlock()

	p.bufferMux.Lock()
	bufferSize := len(p.buffer)
	p.bufferMux.Unlock()

	return map[string]interface{}{
		"name":            p.config.Name,
		"enabled":         p.config.Enabled,
		"metrics_count":   len(p.metrics),
		"buffer_size":     bufferSize,
		"buffer_capacity": p.config.BufferSize,
	}
}
