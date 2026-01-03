//go:build linux

package v4l2detector

/*
#cgo CFLAGS: -I./src
#cgo LDFLAGS: -L./build -lv4l2_detector_lib
#cgo pkg-config: libv4l2 libavcodec libavutil libavformat libudev

#include "v4l2_detector.h"
#include <stdlib.h> // Required for C.free
*/
import "C"

import (
	"fmt"
	"log/slog"
	"sync"
	"unsafe"
)

var findDevicesMutex sync.Mutex

// DeviceInfo is a Go representation of C.struct_v4l2_device_info.
type DeviceInfo struct {
	DevicePath string
	DeviceName string
	DeviceID   string
	Caps       uint32
}

// FormatInfo is a Go representation of C.struct_v4l2_format_info.
type FormatInfo struct {
	PixelFormat uint32
	FormatName  string
	Emulated    bool
}

// Resolution is a Go representation of C.struct_v4l2_resolution.
type Resolution struct {
	Width  uint32
	Height uint32
}

// Framerate is a Go representation of C.struct_v4l2_framerate.
type Framerate struct {
	Numerator   uint32
	Denominator uint32
}

// ControlInfo is a Go representation of C.struct_v4l2_control_info.
type ControlInfo struct {
	ID           uint32
	Name         string
	Type         int32
	Min          int32
	Max          int32
	Step         int32
	DefaultValue int32
	Flags        uint32
}

// MenuItem is a Go representation of C.struct_v4l2_menu_item.
type MenuItem struct {
	ID    uint32
	Index uint32
	Name  string
}

// FindDevices finds all V4L2 devices available on the system.
func FindDevices() ([]DeviceInfo, error) {
	findDevicesMutex.Lock()
	defer findDevicesMutex.Unlock()

	var cDevices *C.struct_v4l2_device_info
	var cCount C.size_t

	ret := C.v4l2_find_devices(&cDevices, &cCount) //nolint:gocritic // CGO false positive
	if ret != 0 {
		return nil, fmt.Errorf("v4l2_find_devices failed with code: %d", ret)
	}

	// Handle case where no devices found or NULL returned
	if cDevices == nil || cCount == 0 {
		return []DeviceInfo{}, nil
	}

	// Defer freeing the C array of device_info structs
	defer C.v4l2_free_devices(cDevices, cCount)

	goDevices := make([]DeviceInfo, cCount)
	// Convert C array pointer to a Go slice header - now safe after NULL check
	cDeviceSlice := (*[1 << 30]C.struct_v4l2_device_info)(unsafe.Pointer(cDevices))[:cCount:cCount]

	for i := range cDeviceSlice {
		cDev := cDeviceSlice[i]
		// Convert C strings to Go strings and immediately free the C allocated memory
		devicePath := C.GoString(cDev.device_path)

		deviceName := C.GoString(cDev.device_name)

		deviceID := C.GoString(cDev.device_id)

		goDevices[i] = DeviceInfo{
			DevicePath: devicePath,
			DeviceName: deviceName,
			DeviceID:   deviceID,
			Caps:       uint32(cDev.caps),
		}
	}

	return goDevices, nil
}

// GetDevicePathByID finds the device path for a given stable device ID.
func GetDevicePathByID(deviceID string) (string, error) {
	devices, err := FindDevices()
	if err != nil {
		return "", fmt.Errorf("failed to find devices: %w", err)
	}

	for _, device := range devices {
		if device.DeviceID == deviceID {
			return device.DevicePath, nil
		}
	}

	return "", fmt.Errorf("device with ID %s not found", deviceID)
}

// GetDeviceFormats returns all supported formats for a device.
func GetDeviceFormats(devicePath string) ([]FormatInfo, error) {
	var cFormats *C.struct_v4l2_format_info
	var cCount C.size_t

	cDevicePath := C.CString(devicePath)
	defer C.free(unsafe.Pointer(cDevicePath))

	ret := C.v4l2_get_formats(cDevicePath, &cFormats, &cCount) //nolint:gocritic // CGO false positive
	if ret != 0 {
		return nil, fmt.Errorf("v4l2_get_formats failed with code: %d", ret)
	}

	if cCount == 0 {
		return nil, nil
	}

	// Convert C array to Go slice
	cFormatSlice := (*[1 << 30]C.struct_v4l2_format_info)(unsafe.Pointer(cFormats))[:cCount:cCount]
	goFormats := make([]FormatInfo, cCount)

	for i := range cFormatSlice {
		cFmt := cFormatSlice[i]
		formatName := C.GoString(cFmt.format_name)

		goFormats[i] = FormatInfo{
			PixelFormat: uint32(cFmt.pixel_format),
			FormatName:  formatName,
			Emulated:    bool(cFmt.emulated),
		}

		// Free the C string
		C.free(unsafe.Pointer(cFmt.format_name))
	}

	// Free the C array
	C.free(unsafe.Pointer(cFormats))

	return goFormats, nil
}

