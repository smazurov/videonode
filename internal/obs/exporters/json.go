package exporters

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/smazurov/videonode/internal/obs"
)

// JSONExporter provides HTTP JSON API endpoints for querying observability data
type JSONExporter struct {
	config    obs.ExporterConfig
	store     *obs.Store
	server    *http.Server
	handler   *http.ServeMux
	ctx       context.Context
	cancel    context.CancelFunc
	running   bool
	runningMu sync.RWMutex
}

// NewJSONExporter creates a new JSON API exporter
func NewJSONExporter(store *obs.Store, bindAddr string) *JSONExporter {
	config := obs.ExporterConfig{
		Name:          "json_api",
		Enabled:       true,
		BufferSize:    0, // Not applicable for JSON API
		FlushInterval: 0, // Not applicable for JSON API
		Config: map[string]interface{}{
			"bind_addr": bindAddr,
		},
	}

	ctx, cancel := context.WithCancel(context.Background())

	handler := http.NewServeMux()
	server := &http.Server{
		Addr:    bindAddr,
		Handler: handler,
	}

	exporter := &JSONExporter{
		config:  config,
		store:   store,
		server:  server,
		handler: handler,
		ctx:     ctx,
		cancel:  cancel,
	}

	// Register API routes
	exporter.registerRoutes()

	return exporter
}

// Name returns the exporter name
func (j *JSONExporter) Name() string {
	return j.config.Name
}

// Config returns the exporter configuration
func (j *JSONExporter) Config() obs.ExporterConfig {
	return j.config
}

// Start starts the JSON API server
func (j *JSONExporter) Start(ctx context.Context) error {
	j.runningMu.Lock()
	defer j.runningMu.Unlock()

	if j.running {
		return fmt.Errorf("JSON API exporter already running")
	}

	go func() {
		if err := j.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			fmt.Printf("JSON API server error: %v\n", err)
		}
	}()

	j.running = true
	return nil
}

// Stop stops the JSON API server
func (j *JSONExporter) Stop() error {
	j.runningMu.Lock()
	defer j.runningMu.Unlock()

	if !j.running {
		return nil
	}

	j.cancel()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := j.server.Shutdown(shutdownCtx)
	j.running = false
	return err
}

// Export is not applicable for JSON API exporter (it serves data on demand)
func (j *JSONExporter) Export(points []obs.DataPoint) error {
	// JSON API doesn't need to export points - it queries the store directly
	return nil
}

// registerRoutes registers HTTP routes for the JSON API
func (j *JSONExporter) registerRoutes() {
	// Query endpoints
	j.handler.HandleFunc("/obs/query", j.handleQuery)
	j.handler.HandleFunc("/obs/series", j.handleListSeries)
	j.handler.HandleFunc("/obs/stats", j.handleStats)

	// Specific data type endpoints
	j.handler.HandleFunc("/obs/metrics", j.handleMetrics)
	j.handler.HandleFunc("/obs/logs", j.handleLogs)

	// Health check
	j.handler.HandleFunc("/obs/health", j.handleHealth)

	// CORS middleware for all routes
	originalHandler := j.handler
	j.handler = http.NewServeMux()
	j.handler.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		j.enableCORS(w, r)
		originalHandler.ServeHTTP(w, r)
	})
}

// enableCORS adds CORS headers
func (j *JSONExporter) enableCORS(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusOK)
		return
	}
}

// handleQuery handles general query requests
func (j *JSONExporter) handleQuery(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" && r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse query options
	opts, err := j.parseQueryOptions(r)
	if err != nil {
		j.writeError(w, "Invalid query parameters", err, http.StatusBadRequest)
		return
	}

	// Execute query
	result, err := j.store.Query(opts)
	if err != nil {
		j.writeError(w, "Query failed", err, http.StatusInternalServerError)
		return
	}

	j.writeJSON(w, result)
}

// handleListSeries handles series listing requests
func (j *JSONExporter) handleListSeries(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	series := j.store.ListSeries()

	response := map[string]interface{}{
		"series": series,
		"count":  len(series),
	}

	j.writeJSON(w, response)
}

// handleStats handles statistics requests
func (j *JSONExporter) handleStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	stats := j.store.Stats()
	j.writeJSON(w, stats)
}

// handleMetrics handles metrics-specific queries
func (j *JSONExporter) handleMetrics(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse query options with metric type pre-set
	opts, err := j.parseQueryOptions(r)
	if err != nil {
		j.writeError(w, "Invalid query parameters", err, http.StatusBadRequest)
		return
	}
	opts.DataType = obs.DataTypeMetric

	// Execute query
	result, err := j.store.Query(opts)
	if err != nil {
		j.writeError(w, "Query failed", err, http.StatusInternalServerError)
		return
	}

	// Format metrics for easier consumption
	response := j.formatMetricsResponse(result)
	j.writeJSON(w, response)
}

// handleLogs handles logs-specific queries
func (j *JSONExporter) handleLogs(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse query options with log type pre-set
	opts, err := j.parseQueryOptions(r)
	if err != nil {
		j.writeError(w, "Invalid query parameters", err, http.StatusBadRequest)
		return
	}
	opts.DataType = obs.DataTypeLog

	// Execute query
	result, err := j.store.Query(opts)
	if err != nil {
		j.writeError(w, "Query failed", err, http.StatusInternalServerError)
		return
	}

	// Format logs for easier consumption
	response := j.formatLogsResponse(result)
	j.writeJSON(w, response)
}

