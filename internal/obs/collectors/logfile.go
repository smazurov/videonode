package collectors

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/smazurov/videonode/internal/obs"
)

// LogFileCollector monitors log files for new entries
type LogFileCollector struct {
	*obs.BaseCollector
	filePath     string
	source       string
	levelRegexes map[obs.LogLevel]*regexp.Regexp
	lastSize     int64
	lastModTime  time.Time
}

// NewLogFileCollector creates a new log file collector
func NewLogFileCollector(filePath, source string) *LogFileCollector {
	config := obs.DefaultCollectorConfig(fmt.Sprintf("logfile_%s", filepath.Base(filePath)))
	config.Interval = 2 * time.Second // Check file every 2 seconds
	config.Labels = obs.Labels{
		"collector_type": "logfile",
		"file_path":      filePath,
		"source":         source,
	}
	config.Config = map[string]interface{}{
		"file_path": filePath,
		"source":    source,
	}

	// Default log level detection patterns
	levelRegexes := map[obs.LogLevel]*regexp.Regexp{
		obs.LogLevelFatal: regexp.MustCompile(`(?i)\b(fatal|panic|critical)\b`),
		obs.LogLevelError: regexp.MustCompile(`(?i)\b(error|err|failed|failure)\b`),
		obs.LogLevelWarn:  regexp.MustCompile(`(?i)\b(warn|warning|caution)\b`),
		obs.LogLevelInfo:  regexp.MustCompile(`(?i)\b(info|information)\b`),
		obs.LogLevelDebug: regexp.MustCompile(`(?i)\b(debug|trace|verbose)\b`),
	}

	return &LogFileCollector{
		BaseCollector: obs.NewBaseCollector(config.Name, config),
		filePath:      filePath,
		source:        source,
		levelRegexes:  levelRegexes,
	}
}

// Start begins monitoring the log file
func (l *LogFileCollector) Start(ctx context.Context, dataChan chan<- obs.DataPoint) error {
	l.SetRunning(true)

	// Initialize file state
	if err := l.initializeFileState(); err != nil {
		l.sendLog(dataChan, obs.LogLevelError, fmt.Sprintf("Failed to initialize file state: %v", err), time.Now())
		return err
	}

	l.sendLog(dataChan, obs.LogLevelInfo, fmt.Sprintf("Started monitoring log file: %s", l.filePath), time.Now())

	ticker := time.NewTicker(l.Interval())
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			l.checkFile(dataChan)

		case <-ctx.Done():
			l.SetRunning(false)
			l.sendLog(dataChan, obs.LogLevelInfo, fmt.Sprintf("Stopped monitoring log file: %s", l.filePath), time.Now())
			return nil

		case <-l.StopChan():
			l.SetRunning(false)
			l.sendLog(dataChan, obs.LogLevelInfo, fmt.Sprintf("Stopped monitoring log file: %s", l.filePath), time.Now())
			return nil
		}
	}
}

// initializeFileState gets the initial state of the file
func (l *LogFileCollector) initializeFileState() error {
	stat, err := os.Stat(l.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			// File doesn't exist yet, start with zero state
			l.lastSize = 0
			l.lastModTime = time.Time{}
			return nil
		}
		return err
	}

	l.lastSize = stat.Size()
	l.lastModTime = stat.ModTime()
	return nil
}

// checkFile checks for new content in the log file
func (l *LogFileCollector) checkFile(dataChan chan<- obs.DataPoint) {
	stat, err := os.Stat(l.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			// File was deleted, reset state
			l.lastSize = 0
			l.lastModTime = time.Time{}
			return
		}
		l.sendLog(dataChan, obs.LogLevelError, fmt.Sprintf("Failed to stat log file %s: %v", l.filePath, err), time.Now())
		return
	}

	currentSize := stat.Size()
	currentModTime := stat.ModTime()

	// Check if file was truncated (log rotation)
	if currentSize < l.lastSize {
		l.sendLog(dataChan, obs.LogLevelInfo, fmt.Sprintf("Log file %s was truncated (size: %d -> %d)", l.filePath, l.lastSize, currentSize), time.Now())
		l.lastSize = 0
	}

	// Check if file has new content
	if currentSize <= l.lastSize && currentModTime.Equal(l.lastModTime) {
		return // No new content
	}

	// Read new content
	l.readNewContent(dataChan, currentSize)

	// Update state
	l.lastSize = currentSize
	l.lastModTime = currentModTime
}

