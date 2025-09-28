//go:build linux

package v4l2_detector

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

// Go representation of C.struct_v4l2_device_info
type DeviceInfo struct {
	DevicePath string
	DeviceName string
	DeviceId   string
	Caps       uint32
}

// Go representation of C.struct_v4l2_format_info
type FormatInfo struct {
	PixelFormat uint32
	FormatName  string
	Emulated    bool
}

// Go representation of C.struct_v4l2_resolution
type Resolution struct {
	Width  uint32
	Height uint32
}

// Go representation of C.struct_v4l2_framerate
type Framerate struct {
	Numerator   uint32
	Denominator uint32
}

// Go representation of C.struct_v4l2_control_info
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

// Go representation of C.struct_v4l2_menu_item
type MenuItem struct {
	ID    uint32
	Index uint32
	Name  string
}

func FindDevices() ([]DeviceInfo, error) {
	findDevicesMutex.Lock()
	defer findDevicesMutex.Unlock()

	var cDevices *C.struct_v4l2_device_info
	var cCount C.size_t

	ret := C.v4l2_find_devices(&cDevices, &cCount)
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

		deviceId := C.GoString(cDev.device_id)

		goDevices[i] = DeviceInfo{
			DevicePath: devicePath,
			DeviceName: deviceName,
			DeviceId:   deviceId,
			Caps:       uint32(cDev.caps),
		}
	}

	return goDevices, nil
}

// GetDevicePathByID finds the device path for a given stable device ID
func GetDevicePathByID(deviceID string) (string, error) {
	devices, err := FindDevices()
	if err != nil {
		return "", fmt.Errorf("failed to find devices: %w", err)
	}

	for _, device := range devices {
		if device.DeviceId == deviceID {
			return device.DevicePath, nil
		}
	}

	return "", fmt.Errorf("device with ID %s not found", deviceID)
}

// GetDeviceFormats returns all supported formats for a device
func GetDeviceFormats(devicePath string) ([]FormatInfo, error) {
	var cFormats *C.struct_v4l2_format_info
	var cCount C.size_t

	cDevicePath := C.CString(devicePath)
	defer C.free(unsafe.Pointer(cDevicePath))

	ret := C.v4l2_get_formats(cDevicePath, &cFormats, &cCount)
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

// GetDeviceResolutions returns all supported resolutions for a device and format
func GetDeviceResolutions(devicePath string, pixelFormat uint32) ([]Resolution, error) {
	var cResolutions *C.struct_v4l2_resolution
	var cCount C.size_t

	cDevicePath := C.CString(devicePath)
	defer C.free(unsafe.Pointer(cDevicePath))

	ret := C.v4l2_get_resolutions(cDevicePath, C.uint32_t(pixelFormat), &cResolutions, &cCount)
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

// GetDeviceFramerates returns all supported framerates for a device, format, and resolution
func GetDeviceFramerates(devicePath string, pixelFormat uint32, width, height uint32) ([]Framerate, error) {
	var cFramerates *C.struct_v4l2_framerate
	var cCount C.size_t

	cDevicePath := C.CString(devicePath)
	defer C.free(unsafe.Pointer(cDevicePath))

	ret := C.v4l2_get_framerates(cDevicePath, C.uint32_t(pixelFormat), C.int(width), C.int(height), &cFramerates, &cCount)
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

// GetFPS converts a framerate to frames per second as a float
func (f Framerate) GetFPS() float64 {
	if f.Numerator == 0 {
		return 0
	}
	return float64(f.Denominator) / float64(f.Numerator)
}
