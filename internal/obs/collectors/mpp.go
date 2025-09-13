package collectors

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/smazurov/videonode/internal/obs"
)

// MPPDevice represents a single MPP device's metrics
type MPPDevice struct {
	Device      string  // Device identifier (e.g., "fdb51000.avsd-plus")
	Load        float64 // Load percentage
	Utilization float64 // Utilization percentage
}

// MPPCollector collects Rockchip MPP (Media Process Platform) metrics
type MPPCollector struct {
	*obs.BaseCollector
	procPath string
}

// NewMPPCollector creates a new MPP metrics collector
func NewMPPCollector() *MPPCollector {
	config := obs.DefaultCollectorConfig("mpp")
	config.Interval = 5 * time.Second
	config.Labels = obs.Labels{
		"collector_type": "mpp",
	}

	return &MPPCollector{
		BaseCollector: obs.NewBaseCollector("mpp", config),
		procPath:      "/proc/mpp_service/load",
	}
}

// Start begins collecting MPP metrics
func (m *MPPCollector) Start(ctx context.Context, dataChan chan<- obs.DataPoint) error {
	m.SetRunning(true)

	ticker := time.NewTicker(m.Interval())
	defer ticker.Stop()

	// Collect initial metrics
	m.collectMetrics(dataChan)

	for {
		select {
		case <-ticker.C:
			m.collectMetrics(dataChan)

		case <-ctx.Done():
			m.SetRunning(false)
			return nil

		case <-m.StopChan():
			m.SetRunning(false)
			return nil
		}
	}
}

// collectMetrics reads the proc file and emits metrics
func (m *MPPCollector) collectMetrics(dataChan chan<- obs.DataPoint) {
	file, err := os.Open(m.procPath)
	if err != nil {
		// File doesn't exist or can't be read - silently skip
		return
	}
	defer file.Close()

	devices, err := m.parseContent(file)
	if err != nil {
		// Parse error - skip this collection
		return
	}

	timestamp := time.Now()
	for _, device := range devices {
		m.sendMetrics(dataChan, device, timestamp)
	}
}

// sendMetrics emits load and utilization metrics for a device
func (m *MPPCollector) sendMetrics(dataChan chan<- obs.DataPoint, device MPPDevice, timestamp time.Time) {
	baseLabels := m.AddLabels(obs.Labels{
		"device": device.Device,
	})

	// Send load metric
	loadPoint := &obs.MetricPoint{
		Name:       "mpp_device_load",
		Value:      device.Load,
		LabelsMap:  baseLabels,
		Timestamp_: timestamp,
	}

	// Send utilization metric
	utilPoint := &obs.MetricPoint{
		Name:       "mpp_device_utilization",
		Value:      device.Utilization,
		LabelsMap:  baseLabels,
		Timestamp_: timestamp,
	}

	// Send metrics to channel (non-blocking)
	select {
	case dataChan <- loadPoint:
	default:
		// Channel full, skip this point
	}

	select {
	case dataChan <- utilPoint:
	default:
		// Channel full, skip this point
	}
}

// parseLine parses a single MPP device line
// Expected format: "fdb51000.avsd-plus        load:  15.50% utilization:  12.25%"
func (m *MPPCollector) parseLine(line string) (*MPPDevice, error) {
	line = strings.TrimSpace(line)
	if line == "" {
		return nil, fmt.Errorf("empty line")
	}

	// Find the device name (first field before whitespace)
	fields := strings.Fields(line)
	if len(fields) < 4 {
		return nil, fmt.Errorf("insufficient fields in line: %s", line)
	}

	device := fields[0]

	// Look for "load:" and "utilization:" patterns
	loadIdx := -1
	utilIdx := -1

	for i, field := range fields {
		if field == "load:" && i+1 < len(fields) {
			loadIdx = i + 1
		}
		if field == "utilization:" && i+1 < len(fields) {
			utilIdx = i + 1
		}
	}

	if loadIdx == -1 {
		return nil, fmt.Errorf("load field not found in line: %s", line)
	}
	if utilIdx == -1 {
		return nil, fmt.Errorf("utilization field not found in line: %s", line)
	}

	// Parse load percentage (remove % suffix)
	loadStr := strings.TrimSuffix(fields[loadIdx], "%")
	load, err := strconv.ParseFloat(loadStr, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid load percentage '%s': %w", fields[loadIdx], err)
	}

	// Parse utilization percentage (remove % suffix)
	utilStr := strings.TrimSuffix(fields[utilIdx], "%")
	utilization, err := strconv.ParseFloat(utilStr, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid utilization percentage '%s': %w", fields[utilIdx], err)
	}

	return &MPPDevice{
		Device:      device,
		Load:        load,
		Utilization: utilization,
	}, nil
}

// parseContent parses the complete /proc/mpp_service/load content
func (m *MPPCollector) parseContent(r io.Reader) ([]MPPDevice, error) {
	var devices []MPPDevice
	scanner := bufio.NewScanner(r)

	for scanner.Scan() {
		line := scanner.Text()
		device, err := m.parseLine(line)
		if err != nil {
			// Skip malformed lines, don't fail entire parsing
			continue
		}
		devices = append(devices, *device)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading content: %w", err)
	}

	return devices, nil
}
