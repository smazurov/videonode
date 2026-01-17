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

	"github.com/smazurov/videonode/internal/logging"
	"github.com/smazurov/videonode/internal/metrics"
)

// MPPCollector collects Rockchip MPP metrics from /proc/mpp_service/load.
type MPPCollector struct {
	logger   logging.Logger
	procPath string
	interval time.Duration
	ctx      context.Context
	cancel   context.CancelFunc
}

// NewMPPCollector creates a new MPP collector.
func NewMPPCollector() *MPPCollector {
	return &MPPCollector{
		logger:   logging.GetLogger("mpp"),
		procPath: "/proc/mpp_service/load",
		interval: 5 * time.Second,
	}
}

// Start begins collecting MPP metrics.
func (m *MPPCollector) Start(ctx context.Context) error {
	m.ctx, m.cancel = context.WithCancel(ctx)
	go m.run()
	return nil
}

// Stop stops the MPP collector.
func (m *MPPCollector) Stop() error {
	if m.cancel != nil {
		m.cancel()
	}
	return nil
}

func (m *MPPCollector) run() {
	m.logger.Info("Starting MPP metrics collection", "path", m.procPath, "interval", m.interval)
	ticker := time.NewTicker(m.interval)
	defer ticker.Stop()

	m.collectMetrics()

	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			m.collectMetrics()
		}
	}
}

func (m *MPPCollector) collectMetrics() {
	file, err := os.Open(m.procPath)
	if err != nil {
		m.logger.Warn("Failed to open MPP proc file", "error", err)
		return
	}
	defer file.Close()

	devices, err := m.parseContent(file)
	if err != nil {
		m.logger.Warn("Failed to parse MPP metrics", "error", err)
		return
	}

	for _, device := range devices {
		metrics.SetMPPDeviceLoad(device.Name, device.Load)
		metrics.SetMPPDeviceUtilization(device.Name, device.Utilization)
	}
}

type mppDevice struct {
	Name        string
	Load        float64
	Utilization float64
}

func (m *MPPCollector) parseContent(r io.Reader) ([]mppDevice, error) {
	var devices []mppDevice
	scanner := bufio.NewScanner(r)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		device, err := m.parseLine(line)
		if err != nil {
			continue
		}
		devices = append(devices, *device)
	}

	return devices, scanner.Err()
}

func (m *MPPCollector) parseLine(line string) (*mppDevice, error) {
	fields := strings.Fields(line)
	if len(fields) < 4 {
		return nil, fmt.Errorf("insufficient fields")
	}

	deviceName := fields[0]
	loadIdx, utilIdx := -1, -1

	for i, field := range fields {
		if field == "load:" && i+1 < len(fields) {
			loadIdx = i + 1
		}
		if field == "utilization:" && i+1 < len(fields) {
			utilIdx = i + 1
		}
	}

	if loadIdx == -1 || utilIdx == -1 {
		return nil, fmt.Errorf("missing load or utilization")
	}

	load, err := strconv.ParseFloat(strings.TrimSuffix(fields[loadIdx], "%"), 64)
	if err != nil {
		return nil, err
	}

	util, err := strconv.ParseFloat(strings.TrimSuffix(fields[utilIdx], "%"), 64)
	if err != nil {
		return nil, err
	}

	return &mppDevice{
		Name:        deviceName,
		Load:        load,
		Utilization: util,
	}, nil
}
