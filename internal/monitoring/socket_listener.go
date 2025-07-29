package monitoring

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// ProgressData represents FFmpeg progress information
type ProgressData struct {
	Frame      string    `json:"frame,omitempty"`
	FPS        string    `json:"fps,omitempty"`
	Bitrate    string    `json:"bitrate,omitempty"`
	TotalSize  string    `json:"total_size,omitempty"`
	OutTimeUs  string    `json:"out_time_us,omitempty"`
	OutTime    string    `json:"out_time,omitempty"`
	DupFrames  string    `json:"dup_frames,omitempty"`
	DropFrames string    `json:"drop_frames,omitempty"`
	Speed      string    `json:"speed,omitempty"`
	Progress   string    `json:"progress,omitempty"`
	Timestamp  time.Time `json:"timestamp"`
	StreamID   string    `json:"stream_id"`
}

// SocketListener listens for FFmpeg progress data on a Unix socket
type SocketListener struct {
	socketPath string
	streamID   string
	listener   net.Listener
	ctx        context.Context
	cancel     context.CancelFunc
	manager    *Manager // Reference to manager for metrics updates
}

// NewSocketListener creates a new socket listener for FFmpeg progress monitoring
func NewSocketListener(socketPath, streamID string, manager *Manager) *SocketListener {
	ctx, cancel := context.WithCancel(context.Background())

	return &SocketListener{
		socketPath: socketPath,
		streamID:   streamID,
		ctx:        ctx,
		cancel:     cancel,
		manager:    manager,
	}
}

// Start begins listening for FFmpeg progress data
func (sl *SocketListener) Start() error {
	// Check if socket file already exists - fail if it does
	if _, err := os.Stat(sl.socketPath); err == nil {
		return fmt.Errorf("socket file already exists: %s", sl.socketPath)
	}

	// Create Unix socket listener
	listener, err := net.Listen("unix", sl.socketPath)
	if err != nil {
		return fmt.Errorf("failed to create Unix socket listener: %w", err)
	}

	sl.listener = listener

	log.Printf("[MONITOR] Started FFmpeg progress listener for stream '%s' on socket: %s",
		sl.streamID, sl.socketPath)
	log.Printf("[MONITOR] Waiting for FFmpeg to connect...")

	// Start accepting connections in a goroutine
	go sl.acceptConnections()

	return nil
}

// Stop stops the socket listener and cleans up
func (sl *SocketListener) Stop() {
	if sl.cancel != nil {
		sl.cancel()
	}

	if sl.listener != nil {
		sl.listener.Close()
	}

	// Do NOT delete socket file - FFmpeg may still be sending data

	log.Printf("[MONITOR] Stopped FFmpeg progress listener for stream '%s'", sl.streamID)
}

// acceptConnections handles incoming connections to the Unix socket
func (sl *SocketListener) acceptConnections() {
	for {
		select {
		case <-sl.ctx.Done():
			log.Printf("[MONITOR] Accept loop stopping for stream '%s'", sl.streamID)
			return
		default:
			log.Printf("[MONITOR] Waiting for FFmpeg connection on stream '%s'...", sl.streamID)

			// Accept connection with timeout
			conn, err := sl.listener.Accept()
			if err != nil {
				select {
				case <-sl.ctx.Done():
					return
				default:
					log.Printf("[MONITOR] [%s] Error accepting connection on socket %s: %v", sl.streamID, sl.socketPath, err)
					continue
				}
			}

			// Log detailed connection info
			log.Printf("[MONITOR] [%s] FFmpeg connected to socket %s - Local: %s, Remote: %s",
				sl.streamID, sl.socketPath, conn.LocalAddr(), conn.RemoteAddr())

			// Handle connection synchronously - wait for it to finish before accepting another
			sl.handleConnection(conn)

			log.Printf("[MONITOR] [%s] Connection finished, ready for next connection on socket %s", sl.streamID, sl.socketPath)
		}
	}
}

