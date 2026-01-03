package collectors

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/smazurov/videonode/internal/obs"
)

func TestParseMPPLine(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected *MPPDevice
		wantErr  bool
	}{
		{
			name:  "valid line with decimals",
			input: "fdb51000.avsd-plus        load:  15.50% utilization:  12.25%",
			expected: &MPPDevice{
				Device:      "fdb51000.avsd-plus",
				Load:        15.50,
				Utilization: 12.25,
			},
			wantErr: false,
		},
		{
			name:  "valid line with zeros",
			input: "fdb50400.vdpu             load:   0.00% utilization:   0.00%",
			expected: &MPPDevice{
				Device:      "fdb50400.vdpu",
				Load:        0.00,
				Utilization: 0.00,
			},
			wantErr: false,
		},
		{
			name:  "valid line with high values",
			input: "fdbd0000.rkvenc-core      load:  95.75% utilization:  88.50%",
			expected: &MPPDevice{
				Device:      "fdbd0000.rkvenc-core",
				Load:        95.75,
				Utilization: 88.50,
			},
			wantErr: false,
		},
		{
			name:     "empty line",
			input:    "",
			expected: nil,
			wantErr:  true,
		},
		{
			name:     "malformed line - missing load",
			input:    "fdb51000.avsd-plus        utilization:  12.25%",
			expected: nil,
			wantErr:  true,
		},
		{
			name:     "malformed line - missing utilization",
			input:    "fdb51000.avsd-plus        load:  15.50%",
			expected: nil,
			wantErr:  true,
		},
		{
			name:     "malformed line - invalid percentage",
			input:    "fdb51000.avsd-plus        load:  invalid% utilization:  12.25%",
			expected: nil,
			wantErr:  true,
		},
	}

	collector := NewMPPCollector(obs.Labels{})

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := collector.parseLine(tt.input)

			if tt.wantErr {
				if err == nil {
					t.Errorf("Expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if result == nil {
				t.Errorf("Expected result but got nil")
				return
			}

			if result.Device != tt.expected.Device {
				t.Errorf("Expected device %s, got %s", tt.expected.Device, result.Device)
			}
			if result.Load != tt.expected.Load {
				t.Errorf("Expected load %.2f, got %.2f", tt.expected.Load, result.Load)
			}
			if result.Utilization != tt.expected.Utilization {
				t.Errorf("Expected utilization %.2f, got %.2f", tt.expected.Utilization, result.Utilization)
			}
		})
	}
}

func TestParseMPPContent(t *testing.T) {
	input := `fdb51000.avsd-plus        load:   0.00% utilization:   0.00%
fdb50400.vdpu             load:  15.50% utilization:  12.25%
fdb50000.vepu             load:  45.00% utilization:  40.00%
fdb90000.jpegd            load:   0.00% utilization:   0.00%
fdba0000.jpege-core       load:  10.25% utilization:   8.50%
fdba4000.jpege-core       load:   0.00% utilization:   0.00%
fdba8000.jpege-core       load:   0.00% utilization:   0.00%
fdbac000.jpege-core       load:   0.00% utilization:   0.00%
fdbb0000.iep              load:   0.00% utilization:   0.00%
fdbd0000.rkvenc-core      load:  80.75% utilization:  75.50%
fdbe0000.rkvenc-core      load:  95.00% utilization:  90.25%
fdc38100.rkvdec-core      load:   0.00% utilization:   0.00%
fdc48100.rkvdec-core      load:   0.00% utilization:   0.00%
fdc70000.av1d             load:   5.50% utilization:   3.75%`

	collector := NewMPPCollector(obs.Labels{})
	devices, err := collector.parseContent(strings.NewReader(input))
	if err != nil {
		t.Fatalf("Failed to parse MPP content: %v", err)
	}

	// Test we got the right number of devices
	if len(devices) != 14 {
		t.Errorf("Expected 14 devices, got %d", len(devices))
	}

	// Test specific devices
	expectedDevices := map[string]struct {
		load        float64
		utilization float64
	}{
		"fdb51000.avsd-plus":   {0.00, 0.00},
		"fdb50400.vdpu":        {15.50, 12.25},
		"fdb50000.vepu":        {45.00, 40.00},
		"fdbd0000.rkvenc-core": {80.75, 75.50},
		"fdbe0000.rkvenc-core": {95.00, 90.25},
		"fdc70000.av1d":        {5.50, 3.75},
	}

	for _, device := range devices {
		if expected, exists := expectedDevices[device.Device]; exists {
			if device.Load != expected.load {
				t.Errorf("Device %s: expected load %.2f, got %.2f",
					device.Device, expected.load, device.Load)
			}
			if device.Utilization != expected.utilization {
				t.Errorf("Device %s: expected utilization %.2f, got %.2f",
					device.Device, expected.utilization, device.Utilization)
			}
		}
	}
}

