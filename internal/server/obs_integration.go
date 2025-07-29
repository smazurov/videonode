package server

import (
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/smazurov/videonode/internal/obs"
	"github.com/smazurov/videonode/internal/obs/collectors"
	"github.com/smazurov/videonode/internal/obs/exporters"
	"github.com/smazurov/videonode/internal/sse"
)

// Global obs manager instance
var globalObsManager *obs.Manager

// SSEBroadcasterAdapter adapts the existing SSE manager to work with obs
type SSEBroadcasterAdapter struct {
	sseManager *sse.Manager
}

func (s *SSEBroadcasterAdapter) BroadcastEvent(eventType string, data interface{}) error {
	return s.sseManager.BroadcastCustomEvent(eventType, data)
}

// setupForVideoNode creates a Manager configured specifically for VideoNode
func setupForVideoNode(promRegistry *prometheus.Registry, sseBroadcaster exporters.SSEBroadcaster) (*obs.Manager, error) {
	// Create manager config with production settings
	config := obs.ManagerConfig{
		StoreConfig: obs.StoreConfig{
			MaxRetentionDuration: 6 * time.Hour,
			MaxPointsPerSeries:   21600, // 6 hours at 1 point per second
			MaxSeries:            3000,
			FlushInterval:        30 * time.Second,
		},
		DataChanSize:  5000,
		WorkerCount:   2,
		FlushInterval: 5 * time.Second,
	}

	manager := obs.NewManager(config)

	// Add system metrics collector
	systemCollector := collectors.NewSystemCollector()
	systemCollector.UpdateConfig(obs.CollectorConfig{
		Name:     "system",
		Enabled:  true,
		Interval: 60 * time.Second,
		Labels:   obs.Labels{"service": "videonode", "instance": "default"},
	})
	if err := manager.AddCollector(systemCollector); err != nil {
		return nil, err
	}

	// Add Prometheus scraper for MediaMTX
	promCollector := collectors.NewPrometheusCollector("http://localhost:9998/metrics")
	promCollector.SetMetricFilter("^(paths|paths_bytes|rtmp|webrtc).*")
	promCollector.UpdateConfig(obs.CollectorConfig{
		Name:     "prometheus_scraper",
		Enabled:  true,
		Interval: 15 * time.Second,
		Labels:   obs.Labels{"service": "videonode", "instance": "default"},
	})
	if err := manager.AddCollector(promCollector); err != nil {
		return nil, err
	}

	// Add Prometheus exporter
	if promRegistry != nil {
		promExporter := exporters.NewPrometheusExporter(promRegistry)
		if err := manager.AddExporter(promExporter); err != nil {
			return nil, err
		}
	}

	// Add SSE exporter to broadcast to existing SSE system
	if sseBroadcaster != nil {
		sseExporter := exporters.NewSSEExporter(sseBroadcaster)
		if err := manager.AddExporter(sseExporter); err != nil {
			return nil, err
		}
	}

	// Note: JSON API endpoints will be added to the existing server via handlers
	// No separate JSON exporter needed

	return manager, nil
}

// InitializeObservability initializes the obs package with VideoNode integration
func InitializeObservability(sseManager *sse.Manager) error {
	log.Println("OBS: Initializing observability system...")

	// Create Prometheus registry
	promRegistry := prometheus.NewRegistry()

	// Create SSE broadcaster adapter
	sseBroadcaster := &SSEBroadcasterAdapter{sseManager: sseManager}

	// Setup obs manager for VideoNode using inline configuration
	manager, err := setupForVideoNode(promRegistry, sseBroadcaster)
	if err != nil {
		return err
	}

	// Start the observability system
	if err := manager.Start(); err != nil {
		return err
	}

	globalObsManager = manager
	log.Println("OBS: Observability system initialized successfully")
	return nil
}