// handleConnection processes data from an FFmpeg connection
func (sl *SocketListener) handleConnection(conn net.Conn) {
	connectionStart := time.Now()
	defer func() {
		conn.Close()
		duration := time.Since(connectionStart)
		log.Printf("[MONITOR] [%s] FFmpeg disconnected from socket %s after %v - Local: %s, Remote: %s",
			sl.streamID, sl.socketPath, duration, conn.LocalAddr(), conn.RemoteAddr())
	}()

	log.Printf("[MONITOR] [%s] Successfully reading FFmpeg progress data on socket %s", sl.streamID, sl.socketPath)

	scanner := bufio.NewScanner(conn)
	var currentProgress ProgressData

	for scanner.Scan() {
		select {
		case <-sl.ctx.Done():
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

					// Update progress data based on key
					sl.updateProgressData(&currentProgress, key, value)
				}
			}

			// Check if this is a complete progress update
			if strings.Contains(line, "progress=") {
				currentProgress.Timestamp = time.Now()
				currentProgress.StreamID = sl.streamID

				// Update Prometheus metrics
				if sl.manager != nil {
					sl.manager.updatePrometheusMetrics(currentProgress)
				}

				// Reset for next progress update
				currentProgress = ProgressData{}
			}
		}
	}

	if err := scanner.Err(); err != nil {
		log.Printf("[MONITOR] [%s] Error reading from socket %s: %v", sl.streamID, sl.socketPath, err)
	}

	log.Printf("[MONITOR] [%s] Scanner finished reading from socket %s", sl.streamID, sl.socketPath)
}

// updateProgressData updates the progress data structure with new key-value pair
func (sl *SocketListener) updateProgressData(progress *ProgressData, key, value string) {
	switch key {
	case "frame":
		progress.Frame = value
	case "fps":
		progress.FPS = value
	case "bitrate":
		progress.Bitrate = value
	case "total_size":
		progress.TotalSize = value
	case "out_time_us":
		progress.OutTimeUs = value
	case "out_time":
		progress.OutTime = value
	case "dup_frames":
		progress.DupFrames = value
	case "drop_frames":
		progress.DropFrames = value
	case "speed":
		progress.Speed = value
	case "progress":
		progress.Progress = value
	}
}

// updatePrometheusMetrics updates Prometheus metrics with progress data
func (m *Manager) updatePrometheusMetrics(progress ProgressData) {
	// Skip if metrics are not initialized
	if m.frameGauge == nil {
		return
	}

	streamID := progress.StreamID

	// Update frame number
	if progress.Frame != "" {
		if frame, err := strconv.ParseFloat(progress.Frame, 64); err == nil {
			m.frameGauge.WithLabelValues(streamID).Set(frame)
		}
	}

	// Update FPS
	if progress.FPS != "" {
		if fps, err := strconv.ParseFloat(progress.FPS, 64); err == nil {
			m.fpsGauge.WithLabelValues(streamID).Set(fps)
		}
	}

	// Update bitrate (remove "kbits/s" suffix and convert to float)
	if progress.Bitrate != "" {
		bitrateStr := strings.TrimSuffix(progress.Bitrate, "kbits/s")
		bitrateStr = strings.TrimSpace(bitrateStr)
		if bitrate, err := strconv.ParseFloat(bitrateStr, 64); err == nil {
			m.bitrateGauge.WithLabelValues(streamID).Set(bitrate)
		}
	}

	// Update total size
	if progress.TotalSize != "" {
		if totalSize, err := strconv.ParseFloat(progress.TotalSize, 64); err == nil {
			m.totalSizeGauge.WithLabelValues(streamID).Set(totalSize)
		}
	}

	// Update processing speed (remove "x" suffix)
	if progress.Speed != "" {
		speedStr := strings.TrimSuffix(progress.Speed, "x")
		speedStr = strings.TrimSpace(speedStr)
		if speed, err := strconv.ParseFloat(speedStr, 64); err == nil {
			m.speedGauge.WithLabelValues(streamID).Set(speed)
		}
	}

	// Update dropped frames
	if progress.DropFrames != "" {
		if dropFrames, err := strconv.ParseFloat(progress.DropFrames, 64); err == nil {
			m.dropFramesGauge.WithLabelValues(streamID).Set(dropFrames)
		}
	}
}

