//go:build linux

package devices

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/jochenvg/go-udev"
	"github.com/smazurov/videonode/internal/logging"
	v4l2detector "github.com/smazurov/videonode/v4l2_detector"
)

type linuxDetector struct {
	ctx         context.Context
	cancel      context.CancelFunc
	broadcaster EventBroadcaster
	lastDevices map[string]DeviceInfo // key is DeviceID
	mu          sync.Mutex
	logger      *slog.Logger
}

func newDetector() DeviceDetector {
	return &linuxDetector{
		lastDevices: make(map[string]DeviceInfo),
		logger:      logging.GetLogger("devices"),
	}
}

// FindDevices returns all currently available V4L2 devices.
func (d *linuxDetector) FindDevices() ([]DeviceInfo, error) {
	v4l2Devices, err := v4l2detector.FindDevices()
	if err != nil {
		return nil, err
	}

	devices := make([]DeviceInfo, len(v4l2Devices))
	for i, v4l2Device := range v4l2Devices {
		// Get device type and ready status in single device open
		status := v4l2detector.GetDeviceStatus(v4l2Device.DevicePath)

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
	v4l2Formats, err := v4l2detector.GetDeviceFormats(devicePath)
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
	return v4l2detector.GetDevicePathByID(deviceID)
}

// GetDeviceResolutions returns supported resolutions for a format.
func (d *linuxDetector) GetDeviceResolutions(devicePath string, pixelFormat uint32) ([]Resolution, error) {
	v4l2Resolutions, err := v4l2detector.GetDeviceResolutions(devicePath, pixelFormat)
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
	v4l2Framerates, err := v4l2detector.GetDeviceFramerates(devicePath, pixelFormat, width, height)
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

// StartMonitoring starts monitoring for device changes using udev and signal monitoring.
func (d *linuxDetector) StartMonitoring(ctx context.Context, broadcaster EventBroadcaster) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	// Store context and broadcaster
	d.ctx, d.cancel = context.WithCancel(ctx)
	d.broadcaster = broadcaster

	// Initialize with current devices
	devices, err := d.FindDevices()
	if err != nil {
		d.logger.Warn("Failed to get initial device list", "error", err)
	} else {
		for _, device := range devices {
			d.lastDevices[device.DeviceID] = device

			// Log initial device status
			switch device.Type {
			case DeviceTypeHDMI:
				status := v4l2detector.GetDVTimings(device.DevicePath)
				if device.Ready {
					d.logger.Info("HDMI device initialized with signal",
						"device_id", device.DeviceID,
						"path", device.DevicePath,
						"resolution", fmt.Sprintf("%dx%d", status.Width, status.Height),
						"fps", fmt.Sprintf("%.2f", status.FPS))
				} else {
					d.logger.Info("HDMI device initialized without signal",
						"device_id", device.DeviceID,
						"path", device.DevicePath,
						"state", signalStateString(status.State))
				}
			case DeviceTypeWebcam:
				d.logger.Debug("Webcam device initialized",
					"device_id", device.DeviceID,
					"path", device.DevicePath)
			}

			// Broadcast initial device state to StreamService
			d.broadcaster.BroadcastDeviceDiscovery("added", device, time.Now().Format(time.RFC3339))
		}
		d.logger.Info("Initialized with V4L2 devices", "count", len(devices))
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
			d.logger.Error("Udev monitor error", "error", err)
		}
	}()

	// Device monitoring goroutine
	go func() {
		d.logger.Info("Udev monitoring started for USB devices")
		for {
			select {
			case <-d.ctx.Done():
				d.logger.Info("Udev monitor stopped")
				return
			case dev, ok := <-deviceCh:
				if !ok {
					d.logger.Info("Udev device channel closed")
					return
				}

				action := dev.Action()
				if action == "add" || action == "remove" {
					d.logger.Debug("Udev event",
						"action", action, "device", dev.Syspath(), "subsystem", dev.Subsystem(), "devtype", dev.Devtype())

					// For add events, give kernel more time to enumerate V4L2 devices
					if action == "add" {
						time.Sleep(1 * time.Second)
					}

					d.checkAndBroadcastDeviceChanges()
				}
			}
		}
	}()

	// Start signal monitoring for HDMI devices
	go d.monitorDeviceSignals()

	return nil
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

// monitorDeviceSignals monitors HDMI devices using events and periodic checks.
func (d *linuxDetector) monitorDeviceSignals() {
	d.logger.Info("Signal monitoring started for HDMI devices")

	// Start periodic check for signal loss detection (30 seconds)
	go d.periodicSignalCheck()

	// Start event-based monitoring for HDMI devices without signal
	d.startEventMonitors()
}

// periodicSignalCheck checks HDMI devices that have signal for signal loss.
func (d *linuxDetector) periodicSignalCheck() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-d.ctx.Done():
			d.logger.Debug("Periodic signal check stopped")
			return
		case <-ticker.C:
			d.checkHDMISignals()
		}
	}
}

