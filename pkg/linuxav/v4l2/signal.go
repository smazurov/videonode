//go:build linux

package v4l2

import (
	"errors"
	"fmt"
	"syscall"
	"unsafe"

	"github.com/smazurov/videonode/internal/logging"
)

var logger = logging.GetLogger("v4l2")

// GetDeviceType returns the type of a V4L2 device (webcam, HDMI, or unknown).
func GetDeviceType(devicePath string) DeviceType {
	status := GetDeviceStatus(devicePath)
	return status.DeviceType
}

// GetDeviceStatus returns the combined device type and ready status.
func GetDeviceStatus(devicePath string) DeviceStatus {
	status := DeviceStatus{
		DeviceType: DeviceTypeUnknown,
		Ready:      false,
	}

	fd, err := syscall.Open(devicePath, syscall.O_RDWR|syscall.O_NONBLOCK, 0)
	if err != nil {
		return status
	}
	defer syscall.Close(fd)

	// Get capabilities to check driver
	capability := v4l2Capability{}
	if err := ioctl(fd, vidiocQuerycap, unsafe.Pointer(&capability)); err != nil {
		return status
	}

	// Try to get DV timings - if it works or returns specific errors, it's HDMI
	timings := v4l2DVTimings{}
	err = ioctl(fd, vidiocGDVTimings, unsafe.Pointer(&timings))

	if err == nil || errors.Is(err, syscall.ENOLINK) || errors.Is(err, syscall.ENOLCK) {
		// Device supports DV timings - it's an HDMI capture device
		status.DeviceType = DeviceTypeHDMI

		// Check if signal is locked
		if err == nil && timings.bt.width > 0 && timings.bt.height > 0 && timings.bt.pixelclock > 0 {
			status.Ready = true
		}
		return status
	}

	// Check if it's a UVC webcam
	driver := cstr(capability.driver[:])
	if driver == "uvcvideo" {
		status.DeviceType = DeviceTypeWebcam
		status.Ready = true
		return status
	}

	// Unknown device type, but openable means ready
	status.DeviceType = DeviceTypeUnknown
	status.Ready = true
	return status
}

// GetDVTimings returns the current DV timings and signal status for HDMI devices.
func GetDVTimings(devicePath string) SignalStatus {
	status := SignalStatus{
		State: SignalStateNoDevice,
	}

	fd, err := syscall.Open(devicePath, syscall.O_RDWR|syscall.O_NONBLOCK, 0)
	if err != nil {
		return status
	}
	defer syscall.Close(fd)

	timings := v4l2DVTimings{}
	err = ioctl(fd, vidiocGDVTimings, unsafe.Pointer(&timings))

	if err == nil {
		// Check if timings are valid
		if timings.bt.width > 0 && timings.bt.height > 0 && timings.bt.pixelclock > 0 {
			status.State = SignalStateLocked
			status.Width = timings.bt.width
			status.Height = timings.bt.height
			status.FPS = calculateFPS(&timings.bt)
			status.Interlaced = timings.bt.interlaced != 0
		} else {
			status.State = SignalStateNoSignal
		}
		return status
	}

	// Check specific error codes
	switch {
	case errors.Is(err, syscall.ENOLINK):
		status.State = SignalStateNoLink
	case errors.Is(err, syscall.ENOLCK):
		status.State = SignalStateUnstable
	case errors.Is(err, syscall.ERANGE):
		status.State = SignalStateOutOfRange
	case errors.Is(err, syscall.ENOTTY):
		status.State = SignalStateNotSupported
	default:
		status.State = SignalStateNoSignal
	}

	return status
}

// QueryDVTimings queries the detected DV timings from the hardware.
// This uses VIDIOC_QUERY_DV_TIMINGS which returns what the hardware is actually detecting
// from the incoming signal, as opposed to GetDVTimings which returns the configured timings.
// Returns the raw timings struct and an error.
// Error codes: ENOLINK = no cable, ENOLCK = signal unstable, ERANGE = out of range.
func QueryDVTimings(devicePath string) (*v4l2DVTimings, error) {
	fd, err := syscall.Open(devicePath, syscall.O_RDWR|syscall.O_NONBLOCK, 0)
	if err != nil {
		logger.Debug("QueryDVTimings: failed to open device",
			"device", devicePath,
			"error", err)
		return nil, err
	}
	defer syscall.Close(fd)

	timings := v4l2DVTimings{}
	err = ioctl(fd, vidiocQueryDVTimings, unsafe.Pointer(&timings))
	if err != nil {
		// Log the specific error
		var errName string
		switch {
		case errors.Is(err, syscall.ENOLINK):
			errName = "ENOLINK (no cable)"
		case errors.Is(err, syscall.ENOLCK):
			errName = "ENOLCK (signal unstable)"
		case errors.Is(err, syscall.ERANGE):
			errName = "ERANGE (out of range)"
		case errors.Is(err, syscall.ENOTTY):
			errName = "ENOTTY (not supported)"
		default:
			errName = err.Error()
		}
		logger.Debug("QueryDVTimings: ioctl failed",
			"device", devicePath,
			"error", errName,
			"errno", err)
		return nil, err
	}

	// Log successful detection
	fps := calculateFPS(&timings.bt)
	logger.Debug("QueryDVTimings: detected signal",
		"device", devicePath,
		"width", timings.bt.width,
		"height", timings.bt.height,
		"pixelclock", timings.bt.pixelclock,
		"fps", fmt.Sprintf("%.2f", fps),
		"interlaced", timings.bt.interlaced != 0)

	return &timings, nil
}