// ShutdownObservability gracefully shuts down the observability system
func ShutdownObservability() {
	if globalObsManager != nil {
		log.Println("OBS: Shutting down observability system...")
		if err := globalObsManager.Stop(); err != nil {
			log.Printf("OBS: Error during shutdown: %v", err)
		} else {
			log.Println("OBS: Observability system shut down successfully")
		}
	}
}

// GetObsManager returns the global obs manager instance
func GetObsManager() *obs.Manager {
	return globalObsManager
}

// AddFFmpegStreamMonitoring adds FFmpeg monitoring for a specific stream
func AddFFmpegStreamMonitoring(streamID, socketPath, logPath string) error {
	if globalObsManager == nil {
		return fmt.Errorf("observability system not initialized")
	}

	log.Printf("OBS: Adding FFmpeg monitoring for stream %s", streamID)
	ffmpegCollector := collectors.NewFFmpegCollector(socketPath, logPath, streamID)
	ffmpegCollector.UpdateConfig(obs.CollectorConfig{
		Name:     "ffmpeg_" + streamID,
		Enabled:  true,
		Interval: 0, // Event-driven
		Labels:   obs.Labels{"stream_id": streamID},
	})
	return globalObsManager.AddCollector(ffmpegCollector)
}

// RemoveFFmpegStreamMonitoring removes FFmpeg monitoring for a specific stream
func RemoveFFmpegStreamMonitoring(streamID string) error {
	if globalObsManager == nil {
		return fmt.Errorf("observability system not initialized")
	}

	collectorName := "ffmpeg_" + streamID
	log.Printf("OBS: Removing FFmpeg monitoring for stream %s", streamID)
	return globalObsManager.RemoveCollector(collectorName)
}

// SendCustomMetric sends a custom metric to the obs system
func SendCustomMetric(name string, value float64, labels obs.Labels) {
	if globalObsManager == nil {
		return
	}

	metric := &obs.MetricPoint{
		Name:       name,
		Value:      value,
		LabelsMap:  labels,
		Timestamp_: time.Now(),
		Unit:       "",
	}

	globalObsManager.SendData(metric)
}

// SendCustomLog sends a custom log entry to the obs system
func SendCustomLog(level obs.LogLevel, message string, source string, fields map[string]interface{}) {
	if globalObsManager == nil {
		return
	}

	logEntry := &obs.LogEntry{
		Message:    message,
		Level:      level,
		LabelsMap:  obs.Labels{"source": source, "component": "videonode"},
		Fields:     fields,
		Timestamp_: time.Now(),
		Source:     source,
	}

	globalObsManager.SendData(logEntry)
}

// GetCurrentMetrics returns current metrics in a format compatible with the existing templates
type CurrentMetrics struct {
	WiFiSignalDBM   float64         `json:"wifi_signal_dbm"`
	WiFiQuality     float64         `json:"wifi_quality"`
	WiFiInterface   string          `json:"wifi_interface"`
	LastUpdate      time.Time       `json:"last_update"`
	IsWiFiAvailable bool            `json:"is_wifi_available"`
	StreamMetrics   []StreamMetrics `json:"stream_metrics"`
}

type StreamMetrics struct {
	StreamID      string    `json:"stream_id"`
	State         string    `json:"state"`
	BytesReceived int64     `json:"bytes_received"`
	BytesSent     int64     `json:"bytes_sent"`
	LastUpdate    time.Time `json:"last_update"`
}

