package monitoring

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/jochenvg/go-udev"
	"github.com/smazurov/videonode/internal/api/models"
	"github.com/smazurov/videonode/v4l2_detector"
)

// EventBroadcaster interface for broadcasting events
type EventBroadcaster interface {
	BroadcastDeviceDiscovery(action string, device models.DeviceInfo, timestamp string)
}

// UdevMonitor monitors for USB device changes and broadcasts SSE events
type UdevMonitor struct {
	ctx          context.Context
	cancel       context.CancelFunc
	broadcaster  EventBroadcaster
	lastDevices  map[string]v4l2_detector.DeviceInfo // key is DeviceId, not DevicePath
}

// NewUdevMonitor creates a new udev monitor
func NewUdevMonitor(broadcaster EventBroadcaster) *UdevMonitor {
	ctx, cancel := context.WithCancel(context.Background())
	return &UdevMonitor{
		ctx:         ctx,
		cancel:      cancel,
		broadcaster: broadcaster,
		lastDevices: make(map[string]v4l2_detector.DeviceInfo),
	}
}

// Start begins monitoring for USB device changes
func (m *UdevMonitor) Start() error {
	// Initialize with current devices to avoid false "added" events on first USB event
	devices, err := v4l2_detector.FindDevices()
	if err != nil {
		log.Printf("Warning: Failed to get initial device list: %v", err)
	} else {
		for _, device := range devices {
			m.lastDevices[device.DeviceId] = device
		}
		log.Printf("Initialized with %d V4L2 devices", len(devices))
	}

	u := udev.Udev{}
	mon := u.NewMonitorFromNetlink("udev")
	if mon == nil {
		return fmt.Errorf("failed to create udev monitor")
	}

	// Monitor USB devices
	mon.FilterAddMatchSubsystemDevtype("usb", "usb_device")

	deviceCh, errCh, err := mon.DeviceChan(m.ctx)
	if err != nil {
		return fmt.Errorf("failed to get udev device channel: %w", err)
	}

	// Error monitoring goroutine
	go func() {
		for err := range errCh {
			log.Printf("Udev monitor error: %v", err)
		}
	}()

	// Device monitoring goroutine
	go func() {
		log.Println("Udev monitoring started for USB devices")
		for {
			select {
			case <-m.ctx.Done():
				log.Println("Udev monitor stopped")
				return
			case dev, ok := <-deviceCh:
				if !ok {
					log.Println("Udev device channel closed")
					return
				}

				action := dev.Action()
				if action == "add" || action == "remove" {
					log.Printf("Udev event: %s for device %s (Subsystem: %s, Devtype: %s)",
						action, dev.Syspath(), dev.Subsystem(), dev.Devtype())
					
					// For add events, give kernel more time to enumerate V4L2 devices
					if action == "add" {
						time.Sleep(1 * time.Second)
					}
					
					m.checkAndBroadcastDeviceChanges()
				}
			}
		}
	}()

	return nil
}

// Stop stops the udev monitor
func (m *UdevMonitor) Stop() {
	m.cancel()
}

// checkAndBroadcastDeviceChanges checks for V4L2 device changes and broadcasts if needed
func (m *UdevMonitor) checkAndBroadcastDeviceChanges() {
	devices, err := v4l2_detector.FindDevices()
	if err != nil {
		log.Printf("Error getting device data: %v", err)
		return
	}

	// Build current device map by DeviceId (stable identifier)
	currentDevices := make(map[string]v4l2_detector.DeviceInfo)
	for _, device := range devices {
		currentDevices[device.DeviceId] = device
	}


	// Check for removed devices
	for deviceId, oldDevice := range m.lastDevices {
		if _, exists := currentDevices[deviceId]; !exists {
			device := models.DeviceInfo{
				DevicePath: oldDevice.DevicePath,
				DeviceName: oldDevice.DeviceName,
				DeviceId:   oldDevice.DeviceId,
				Caps:       oldDevice.Caps,
			}
			m.broadcaster.BroadcastDeviceDiscovery("removed", device, time.Now().Format(time.RFC3339))
			log.Printf("Device removed: %s (%s) [ID: %s]", oldDevice.DevicePath, oldDevice.DeviceName, deviceId)
		}
	}

	// Check for added devices and changed devices
	for deviceId, newDevice := range currentDevices {
		oldDevice, exists := m.lastDevices[deviceId]
		device := models.DeviceInfo{
			DevicePath: newDevice.DevicePath,
			DeviceName: newDevice.DeviceName,
			DeviceId:   newDevice.DeviceId,
			Caps:       newDevice.Caps,
		}
		
		if !exists {
			// New device
			m.broadcaster.BroadcastDeviceDiscovery("added", device, time.Now().Format(time.RFC3339))
			log.Printf("Device added: %s (%s) [ID: %s]", newDevice.DevicePath, newDevice.DeviceName, deviceId)
		} else if oldDevice != newDevice {
			// Device changed - any property different
			m.broadcaster.BroadcastDeviceDiscovery("changed", device, time.Now().Format(time.RFC3339))
			log.Printf("Device changed: %s (%s) [ID: %s]", newDevice.DevicePath, newDevice.DeviceName, deviceId)
		}
	}

	// Update last devices
	m.lastDevices = currentDevices
}