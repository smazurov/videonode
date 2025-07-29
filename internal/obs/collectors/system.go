package collectors

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/smazurov/videonode/internal/obs"
)

// SystemCollector collects system metrics like CPU, memory, disk, and network
type SystemCollector struct {
	*obs.BaseCollector
}

// NewSystemCollector creates a new system metrics collector
func NewSystemCollector() *SystemCollector {
	config := obs.DefaultCollectorConfig("system")
	config.Interval = 10 * time.Second
	config.Labels = obs.Labels{
		"collector_type": "system",
	}

	return &SystemCollector{
		BaseCollector: obs.NewBaseCollector("system", config),
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

// collectMetrics collects all system metrics
func (s *SystemCollector) collectMetrics(dataChan chan<- obs.DataPoint) {
	timestamp := time.Now()

	// Collect CPU metrics
	s.collectCPUMetrics(dataChan, timestamp)

	// Collect memory metrics
	s.collectMemoryMetrics(dataChan, timestamp)

	// Collect disk metrics
	s.collectDiskMetrics(dataChan, timestamp)

	// Collect network metrics
	s.collectNetworkMetrics(dataChan, timestamp)

	// Collect load average
	s.collectLoadAverage(dataChan, timestamp)
}

// collectCPUMetrics collects CPU usage statistics
func (s *SystemCollector) collectCPUMetrics(dataChan chan<- obs.DataPoint, timestamp time.Time) {
	// Read /proc/stat for CPU usage
	file, err := os.Open("/proc/stat")
	if err != nil {
		s.sendLog(dataChan, obs.LogLevelError, fmt.Sprintf("Failed to read /proc/stat: %v", err), timestamp)
		return
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "cpu ") {
			fields := strings.Fields(line)
			if len(fields) < 8 {
				continue
			}

			// Parse CPU times
			user, _ := strconv.ParseFloat(fields[1], 64)
			nice, _ := strconv.ParseFloat(fields[2], 64)
			system, _ := strconv.ParseFloat(fields[3], 64)
			idle, _ := strconv.ParseFloat(fields[4], 64)
			iowait, _ := strconv.ParseFloat(fields[5], 64)
			irq, _ := strconv.ParseFloat(fields[6], 64)
			softirq, _ := strconv.ParseFloat(fields[7], 64)

			total := user + nice + system + idle + iowait + irq + softirq

			// Calculate percentages
			if total > 0 {
				s.sendMetric(dataChan, "cpu_user_percent", (user/total)*100, obs.Labels{"type": "user"}, timestamp)
				s.sendMetric(dataChan, "cpu_system_percent", (system/total)*100, obs.Labels{"type": "system"}, timestamp)
				s.sendMetric(dataChan, "cpu_idle_percent", (idle/total)*100, obs.Labels{"type": "idle"}, timestamp)
				s.sendMetric(dataChan, "cpu_iowait_percent", (iowait/total)*100, obs.Labels{"type": "iowait"}, timestamp)
				s.sendMetric(dataChan, "cpu_usage_percent", ((total-idle)/total)*100, obs.Labels{"type": "total"}, timestamp)
			}
			break
		}
	}
}

// collectMemoryMetrics collects memory usage statistics
func (s *SystemCollector) collectMemoryMetrics(dataChan chan<- obs.DataPoint, timestamp time.Time) {
	// Read /proc/meminfo
	file, err := os.Open("/proc/meminfo")
	if err != nil {
		s.sendLog(dataChan, obs.LogLevelError, fmt.Sprintf("Failed to read /proc/meminfo: %v", err), timestamp)
		return
	}
	defer file.Close()

	memInfo := make(map[string]float64)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		fields := strings.Fields(line)
		if len(fields) >= 2 {
			key := strings.TrimSuffix(fields[0], ":")
			value, err := strconv.ParseFloat(fields[1], 64)
			if err == nil {
				memInfo[key] = value * 1024 // Convert kB to bytes
			}
		}
	}

	// Send memory metrics
	if total, ok := memInfo["MemTotal"]; ok {
		s.sendMetric(dataChan, "memory_total_bytes", total, obs.Labels{"type": "total"}, timestamp)

		if available, ok := memInfo["MemAvailable"]; ok {
			s.sendMetric(dataChan, "memory_available_bytes", available, obs.Labels{"type": "available"}, timestamp)
			s.sendMetric(dataChan, "memory_usage_percent", ((total-available)/total)*100, obs.Labels{"type": "usage"}, timestamp)
		}

		if free, ok := memInfo["MemFree"]; ok {
			s.sendMetric(dataChan, "memory_free_bytes", free, obs.Labels{"type": "free"}, timestamp)
		}

		if buffers, ok := memInfo["Buffers"]; ok {
			s.sendMetric(dataChan, "memory_buffers_bytes", buffers, obs.Labels{"type": "buffers"}, timestamp)
		}

		if cached, ok := memInfo["Cached"]; ok {
			s.sendMetric(dataChan, "memory_cached_bytes", cached, obs.Labels{"type": "cached"}, timestamp)
		}
	}

	// Swap metrics
	if swapTotal, ok := memInfo["SwapTotal"]; ok {
		s.sendMetric(dataChan, "swap_total_bytes", swapTotal, obs.Labels{"type": "total"}, timestamp)

		if swapFree, ok := memInfo["SwapFree"]; ok {
			s.sendMetric(dataChan, "swap_free_bytes", swapFree, obs.Labels{"type": "free"}, timestamp)
			if swapTotal > 0 {
				s.sendMetric(dataChan, "swap_usage_percent", ((swapTotal-swapFree)/swapTotal)*100, obs.Labels{"type": "usage"}, timestamp)
			}
		}
	}
}

