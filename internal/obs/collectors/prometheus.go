package collectors

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/smazurov/videonode/internal/obs"
)

// PrometheusCollector scrapes metrics from Prometheus-compatible endpoints
type PrometheusCollector struct {
	*obs.BaseCollector
	endpoint     string
	client       *http.Client
	metricFilter *regexp.Regexp
	labelFilters map[string]*regexp.Regexp
}

// PrometheusMetric represents a parsed Prometheus metric
type PrometheusMetric struct {
	Name      string
	Value     float64
	Labels    map[string]string
	Timestamp time.Time
	Help      string
	Type      string
}

// NewPrometheusCollector creates a new Prometheus scraper collector
func NewPrometheusCollector(endpoint string) *PrometheusCollector {
	config := obs.DefaultCollectorConfig("prometheus")
	config.Interval = 15 * time.Second // Scrape every 15 seconds
	config.Labels = obs.Labels{
		"collector_type": "prometheus",
		"endpoint":       endpoint,
	}
	config.Timeout = 10 * time.Second
	config.Config = map[string]interface{}{
		"endpoint": endpoint,
	}

	return &PrometheusCollector{
		BaseCollector: obs.NewBaseCollector(config.Name, config),
		endpoint:      endpoint,
		client: &http.Client{
			Timeout: config.Timeout,
		},
		labelFilters: make(map[string]*regexp.Regexp),
	}
}

// SetMetricFilter sets a regex filter for metric names (only matching metrics will be collected)
func (p *PrometheusCollector) SetMetricFilter(pattern string) error {
	if pattern == "" {
		p.metricFilter = nil
		return nil
	}

	regex, err := regexp.Compile(pattern)
	if err != nil {
		return fmt.Errorf("invalid metric filter pattern: %w", err)
	}
	p.metricFilter = regex
	return nil
}

// SetLabelFilter sets a regex filter for a specific label (only metrics with matching label values will be collected)
func (p *PrometheusCollector) SetLabelFilter(labelName, pattern string) error {
	if pattern == "" {
		delete(p.labelFilters, labelName)
		return nil
	}

	regex, err := regexp.Compile(pattern)
	if err != nil {
		return fmt.Errorf("invalid label filter pattern for %s: %w", labelName, err)
	}
	p.labelFilters[labelName] = regex
	return nil
}

// Start begins scraping metrics from the Prometheus endpoint
func (p *PrometheusCollector) Start(ctx context.Context, dataChan chan<- obs.DataPoint) error {
	p.SetRunning(true)

	p.sendLog(dataChan, obs.LogLevelInfo, fmt.Sprintf("Starting Prometheus scraper for endpoint: %s", p.endpoint), time.Now())

	// Perform initial scrape with context
	p.scrapeMetricsWithContext(ctx, dataChan)

	ticker := time.NewTicker(p.Interval())
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			// Check if context is done before scraping
			select {
			case <-ctx.Done():
				p.SetRunning(false)
				p.sendLog(dataChan, obs.LogLevelInfo, fmt.Sprintf("Stopped Prometheus scraper for endpoint: %s", p.endpoint), time.Now())
				return nil
			default:
				p.scrapeMetricsWithContext(ctx, dataChan)
			}

		case <-ctx.Done():
			p.SetRunning(false)
			p.sendLog(dataChan, obs.LogLevelInfo, fmt.Sprintf("Stopped Prometheus scraper for endpoint: %s", p.endpoint), time.Now())
			return nil

		case <-p.StopChan():
			p.SetRunning(false)
			p.sendLog(dataChan, obs.LogLevelInfo, fmt.Sprintf("Stopped Prometheus scraper for endpoint: %s", p.endpoint), time.Now())
			return nil
		}
	}
}

