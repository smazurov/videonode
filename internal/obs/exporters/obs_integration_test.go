package exporters

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/smazurov/videonode/internal/obs"
)

// TestOBSBug_MetricAccumulation demonstrates the bug where metrics accumulate
// instead of being replaced when label values change.
func TestOBSBug_MetricAccumulation(t *testing.T) {
	exporter := NewPromExporter()

	// Simulate changing test metrics - these SHOULD update the same metric
	// but currently create new metrics each time
	for i := range 5 {
		metric := &obs.MetricPoint{
			Name:  "test_metrics",
			Value: 1.0,
			LabelsMap: obs.Labels{
				"collector":     "test",
				"instance":      "default",
				"service":       "videonode",
				"value_1":       fmt.Sprintf("%.2f", 2.0+float64(i)*0.1), // Changes each iteration
				"value_2":       fmt.Sprintf("%.2f", 2.5+float64(i)*0.1), // Changes each iteration
				"value_3":       fmt.Sprintf("%.2f", 3.0+float64(i)*0.1), // Changes each iteration
				"net_interface": "lo",
				"net_rx_bytes":  fmt.Sprintf("%d", 1000000+i*100000), // Changes each iteration
				"net_tx_bytes":  fmt.Sprintf("%d", 500000+i*50000),   // Changes each iteration
			},
			TimestampUnix: time.Now(),
		}
		if err := exporter.Export([]obs.DataPoint{metric}); err != nil {
			t.Fatalf("Export failed: %v", err)
		}
		exporter.ForceFlush()
	}

	// BUG: Should have 1 metric but has 5
	actualCount := len(exporter.collector.metrics)
	expectedCount := 1

	if actualCount != expectedCount {
		t.Errorf("BUG DETECTED: Metrics are accumulating!\n"+
			"  Expected: %d metric (updated with latest values)\n"+
			"  Got: %d metrics (each update created a new metric)\n"+
			"  This proves metrics with changing label values create duplicates",
			expectedCount, actualCount)

		// Show all the accumulated metrics
		for key := range exporter.collector.metrics {
			t.Logf("  Accumulated metric key: %s", key)
		}
	}
}

// TestOBSBug_FFmpegStreamMetrics demonstrates the specific bug with FFmpeg metrics.
func TestOBSBug_FFmpegStreamMetrics(t *testing.T) {
	exporter := NewPromExporter()

	// Simulate real FFmpeg metrics progression over time
	streamUpdates := []struct {
		fps       string
		dropped   string
		duplicate string
		speed     string
	}{
		{"0.00", "0", "7", "0"},
		{"28.00", "0", "14", "0.467"},
		{"29.33", "0", "22", "0.667"},
		{"28.99", "0", "29", "0.733"},
		{"29.59", "0", "37", "0.8"},
		{"29.33", "0", "44", "0.822"},
	}

	for _, update := range streamUpdates {
		metric := &obs.MetricPoint{
			Name:  "ffmpeg_stream_metrics",
			Value: 1.0,
			LabelsMap: obs.Labels{
				"stream_id":        "proper_stream",
				"fps":              update.fps,
				"dropped_frames":   update.dropped,
				"duplicate_frames": update.duplicate,
				"processing_speed": update.speed,
			},
			TimestampUnix: time.Now(),
		}
		if err := exporter.Export([]obs.DataPoint{metric}); err != nil {
			t.Fatalf("Export failed: %v", err)
		}
	}

	exporter.ForceFlush()

	// BUG: Should have 1 metric but has 6 (one for each update)
	actualCount := len(exporter.collector.metrics)
	expectedCount := 1

	if actualCount != expectedCount {
		t.Errorf("BUG DETECTED: FFmpeg metrics are accumulating!\n"+
			"  Expected: %d metric for stream 'proper_stream'\n"+
			"  Got: %d metrics (one for each frame count update)\n"+
			"  This is exactly the bug seen in production!",
			expectedCount, actualCount)
	}

	// Test the actual Prometheus output
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	w := httptest.NewRecorder()
	handler := exporter.GetHandler()
	handler.ServeHTTP(w, req)

	body := w.Body.String()
	lines := strings.Split(body, "\n")

	// Count how many times the metric appears
	metricCount := 0
	for _, line := range lines {
		if strings.Contains(line, "obs_ffmpeg_stream_metrics{") {
			metricCount++
			if metricCount <= 10 {
				t.Logf("  Duplicate metric line %d: %s", metricCount, line)
			}
		}
	}

	if metricCount != expectedCount {
		t.Errorf("Prometheus output has %d duplicate metrics (expected %d)",
			metricCount, expectedCount)
	}
}