// SetDVTimings configures the driver with the specified DV timings.
// This uses VIDIOC_S_DV_TIMINGS to apply timings that were detected by QueryDVTimings.
// After calling this, GetDVTimings will return the newly configured timings.
func SetDVTimings(devicePath string, timings *v4l2DVTimings) error {
	if timings == nil {
		return errors.New("timings cannot be nil")
	}

	fd, err := syscall.Open(devicePath, syscall.O_RDWR|syscall.O_NONBLOCK, 0)
	if err != nil {
		logger.Debug("SetDVTimings: failed to open device",
			"device", devicePath,
			"error", err)
		return err
	}
	defer syscall.Close(fd)

	logger.Debug("SetDVTimings: applying timings",
		"device", devicePath,
		"width", timings.bt.width,
		"height", timings.bt.height,
		"pixelclock", timings.bt.pixelclock)

	err = ioctl(fd, vidiocSDVTimings, unsafe.Pointer(timings))
	if err != nil {
		logger.Debug("SetDVTimings: ioctl failed",
			"device", devicePath,
			"error", err)
		return err
	}

	logger.Debug("SetDVTimings: timings applied successfully",
		"device", devicePath)
	return nil
}

// WaitForSourceChange waits for a source change event with timeout.
// Returns the change flags on success, 0 on timeout, or an error.
func WaitForSourceChange(devicePath string, timeoutMs int) (int, error) {
	fd, err := syscall.Open(devicePath, syscall.O_RDWR|syscall.O_NONBLOCK, 0)
	if err != nil {
		return 0, err
	}
	defer syscall.Close(fd)

	// Subscribe to source change events
	sub := v4l2EventSubscription{
		typ: v4l2EventSourceChange,
	}

	if err := ioctl(fd, vidiocSubscribeEvent, unsafe.Pointer(&sub)); err != nil {
		if errors.Is(err, syscall.ENOTTY) || errors.Is(err, syscall.EINVAL) {
			return 0, ErrEventsNotSupported
		}
		return 0, err
	}

	// Ensure we unsubscribe when done
	defer func() { _ = ioctl(fd, vidiocUnsubscribeEvent, unsafe.Pointer(&sub)) }()

	// Wait for event using select with exception fd set (for V4L2 events)
	var exceptFds syscall.FdSet
	exceptFds.Bits[fd/64] |= 1 << (uint(fd) % 64)

	var tv *syscall.Timeval
	if timeoutMs > 0 {
		tv = makeTimeval(timeoutMs)
	}

	n, err := syscall.Select(fd+1, nil, nil, &exceptFds, tv)
	if err != nil {
		return 0, err
	}

	if n == 0 {
		return 0, nil // Timeout
	}

	// Dequeue the event
	event := v4l2Event{}
	if err := ioctl(fd, vidiocDqevent, unsafe.Pointer(&event)); err != nil {
		return 0, err
	}

	// Return the change flags
	return int(event.getSrcChangeChanges()), nil
}

// IsDeviceReady checks if a V4L2 device is ready (has signal for HDMI, exists for webcam).
func IsDeviceReady(devicePath string) bool {
	status := GetDeviceStatus(devicePath)
	return status.Ready
}

// calculateFPS calculates the frame rate from DV timings.
func calculateFPS(bt *v4l2BTTimings) float64 {
	if bt.pixelclock == 0 {
		return 0
	}

	totalWidth := uint64(bt.width + bt.hfrontporch + bt.hsync + bt.hbackporch)
	totalHeight := uint64(bt.height + bt.vfrontporch + bt.vsync + bt.vbackporch)

	if bt.interlaced != 0 {
		totalHeight /= 2
	}

	if totalWidth == 0 || totalHeight == 0 {
		return 0
	}

	return float64(bt.pixelclock) / float64(totalWidth*totalHeight)
}

// ErrEventsNotSupported is returned when the device doesn't support V4L2 events.
var ErrEventsNotSupported = syscall.ENOTSUP