// GetCurrentMetrics returns current metrics from the obs system
func GetCurrentMetrics() (CurrentMetrics, error) {
	if globalObsManager == nil {
		return CurrentMetrics{}, fmt.Errorf("observability system not initialized")
	}

	// Query recent system metrics from obs
	recentTime := time.Now().Add(-5 * time.Minute)

	// Get WiFi metrics
	wifiResult, err := globalObsManager.Query(obs.QueryOptions{
		DataType: obs.DataTypeMetric,
		Name:     "wifi_signal_strength_dbm",
		Start:    recentTime,
		End:      time.Now(),
		Limit:    1,
	})

	metrics := CurrentMetrics{
		WiFiSignalDBM:   -100, // Default
		WiFiQuality:     0,
		WiFiInterface:   "",
		LastUpdate:      time.Now(),
		IsWiFiAvailable: false,
		StreamMetrics:   []StreamMetrics{},
	}

	// Extract WiFi data if available
	if err == nil && len(wifiResult.Points) > 0 {
		if wifiPoint, ok := wifiResult.Points[0].(*obs.MetricPoint); ok {
			metrics.WiFiSignalDBM = wifiPoint.Value
			metrics.IsWiFiAvailable = true
			if iface, ok := wifiPoint.Labels()["interface"]; ok {
				metrics.WiFiInterface = iface
			}

			// Calculate quality from signal strength
			if wifiPoint.Value >= -30 {
				metrics.WiFiQuality = 100
			} else if wifiPoint.Value <= -90 {
				metrics.WiFiQuality = 0
			} else {
				metrics.WiFiQuality = 100 - ((wifiPoint.Value+30)/-60)*100
			}
		}
	}

	// Get stream metrics (MediaMTX data)
	streamResult, err := globalObsManager.Query(obs.QueryOptions{
		DataType: obs.DataTypeMetric,
		Start:    recentTime,
		End:      time.Now(),
		Labels:   obs.Labels{"prometheus_endpoint": "http://localhost:9998/metrics"},
		Limit:    100,
	})

	if err == nil {
		streamMap := make(map[string]*StreamMetrics)

		for _, point := range streamResult.Points {
			if metricPoint, ok := point.(*obs.MetricPoint); ok {
				labels := metricPoint.Labels()
				if streamName, ok := labels["name"]; ok {
					if streamMap[streamName] == nil {
						streamMap[streamName] = &StreamMetrics{
							StreamID:   streamName,
							State:      "unknown",
							LastUpdate: metricPoint.Timestamp(),
						}
					}

					stream := streamMap[streamName]

					// Update based on metric name
					switch metricPoint.Name {
					case "paths_bytes_received":
						stream.BytesReceived = int64(metricPoint.Value)
					case "paths_bytes_sent":
						stream.BytesSent = int64(metricPoint.Value)
					case "paths":
						if state, ok := labels["state"]; ok {
							stream.State = state
						}
					}
				}
			}
		}

		// Convert to slice
		for _, stream := range streamMap {
			metrics.StreamMetrics = append(metrics.StreamMetrics, *stream)
		}
	}

	return metrics, nil
}

// GetPrometheusHandler returns the Prometheus metrics handler from obs
func GetPrometheusHandler() http.Handler {
	if globalObsManager == nil {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "Observability system not initialized", http.StatusInternalServerError)
		})
	}

	// Note: Using simplified Prometheus export - could be enhanced to use the actual Prometheus exporter from the manager

	// For now, return a simple handler that queries obs data and formats as Prometheus
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")

		// Query recent metrics
		result, err := globalObsManager.Query(obs.QueryOptions{
			DataType: obs.DataTypeMetric,
			Start:    time.Now().Add(-1 * time.Minute),
			End:      time.Now(),
		})

		if err != nil {
			http.Error(w, "Failed to query metrics", http.StatusInternalServerError)
			return
		}

		// Convert to Prometheus format (simplified)
		for _, point := range result.Points {
			if metricPoint, ok := point.(*obs.MetricPoint); ok {
				// Format: metric_name{labels} value timestamp
				metricName := metricPoint.Name

				// Build labels string
				var labelPairs []string
				for k, v := range metricPoint.Labels() {
					labelPairs = append(labelPairs, fmt.Sprintf(`%s="%s"`, k, v))
				}

				labelsStr := ""
				if len(labelPairs) > 0 {
					labelsStr = "{" + strings.Join(labelPairs, ",") + "}"
				}

				timestamp := metricPoint.Timestamp().UnixNano() / 1000000 // Convert to milliseconds
				fmt.Fprintf(w, "%s%s %g %d\n", metricName, labelsStr, metricPoint.Value, timestamp)
			}
		}
	})
}