// checkHDMISignals checks only HDMI devices for signal status.
func (d *linuxDetector) checkHDMISignals() {
	d.mu.Lock()
	defer d.mu.Unlock()

	for deviceID, device := range d.lastDevices {
		// Skip non-HDMI devices (use cached type)
		if device.Type != DeviceTypeHDMI {
			continue
		}

		// Skip devices without signal - they're handled by event monitors
		if !device.Ready {
			continue
		}

		// Get current signal status using non-querying method (only for devices with signal)
		status := v4l2detector.GetDVTimings(device.DevicePath)
		newReady := (status.State == v4l2detector.SignalStateLocked)

		// Log periodic check at debug level
		if d.logger.Enabled(d.ctx, slog.LevelDebug) {
			if newReady {
				d.logger.Debug("HDMI device signal check",
					"device_id", deviceID,
					"path", device.DevicePath,
					"state", "locked",
					"resolution", fmt.Sprintf("%dx%d", status.Width, status.Height),
					"fps", fmt.Sprintf("%.2f", status.FPS))
			} else {
				d.logger.Debug("HDMI device signal check",
					"device_id", deviceID,
					"path", device.DevicePath,
					"state", signalStateString(status.State))
			}
		}

		// Check if status changed
		if device.Ready != newReady {
			if newReady {
				// Signal acquired
				d.logger.Info("HDMI device signal acquired",
					"device_id", deviceID,
					"device_name", device.DeviceName,
					"resolution", fmt.Sprintf("%dx%d", status.Width, status.Height),
					"fps", fmt.Sprintf("%.2f", status.FPS))
			} else {
				// Signal lost
				reason := signalStateString(status.State)
				d.logger.Warn("HDMI device signal lost",
					"device_id", deviceID,
					"device_name", device.DeviceName,
					"reason", reason)

				// Start event monitor for this device
				go d.monitorDeviceEvents(deviceID, device.DevicePath)
			}

			device.Ready = newReady
			d.lastDevices[deviceID] = device
			d.broadcaster.BroadcastDeviceDiscovery("status_changed", device, time.Now().Format(time.RFC3339))
		}
	}
}

// startEventMonitors starts event monitoring for HDMI devices without signal.
func (d *linuxDetector) startEventMonitors() {
	d.mu.Lock()
	defer d.mu.Unlock()

	for deviceID, device := range d.lastDevices {
		// Only monitor HDMI devices (use cached type)
		if device.Type != DeviceTypeHDMI {
			continue
		}

		// If device doesn't have signal, start event monitor
		if !device.Ready {
			go d.monitorDeviceEvents(deviceID, device.DevicePath)
		}
	}
}