// Manager manages multiple socket listeners for different streams
type Manager struct {
	listeners map[string]*SocketListener
	// Prometheus metrics
	frameGauge      *prometheus.GaugeVec
	fpsGauge        *prometheus.GaugeVec
	bitrateGauge    *prometheus.GaugeVec
	totalSizeGauge  *prometheus.GaugeVec
	speedGauge      *prometheus.GaugeVec
	dropFramesGauge *prometheus.GaugeVec
	registry        *prometheus.Registry
}

// NewManager creates a new monitoring manager with a Prometheus registry
func NewManager() *Manager {
	return &Manager{
		listeners: make(map[string]*SocketListener),
	}
}

// NewManagerWithRegistry creates a new monitoring manager with the provided Prometheus registry
func NewManagerWithRegistry(registry *prometheus.Registry) *Manager {
	// Create Prometheus metrics with stream_id label
	frameGauge := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "ffmpeg_frame_number",
			Help: "Current frame number being processed by FFmpeg",
		},
		[]string{"stream_id"},
	)

	fpsGauge := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "ffmpeg_fps",
			Help: "Current frames per second from FFmpeg",
		},
		[]string{"stream_id"},
	)

	bitrateGauge := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "ffmpeg_bitrate_kbps",
			Help: "Current bitrate in kbps from FFmpeg",
		},
		[]string{"stream_id"},
	)

	totalSizeGauge := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "ffmpeg_total_size_bytes",
			Help: "Total size in bytes processed by FFmpeg",
		},
		[]string{"stream_id"},
	)

	speedGauge := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "ffmpeg_processing_speed",
			Help: "Processing speed multiplier from FFmpeg (e.g., 1.5x)",
		},
		[]string{"stream_id"},
	)

	dropFramesGauge := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "ffmpeg_dropped_frames",
			Help: "Number of dropped frames from FFmpeg",
		},
		[]string{"stream_id"},
	)

	// Register metrics with the provided registry
	registry.MustRegister(frameGauge, fpsGauge, bitrateGauge, totalSizeGauge, speedGauge, dropFramesGauge)

	return &Manager{
		listeners:       make(map[string]*SocketListener),
		frameGauge:      frameGauge,
		fpsGauge:        fpsGauge,
		bitrateGauge:    bitrateGauge,
		totalSizeGauge:  totalSizeGauge,
		speedGauge:      speedGauge,
		dropFramesGauge: dropFramesGauge,
		registry:        registry,
	}
}

// deleteMetricsForStream removes Prometheus metrics for a specific stream
func (m *Manager) deleteMetricsForStream(streamID string) {
	// Skip if metrics are not initialized
	if m.frameGauge == nil {
		return
	}

	m.frameGauge.DeleteLabelValues(streamID)
	m.fpsGauge.DeleteLabelValues(streamID)
	m.bitrateGauge.DeleteLabelValues(streamID)
	m.totalSizeGauge.DeleteLabelValues(streamID)
	m.speedGauge.DeleteLabelValues(streamID)
	m.dropFramesGauge.DeleteLabelValues(streamID)
}

// StartMonitoring starts monitoring for a stream
func (m *Manager) StartMonitoring(streamID, socketPath string) error {
	if _, exists := m.listeners[streamID]; exists {
		return fmt.Errorf("monitoring already active for stream: %s", streamID)
	}

	listener := NewSocketListener(socketPath, streamID, m)
	if err := listener.Start(); err != nil {
		return fmt.Errorf("failed to start monitoring for stream '%s': %w", streamID, err)
	}

	m.listeners[streamID] = listener
	return nil
}

// StopMonitoring stops monitoring for a stream
func (m *Manager) StopMonitoring(streamID string) {
	if listener, exists := m.listeners[streamID]; exists {
		listener.Stop()
		delete(m.listeners, streamID)

		// Clean up Prometheus metrics for this stream
		m.deleteMetricsForStream(streamID)
	}
}

// StopAll stops all active monitoring
func (m *Manager) StopAll() {
	for streamID, listener := range m.listeners {
		listener.Stop()

		// Clean up Prometheus metrics for this stream
		m.deleteMetricsForStream(streamID)

		delete(m.listeners, streamID)
	}
}

// GetActiveStreams returns list of streams being monitored
func (m *Manager) GetActiveStreams() []string {
	streams := make([]string, 0, len(m.listeners))
	for streamID := range m.listeners {
		streams = append(streams, streamID)
	}
	return streams
}