// TestOBSBug_PrometheusKeyGeneration tests the root cause - the key generation.
func TestOBSBug_PrometheusKeyGeneration(t *testing.T) {
	collector := NewDynamicCollector()

	// Test that the same metric with different non-identifying label values
	// creates different keys (this is the bug)
	labels1 := map[string]string{
		"stream_id": "test",
		"fps":       "30.0",
		"dup":       "10",
	}

	labels2 := map[string]string{
		"stream_id": "test",
		"fps":       "30.0",
		"dup":       "20", // Only this changed
	}

	key1 := collector.createMetricKey("test_metric", labels1)
	key2 := collector.createMetricKey("test_metric", labels2)

	// BUG: These should be the same key (only stream_id matters for identity)
	// but they're different because ALL label values are included
	if key1 == key2 {
		t.Log("Keys are correctly the same (bug is fixed)")
	} else {
		t.Errorf("BUG DETECTED: Key generation includes non-identifying labels!\n"+
			"  Key1: %s\n"+
			"  Key2: %s\n"+
			"  These should be the same (only stream_id should identify the metric)",
			key1, key2)
	}
}

// TestOBS_CorrectMetricSeparation tests that FFmpeg should send separate metrics
// not one combined metric with values as labels.
func TestOBS_CorrectMetricSeparation(t *testing.T) {
	exporter := NewPromExporter()

	// How it SHOULD work: separate metrics for each measurement
	correctMetrics := []obs.DataPoint{
		&obs.MetricPoint{
			Name:          "ffmpeg_fps",
			Value:         30.0,
			LabelsMap:     obs.Labels{"stream_id": "test"},
			TimestampUnix: time.Now(),
		},
		&obs.MetricPoint{
			Name:          "ffmpeg_dropped_frames_total",
			Value:         5.0,
			LabelsMap:     obs.Labels{"stream_id": "test"},
			TimestampUnix: time.Now(),
		},
		&obs.MetricPoint{
			Name:          "ffmpeg_duplicate_frames_total",
			Value:         100.0,
			LabelsMap:     obs.Labels{"stream_id": "test"},
			TimestampUnix: time.Now(),
		},
	}

	for _, m := range correctMetrics {
		if err := exporter.Export([]obs.DataPoint{m}); err != nil {
			t.Fatalf("Export failed: %v", err)
		}
	}
	exporter.ForceFlush()

	// Update with new values
	updatedMetrics := []obs.DataPoint{
		&obs.MetricPoint{
			Name:          "ffmpeg_fps",
			Value:         29.5,
			LabelsMap:     obs.Labels{"stream_id": "test"},
			TimestampUnix: time.Now(),
		},
		&obs.MetricPoint{
			Name:          "ffmpeg_dropped_frames_total",
			Value:         6.0,
			LabelsMap:     obs.Labels{"stream_id": "test"},
			TimestampUnix: time.Now(),
		},
		&obs.MetricPoint{
			Name:          "ffmpeg_duplicate_frames_total",
			Value:         120.0,
			LabelsMap:     obs.Labels{"stream_id": "test"},
			TimestampUnix: time.Now(),
		},
	}

	for _, m := range updatedMetrics {
		if err := exporter.Export([]obs.DataPoint{m}); err != nil {
			t.Fatalf("Export failed: %v", err)
		}
	}
	exporter.ForceFlush()

	// Should still have exactly 3 metrics (updated, not duplicated)
	if len(exporter.collector.metrics) != 3 {
		t.Logf("Note: When done correctly, there should be 3 metrics (fps, dropped, duplicate)")
		t.Logf("Currently have %d metrics", len(exporter.collector.metrics))
	}

	// Check values are updated
	for key, metric := range exporter.collector.metrics {
		switch {
		case strings.Contains(key, "ffmpeg_fps"):
			if metric.Value != 29.5 {
				t.Errorf("FPS not updated: got %f, want 29.5", metric.Value)
			}
		case strings.Contains(key, "dropped"):
			if metric.Value != 6.0 {
				t.Errorf("Dropped frames not updated: got %f, want 6.0", metric.Value)
			}
		case strings.Contains(key, "duplicate"):
			if metric.Value != 120.0 {
				t.Errorf("Duplicate frames not updated: got %f, want 120.0", metric.Value)
			}
		}
	}
}