// GetDeviceResolutions returns all supported resolutions for a device and format.
func GetDeviceResolutions(devicePath string, pixelFormat uint32) ([]Resolution, error) {
	var cResolutions *C.struct_v4l2_resolution
	var cCount C.size_t

	cDevicePath := C.CString(devicePath)
	defer C.free(unsafe.Pointer(cDevicePath))

	ret := C.v4l2_get_resolutions(cDevicePath, C.uint32_t(pixelFormat), &cResolutions, &cCount) //nolint:gocritic // CGO false positive
	if ret != 0 {
		// Return empty list for ENOTTY (-25) - multiplanar devices don't support resolution enumeration
		if ret == -25 {
			logger := slog.With("component", "v4l2_detector")
			logger.Info("Resolution enumeration not supported for multiplanar device", "device_path", devicePath, "pixel_format", pixelFormat)
			return []Resolution{}, nil
		}
		return nil, fmt.Errorf("v4l2_get_resolutions failed with code: %d", ret)
	}

	if cCount == 0 {
		return nil, nil
	}

	// Convert C array to Go slice
	cResSlice := (*[1 << 30]C.struct_v4l2_resolution)(unsafe.Pointer(cResolutions))[:cCount:cCount]
	goResolutions := make([]Resolution, cCount)

	for i := range cResSlice {
		cRes := cResSlice[i]
		goResolutions[i] = Resolution{
			Width:  uint32(cRes.width),
			Height: uint32(cRes.height),
		}
	}

	// Free the C array
	C.free(unsafe.Pointer(cResolutions))

	return goResolutions, nil
}

// GetDeviceFramerates returns all supported framerates for a device, format, and resolution.
func GetDeviceFramerates(devicePath string, pixelFormat uint32, width, height uint32) ([]Framerate, error) {
	var cFramerates *C.struct_v4l2_framerate
	var cCount C.size_t

	cDevicePath := C.CString(devicePath)
	defer C.free(unsafe.Pointer(cDevicePath))

	ret := C.v4l2_get_framerates(cDevicePath, C.uint32_t(pixelFormat), C.int(width), C.int(height), &cFramerates, &cCount) //nolint:gocritic // CGO false positive
	if ret != 0 {
		return nil, fmt.Errorf("v4l2_get_framerates failed with code: %d", ret)
	}

	if cCount == 0 {
		return nil, nil
	}

	// Convert C array to Go slice
	cRateSlice := (*[1 << 30]C.struct_v4l2_framerate)(unsafe.Pointer(cFramerates))[:cCount:cCount]
	goFramerates := make([]Framerate, cCount)

	for i := range cRateSlice {
		cRate := cRateSlice[i]
		goFramerates[i] = Framerate{
			Numerator:   uint32(cRate.numerator),
			Denominator: uint32(cRate.denominator),
		}
	}

	// Free the C array
	C.free(unsafe.Pointer(cFramerates))

	return goFramerates, nil
}

// GetFPS converts a framerate to frames per second as a float.
func (f Framerate) GetFPS() float64 {
	if f.Numerator == 0 {
		return 0
	}
	return float64(f.Denominator) / float64(f.Numerator)
}

// DeviceType represents the type of V4L2 device.
type DeviceType int

// DeviceType constants define the types of V4L2 devices.
const (
	DeviceTypeWebcam  DeviceType = 0
	DeviceTypeHDMI    DeviceType = 1
	DeviceTypeUnknown DeviceType = -1
)

// SignalState represents the state of a video signal.
type SignalState int

// SignalState constants define the states of a video signal.
const (
	SignalStateNoDevice     SignalState = -1
	SignalStateNoLink       SignalState = 0 // No cable connected
	SignalStateNoSignal     SignalState = 1 // Cable connected, no signal
	SignalStateUnstable     SignalState = 2 // Signal present but unstable
	SignalStateLocked       SignalState = 3 // Signal locked and stable
	SignalStateOutOfRange   SignalState = 4 // Signal out of supported range
	SignalStateNotSupported SignalState = 5 // Device doesn't support DV timings
)

// SignalStatus contains detailed signal information.
type SignalStatus struct {
	State      SignalState
	Width      uint32
	Height     uint32
	FPS        float64
	Interlaced bool
}

// GetDeviceType returns the type of a V4L2 device.
func GetDeviceType(devicePath string) DeviceType {
	cPath := C.CString(devicePath)
	defer C.free(unsafe.Pointer(cPath))

	deviceType := C.v4l2_get_device_type(cPath)
	return DeviceType(deviceType)
}

// GetDVTimings gets current DV timings and signal status (non-querying).
func GetDVTimings(devicePath string) SignalStatus {
	cPath := C.CString(devicePath)
	defer C.free(unsafe.Pointer(cPath))

	cStatus := C.v4l2_get_dv_timings(cPath)

	return SignalStatus{
		State:      SignalState(cStatus.state),
		Width:      uint32(cStatus.width),
		Height:     uint32(cStatus.height),
		FPS:        float64(cStatus.fps),
		Interlaced: cStatus.interlaced != 0,
	}
}

// WaitForSourceChange waits for a source change event (blocking).
func WaitForSourceChange(devicePath string, timeoutMs int) (int, error) {
	cPath := C.CString(devicePath)
	defer C.free(unsafe.Pointer(cPath))

	result := C.v4l2_wait_for_source_change(cPath, C.int(timeoutMs))

	if result < 0 {
		if result == -2 {
			return 0, fmt.Errorf("device does not support events")
		}
		return 0, fmt.Errorf("error waiting for source change: %d", result)
	}

	return int(result), nil
}

// IsDeviceReady checks if a V4L2 device is ready (has signal for HDMI, exists for webcam).
func IsDeviceReady(devicePath string) bool {
	cPath := C.CString(devicePath)
	defer C.free(unsafe.Pointer(cPath))

	ready := C.v4l2_device_is_ready(cPath)
	return ready == 1
}

// DeviceStatus contains combined device type and ready status.
type DeviceStatus struct {
	DeviceType DeviceType
	Ready      bool
}

// GetDeviceStatus returns device type and ready status in a single device open.
func GetDeviceStatus(devicePath string) DeviceStatus {
	cPath := C.CString(devicePath)
	defer C.free(unsafe.Pointer(cPath))

	cStatus := C.v4l2_get_device_status(cPath)

	return DeviceStatus{
		DeviceType: DeviceType(cStatus.device_type),
		Ready:      cStatus.ready == 1,
	}
}