// scrapeMetricsWithContext performs a single scrape of the Prometheus endpoint with context
func (p *PrometheusCollector) scrapeMetricsWithContext(ctx context.Context, dataChan chan<- obs.DataPoint) {
	scrapeStart := time.Now()

	req, err := http.NewRequestWithContext(ctx, "GET", p.endpoint, nil)
	if err != nil {
		p.sendLog(dataChan, obs.LogLevelError, fmt.Sprintf("Failed to create request: %v", err), time.Now())
		return
	}

	resp, err := p.client.Do(req)
	if err != nil {
		p.sendLog(dataChan, obs.LogLevelError, fmt.Sprintf("Failed to scrape %s: %v", p.endpoint, err), time.Now())
		p.sendScrapeMetrics(dataChan, scrapeStart, 0, false)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		p.sendLog(dataChan, obs.LogLevelError, fmt.Sprintf("Scrape failed with status %d: %s", resp.StatusCode, p.endpoint), time.Now())
		p.sendScrapeMetrics(dataChan, scrapeStart, 0, false)
		return
	}

	// Parse metrics
	metrics, err := p.parsePrometheusResponse(resp.Body)
	if err != nil {
		p.sendLog(dataChan, obs.LogLevelError, fmt.Sprintf("Failed to parse Prometheus response: %v", err), time.Now())
		p.sendScrapeMetrics(dataChan, scrapeStart, 0, false)
		return
	}

	// Send metrics to data channel
	sentCount := 0
	timestamp := time.Now()

	for _, metric := range metrics {
		if p.shouldIncludeMetric(metric) {
			p.sendPrometheusMetric(dataChan, metric, timestamp)
			sentCount++
		}
	}

	p.sendScrapeMetrics(dataChan, scrapeStart, sentCount, true)

	if sentCount > 0 {
		p.sendLog(dataChan, obs.LogLevelDebug, fmt.Sprintf("Scraped %d metrics from %s in %v", sentCount, p.endpoint, time.Since(scrapeStart)), time.Now())
	}
}

// parsePrometheusResponse parses Prometheus text format
func (p *PrometheusCollector) parsePrometheusResponse(body io.Reader) ([]PrometheusMetric, error) {
	var metrics []PrometheusMetric
	scanner := bufio.NewScanner(body)

	var currentHelp, currentType string

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines and comments (except HELP and TYPE)
		if line == "" {
			continue
		}

		if strings.HasPrefix(line, "#") {
			if strings.HasPrefix(line, "# HELP ") {
				parts := strings.SplitN(line[7:], " ", 2)
				if len(parts) == 2 {
					currentHelp = parts[1]
				}
			} else if strings.HasPrefix(line, "# TYPE ") {
				parts := strings.SplitN(line[7:], " ", 2)
				if len(parts) == 2 {
					currentType = parts[1]
				}
			}
			continue
		}

		// Parse metric line
		metric, err := p.parseMetricLine(line, currentHelp, currentType)
		if err != nil {
			// Log but continue processing other metrics
			continue
		}

		metrics = append(metrics, metric)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading response: %w", err)
	}

	return metrics, nil
}

// parseMetricLine parses a single metric line
func (p *PrometheusCollector) parseMetricLine(line, help, metricType string) (PrometheusMetric, error) {
	// Find the metric name and labels
	var name string
	var labels map[string]string
	var valueStr string
	var timestamp time.Time

	// Check for labels
	if strings.Contains(line, "{") {
		// Format: metric_name{label1="value1",label2="value2"} value [timestamp]
		openIdx := strings.Index(line, "{")
		closeIdx := strings.Index(line, "}")

		if closeIdx == -1 || closeIdx < openIdx {
			return PrometheusMetric{}, fmt.Errorf("malformed metric line: %s", line)
		}

		name = strings.TrimSpace(line[:openIdx])
		labelsStr := line[openIdx+1 : closeIdx]
		valueAndTimestamp := strings.TrimSpace(line[closeIdx+1:])

		// Parse labels
		var err error
		labels, err = p.parseLabels(labelsStr)
		if err != nil {
			return PrometheusMetric{}, fmt.Errorf("failed to parse labels: %w", err)
		}

		// Parse value and optional timestamp
		valueStr, timestamp = p.parseValueAndTimestamp(valueAndTimestamp)
	} else {
		// Format: metric_name value [timestamp]
		parts := strings.Fields(line)
		if len(parts) < 2 {
			return PrometheusMetric{}, fmt.Errorf("invalid metric line format: %s", line)
		}

		name = parts[0]
		labels = make(map[string]string)
		valueStr, timestamp = p.parseValueAndTimestamp(strings.Join(parts[1:], " "))
	}

	// Parse value
	value, err := strconv.ParseFloat(valueStr, 64)
	if err != nil {
		return PrometheusMetric{}, fmt.Errorf("invalid metric value: %s", valueStr)
	}

	if timestamp.IsZero() {
		timestamp = time.Now()
	}

	return PrometheusMetric{
		Name:      name,
		Value:     value,
		Labels:    labels,
		Timestamp: timestamp,
		Help:      help,
		Type:      metricType,
	}, nil
}