// monitorDeviceEvents waits for source change events on a specific device.
func (d *linuxDetector) monitorDeviceEvents(deviceID, devicePath string) {
	d.logger.Debug("Starting event monitor for HDMI device", "device_id", deviceID)

	for {
		select {
		case <-d.ctx.Done():
			d.logger.Debug("Event monitor stopped", "device_id", deviceID)
			return
		default:
			// Wait for source change event (blocking with 60 second timeout)
			result, err := v4l2detector.WaitForSourceChange(devicePath, 60000)
			if err != nil {
				d.logger.Debug("Event monitoring not supported, falling back to polling only",
					"device_id", deviceID,
					"error", err)
				return
			}

			if result > 0 {
				d.logger.Debug("Source change event received", "device_id", deviceID, "changes", result)

				// Event occurred, check signal status
				status := v4l2detector.GetDVTimings(devicePath)
				ready := (status.State == v4l2detector.SignalStateLocked)

				d.mu.Lock()
				if device, exists := d.lastDevices[deviceID]; exists {
					if ready && !device.Ready {
						d.logger.Info("HDMI device signal acquired (via event)",
							"device_id", deviceID,
							"device_name", device.DeviceName,
							"resolution", fmt.Sprintf("%dx%d", status.Width, status.Height),
							"fps", fmt.Sprintf("%.2f", status.FPS))

						device.Ready = ready
						d.lastDevices[deviceID] = device
						d.broadcaster.BroadcastDeviceDiscovery("status_changed", device, time.Now().Format(time.RFC3339))
						d.mu.Unlock()

						// Signal is now present, stop event monitoring
						d.logger.Debug("Stopping event monitor, signal present", "device_id", deviceID)
						return
					} else if !ready {
						d.logger.Warn("Source change event but signal not locked",
							"device_id", deviceID,
							"state", signalStateString(status.State))
					}
				}
				d.mu.Unlock()
			}
		}
	}
}

// signalStateString converts signal state to human-readable string.
func signalStateString(state v4l2detector.SignalState) string {
	switch state {
	case v4l2detector.SignalStateNoLink:
		return "no_link"
	case v4l2detector.SignalStateNoSignal:
		return "no_signal"
	case v4l2detector.SignalStateUnstable:
		return "unstable"
	case v4l2detector.SignalStateLocked:
		return "locked"
	case v4l2detector.SignalStateOutOfRange:
		return "out_of_range"
	case v4l2detector.SignalStateNotSupported:
		return "not_supported"
	default:
		return "no_device"
	}
}

// checkAndBroadcastDeviceChanges checks for V4L2 device changes and broadcasts if needed.
func (d *linuxDetector) checkAndBroadcastDeviceChanges() {
	devices, err := d.FindDevices()
	if err != nil {
		d.logger.Error("Error getting device data", "error", err)
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
			d.logger.Info("Device removed", "device", oldDevice.DevicePath, "name", oldDevice.DeviceName, "id", deviceID)
			delete(d.lastDevices, deviceID)
		}
	}

	// Check for added devices
	for deviceID, newDevice := range currentDevices {
		oldDevice, exists := d.lastDevices[deviceID]

		if !exists {
			// New device
			d.broadcaster.BroadcastDeviceDiscovery("added", newDevice, time.Now().Format(time.RFC3339))
			d.logger.Info("Device added", "device", newDevice.DevicePath, "name", newDevice.DeviceName, "id", deviceID)
			d.lastDevices[deviceID] = newDevice

			// If it's an HDMI device without signal, start event monitoring (use cached type)
			if newDevice.Type == DeviceTypeHDMI && !newDevice.Ready {
				go d.monitorDeviceEvents(deviceID, newDevice.DevicePath)
			}
		} else if oldDevice != newDevice {
			// Device changed (shouldn't happen often)
			d.broadcaster.BroadcastDeviceDiscovery("changed", newDevice, time.Now().Format(time.RFC3339))
			d.logger.Info("Device changed", "device", newDevice.DevicePath, "name", newDevice.DeviceName, "id", deviceID)
			d.lastDevices[deviceID] = newDevice
		}
	}
}
