//go:build linux

package v4l2

import (
	"errors"
	"syscall"
	"unsafe"
)

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
	cap := v4l2_capability{}
	if err := ioctl(fd, VIDIOC_QUERYCAP, unsafe.Pointer(&cap)); err != nil {
		return status
	}

	// Try to get DV timings - if it works or returns specific errors, it's HDMI
	timings := v4l2_dv_timings{}
	err = ioctl(fd, VIDIOC_G_DV_TIMINGS, unsafe.Pointer(&timings))

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
	driver := cstr(cap.driver[:])
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

	timings := v4l2_dv_timings{}
	err = ioctl(fd, VIDIOC_G_DV_TIMINGS, unsafe.Pointer(&timings))

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

// WaitForSourceChange waits for a source change event with timeout.
// Returns the change flags on success, 0 on timeout, or an error.
func WaitForSourceChange(devicePath string, timeoutMs int) (int, error) {
	fd, err := syscall.Open(devicePath, syscall.O_RDWR|syscall.O_NONBLOCK, 0)
	if err != nil {
		return 0, err
	}
	defer syscall.Close(fd)

	// Subscribe to source change events
	sub := v4l2_event_subscription{
		typ: V4L2_EVENT_SOURCE_CHANGE,
	}

	if subErr := ioctl(fd, VIDIOC_SUBSCRIBE_EVENT, unsafe.Pointer(&sub)); subErr != nil {
		if errors.Is(subErr, syscall.ENOTTY) || errors.Is(subErr, syscall.EINVAL) {
			return 0, ErrEventsNotSupported
		}
		return 0, subErr
	}

	// Ensure we unsubscribe when done
	defer func() { _ = ioctl(fd, VIDIOC_UNSUBSCRIBE_EVENT, unsafe.Pointer(&sub)) }()

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
	event := v4l2_event{}
	if err := ioctl(fd, VIDIOC_DQEVENT, unsafe.Pointer(&event)); err != nil {
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
func calculateFPS(bt *v4l2_bt_timings) float64 {
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