// parseLabels parses label string like: label1="value1",label2="value2"
func (p *PrometheusCollector) parseLabels(labelsStr string) (map[string]string, error) {
	labels := make(map[string]string)

	if strings.TrimSpace(labelsStr) == "" {
		return labels, nil
	}

	// Simple regex to match label pairs
	labelRegex := regexp.MustCompile(`(\w+)="([^"]*)"`)
	matches := labelRegex.FindAllStringSubmatch(labelsStr, -1)

	for _, match := range matches {
		if len(match) == 3 {
			labels[match[1]] = match[2]
		}
	}

	return labels, nil
}

// parseValueAndTimestamp parses value and optional timestamp
func (p *PrometheusCollector) parseValueAndTimestamp(valueStr string) (string, time.Time) {
	parts := strings.Fields(valueStr)
	if len(parts) == 0 {
		return "", time.Time{}
	}

	value := parts[0]

	if len(parts) > 1 {
		// Try to parse timestamp (Unix milliseconds)
		if ts, err := strconv.ParseInt(parts[1], 10, 64); err == nil {
			return value, time.Unix(ts/1000, (ts%1000)*1000000)
		}
	}

	return value, time.Time{}
}

// shouldIncludeMetric checks if a metric should be included based on filters
func (p *PrometheusCollector) shouldIncludeMetric(metric PrometheusMetric) bool {
	// Check metric name filter
	if p.metricFilter != nil && !p.metricFilter.MatchString(metric.Name) {
		return false
	}

	// Check label filters
	for labelName, regex := range p.labelFilters {
		labelValue, exists := metric.Labels[labelName]
		if !exists || !regex.MatchString(labelValue) {
			return false
		}
	}

	return true
}

// sendPrometheusMetric sends a Prometheus metric as an obs metric
func (p *PrometheusCollector) sendPrometheusMetric(dataChan chan<- obs.DataPoint, metric PrometheusMetric, timestamp time.Time) {
	// Merge metric labels with collector labels
	allLabels := make(obs.Labels)
	for k, v := range metric.Labels {
		allLabels[k] = v
	}
	allLabels["prometheus_endpoint"] = p.endpoint
	if metric.Type != "" {
		allLabels["prometheus_type"] = metric.Type
	}

	point := &obs.MetricPoint{
		Name:       metric.Name,
		Value:      metric.Value,
		LabelsMap:  p.AddLabels(allLabels),
		Timestamp_: metric.Timestamp,
		Unit:       p.getMetricUnit(metric.Name, metric.Type),
	}

	select {
	case dataChan <- point:
	default:
		// Channel full, skip this point
	}
}

// sendScrapeMetrics sends metrics about the scrape operation itself
func (p *PrometheusCollector) sendScrapeMetrics(dataChan chan<- obs.DataPoint, scrapeStart time.Time, metricCount int, success bool) {
	// Scrape metadata metrics removed - not reported via SSE
}

// getMetricUnit determines the unit for a metric based on name and type
func (p *PrometheusCollector) getMetricUnit(name, metricType string) string {
	switch metricType {
	case "counter":
		return "count"
	case "gauge":
		// Try to infer from name
		switch {
		case strings.Contains(name, "_bytes"):
			return "bytes"
		case strings.Contains(name, "_seconds"):
			return "seconds"
		case strings.Contains(name, "_percent"):
			return "percent"
		case strings.Contains(name, "_ratio"):
			return "ratio"
		default:
			return ""
		}
	case "histogram":
		if strings.HasSuffix(name, "_bucket") {
			return "count"
		} else if strings.HasSuffix(name, "_sum") {
			return ""
		} else if strings.HasSuffix(name, "_count") {
			return "count"
		}
		return ""
	case "summary":
		if strings.HasSuffix(name, "_sum") {
			return ""
		} else if strings.HasSuffix(name, "_count") {
			return "count"
		}
		return ""
	default:
		// Try to infer from name
		switch {
		case strings.Contains(name, "_bytes"):
			return "bytes"
		case strings.Contains(name, "_seconds"):
			return "seconds"
		case strings.Contains(name, "_total"):
			return "count"
		case strings.Contains(name, "_percent"):
			return "percent"
		default:
			return ""
		}
	}
}

// Helper methods

func (p *PrometheusCollector) sendLog(dataChan chan<- obs.DataPoint, level obs.LogLevel, message string, timestamp time.Time) {
	point := &obs.LogEntry{
		Message:    message,
		Level:      level,
		LabelsMap:  p.AddLabels(obs.Labels{"source": "prometheus_collector"}),
		Fields:     make(map[string]interface{}),
		Timestamp_: timestamp,
		Source:     "prometheus_collector",
	}

	select {
	case dataChan <- point:
	default:
		// Channel full, skip this point
	}
}
