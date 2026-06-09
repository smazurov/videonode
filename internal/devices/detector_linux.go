//go:build linux

package devices

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/smazurov/videonode/internal/logging"
	"github.com/smazurov/videonode/pkg/linuxav/hotplug"
	"github.com/smazurov/videonode/pkg/linuxav/v4l2"
)

type linuxDetector struct {
	ctx         context.Context
	cancel      context.CancelFunc
	broadcaster EventBroadcaster
	lastDevices map[string]DeviceInfo // key is DeviceID
	mu          sync.Mutex
	logger      logging.Logger
}

func newDetector() DeviceDetector {
	return &linuxDetector{
		lastDevices: make(map[string]DeviceInfo),
		logger:      logging.GetLogger("devices"),
	}
}

// FindDevices returns all currently available V4L2 devices.
func (d *linuxDetector) FindDevices() ([]DeviceInfo, error) {
	v4l2Devices, err := v4l2.FindDevices()
	if err != nil {
		return nil, err
	}

	devices := make([]DeviceInfo, len(v4l2Devices))
	for i, v4l2Device := range v4l2Devices {
		// Get device type and ready status in single device open
		status := v4l2.GetDeviceStatus(v4l2Device.DevicePath)

		devices[i] = DeviceInfo{
			DevicePath: v4l2Device.DevicePath,
			DeviceName: v4l2Device.DeviceName,
			DeviceID:   v4l2Device.DeviceID,
			Caps:       v4l2Device.Caps,
			Ready:      status.Ready,
			Type:       DeviceType(status.DeviceType),
		}
	}

	return devices, nil
}