// TestOBS_NoMetricExpiration tests that metrics never expire (memory leak).
func TestOBS_NoMetricExpiration(t *testing.T) {
	exporter := NewPromExporter()

	// Create metrics with old timestamps
	oldTime := time.Now().Add(-1 * time.Hour)

	for i := range 100 {
		metric := &obs.MetricPoint{
			Name:          "old_metric",
			Value:         float64(i),
			LabelsMap:     obs.Labels{"index": fmt.Sprintf("%d", i)},
			TimestampUnix: oldTime,
		}
		if err := exporter.Export([]obs.DataPoint{metric}); err != nil {
			t.Fatalf("Export failed: %v", err)
		}
	}

	exporter.ForceFlush()

	// All 100 metrics are still there (no expiration)
	if len(exporter.collector.metrics) == 100 {
		t.Error("BUG DETECTED: No metric expiration!\n" +
			"  Created 100 metrics 1 hour ago\n" +
			"  All 100 are still in memory\n" +
			"  This is a memory leak!")
	}

	// There's no cleanup mechanism to test
	t.Log("Note: There is no CleanupStaleMetrics method implemented")
}

// TestOBSBug_StateTransitions tests that state changes don't create duplicate metrics.
func TestOBSBug_StateTransitions(t *testing.T) {
	exporter := NewPromExporter()

	// Simulate stream state transition: notReady -> ready
	metrics := []obs.DataPoint{
		&obs.MetricPoint{
			Name:  "paths",
			Value: 1.0,
			LabelsMap: obs.Labels{
				"name":  "proper_stream",
				"state": "notReady",
			},
			TimestampUnix: time.Now(),
		},
		&obs.MetricPoint{
			Name:  "paths",
			Value: 1.0,
			LabelsMap: obs.Labels{
				"name":  "proper_stream",
				"state": "ready", // State changed
			},
			TimestampUnix: time.Now(),
		},
	}

	for _, m := range metrics {
		if err := exporter.Export([]obs.DataPoint{m}); err != nil {
			t.Fatalf("Export failed: %v", err)
		}
	}
	exporter.ForceFlush()

	// Should have only 1 metric (ready state replaced notReady)
	if len(exporter.collector.metrics) != 1 {
		t.Errorf("State transition bug: expected 1 metric, got %d", len(exporter.collector.metrics))
		for key := range exporter.collector.metrics {
			t.Logf("  Key: %s", key)
		}
	}

	// Check that the final state is 'ready'
	for _, metric := range exporter.collector.metrics {
		// The metric should have 'ready' in its labels since it's the latest
		hasReady := false
		for i, labelName := range metric.LabelNames {
			if labelName == "state" && metric.LabelValues[i] == "ready" {
				hasReady = true
				break
			}
		}
		if !hasReady {
			t.Error("Final metric should have state=ready")
		}
	}
}

