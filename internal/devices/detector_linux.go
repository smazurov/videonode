//go:build linux

package devices

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
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
		d.logger.Warn("Failed to get initial device list", "error", err)
	} else {
		for _, device := range devices {
			d.lastDevices[device.DeviceID] = device

			// Log initial device status
			switch device.Type {
			case DeviceTypeHDMI:
				status := v4l2.GetDVTimings(device.DevicePath)
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

	// Start hotplug monitoring via netlink
	go d.monitorHotplug()

	// Start signal monitoring for HDMI devices
	go d.monitorDeviceSignals()

	return nil
}

// monitorHotplug monitors for device additions/removals via netlink.
func (d *linuxDetector) monitorHotplug() {
	monitor, err := hotplug.NewMonitor()
	if err != nil {
		d.logger.Warn("Failed to create hotplug monitor, falling back to polling", "error", err)
		d.pollDeviceChanges()
		return
	}
	defer func() { _ = monitor.Close() }()

	// Filter for USB devices (which includes USB webcams and capture cards)
	monitor.AddSubsystemFilter(hotplug.SubsystemUSB)

	events := make(chan hotplug.Event, 32)
	go func() {
		if err := monitor.Run(d.ctx, events); err != nil && !errors.Is(err, context.Canceled) {
			d.logger.Error("Hotplug monitor error", "error", err)
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
					"action", event.Action,
					"devpath", event.DevPath,
					"devtype", event.DevType)

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
		status := v4l2.GetDVTimings(device.DevicePath)
		newReady := (status.State == v4l2.SignalStateLocked)

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
			result, err := v4l2.WaitForSourceChange(devicePath, 60000)
			if err != nil {
				d.logger.Debug("Event monitoring not supported, falling back to polling only",
					"device_id", deviceID,
					"error", err)
				return
			}

			if result > 0 {
				d.logger.Debug("Source change event received", "device_id", deviceID, "changes", result)

				// Use proper V4L2 workflow: QUERY_DV_TIMINGS + S_DV_TIMINGS + G_DV_TIMINGS
				// This is required because the driver doesn't auto-switch to detected timings
				const maxRetries = 5
				const retryDelay = 1 * time.Second
				var status v4l2.SignalStatus
				var ready bool
				var attempt int

				for attempt = 0; attempt < maxRetries; attempt++ {
					d.logger.Debug("Attempting signal detection",
						"device_id", deviceID,
						"attempt", attempt+1,
						"device_path", devicePath)

					// Step 1: Query detected timings from hardware
					timings, err := v4l2.QueryDVTimings(devicePath)
					if err != nil {
						d.logger.Debug("QueryDVTimings failed",
							"device_id", deviceID,
							"attempt", attempt+1,
							"error", err)
						time.Sleep(retryDelay)
						continue
					}
					d.logger.Debug("QueryDVTimings succeeded",
						"device_id", deviceID,
						"attempt", attempt+1)

					// Step 2: Apply detected timings to driver
					if err := v4l2.SetDVTimings(devicePath, timings); err != nil {
						d.logger.Debug("SetDVTimings failed",
							"device_id", deviceID,
							"attempt", attempt+1,
							"error", err)
						time.Sleep(retryDelay)
						continue
					}
					d.logger.Debug("SetDVTimings succeeded",
						"device_id", deviceID,
						"attempt", attempt+1)

					// Step 3: Verify signal is locked with GetDVTimings
					status = v4l2.GetDVTimings(devicePath)
					d.logger.Debug("GetDVTimings result",
						"device_id", deviceID,
						"state", signalStateString(status.State),
						"width", status.Width,
						"height", status.Height,
						"fps", fmt.Sprintf("%.2f", status.FPS))

					if status.State == v4l2.SignalStateLocked {
						ready = true
						d.logger.Info("Signal locked successfully",
							"device_id", deviceID,
							"resolution", fmt.Sprintf("%dx%d", status.Width, status.Height),
							"fps", fmt.Sprintf("%.2f", status.FPS),
							"attempts", attempt+1)
						break
					}

					d.logger.Debug("Signal not locked yet, retrying",
						"device_id", deviceID,
						"attempt", attempt+1,
						"state", signalStateString(status.State))
					time.Sleep(retryDelay)
				}

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
						d.logger.Error("Signal not locked after retries",
							"device_id", deviceID,
							"retries", attempt+1,
							"state", signalStateString(status.State))
					}
				}
				d.mu.Unlock()
			}
		}
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
