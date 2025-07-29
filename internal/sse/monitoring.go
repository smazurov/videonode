package sse

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/jochenvg/go-udev"
	"github.com/smazurov/videonode/v4l2_detector"
)

// startPollingMonitor starts the polling-based device monitor
func (m *Manager) startPollingMonitor() {
	ctx, cancel := context.WithCancel(m.ctx)
	m.deviceMonitorCancel = cancel

	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		m.runPollingMonitor(ctx)
	}()
}

// runPollingMonitor runs the polling loop
func (m *Manager) runPollingMonitor(ctx context.Context) {
	ticker := time.NewTicker(m.pollingInterval)
	defer ticker.Stop()

	log.Printf("SSE: Started polling monitor with interval %v", m.pollingInterval)

	for {
		select {
		case <-ctx.Done():
			log.Println("SSE: Polling monitor stopped")
			return
		case <-ticker.C:
			m.checkAndBroadcastDeviceChanges()
		}
	}
}

// startUdevMonitoring starts the udev-based device monitor
func (m *Manager) startUdevMonitoring() error {
	ctx, cancel := context.WithCancel(m.ctx)
	m.udevMonitorCancel = cancel

	u := udev.Udev{}
	mon := u.NewMonitorFromNetlink("udev")
	if mon == nil {
		return fmt.Errorf("failed to create udev monitor")
	}

	mon.FilterAddMatchSubsystemDevtype("usb", "usb_device")

	deviceCh, errCh, err := mon.DeviceChan(ctx)
	if err != nil {
		return fmt.Errorf("failed to get udev device channel: %w", err)
	}

	// Error monitoring goroutine
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		for err := range errCh {
			log.Printf("SSE: Udev monitor error: %v", err)
			// Check if we should restart polling
			select {
			case <-ctx.Done():
				return
			default:
				// Consider switching to polling on persistent errors
			}
		}
	}()

	// Device monitoring goroutine
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		m.runUdevMonitor(ctx, deviceCh)
	}()

	log.Println("SSE: Udev monitoring started for USB devices")
	return nil
}

// runUdevMonitor runs the udev monitoring loop
func (m *Manager) runUdevMonitor(ctx context.Context, deviceCh <-chan *udev.Device) {
	for {
		select {
		case <-ctx.Done():
			log.Println("SSE: Udev monitor stopped")
			return
		case dev, ok := <-deviceCh:
			if !ok {
				log.Println("SSE: Udev device channel closed")
				// Attempt to restart with polling
				m.startPollingMonitor()
				return
			}

			action := dev.Action()
			if action == "add" || action == "remove" {
				log.Printf("SSE: Udev event: %s for device %s (Subsystem: %s, Devtype: %s)",
					action, dev.Syspath(), dev.Subsystem(), dev.Devtype())
				m.checkAndBroadcastDeviceChanges()
			}
		}
	}
}

// checkAndBroadcastDeviceChanges checks for device changes and broadcasts if needed
func (m *Manager) checkAndBroadcastDeviceChanges() {
	if m.getDevicesData == nil {
		log.Println("SSE: No device data function provided")
		return
	}

	devices, err := m.getDevicesData()
	if err != nil {
		log.Printf("SSE: Error getting device data: %v", err)
		m.BroadcastEvent(EventError, map[string]string{
			"message": fmt.Sprintf("Failed to get device data: %v", err),
		})
		return
	}

	// Check if devices have changed
	m.lastDevicesMutex.RLock()
	changed := !m.devicesEqual(m.lastDevices, devices)
	m.lastDevicesMutex.RUnlock()

	if changed {
		// Update stored devices
		m.lastDevicesMutex.Lock()
		m.lastDevices = devices
		m.lastDevicesMutex.Unlock()

		// Broadcast the change
		deviceEvent := struct {
			Devices   []v4l2_detector.DeviceInfo `json:"devices"`
			Count     int                        `json:"count"`
			Timestamp string                     `json:"timestamp"`
		}{
			Devices:   devices.Devices,
			Count:     devices.Count,
			Timestamp: time.Now().Format(time.RFC3339),
		}

		if err := m.BroadcastEvent(EventDeviceDiscovery, deviceEvent); err != nil {
			log.Printf("SSE: Error broadcasting device event: %v", err)
		} else {
			log.Printf("SSE: Broadcasted device update (count: %d)", devices.Count)
		}
	}
}

// devicesEqual efficiently compares two device responses
func (m *Manager) devicesEqual(a, b DeviceResponse) bool {
	if a.Count != b.Count {
		return false
	}

	if len(a.Devices) != len(b.Devices) {
		return false
	}

	// For small lists, direct comparison is more efficient
	if len(a.Devices) <= 10 {
		// Create a simple lookup for b devices
		bPaths := make(map[string]struct{}, len(b.Devices))
		for _, device := range b.Devices {
			bPaths[device.DevicePath] = struct{}{}
		}

		// Check if all devices in a exist in b
		for _, device := range a.Devices {
			if _, exists := bPaths[device.DevicePath]; !exists {
				return false
			}
		}
		return true
	}

	// For larger lists, use a more thorough comparison
	aDevices := make(map[string]struct{}, len(a.Devices))
	for _, device := range a.Devices {
		aDevices[device.DevicePath] = struct{}{}
	}

	for _, device := range b.Devices {
		if _, exists := aDevices[device.DevicePath]; !exists {
			return false
		}
		delete(aDevices, device.DevicePath)
	}

	return len(aDevices) == 0
}