// TestOBS_DuplicateCollectorStartup tests that collectors are started only once.
func TestOBS_DuplicateCollectorStartup(t *testing.T) {
	config := obs.DefaultManagerConfig()
	config.WorkerCount = 1
	manager := obs.NewManager(config)

	// Add Prometheus exporter to track metrics
	promExporter := NewPromExporter()
	err := manager.AddExporter(promExporter)
	if err != nil {
		t.Fatalf("Failed to add Prometheus exporter: %v", err)
	}

	// Create a simple test collector that generates a metric immediately
	testCollector := &TestCounterCollector{
		BaseCollector: obs.NewBaseCollector("test_counter", obs.DefaultCollectorConfig("test_counter")),
		counter:       0,
	}

	// Add collector BEFORE starting manager (simulates the bug scenario)
	err = manager.AddCollector(testCollector)
	if err != nil {
		t.Fatalf("Failed to add test collector: %v", err)
	}

	// This should NOT start the collector yet since manager isn't started
	time.Sleep(50 * time.Millisecond)
	if testCollector.startCount != 0 {
		t.Errorf("Collector started %d times before manager.Start() called", testCollector.startCount)
	}

	// Now start the manager - collector should start ONCE
	err = manager.Start()
	if err != nil {
		t.Fatalf("Failed to start manager: %v", err)
	}
	defer manager.Stop()

	// Wait for startup
	time.Sleep(100 * time.Millisecond)

	// Collector should have been started exactly ONCE
	if testCollector.startCount != 1 {
		t.Errorf("Collector was started %d times (expected 1)", testCollector.startCount)
	}
}

// TestCounterCollector is a simple collector that tracks how many times Start() is called.
type TestCounterCollector struct {
	*obs.BaseCollector
	counter    int
	startCount int
}

func (t *TestCounterCollector) Start(ctx context.Context, dataChan chan<- obs.DataPoint) error {
	t.startCount++
	t.SetRunning(true)
	defer t.SetRunning(false)

	// Send one metric immediately to prove we're running
	t.counter++
	point := &obs.MetricPoint{
		Name:          "test_start_counter",
		Value:         float64(t.counter),
		LabelsMap:     obs.Labels{"start_count": fmt.Sprintf("%d", t.startCount)},
		TimestampUnix: time.Now(),
	}

	select {
	case dataChan <- point:
	default:
	}

	// Wait for cancellation
	<-ctx.Done()
	return nil
}

func (t *TestCounterCollector) Stop() error {
	return t.BaseCollector.Stop()
}

// TestOBS_SystemMetricsFormat tests that system metrics are using wrong format.
func TestOBS_SystemMetricsFormat(t *testing.T) {
	// Test that system metrics are properly formatted as separate metrics
	exporter := NewPromExporter()

	// This is the CORRECT format - separate metrics with proper values
	testMetrics := []obs.DataPoint{
		&obs.MetricPoint{
			Name:          "test_value_1",
			Value:         2.5,
			LabelsMap:     obs.Labels{"collector": "test"},
			TimestampUnix: time.Now(),
		},
		&obs.MetricPoint{
			Name:          "test_value_2",
			Value:         2.8,
			LabelsMap:     obs.Labels{"collector": "test"},
			TimestampUnix: time.Now(),
		},
		&obs.MetricPoint{
			Name:          "system_net_rx_bytes",
			Value:         1000000,
			LabelsMap:     obs.Labels{"collector": "system", "interface": "lo"},
			TimestampUnix: time.Now(),
		},
	}

	for _, metric := range testMetrics {
		if err := exporter.Export([]obs.DataPoint{metric}); err != nil {
			t.Fatalf("Export failed: %v", err)
		}
	}
	exporter.ForceFlush()

	// Check the output
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	w := httptest.NewRecorder()
	handler := exporter.GetHandler()
	handler.ServeHTTP(w, req)

	body := w.Body.String()

	// Verify correct format: separate metrics with proper values
	if !strings.Contains(body, "obs_test_value_1") {
		t.Error("Missing test_value_1 metric")
	}
	if !strings.Contains(body, "obs_test_value_2") {
		t.Error("Missing test_value_2 metric")
	}
	if !strings.Contains(body, "obs_system_net_rx_bytes") {
		t.Error("Missing system_net_rx_bytes metric")
	}

	// Verify values are not used as labels (which was the old bug)
	if strings.Contains(body, `load_1m="2.5"`) {
		t.Error("BUG: System metrics still using values as labels")
	}
}
