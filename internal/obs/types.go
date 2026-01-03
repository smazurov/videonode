package obs

import (
	"fmt"
	"time"
)

// DataType represents the type of observability data.
type DataType string

// DataType constants for metrics, logs, and traces.
const (
	DataTypeMetric DataType = "metric"
	DataTypeLog    DataType = "log"
	DataTypeTrace  DataType = "trace"
)

// LogLevel represents the severity level of a log entry.
type LogLevel string

// LogLevel constants for different severity levels.
const (
	LogLevelTrace LogLevel = "trace"
	LogLevelDebug LogLevel = "debug"
	LogLevelInfo  LogLevel = "info"
	LogLevelWarn  LogLevel = "warn"
	LogLevelError LogLevel = "error"
	LogLevelFatal LogLevel = "fatal"
)

// Labels represents key-value pairs for metadata.
type Labels map[string]string

// DataPoint is the generic interface for all observability data.
type DataPoint interface {
	Type() DataType
	Timestamp() time.Time
	Labels() Labels
	String() string
}

// MetricPoint represents a single metric measurement.
type MetricPoint struct {
	Name          string    `json:"name"`
	Value         float64   `json:"value"`
	LabelsMap     Labels    `json:"labels"`
	TimestampUnix time.Time `json:"timestamp"`
	Unit          string    `json:"unit,omitempty"`
}

// Type returns the data type for MetricPoint.
func (m *MetricPoint) Type() DataType {
	return DataTypeMetric
}

// Timestamp returns the timestamp for MetricPoint.
func (m *MetricPoint) Timestamp() time.Time {
	return m.TimestampUnix
}

// Labels returns the labels for MetricPoint.
func (m *MetricPoint) Labels() Labels {
	return m.LabelsMap
}

func (m *MetricPoint) String() string {
	return fmt.Sprintf("Metric{name=%s, value=%f, labels=%v, timestamp=%s}",
		m.Name, m.Value, m.LabelsMap, m.TimestampUnix.Format(time.RFC3339))
}

// LogEntry represents a single log entry.
type LogEntry struct {
	Message       string         `json:"message"`
	Level         LogLevel       `json:"level"`
	LabelsMap     Labels         `json:"labels"`
	Fields        map[string]any `json:"fields"`
	TimestampUnix time.Time      `json:"timestamp"`
	Source        string         `json:"source,omitempty"`
}

// Type returns the data type for LogEntry.
func (l *LogEntry) Type() DataType {
	return DataTypeLog
}

// Timestamp returns the timestamp for LogEntry.
func (l *LogEntry) Timestamp() time.Time {
	return l.TimestampUnix
}

// Labels returns the labels for LogEntry.
func (l *LogEntry) Labels() Labels {
	return l.LabelsMap
}

func (l *LogEntry) String() string {
	return fmt.Sprintf("Log{level=%s, message=%s, labels=%v, timestamp=%s}",
		l.Level, l.Message, l.LabelsMap, l.TimestampUnix.Format(time.RFC3339))
}

// SpanEntry represents a trace span (placeholder for future implementation).
type SpanEntry struct {
	TraceID       string        `json:"trace_id"`
	SpanID        string        `json:"span_id"`
	ParentID      string        `json:"parent_id,omitempty"`
	Operation     string        `json:"operation"`
	Duration      time.Duration `json:"duration"`
	LabelsMap     Labels        `json:"labels"`
	TimestampUnix time.Time     `json:"timestamp"`
}

// Type returns the data type for SpanEntry.
func (s *SpanEntry) Type() DataType {
	return DataTypeTrace
}

// Timestamp returns the timestamp for SpanEntry.
func (s *SpanEntry) Timestamp() time.Time {
	return s.TimestampUnix
}

// Labels returns the labels for SpanEntry.
func (s *SpanEntry) Labels() Labels {
	return s.LabelsMap
}

func (s *SpanEntry) String() string {
	return fmt.Sprintf("Span{trace_id=%s, operation=%s, duration=%s, timestamp=%s}",
		s.TraceID, s.Operation, s.Duration, s.TimestampUnix.Format(time.RFC3339))
}

// QueryOptions represents options for querying the store.
type QueryOptions struct {
	DataType   DataType      `json:"data_type"`
	Name       string        `json:"name,omitempty"`   // Metric name or log source
	Labels     Labels        `json:"labels,omitempty"` // Label filters
	Start      time.Time     `json:"start"`
	End        time.Time     `json:"end"`
	Limit      int           `json:"limit,omitempty"`      // Max number of results
	Aggregator string        `json:"aggregator,omitempty"` // For metrics: sum, avg, min, max, p99, etc.
	Step       time.Duration `json:"step,omitempty"`       // Aggregation window
}

// QueryResult represents the result of a query.
type QueryResult struct {
	DataType  DataType    `json:"data_type"`
	Name      string      `json:"name"`
	Labels    Labels      `json:"labels"`
	Points    []DataPoint `json:"points"`
	Total     int         `json:"total"`
	Truncated bool        `json:"truncated"`
}

// AggregatedPoint represents an aggregated metric value over a time window.
type AggregatedPoint struct {
	Timestamp time.Time `json:"timestamp"`
	Value     float64   `json:"value"`
	Count     int       `json:"count"`
}

// SeriesInfo represents metadata about a time series.
type SeriesInfo struct {
	Name       string    `json:"name"`
	DataType   DataType  `json:"data_type"`
	Labels     Labels    `json:"labels"`
	FirstSeen  time.Time `json:"first_seen"`
	LastSeen   time.Time `json:"last_seen"`
	PointCount int64     `json:"point_count"`
}

// StoreConfig represents configuration for the in-memory store.
type StoreConfig struct {
	MaxRetentionDuration time.Duration `json:"max_retention_duration"`
	MaxPointsPerSeries   int           `json:"max_points_per_series"`
	MaxSeries            int           `json:"max_series"`
	FlushInterval        time.Duration `json:"flush_interval"`
}

// DefaultStoreConfig returns a default configuration for the store.
func DefaultStoreConfig() StoreConfig {
	return StoreConfig{
		MaxRetentionDuration: 24 * time.Hour,
		MaxPointsPerSeries:   86400, // One point per second for 24 hours
		MaxSeries:            10000,
		FlushInterval:        30 * time.Second,
	}
}