// readNewContent reads new content from the file
func (l *LogFileCollector) readNewContent(dataChan chan<- obs.DataPoint, currentSize int64) {
	file, err := os.Open(l.filePath)
	if err != nil {
		l.sendLog(dataChan, obs.LogLevelError, fmt.Sprintf("Failed to open log file %s: %v", l.filePath, err), time.Now())
		return
	}
	defer file.Close()

	// Seek to last read position
	if _, err := file.Seek(l.lastSize, 0); err != nil {
		l.sendLog(dataChan, obs.LogLevelError, fmt.Sprintf("Failed to seek in log file %s: %v", l.filePath, err), time.Now())
		return
	}

	scanner := bufio.NewScanner(file)
	timestamp := time.Now()
	lineCount := 0

	for scanner.Scan() {
		line := scanner.Text()
		l.processLogLine(dataChan, line, timestamp)
		lineCount++
	}

	if err := scanner.Err(); err != nil {
		l.sendLog(dataChan, obs.LogLevelError, fmt.Sprintf("Error scanning log file %s: %v", l.filePath, err), time.Now())
	}

	if lineCount > 0 {
		l.sendMetric(dataChan, "logfile_lines_processed_total", float64(lineCount), obs.Labels{"file": filepath.Base(l.filePath)}, timestamp)
	}
}

// processLogLine processes a single log line
func (l *LogFileCollector) processLogLine(dataChan chan<- obs.DataPoint, line string, timestamp time.Time) {
	line = strings.TrimSpace(line)
	if line == "" {
		return
	}

	// Detect log level
	level := l.detectLogLevel(line)

	// Extract timestamp from log line if possible
	logTimestamp := l.extractTimestamp(line)
	if logTimestamp.IsZero() {
		logTimestamp = timestamp
	}

	// Create base labels
	baseLabels := obs.Labels{
		"file":   filepath.Base(l.filePath),
		"source": l.source,
	}

	// Send log entry
	logEntry := &obs.LogEntry{
		Message:   line,
		Level:     level,
		LabelsMap: l.AddLabels(baseLabels),
		Fields: map[string]interface{}{
			"file_path": l.filePath,
			"raw_line":  line,
		},
		Timestamp_: logTimestamp,
		Source:     l.source,
	}

	select {
	case dataChan <- logEntry:
	default:
		// Channel full, skip this entry
	}

	// Send metrics based on log content
	l.sendLogMetrics(dataChan, line, level, baseLabels, timestamp)
}

// detectLogLevel detects the log level from the line content
func (l *LogFileCollector) detectLogLevel(line string) obs.LogLevel {
	// Check patterns in order of severity (highest to lowest)
	for _, level := range []obs.LogLevel{
		obs.LogLevelFatal,
		obs.LogLevelError,
		obs.LogLevelWarn,
		obs.LogLevelInfo,
		obs.LogLevelDebug,
	} {
		if regex, ok := l.levelRegexes[level]; ok && regex.MatchString(line) {
			return level
		}
	}

	// Check for explicit level indicators (e.g., "[ERROR]", "INFO:", etc.)
	upperLine := strings.ToUpper(line)
	switch {
	case strings.Contains(upperLine, "[FATAL]") || strings.Contains(upperLine, "FATAL:"):
		return obs.LogLevelFatal
	case strings.Contains(upperLine, "[ERROR]") || strings.Contains(upperLine, "ERROR:"):
		return obs.LogLevelError
	case strings.Contains(upperLine, "[WARN]") || strings.Contains(upperLine, "WARN:") ||
		strings.Contains(upperLine, "[WARNING]") || strings.Contains(upperLine, "WARNING:"):
		return obs.LogLevelWarn
	case strings.Contains(upperLine, "[INFO]") || strings.Contains(upperLine, "INFO:"):
		return obs.LogLevelInfo
	case strings.Contains(upperLine, "[DEBUG]") || strings.Contains(upperLine, "DEBUG:") ||
		strings.Contains(upperLine, "[TRACE]") || strings.Contains(upperLine, "TRACE:"):
		return obs.LogLevelDebug
	default:
		return obs.LogLevelInfo // Default to info level
	}
}

