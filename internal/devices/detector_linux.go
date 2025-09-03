//go:build linux

package devices

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/jochenvg/go-udev"
	"github.com/smazurov/videonode/v4l2_detector"
)

type linuxDetector struct {
	ctx          context.Context
	cancel       context.CancelFunc
	broadcaster  EventBroadcaster
	lastDevices  map[string]DeviceInfo // key is DeviceId
	mu           sync.Mutex
}

func newDetector() DeviceDetector {
	return &linuxDetector{
		lastDevices: make(map[string]DeviceInfo),
	}
}

// FindDevices returns all currently available V4L2 devices
func (d *linuxDetector) FindDevices() ([]DeviceInfo, error) {
	v4l2Devices, err := v4l2_detector.FindDevices()
	if err != nil {
		return nil, err
	}
	
	devices := make([]DeviceInfo, len(v4l2Devices))
	for i, v4l2Device := range v4l2Devices {
		devices[i] = DeviceInfo{
			DevicePath: v4l2Device.DevicePath,
			DeviceName: v4l2Device.DeviceName,
			DeviceId:   v4l2Device.DeviceId,
			Caps:       v4l2Device.Caps,
		}
	}
	
	return devices, nil
}

// GetDeviceFormats returns supported formats for a device
func (d *linuxDetector) GetDeviceFormats(devicePath string) ([]FormatInfo, error) {
	v4l2Formats, err := v4l2_detector.GetDeviceFormats(devicePath)
	if err != nil {
		return nil, err
	}
	
	formats := make([]FormatInfo, len(v4l2Formats))
	for i, v4l2Format := range v4l2Formats {
		formats[i] = FormatInfo{
			PixelFormat: v4l2Format.PixelFormat,
			FormatName:  v4l2Format.FormatName,
			Emulated:    v4l2Format.Emulated,
		}
	}
	
	return formats, nil
}

// GetDevicePathByID returns the device path for a given device ID
func (d *linuxDetector) GetDevicePathByID(deviceID string) (string, error) {
	return v4l2_detector.GetDevicePathByID(deviceID)
}

// GetDeviceResolutions returns supported resolutions for a format
func (d *linuxDetector) GetDeviceResolutions(devicePath string, pixelFormat uint32) ([]Resolution, error) {
	v4l2Resolutions, err := v4l2_detector.GetDeviceResolutions(devicePath, pixelFormat)
	if err != nil {
		return nil, err
	}
	
	resolutions := make([]Resolution, len(v4l2Resolutions))
	for i, v4l2Res := range v4l2Resolutions {
		resolutions[i] = Resolution{
			Width:  v4l2Res.Width,
			Height: v4l2Res.Height,
		}
	}
	
	return resolutions, nil
}

// GetDeviceFramerates returns supported framerates for a resolution
func (d *linuxDetector) GetDeviceFramerates(devicePath string, pixelFormat uint32, width, height uint32) ([]Framerate, error) {
	v4l2Framerates, err := v4l2_detector.GetDeviceFramerates(devicePath, pixelFormat, width, height)
	if err != nil {
		return nil, err
	}
	
	framerates := make([]Framerate, len(v4l2Framerates))
	for i, v4l2Fr := range v4l2Framerates {
		framerates[i] = Framerate{
			Numerator:   v4l2Fr.Numerator,
			Denominator: v4l2Fr.Denominator,
		}
	}
	
	return framerates, nil
}

// StartMonitoring starts monitoring for device changes using udev
func (d *linuxDetector) StartMonitoring(ctx context.Context, broadcaster EventBroadcaster) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	
	// Store context and broadcaster
	d.ctx, d.cancel = context.WithCancel(ctx)
	d.broadcaster = broadcaster
	
	// Initialize with current devices to avoid false "added" events
	devices, err := d.FindDevices()
	if err != nil {
		log.Printf("Warning: Failed to get initial device list: %v", err)
	} else {
		for _, device := range devices {
			d.lastDevices[device.DeviceId] = device
		}
		log.Printf("Initialized with %d V4L2 devices", len(devices))
	}
	
	// Start udev monitoring
	u := udev.Udev{}
	mon := u.NewMonitorFromNetlink("udev")
	if mon == nil {
		return fmt.Errorf("failed to create udev monitor")
	}
	
	// Monitor USB devices
	mon.FilterAddMatchSubsystemDevtype("usb", "usb_device")
	
	deviceCh, errCh, err := mon.DeviceChan(d.ctx)
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
			case <-d.ctx.Done():
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
					
					d.checkAndBroadcastDeviceChanges()
				}
			}
		}
	}()
	
	return nil
}

// StopMonitoring stops the device monitoring
func (d *linuxDetector) StopMonitoring() {
	d.mu.Lock()
	defer d.mu.Unlock()
	
	if d.cancel != nil {
		d.cancel()
		d.cancel = nil
	}
}

// checkAndBroadcastDeviceChanges checks for V4L2 device changes and broadcasts if needed
func (d *linuxDetector) checkAndBroadcastDeviceChanges() {
	devices, err := d.FindDevices()
	if err != nil {
		log.Printf("Error getting device data: %v", err)
		return
	}
	
	// Build current device map by DeviceId
	currentDevices := make(map[string]DeviceInfo)
	for _, device := range devices {
		currentDevices[device.DeviceId] = device
	}
	
	d.mu.Lock()
	defer d.mu.Unlock()
	
	// Check for removed devices
	for deviceId, oldDevice := range d.lastDevices {
		if _, exists := currentDevices[deviceId]; !exists {
			d.broadcaster.BroadcastDeviceDiscovery("removed", oldDevice, time.Now().Format(time.RFC3339))
			log.Printf("Device removed: %s (%s) [ID: %s]", oldDevice.DevicePath, oldDevice.DeviceName, deviceId)
		}
	}
	
	// Check for added and changed devices
	for deviceId, newDevice := range currentDevices {
		oldDevice, exists := d.lastDevices[deviceId]
		
		if !exists {
			// New device
			d.broadcaster.BroadcastDeviceDiscovery("added", newDevice, time.Now().Format(time.RFC3339))
			log.Printf("Device added: %s (%s) [ID: %s]", newDevice.DevicePath, newDevice.DeviceName, deviceId)
		} else if oldDevice != newDevice {
			// Device changed
			d.broadcaster.BroadcastDeviceDiscovery("changed", newDevice, time.Now().Format(time.RFC3339))
			log.Printf("Device changed: %s (%s) [ID: %s]", newDevice.DevicePath, newDevice.DeviceName, deviceId)
		}
	}
	
	// Update last devices
	d.lastDevices = currentDevices
}