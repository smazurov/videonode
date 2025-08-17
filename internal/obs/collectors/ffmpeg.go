package collectors

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"os"
	"strconv"
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
	// Check if socket file already exists - fail if it does
	if _, err := os.Stat(f.socketPath); err == nil {
		f.sendLog(dataChan, obs.LogLevelError, fmt.Sprintf("Socket file already exists: %s", f.socketPath), time.Now())
		return
	}

	// Create Unix socket listener
	listener, err := net.Listen("unix", f.socketPath)
	if err != nil {
		f.sendLog(dataChan, obs.LogLevelError, fmt.Sprintf("Failed to create Unix socket listener: %v", err), time.Now())
		return
	}

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
	baseLabels := obs.Labels{
		"stream_id": f.streamID,
		"source":    "ffmpeg_progress",
	}

	// Send frame number
	if frameStr, ok := progressData["frame"]; ok && frameStr != "" {
		if frame, err := strconv.ParseFloat(frameStr, 64); err == nil {
			f.sendMetric(dataChan, "ffmpeg_frame_number", frame, baseLabels, timestamp)
		}
	}

	// Send FPS
	if fpsStr, ok := progressData["fps"]; ok && fpsStr != "" {
		if fps, err := strconv.ParseFloat(fpsStr, 64); err == nil {
			f.sendMetric(dataChan, "ffmpeg_fps", fps, baseLabels, timestamp)
		}
	}

	// Send bitrate (remove "kbits/s" suffix)
	if bitrateStr, ok := progressData["bitrate"]; ok && bitrateStr != "" {
		cleanBitrate := strings.TrimSuffix(bitrateStr, "kbits/s")
		cleanBitrate = strings.TrimSpace(cleanBitrate)
		if bitrate, err := strconv.ParseFloat(cleanBitrate, 64); err == nil {
			f.sendMetric(dataChan, "ffmpeg_bitrate_kbps", bitrate, baseLabels, timestamp)
		}
	}

	// Send total size
	if totalSizeStr, ok := progressData["total_size"]; ok && totalSizeStr != "" {
		if totalSize, err := strconv.ParseFloat(totalSizeStr, 64); err == nil {
			f.sendMetric(dataChan, "ffmpeg_total_size_bytes", totalSize, baseLabels, timestamp)
		}
	}

	// Send processing speed (remove "x" suffix)
	if speedStr, ok := progressData["speed"]; ok && speedStr != "" {
		cleanSpeed := strings.TrimSuffix(speedStr, "x")
		cleanSpeed = strings.TrimSpace(cleanSpeed)
		if speed, err := strconv.ParseFloat(cleanSpeed, 64); err == nil {
			f.sendMetric(dataChan, "ffmpeg_processing_speed", speed, baseLabels, timestamp)
		}
	}

	// Send dropped frames
	if dropFramesStr, ok := progressData["drop_frames"]; ok && dropFramesStr != "" {
		if dropFrames, err := strconv.ParseFloat(dropFramesStr, 64); err == nil {
			f.sendMetric(dataChan, "ffmpeg_dropped_frames", dropFrames, baseLabels, timestamp)
		}
	}

	// Send duplicate frames
	if dupFramesStr, ok := progressData["dup_frames"]; ok && dupFramesStr != "" {
		if dupFrames, err := strconv.ParseFloat(dupFramesStr, 64); err == nil {
			f.sendMetric(dataChan, "ffmpeg_duplicate_frames", dupFrames, baseLabels, timestamp)
		}
	}

	// Send out time (duration processed)
	if outTimeStr, ok := progressData["out_time"]; ok && outTimeStr != "" {
		if duration, err := f.parseFFmpegDuration(outTimeStr); err == nil {
			f.sendMetric(dataChan, "ffmpeg_out_time_seconds", duration.Seconds(), baseLabels, timestamp)
		}
	}

	// Send progress status
	if progressStr, ok := progressData["progress"]; ok && progressStr != "" {
		var progressValue float64
		switch progressStr {
		case "continue":
			progressValue = 1
		case "end":
			progressValue = 0
		default:
			progressValue = -1 // Unknown status
		}
		f.sendMetric(dataChan, "ffmpeg_progress_status", progressValue, baseLabels, timestamp)
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

// parseFFmpegDuration parses FFmpeg duration format (HH:MM:SS.mmm)
func (f *FFmpegCollector) parseFFmpegDuration(durationStr string) (time.Duration, error) {
	// Handle format like "00:01:23.45"
	parts := strings.Split(durationStr, ":")
	if len(parts) != 3 {
		return 0, fmt.Errorf("invalid duration format: %s", durationStr)
	}

	hours, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, err
	}

	minutes, err := strconv.Atoi(parts[1])
	if err != nil {
		return 0, err
	}

	seconds, err := strconv.ParseFloat(parts[2], 64)
	if err != nil {
		return 0, err
	}

	total := time.Duration(hours)*time.Hour +
		time.Duration(minutes)*time.Minute +
		time.Duration(seconds*float64(time.Second))

	return total, nil
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
	case strings.Contains(name, "_bytes"):
		return "bytes"
	case strings.Contains(name, "_kbps"):
		return "kbps"
	case strings.Contains(name, "_fps"):
		return "fps"
	case strings.Contains(name, "_frames"):
		return "count"
	case strings.Contains(name, "_seconds"):
		return "seconds"
	case strings.Contains(name, "_total"):
		return "count"
	case strings.Contains(name, "_speed"):
		return "ratio"
	case strings.Contains(name, "_status"):
		return "enum"
	default:
		return ""
	}
}
