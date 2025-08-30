package collectors

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/smazurov/videonode/internal/obs"
)

// FFmpegCollector collects FFmpeg progress data and logs
type FFmpegCollector struct {
	*obs.BaseCollector
	socketPath string
	logPath    string
	streamID   string
	listener   net.Listener
	cancelFunc context.CancelFunc
	stopOnce   sync.Once
}

// NewFFmpegCollector creates a new FFmpeg collector
func NewFFmpegCollector(socketPath, logPath, streamID string) *FFmpegCollector {
	config := obs.DefaultCollectorConfig("ffmpeg")
	config.Interval = 0 // Event-driven, not time-based
	config.Labels = obs.Labels{
		"collector_type": "ffmpeg",
		"stream_id":      streamID,
	}
	config.Config = map[string]interface{}{
		"socket_path": socketPath,
		"log_path":    logPath,
		"stream_id":   streamID,
	}

	return &FFmpegCollector{
		BaseCollector: obs.NewBaseCollector(fmt.Sprintf("ffmpeg_%s", streamID), config),
		socketPath:    socketPath,
		logPath:       logPath,
		streamID:      streamID,
	}
}

// Start begins collecting FFmpeg data
func (f *FFmpegCollector) Start(ctx context.Context, dataChan chan<- obs.DataPoint) error {
	log.Printf("OBS: Starting FFmpeg collector for stream '%s' with socket: %s", f.streamID, f.socketPath)
	f.SetRunning(true)

	// Create a cancellable context for this collector
	collectorCtx, cancel := context.WithCancel(ctx)
	f.cancelFunc = cancel

	// Start socket listener for progress data
	if f.socketPath != "" {
		go f.startSocketListener(collectorCtx, dataChan)
	}

	// Start log file monitoring
	if f.logPath != "" {
		go f.startLogMonitoring(collectorCtx, dataChan)
	}

	// Wait for context cancellation
	<-collectorCtx.Done()
	f.SetRunning(false)
	return nil
}

// startSocketListener starts listening for FFmpeg progress on Unix socket
func (f *FFmpegCollector) startSocketListener(ctx context.Context, dataChan chan<- obs.DataPoint) {
	log.Printf("FFmpeg collector: Starting to listen on socket %s for stream %s", f.socketPath, f.streamID)

	// Clean up any existing socket file (could be stale from previous run)
	if err := os.Remove(f.socketPath); err != nil && !os.IsNotExist(err) {
		log.Printf("FFmpeg collector: Failed to clean up old socket file %s: %v", f.socketPath, err)
		// Continue anyway - the Listen call will fail if there's a real problem
	}

	// Create Unix socket listener
	listener, err := net.Listen("unix", f.socketPath)
	if err != nil {
		log.Printf("FFmpeg collector ERROR: Failed to create Unix socket listener for %s: %v", f.socketPath, err)
		return
	}

	log.Printf("FFmpeg collector: Successfully created listener on socket %s for stream %s", f.socketPath, f.streamID)
	f.listener = listener
	defer func() {
		listener.Close()
		// Clean up socket file when we're done
		os.Remove(f.socketPath)
		f.sendLog(dataChan, obs.LogLevelInfo, fmt.Sprintf("Cleaned up socket file: %s", f.socketPath), time.Now())
	}()

	f.sendLog(dataChan, obs.LogLevelInfo, fmt.Sprintf("Started FFmpeg progress listener for stream '%s' on socket: %s", f.streamID, f.socketPath), time.Now())

	// Create a channel to signal when Accept should stop
	acceptDone := make(chan struct{})
	defer close(acceptDone)

	// Accept connections loop
	for {
		// Check if context is done before trying to accept
		select {
		case <-ctx.Done():
			f.sendLog(dataChan, obs.LogLevelInfo, fmt.Sprintf("Socket listener stopping for stream '%s' - context cancelled", f.streamID), time.Now())
			return
		default:
		}

		// Set a deadline on the listener to periodically check context
		if tcpListener, ok := listener.(*net.UnixListener); ok {
			tcpListener.SetDeadline(time.Now().Add(1 * time.Second))
		}

		f.sendLog(dataChan, obs.LogLevelDebug, fmt.Sprintf("Waiting for FFmpeg connection on stream '%s'...", f.streamID), time.Now())

		// Accept connection
		conn, err := listener.Accept()
		if err != nil {
			// Check if it's a timeout error
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				// Timeout is expected, just continue to check context
				continue
			}

			// Check if context was cancelled
			select {
			case <-ctx.Done():
				f.sendLog(dataChan, obs.LogLevelInfo, fmt.Sprintf("Socket listener stopping for stream '%s' - accept interrupted", f.streamID), time.Now())
				return
			default:
				// Check if it's because the listener was closed
				if strings.Contains(err.Error(), "use of closed network connection") {
					f.sendLog(dataChan, obs.LogLevelInfo, fmt.Sprintf("Socket listener closed for stream '%s'", f.streamID), time.Now())
					return
				}
				f.sendLog(dataChan, obs.LogLevelError, fmt.Sprintf("Error accepting connection on socket %s: %v", f.socketPath, err), time.Now())
				continue
			}
		}

		f.sendLog(dataChan, obs.LogLevelInfo, fmt.Sprintf("FFmpeg connected to socket %s - Local: %s, Remote: %s", f.socketPath, conn.LocalAddr(), conn.RemoteAddr()), time.Now())

		// Handle connection in a goroutine
		go f.handleConnection(ctx, conn, dataChan)
	}
}