// collectDiskMetrics collects disk usage and I/O statistics
func (s *SystemCollector) collectDiskMetrics(dataChan chan<- obs.DataPoint, timestamp time.Time) {
	// Get disk usage for root filesystem
	cmd := exec.Command("df", "-B1", "/")
	output, err := cmd.Output()
	if err == nil {
		lines := strings.Split(string(output), "\n")
		if len(lines) >= 2 {
			fields := strings.Fields(lines[1])
			if len(fields) >= 6 {
				total, _ := strconv.ParseFloat(fields[1], 64)
				used, _ := strconv.ParseFloat(fields[2], 64)
				available, _ := strconv.ParseFloat(fields[3], 64)

				labels := obs.Labels{"device": "root", "mountpoint": "/"}
				s.sendMetric(dataChan, "disk_total_bytes", total, labels, timestamp)
				s.sendMetric(dataChan, "disk_used_bytes", used, labels, timestamp)
				s.sendMetric(dataChan, "disk_available_bytes", available, labels, timestamp)
				if total > 0 {
					s.sendMetric(dataChan, "disk_usage_percent", (used/total)*100, labels, timestamp)
				}
			}
		}
	}

	// Read disk I/O stats from /proc/diskstats
	file, err := os.Open("/proc/diskstats")
	if err != nil {
		return
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) >= 14 {
			device := fields[2]

			// Skip loop devices and partitions (keep only main devices)
			if strings.HasPrefix(device, "loop") ||
				strings.Contains(device, "ram") ||
				(len(device) > 3 && device[len(device)-1] >= '0' && device[len(device)-1] <= '9') {
				continue
			}

			readsCompleted, _ := strconv.ParseFloat(fields[3], 64)
			readsSectors, _ := strconv.ParseFloat(fields[5], 64)
			writesCompleted, _ := strconv.ParseFloat(fields[7], 64)
			writesSectors, _ := strconv.ParseFloat(fields[9], 64)

			labels := obs.Labels{"device": device}
			s.sendMetric(dataChan, "disk_reads_total", readsCompleted, labels, timestamp)
			s.sendMetric(dataChan, "disk_writes_total", writesCompleted, labels, timestamp)
			s.sendMetric(dataChan, "disk_read_bytes_total", readsSectors*512, labels, timestamp) // 512 bytes per sector
			s.sendMetric(dataChan, "disk_write_bytes_total", writesSectors*512, labels, timestamp)
		}
	}
}

