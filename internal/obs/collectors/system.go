package collectors

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/smazurov/videonode/internal/obs"
)

// SystemCollector collects essential system metrics: load average and network stats
type SystemCollector struct {
	*obs.BaseCollector
	allowedInterfaces map[string]bool
	primaryInterface  string
}

// NewSystemCollector creates a new system metrics collector
func NewSystemCollector() *SystemCollector {
	config := obs.DefaultCollectorConfig("system")
	config.Interval = 10 * time.Second
	config.Labels = obs.Labels{
		"collector_type": "system",
	}

	return &SystemCollector{
		BaseCollector:     obs.NewBaseCollector("system", config),
		allowedInterfaces: make(map[string]bool),
	}
}

// SetNetworkInterfaces sets the allowed network interfaces to monitor
func (s *SystemCollector) SetNetworkInterfaces(interfaces []string) {
	s.allowedInterfaces = make(map[string]bool)
	for _, iface := range interfaces {
		if strings.TrimSpace(iface) != "" {
			s.allowedInterfaces[strings.TrimSpace(iface)] = true
			// Set first interface as primary
			if s.primaryInterface == "" {
				s.primaryInterface = strings.TrimSpace(iface)
			}
		}
	}
}

// Start begins collecting system metrics
func (s *SystemCollector) Start(ctx context.Context, dataChan chan<- obs.DataPoint) error {
	s.SetRunning(true)

	ticker := time.NewTicker(s.Interval())
	defer ticker.Stop()

	// Collect initial metrics
	s.collectMetrics(dataChan)

	for {
		select {
		case <-ticker.C:
			s.collectMetrics(dataChan)

		case <-ctx.Done():
			s.SetRunning(false)
			return nil

		case <-s.StopChan():
			s.SetRunning(false)
			return nil
		}
	}
}

// collectMetrics collects essential system metrics: load average and network stats
func (s *SystemCollector) collectMetrics(dataChan chan<- obs.DataPoint) {
	timestamp := time.Now()

	// Collect load average and network stats for primary interface
	loadAvg := s.getLoadAverage()
	networkStats := s.getNetworkStats()

	// Send as single consolidated metric
	if loadAvg != nil && networkStats != nil {
		s.sendSystemMetrics(dataChan, loadAvg, networkStats, timestamp)
	}
}

// LoadAverageData holds load average values
type LoadAverageData struct {
	OneMin     float64
	FiveMin    float64
	FifteenMin float64
}

// NetworkStatsData holds network interface statistics
type NetworkStatsData struct {
	Interface string
	RxBytes   float64
	TxBytes   float64
	RxPackets float64
	TxPackets float64
}

// getLoadAverage reads load average from /proc/loadavg
func (s *SystemCollector) getLoadAverage() *LoadAverageData {
	file, err := os.Open("/proc/loadavg")
	if err != nil {
		return nil
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	if scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) >= 3 {
			load1, _ := strconv.ParseFloat(fields[0], 64)
			load5, _ := strconv.ParseFloat(fields[1], 64)
			load15, _ := strconv.ParseFloat(fields[2], 64)

			return &LoadAverageData{
				OneMin:     load1,
				FiveMin:    load5,
				FifteenMin: load15,
			}
		}
	}
	return nil
}

// getNetworkStats reads network statistics for the primary interface
func (s *SystemCollector) getNetworkStats() *NetworkStatsData {
	if s.primaryInterface == "" {
		return nil
	}

	file, err := os.Open("/proc/net/dev")
	if err != nil {
		return nil
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	// Skip header lines
	scanner.Scan()
	scanner.Scan()

	for scanner.Scan() {
		line := scanner.Text()
		colonIndex := strings.Index(line, ":")
		if colonIndex == -1 {
			continue
		}

		interfaceName := strings.TrimSpace(line[:colonIndex])
		if interfaceName != s.primaryInterface {
			continue
		}

		fields := strings.Fields(line[colonIndex+1:])
		if len(fields) >= 16 {
			rxBytes, _ := strconv.ParseFloat(fields[0], 64)
			rxPackets, _ := strconv.ParseFloat(fields[1], 64)
			txBytes, _ := strconv.ParseFloat(fields[8], 64)
			txPackets, _ := strconv.ParseFloat(fields[9], 64)

			return &NetworkStatsData{
				Interface: interfaceName,
				RxBytes:   rxBytes,
				TxBytes:   txBytes,
				RxPackets: rxPackets,
				TxPackets: txPackets,
			}
		}
	}
	return nil
}

// sendSystemMetrics sends consolidated system metrics as a single data point
func (s *SystemCollector) sendSystemMetrics(dataChan chan<- obs.DataPoint, loadAvg *LoadAverageData, networkStats *NetworkStatsData, timestamp time.Time) {
	labels := obs.Labels{
		"load_1m":        fmt.Sprintf("%.2f", loadAvg.OneMin),
		"load_5m":        fmt.Sprintf("%.2f", loadAvg.FiveMin),
		"load_15m":       fmt.Sprintf("%.2f", loadAvg.FifteenMin),
		"net_interface":  networkStats.Interface,
		"net_rx_bytes":   fmt.Sprintf("%.0f", networkStats.RxBytes),
		"net_tx_bytes":   fmt.Sprintf("%.0f", networkStats.TxBytes),
		"net_rx_packets": fmt.Sprintf("%.0f", networkStats.RxPackets),
		"net_tx_packets": fmt.Sprintf("%.0f", networkStats.TxPackets),
	}

	point := &obs.MetricPoint{
		Name:       "system_metrics",
		Value:      1.0, // Indicator metric
		LabelsMap:  s.AddLabels(labels),
		Timestamp_: timestamp,
		Unit:       "info",
	}

	select {
	case dataChan <- point:
	default:
		// Channel full, skip this point
	}
}