func TestParseMPPContentWithEmptyLines(t *testing.T) {
	input := `fdb51000.avsd-plus        load:   0.00% utilization:   0.00%

fdb50400.vdpu             load:  15.50% utilization:  12.25%

fdb50000.vepu             load:  45.00% utilization:  40.00%
`

	collector := NewMPPCollector(obs.Labels{})
	devices, err := collector.parseContent(strings.NewReader(input))
	if err != nil {
		t.Fatalf("Failed to parse MPP content with empty lines: %v", err)
	}

	// Should parse only valid lines, skip empty ones
	if len(devices) != 3 {
		t.Errorf("Expected 3 devices, got %d", len(devices))
	}
}

func TestMPPCollectorInitialization(t *testing.T) {
	collector := NewMPPCollector(obs.Labels{})

	if collector.Name() != "mpp" {
		t.Errorf("Expected collector name 'mpp', got %s", collector.Name())
	}

	if collector.Interval() != 5*time.Second {
		t.Errorf("Expected interval 5s, got %v", collector.Interval())
	}

	if collector.procPath != "/proc/mpp_service/load" {
		t.Errorf("Expected procPath '/proc/mpp_service/load', got %s", collector.procPath)
	}
}

func TestMPPCollectorStart(t *testing.T) {
	collector := NewMPPCollector(obs.Labels{})

	// Override procPath to use test content
	collector.procPath = "/tmp/test_mpp_load"

	// Create test file
	testContent := `fdb51000.avsd-plus        load:  15.50% utilization:  12.25%
fdb50400.vdpu             load:  25.00% utilization:  20.00%`

	if err := writeTestFile(collector.procPath, testContent); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}
	defer func() {
		if err := removeTestFile(collector.procPath); err != nil {
			t.Errorf("removeTestFile failed: %v", err)
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	dataChan := make(chan obs.DataPoint, 100)
	errChan := make(chan error, 1)

	go func() {
		errChan <- collector.Start(ctx, dataChan)
	}()

	// Collect metrics for a short time
	metrics := make([]obs.DataPoint, 0)
	timeout := time.After(200 * time.Millisecond)

collectLoop:
	for {
		select {
		case dp := <-dataChan:
			metrics = append(metrics, dp)
		case <-timeout:
			break collectLoop
		}
	}

	// Wait for collector to stop
	<-errChan

	// Verify we got some metrics
	if len(metrics) == 0 {
		t.Error("No metrics collected")
	}

	// Check metric names and labels
	expectedMetrics := map[string]bool{
		"mpp_device_load":        false,
		"mpp_device_utilization": false,
	}

	for _, dp := range metrics {
		if mp, ok := dp.(*obs.MetricPoint); ok {
			if _, exists := expectedMetrics[mp.Name]; exists {
				expectedMetrics[mp.Name] = true

				// Check required labels
				labels := mp.Labels()
				if _, hasDevice := labels["device"]; !hasDevice {
					t.Errorf("Metric %s missing 'device' label", mp.Name)
				}
				if labels["collector"] != "mpp" {
					t.Errorf("Metric %s expected collector=mpp, got %s", mp.Name, labels["collector"])
				}
			}
		}
	}

	// Verify all expected metrics were found
	for metric, found := range expectedMetrics {
		if !found {
			t.Errorf("Expected metric %s not found", metric)
		}
	}
}

func TestMPPCollectorMissingFile(t *testing.T) {
	collector := NewMPPCollector(obs.Labels{})
	collector.procPath = "/non/existent/path/mpp_service/load"

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	dataChan := make(chan obs.DataPoint, 10)

	// Should not error but should handle gracefully
	err := collector.Start(ctx, dataChan)
	if err != nil {
		t.Errorf("Collector should handle missing file gracefully, got error: %v", err)
	}

	// Should not emit any metrics
	select {
	case <-dataChan:
		t.Error("Should not emit metrics when file is missing")
	case <-time.After(100 * time.Millisecond):
		// Expected - no metrics
	}
}

// Helper functions for file operations.
func writeTestFile(path, content string) error {
	return os.WriteFile(path, []byte(content), 0o644)
}

func removeTestFile(path string) error {
	return os.Remove(path)
}

func TestMPPCollectorMultipleDevices(t *testing.T) {
	collector := NewMPPCollector(obs.Labels{})

	// Override procPath to use test content with multiple devices
	collector.procPath = "/tmp/test_mpp_multiple_devices"

	// Create test file with 3 different devices
	testContent := `fdb51000.avsd-plus        load:   5.00% utilization:   3.00%
fdb50400.vdpu             load:  15.50% utilization:  12.25%
fdbd0000.rkvenc-core      load:  85.75% utilization:  80.50%`

	if err := writeTestFile(collector.procPath, testContent); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}
	defer func() {
		if err := removeTestFile(collector.procPath); err != nil {
			t.Errorf("removeTestFile failed: %v", err)
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	dataChan := make(chan obs.DataPoint, 100)
	errChan := make(chan error, 1)

	go func() {
		errChan <- collector.Start(ctx, dataChan)
	}()

	// Collect all metrics
	allMetrics := make([]obs.DataPoint, 0)
	timeout := time.After(200 * time.Millisecond)

collectLoop:
	for {
		select {
		case dp := <-dataChan:
			allMetrics = append(allMetrics, dp)
		case <-timeout:
			break collectLoop
		}
	}

	// Wait for collector to stop
	<-errChan

	if len(allMetrics) == 0 {
		t.Fatal("No metrics collected")
	}

	// Group metrics by device and type
	deviceMetrics := make(map[string]map[string]float64)
	for _, dp := range allMetrics {
		if mp, ok := dp.(*obs.MetricPoint); ok {
			labels := mp.Labels()
			device := labels["device"]

			if deviceMetrics[device] == nil {
				deviceMetrics[device] = make(map[string]float64)
			}
			deviceMetrics[device][mp.Name] = mp.Value
		}
	}

	// Verify we have metrics for all 3 devices
	expectedDevices := []string{
		"fdb51000.avsd-plus",
		"fdb50400.vdpu",
		"fdbd0000.rkvenc-core",
	}

	for _, expectedDevice := range expectedDevices {
		if _, exists := deviceMetrics[expectedDevice]; !exists {
			t.Errorf("Missing metrics for device: %s", expectedDevice)
			t.Logf("Found devices: %v", getKeys(deviceMetrics))
		} else {
			// Verify both load and utilization metrics exist
			if _, hasLoad := deviceMetrics[expectedDevice]["mpp_device_load"]; !hasLoad {
				t.Errorf("Missing load metric for device: %s", expectedDevice)
			}
			if _, hasUtil := deviceMetrics[expectedDevice]["mpp_device_utilization"]; !hasUtil {
				t.Errorf("Missing utilization metric for device: %s", expectedDevice)
			}
		}
	}

	// Verify specific values
	expectedValues := map[string]map[string]float64{
		"fdb51000.avsd-plus": {
			"mpp_device_load":        5.00,
			"mpp_device_utilization": 3.00,
		},
		"fdb50400.vdpu": {
			"mpp_device_load":        15.50,
			"mpp_device_utilization": 12.25,
		},
		"fdbd0000.rkvenc-core": {
			"mpp_device_load":        85.75,
			"mpp_device_utilization": 80.50,
		},
	}

	for device, expectedMetrics := range expectedValues {
		if actualMetrics, exists := deviceMetrics[device]; exists {
			for metricName, expectedValue := range expectedMetrics {
				if actualValue, hasMetric := actualMetrics[metricName]; hasMetric {
					if actualValue != expectedValue {
						t.Errorf("Device %s metric %s: expected %.2f, got %.2f",
							device, metricName, expectedValue, actualValue)
					}
				}
			}
		}
	}
}

func getKeys(m map[string]map[string]float64) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