// extractTimestamp attempts to extract timestamp from log line
func (l *LogFileCollector) extractTimestamp(line string) time.Time {
	// Common timestamp patterns
	patterns := []string{
		"2006-01-02T15:04:05.000Z", // ISO 8601 with milliseconds
		"2006-01-02T15:04:05Z",     // ISO 8601
		"2006-01-02 15:04:05.000",  // Standard with milliseconds
		"2006-01-02 15:04:05",      // Standard
		"Jan 02 15:04:05",          // Syslog style
		"2006/01/02 15:04:05",      // Alternative format
		"15:04:05.000",             // Time only with milliseconds
		"15:04:05",                 // Time only
	}

	// Try to find timestamp at the beginning of the line
	words := strings.Fields(line)
	if len(words) == 0 {
		return time.Time{}
	}

	// Check first few words for timestamp patterns
	for i := 0; i < min(3, len(words)); i++ {
		candidate := strings.Join(words[:i+1], " ")

		for _, pattern := range patterns {
			if timestamp, err := time.Parse(pattern, candidate); err == nil {
				return timestamp
			}
		}
	}

	return time.Time{}
}

// sendLogMetrics sends metrics based on log content
func (l *LogFileCollector) sendLogMetrics(dataChan chan<- obs.DataPoint, line string, level obs.LogLevel, baseLabels obs.Labels, timestamp time.Time) {
	// Count log entries by level
	levelLabels := obs.Labels{}
	for k, v := range baseLabels {
		levelLabels[k] = v
	}
	levelLabels["level"] = string(level)

	l.sendMetric(dataChan, "logfile_entries_total", 1, levelLabels, timestamp)

	// Count specific error patterns
	lowerLine := strings.ToLower(line)

	if strings.Contains(lowerLine, "exception") {
		l.sendMetric(dataChan, "logfile_exceptions_total", 1, baseLabels, timestamp)
	}

	if strings.Contains(lowerLine, "timeout") {
		l.sendMetric(dataChan, "logfile_timeouts_total", 1, baseLabels, timestamp)
	}

	if strings.Contains(lowerLine, "connection") && (strings.Contains(lowerLine, "failed") || strings.Contains(lowerLine, "error")) {
		l.sendMetric(dataChan, "logfile_connection_errors_total", 1, baseLabels, timestamp)
	}

	if strings.Contains(lowerLine, "out of memory") || strings.Contains(lowerLine, "oom") {
		l.sendMetric(dataChan, "logfile_memory_errors_total", 1, baseLabels, timestamp)
	}
}

// SetLevelRegex sets a custom regex pattern for a specific log level
func (l *LogFileCollector) SetLevelRegex(level obs.LogLevel, pattern string) error {
	regex, err := regexp.Compile(pattern)
	if err != nil {
		return fmt.Errorf("invalid regex pattern for level %s: %w", level, err)
	}
	l.levelRegexes[level] = regex
	return nil
}

// Helper methods

func (l *LogFileCollector) sendMetric(dataChan chan<- obs.DataPoint, name string, value float64, labels obs.Labels, timestamp time.Time) {
	point := &obs.MetricPoint{
		Name:       name,
		Value:      value,
		LabelsMap:  l.AddLabels(labels),
		Timestamp_: timestamp,
		Unit:       "count",
	}

	select {
	case dataChan <- point:
	default:
		// Channel full, skip this point
	}
}

func (l *LogFileCollector) sendLog(dataChan chan<- obs.DataPoint, level obs.LogLevel, message string, timestamp time.Time) {
	point := &obs.LogEntry{
		Message:    message,
		Level:      level,
		LabelsMap:  l.AddLabels(obs.Labels{"source": "logfile_collector"}),
		Fields:     make(map[string]interface{}),
		Timestamp_: timestamp,
		Source:     "logfile_collector",
	}

	select {
	case dataChan <- point:
	default:
		// Channel full, skip this point
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