// handleHealth handles health check requests
func (j *JSONExporter) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	health := map[string]interface{}{
		"status":    "healthy",
		"timestamp": time.Now().Format(time.RFC3339),
		"exporter":  j.config.Name,
	}

	j.writeJSON(w, health)
}

// parseQueryOptions parses HTTP request parameters into query options
func (j *JSONExporter) parseQueryOptions(r *http.Request) (obs.QueryOptions, error) {
	var opts obs.QueryOptions

	// Parse data type
	if dataTypeStr := r.URL.Query().Get("type"); dataTypeStr != "" {
		opts.DataType = obs.DataType(dataTypeStr)
	}

	// Parse name
	opts.Name = r.URL.Query().Get("name")

	// Parse time range
	if startStr := r.URL.Query().Get("start"); startStr != "" {
		start, err := time.Parse(time.RFC3339, startStr)
		if err != nil {
			return opts, fmt.Errorf("invalid start time: %w", err)
		}
		opts.Start = start
	}

	if endStr := r.URL.Query().Get("end"); endStr != "" {
		end, err := time.Parse(time.RFC3339, endStr)
		if err != nil {
			return opts, fmt.Errorf("invalid end time: %w", err)
		}
		opts.End = end
	}

	// Default time range if not specified (last hour)
	if opts.Start.IsZero() && opts.End.IsZero() {
		opts.End = time.Now()
		opts.Start = opts.End.Add(-1 * time.Hour)
	}

	// Parse limit
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		limit, err := strconv.Atoi(limitStr)
		if err != nil {
			return opts, fmt.Errorf("invalid limit: %w", err)
		}
		opts.Limit = limit
	}

	// Parse aggregator
	opts.Aggregator = r.URL.Query().Get("aggregator")

	// Parse step
	if stepStr := r.URL.Query().Get("step"); stepStr != "" {
		step, err := time.ParseDuration(stepStr)
		if err != nil {
			return opts, fmt.Errorf("invalid step: %w", err)
		}
		opts.Step = step
	}

	// Parse labels
	opts.Labels = make(obs.Labels)
	for key, values := range r.URL.Query() {
		if key != "type" && key != "name" && key != "start" && key != "end" &&
			key != "limit" && key != "aggregator" && key != "step" && len(values) > 0 {
			opts.Labels[key] = values[0]
		}
	}

	return opts, nil
}

// formatMetricsResponse formats query result for metrics API
func (j *JSONExporter) formatMetricsResponse(result *obs.QueryResult) map[string]interface{} {
	var metrics []map[string]interface{}

	for _, point := range result.Points {
		if metricPoint, ok := point.(*obs.MetricPoint); ok {
			metric := map[string]interface{}{
				"name":      metricPoint.Name,
				"value":     metricPoint.Value,
				"unit":      metricPoint.Unit,
				"labels":    metricPoint.Labels(),
				"timestamp": metricPoint.Timestamp().Format(time.RFC3339),
			}
			metrics = append(metrics, metric)
		}
	}

	return map[string]interface{}{
		"data_type": result.DataType,
		"name":      result.Name,
		"labels":    result.Labels,
		"metrics":   metrics,
		"total":     result.Total,
		"truncated": result.Truncated,
		"timestamp": time.Now().Format(time.RFC3339),
	}
}

// formatLogsResponse formats query result for logs API
func (j *JSONExporter) formatLogsResponse(result *obs.QueryResult) map[string]interface{} {
	var logs []map[string]interface{}
	logsByLevel := make(map[string]int)

	for _, point := range result.Points {
		if logEntry, ok := point.(*obs.LogEntry); ok {
			log := map[string]interface{}{
				"level":     string(logEntry.Level),
				"message":   logEntry.Message,
				"source":    logEntry.Source,
				"labels":    logEntry.Labels(),
				"fields":    logEntry.Fields,
				"timestamp": logEntry.Timestamp().Format(time.RFC3339),
			}
			logs = append(logs, log)

			// Count by level
			levelStr := string(logEntry.Level)
			logsByLevel[levelStr]++
		}
	}

	return map[string]interface{}{
		"data_type": result.DataType,
		"name":      result.Name,
		"labels":    result.Labels,
		"logs":      logs,
		"by_level":  logsByLevel,
		"total":     result.Total,
		"truncated": result.Truncated,
		"timestamp": time.Now().Format(time.RFC3339),
	}
}

// Helper methods

func (j *JSONExporter) writeJSON(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")

	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")

	if err := encoder.Encode(data); err != nil {
		http.Error(w, "Failed to encode JSON", http.StatusInternalServerError)
	}
}

func (j *JSONExporter) writeError(w http.ResponseWriter, message string, err error, statusCode int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	errorResponse := map[string]interface{}{
		"error":     message,
		"timestamp": time.Now().Format(time.RFC3339),
	}

	if err != nil {
		errorResponse["details"] = err.Error()
	}

	json.NewEncoder(w).Encode(errorResponse)
}

// GetBindAddr returns the bind address for the API server
func (j *JSONExporter) GetBindAddr() string {
	if addr, ok := j.config.Config["bind_addr"].(string); ok {
		return addr
	}
	return ":8091" // Default
}

// Stats returns statistics about the exporter
func (j *JSONExporter) Stats() map[string]interface{} {
	j.runningMu.RLock()
	running := j.running
	j.runningMu.RUnlock()

	return map[string]interface{}{
		"name":      j.config.Name,
		"enabled":   j.config.Enabled,
		"running":   running,
		"bind_addr": j.GetBindAddr(),
	}
}