// handleConnection processes data from an FFmpeg connection
func (f *FFmpegCollector) handleConnection(ctx context.Context, conn net.Conn, dataChan chan<- obs.DataPoint) {
	connectionStart := time.Now()
	defer func() {
		conn.Close()
		duration := time.Since(connectionStart)
		f.sendLog(dataChan, obs.LogLevelInfo, fmt.Sprintf("FFmpeg disconnected from socket %s after %v - Local: %s, Remote: %s", f.socketPath, duration, conn.LocalAddr(), conn.RemoteAddr()), time.Now())
	}()

	f.sendLog(dataChan, obs.LogLevelInfo, fmt.Sprintf("Successfully reading FFmpeg progress data on socket %s", f.socketPath), time.Now())

	scanner := bufio.NewScanner(conn)
	progressData := make(map[string]string)

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return
		default:
			line := strings.TrimSpace(scanner.Text())
			if line == "" {
				continue
			}

			// Parse key-value pairs
			if strings.Contains(line, "=") {
				parts := strings.SplitN(line, "=", 2)
				if len(parts) == 2 {
					key := strings.TrimSpace(parts[0])
					value := strings.TrimSpace(parts[1])
					progressData[key] = value
				}
			}

			// Check if this is a complete progress update
			if strings.Contains(line, "progress=") {
				timestamp := time.Now()
				f.sendProgressMetrics(dataChan, progressData, timestamp)

				// Reset for next progress update
				progressData = make(map[string]string)
			}
		}
	}

	if err := scanner.Err(); err != nil {
		f.sendLog(dataChan, obs.LogLevelError, fmt.Sprintf("Error reading from socket %s: %v", f.socketPath, err), time.Now())
	}

	f.sendLog(dataChan, obs.LogLevelDebug, fmt.Sprintf("Scanner finished reading from socket %s", f.socketPath), time.Now())
}

// sendProgressMetrics converts FFmpeg progress data to metrics
func (f *FFmpegCollector) sendProgressMetrics(dataChan chan<- obs.DataPoint, progressData map[string]string, timestamp time.Time) {
	// Extract and clean values
	fpsStr := progressData["fps"]
	dropFramesStr := progressData["drop_frames"]
	dupFramesStr := progressData["dup_frames"]
	speedStr := progressData["speed"]

	// Clean speed (remove "x" suffix)
	if speedStr != "" {
		speedStr = strings.TrimSuffix(speedStr, "x")
		speedStr = strings.TrimSpace(speedStr)
	}

	// Send consolidated stream metrics
	streamMetrics := &obs.MetricPoint{
		Name:  "ffmpeg_stream_metrics",
		Value: 1.0, // Indicates stream is active
		LabelsMap: map[string]string{
			"stream_id":        f.streamID,
			"fps":              fpsStr,
			"dropped_frames":   dropFramesStr,
			"duplicate_frames": dupFramesStr,
			"processing_speed": speedStr,
		},
		Timestamp_: timestamp,
	}

	select {
	case dataChan <- streamMetrics:
		// Successfully sent stream metrics
	default:
		log.Printf("FFmpeg: WARNING - Data channel full, metrics dropped!")
	}
}

// startLogMonitoring monitors FFmpeg log file for errors and warnings
func (f *FFmpegCollector) startLogMonitoring(ctx context.Context, dataChan chan<- obs.DataPoint) {
	// This is a simplified implementation - for production use, consider using a proper log tailing library
	if _, err := os.Stat(f.logPath); os.IsNotExist(err) {
		f.sendLog(dataChan, obs.LogLevelWarn, fmt.Sprintf("FFmpeg log file does not exist: %s", f.logPath), time.Now())
		return
	}

	f.sendLog(dataChan, obs.LogLevelInfo, fmt.Sprintf("Starting FFmpeg log monitoring for: %s", f.logPath), time.Now())

	// For this implementation, we'll check the log file periodically
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	var lastSize int64 = 0

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			f.checkLogFile(dataChan, &lastSize)
		}
	}
}