// GetDeviceFormats returns supported formats for a device.
func (d *linuxDetector) GetDeviceFormats(devicePath string) ([]FormatInfo, error) {
	v4l2Formats, err := v4l2.GetFormats(devicePath)
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

// GetDevicePathByID returns the device path for a given device ID.
func (d *linuxDetector) GetDevicePathByID(deviceID string) (string, error) {
	return v4l2.GetDevicePathByID(deviceID)
}

// GetDeviceResolutions returns supported resolutions for a format.
func (d *linuxDetector) GetDeviceResolutions(devicePath string, pixelFormat uint32) ([]Resolution, error) {
	v4l2Resolutions, err := v4l2.GetResolutions(devicePath, pixelFormat)
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

// GetDeviceFramerates returns supported framerates for a resolution.
func (d *linuxDetector) GetDeviceFramerates(devicePath string, pixelFormat uint32, width, height uint32) ([]Framerate, error) {
	v4l2Framerates, err := v4l2.GetFramerates(devicePath, pixelFormat, width, height)
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

// StartMonitoring starts monitoring for device changes using periodic polling and signal monitoring.
func (d *linuxDetector) StartMonitoring(ctx context.Context, broadcaster EventBroadcaster) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	// Store context and broadcaster
	d.ctx, d.cancel = context.WithCancel(ctx)
	d.broadcaster = broadcaster

	// Initialize with current devices
	devices, err := d.FindDevices()
	if err != nil {
		d.logger.Warn("Failed to get initial device list", logging.KeyError, err)
	} else {
		for _, device := range devices {
			d.lastDevices[device.DeviceID] = device

			// Log initial device status
			switch device.Type {
			case DeviceTypeHDMI:
				status := v4l2.GetDVTimings(device.DevicePath)
				if device.Ready {
					d.logger.Info("HDMI device initialized with signal",
						logging.KeyDeviceID, device.DeviceID,
						logging.KeyPath, device.DevicePath,
						logging.KeyResolution, fmt.Sprintf("%dx%d", status.Width, status.Height),
						logging.KeyFPS, fmt.Sprintf("%.2f", status.FPS))
				} else {
					d.logger.Info("HDMI device initialized without signal",
						logging.KeyDeviceID, device.DeviceID,
						logging.KeyPath, device.DevicePath,
						logging.KeyState, signalStateString(status.State))
				}
			case DeviceTypeWebcam:
				d.logger.Debug("Webcam device initialized",
					logging.KeyDeviceID, device.DeviceID,
					logging.KeyPath, device.DevicePath)
			}

			// Broadcast initial device state to StreamService
			d.broadcaster.BroadcastDeviceDiscovery("added", device, time.Now().Format(time.RFC3339))
		}
		d.logger.Info("Initialized with V4L2 devices", logging.KeyDeviceCount, len(devices))
	}

	// Start hotplug monitoring via netlink
	go d.monitorHotplug()

	return nil
}

// monitorHotplug monitors for device additions/removals via netlink.
func (d *linuxDetector) monitorHotplug() {
	monitor, err := hotplug.NewMonitor()
	if err != nil {
		d.logger.Warn("Failed to create hotplug monitor, falling back to polling", logging.KeyError, err)
		d.pollDeviceChanges()
		return
	}
	defer func() { _ = monitor.Close() }()

	// Filter for USB devices (which includes USB webcams and capture cards)
	monitor.AddSubsystemFilter(hotplug.SubsystemUSB)

	events := make(chan hotplug.Event, 32)
	go func() {
		if err := monitor.Run(d.ctx, events); err != nil && !errors.Is(err, context.Canceled) {
			d.logger.Error("Hotplug monitor error", logging.KeyError, err)
		}
	}()

	d.logger.Info("Hotplug monitoring started via netlink")

	for {
		select {
		case <-d.ctx.Done():
			d.logger.Info("Hotplug monitor stopped")
			return
		case event, ok := <-events:
			if !ok {
				d.logger.Info("Hotplug event channel closed")
				return
			}

			// Only process USB device add/remove events
			if event.DevType != "usb_device" {
				continue
			}

			if event.Action == hotplug.ActionAdd || event.Action == hotplug.ActionRemove {
				d.logger.Debug("USB hotplug event",
					logging.KeyAction, event.Action,
					logging.KeyDevPath, event.DevPath,
					logging.KeyDevType, event.DevType)

				// Give kernel time to enumerate V4L2 devices for add events
				if event.Action == hotplug.ActionAdd {
					time.Sleep(1 * time.Second)
				}

				d.checkAndBroadcastDeviceChanges()
			}
		}
	}
}

// pollDeviceChanges is a fallback that periodically checks for device additions/removals.
func (d *linuxDetector) pollDeviceChanges() {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	d.logger.Info("Device polling started (fallback mode, checking every 2 seconds)")

	for {
		select {
		case <-d.ctx.Done():
			d.logger.Info("Device polling stopped")
			return
		case <-ticker.C:
			d.checkAndBroadcastDeviceChanges()
		}
	}
}

// StopMonitoring stops the device monitoring.
func (d *linuxDetector) StopMonitoring() {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.cancel != nil {
		d.cancel()
		d.cancel = nil
	}
}

// signalStateString converts signal state to human-readable string.
func signalStateString(state v4l2.SignalState) string {
	switch state {
	case v4l2.SignalStateNoLink:
		return "no_link"
	case v4l2.SignalStateNoSignal:
		return "no_signal"
	case v4l2.SignalStateUnstable:
		return "unstable"
	case v4l2.SignalStateLocked:
		return "locked"
	case v4l2.SignalStateOutOfRange:
		return "out_of_range"
	case v4l2.SignalStateNotSupported:
		return "not_supported"
	default:
		return "no_device"
	}
}

// checkAndBroadcastDeviceChanges checks for V4L2 device changes and broadcasts if needed.
func (d *linuxDetector) checkAndBroadcastDeviceChanges() {
	devices, err := d.FindDevices()
	if err != nil {
		d.logger.Error("Error getting device data", logging.KeyError, err)
		return
	}

	// Build current device map by DeviceID
	currentDevices := make(map[string]DeviceInfo)
	for _, device := range devices {
		currentDevices[device.DeviceID] = device
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	// Check for removed devices
	for deviceID, oldDevice := range d.lastDevices {
		if _, exists := currentDevices[deviceID]; !exists {
			d.broadcaster.BroadcastDeviceDiscovery("removed", oldDevice, time.Now().Format(time.RFC3339))
			d.logger.Info("Device removed", logging.KeyDevice, oldDevice.DevicePath, logging.KeyName, oldDevice.DeviceName, logging.KeyDeviceID, deviceID)
			delete(d.lastDevices, deviceID)
		}
	}

	// Check for added devices
	for deviceID, newDevice := range currentDevices {
		oldDevice, exists := d.lastDevices[deviceID]

		if !exists {
			// New device
			d.broadcaster.BroadcastDeviceDiscovery("added", newDevice, time.Now().Format(time.RFC3339))
			d.logger.Info("Device added", logging.KeyDevice, newDevice.DevicePath, logging.KeyName, newDevice.DeviceName, logging.KeyDeviceID, deviceID)
			d.lastDevices[deviceID] = newDevice
		} else if oldDevice != newDevice {
			// Device changed (shouldn't happen often)
			d.broadcaster.BroadcastDeviceDiscovery("changed", newDevice, time.Now().Format(time.RFC3339))
			d.logger.Info("Device changed", logging.KeyDevice, newDevice.DevicePath, logging.KeyName, newDevice.DeviceName, logging.KeyDeviceID, deviceID)
			d.lastDevices[deviceID] = newDevice
		}
	}
}