// collectNetworkMetrics collects network interface statistics
func (s *SystemCollector) collectNetworkMetrics(dataChan chan<- obs.DataPoint, timestamp time.Time) {
	// Read /proc/net/dev
	file, err := os.Open("/proc/net/dev")
	if err != nil {
		s.sendLog(dataChan, obs.LogLevelError, fmt.Sprintf("Failed to read /proc/net/dev: %v", err), timestamp)
		return
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
		fields := strings.Fields(line[colonIndex+1:])

		if len(fields) >= 16 {
			rxBytes, _ := strconv.ParseFloat(fields[0], 64)
			rxPackets, _ := strconv.ParseFloat(fields[1], 64)
			rxErrors, _ := strconv.ParseFloat(fields[2], 64)
			rxDropped, _ := strconv.ParseFloat(fields[3], 64)

			txBytes, _ := strconv.ParseFloat(fields[8], 64)
			txPackets, _ := strconv.ParseFloat(fields[9], 64)
			txErrors, _ := strconv.ParseFloat(fields[10], 64)
			txDropped, _ := strconv.ParseFloat(fields[11], 64)

			labels := obs.Labels{"interface": interfaceName}
			s.sendMetric(dataChan, "network_receive_bytes_total", rxBytes, labels, timestamp)
			s.sendMetric(dataChan, "network_receive_packets_total", rxPackets, labels, timestamp)
			s.sendMetric(dataChan, "network_receive_errors_total", rxErrors, labels, timestamp)
			s.sendMetric(dataChan, "network_receive_dropped_total", rxDropped, labels, timestamp)

			s.sendMetric(dataChan, "network_transmit_bytes_total", txBytes, labels, timestamp)
			s.sendMetric(dataChan, "network_transmit_packets_total", txPackets, labels, timestamp)
			s.sendMetric(dataChan, "network_transmit_errors_total", txErrors, labels, timestamp)
			s.sendMetric(dataChan, "network_transmit_dropped_total", txDropped, labels, timestamp)
		}
	}
}

// collectLoadAverage collects system load average
func (s *SystemCollector) collectLoadAverage(dataChan chan<- obs.DataPoint, timestamp time.Time) {
	// Read /proc/loadavg
	file, err := os.Open("/proc/loadavg")
	if err != nil {
		s.sendLog(dataChan, obs.LogLevelError, fmt.Sprintf("Failed to read /proc/loadavg: %v", err), timestamp)
		return
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	if scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) >= 3 {
			load1, _ := strconv.ParseFloat(fields[0], 64)
			load5, _ := strconv.ParseFloat(fields[1], 64)
			load15, _ := strconv.ParseFloat(fields[2], 64)

			s.sendMetric(dataChan, "load_average", load1, obs.Labels{"period": "1m"}, timestamp)
			s.sendMetric(dataChan, "load_average", load5, obs.Labels{"period": "5m"}, timestamp)
			s.sendMetric(dataChan, "load_average", load15, obs.Labels{"period": "15m"}, timestamp)
		}
	}
}

// Helper methods

func (s *SystemCollector) sendMetric(dataChan chan<- obs.DataPoint, name string, value float64, labels obs.Labels, timestamp time.Time) {
	point := &obs.MetricPoint{
		Name:       name,
		Value:      value,
		LabelsMap:  s.AddLabels(labels),
		Timestamp_: timestamp,
		Unit:       s.getMetricUnit(name),
	}

	select {
	case dataChan <- point:
	default:
		// Channel full, skip this point
	}
}

func (s *SystemCollector) sendLog(dataChan chan<- obs.DataPoint, level obs.LogLevel, message string, timestamp time.Time) {
	point := &obs.LogEntry{
		Message:    message,
		Level:      level,
		LabelsMap:  s.AddLabels(obs.Labels{"source": "system_collector"}),
		Fields:     make(map[string]interface{}),
		Timestamp_: timestamp,
		Source:     "system_collector",
	}

	select {
	case dataChan <- point:
	default:
		// Channel full, skip this point
	}
}

func (s *SystemCollector) getMetricUnit(name string) string {
	switch {
	case strings.Contains(name, "_bytes"):
		return "bytes"
	case strings.Contains(name, "_percent"):
		return "percent"
	case strings.Contains(name, "_total"):
		return "count"
	case strings.Contains(name, "load_average"):
		return "ratio"
	default:
		return ""
	}
}