// checkLogFile checks for new content in the log file
func (f *FFmpegCollector) checkLogFile(dataChan chan<- obs.DataPoint, lastSize *int64) {
	file, err := os.Open(f.logPath)
	if err != nil {
		f.sendLog(dataChan, obs.LogLevelError, fmt.Sprintf("Failed to open log file %s: %v", f.logPath, err), time.Now())
		return
	}
	defer file.Close()

	stat, err := file.Stat()
	if err != nil {
		f.sendLog(dataChan, obs.LogLevelError, fmt.Sprintf("Failed to stat log file %s: %v", f.logPath, err), time.Now())
		return
	}

	currentSize := stat.Size()
	if currentSize <= *lastSize {
		return // No new content
	}

	// Seek to last read position
	file.Seek(*lastSize, 0)

	scanner := bufio.NewScanner(file)
	timestamp := time.Now()

	for scanner.Scan() {
		line := scanner.Text()
		f.parseLogLine(dataChan, line, timestamp)
	}

	*lastSize = currentSize
}

// parseLogLine parses a single log line and extracts relevant information
func (f *FFmpegCollector) parseLogLine(dataChan chan<- obs.DataPoint, line string, timestamp time.Time) {
	line = strings.TrimSpace(line)
	if line == "" {
		return
	}

	baseLabels := obs.Labels{
		"stream_id": f.streamID,
		"source":    "ffmpeg_log",
	}

	// Determine log level based on content
	var level obs.LogLevel
	switch {
	case strings.Contains(strings.ToLower(line), "error"):
		level = obs.LogLevelError
	case strings.Contains(strings.ToLower(line), "warning"):
		level = obs.LogLevelWarn
	case strings.Contains(strings.ToLower(line), "info"):
		level = obs.LogLevelInfo
	default:
		level = obs.LogLevelDebug
	}

	// Send log entry
	logEntry := &obs.LogEntry{
		Message:    line,
		Level:      level,
		LabelsMap:  f.AddLabels(baseLabels),
		Fields:     map[string]interface{}{"raw_line": line},
		Timestamp_: timestamp,
		Source:     "ffmpeg_log",
	}

	select {
	case dataChan <- logEntry:
	default:
		// Channel full, skip this entry
	}

	// Extract metrics from log lines if possible
	f.extractMetricsFromLogLine(dataChan, line, timestamp, baseLabels)
}

// extractMetricsFromLogLine extracts metrics from log content
func (f *FFmpegCollector) extractMetricsFromLogLine(dataChan chan<- obs.DataPoint, line string, timestamp time.Time, baseLabels obs.Labels) {
	// Example: Extract encoding parameters, errors, etc.

	// Count error messages
	if strings.Contains(strings.ToLower(line), "error") {
		f.sendMetric(dataChan, "ffmpeg_log_errors_total", 1, baseLabels, timestamp)
	}

	// Count warning messages
	if strings.Contains(strings.ToLower(line), "warning") {
		f.sendMetric(dataChan, "ffmpeg_log_warnings_total", 1, baseLabels, timestamp)
	}

	// Extract frame drops/skips from log
	if strings.Contains(strings.ToLower(line), "dropping") || strings.Contains(strings.ToLower(line), "skipping") {
		f.sendMetric(dataChan, "ffmpeg_log_drops_total", 1, baseLabels, timestamp)
	}
}

// Stop stops the FFmpeg collector
func (f *FFmpegCollector) Stop() error {
	var stopErr error

	// Use sync.Once to ensure we only stop once
	f.stopOnce.Do(func() {
		// Cancel the context first to stop all goroutines
		if f.cancelFunc != nil {
			f.cancelFunc()
		}

		// Close the listener if it exists
		if f.listener != nil {
			f.listener.Close()
			f.listener = nil
		}

		// Call base Stop
		stopErr = f.BaseCollector.Stop()
	})

	return stopErr
}

// Helper methods

func (f *FFmpegCollector) sendMetric(dataChan chan<- obs.DataPoint, name string, value float64, labels obs.Labels, timestamp time.Time) {
	point := &obs.MetricPoint{
		Name:       name,
		Value:      value,
		LabelsMap:  f.AddLabels(labels),
		Timestamp_: timestamp,
		Unit:       f.getMetricUnit(name),
	}

	select {
	case dataChan <- point:
	default:
		// Channel full, skip this point
	}
}

func (f *FFmpegCollector) sendLog(dataChan chan<- obs.DataPoint, level obs.LogLevel, message string, timestamp time.Time) {
	point := &obs.LogEntry{
		Message:    message,
		Level:      level,
		LabelsMap:  f.AddLabels(obs.Labels{"source": "ffmpeg_collector"}),
		Fields:     make(map[string]interface{}),
		Timestamp_: timestamp,
		Source:     "ffmpeg_collector",
	}

	select {
	case dataChan <- point:
	default:
		// Channel full, skip this point
	}
}

func (f *FFmpegCollector) getMetricUnit(name string) string {
	switch {
	case strings.Contains(name, "_total"):
		return "count"
	default:
		return ""
	}
}
