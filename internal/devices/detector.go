package devices

import (
	"context"
)

// DeviceInfo represents information about a V4L2 device
type DeviceInfo struct {
	DevicePath string
	DeviceName string
	DeviceId   string
	Caps       uint32
}

// EventBroadcaster interface for broadcasting device events
type EventBroadcaster interface {
	BroadcastDeviceDiscovery(action string, device DeviceInfo, timestamp string)
}

// FormatInfo represents information about a video format
type FormatInfo struct {
	PixelFormat uint32
	FormatName  string
	Emulated    bool
}

// Resolution represents a video resolution
type Resolution struct {
	Width  uint32
	Height uint32
}

// Framerate represents a video framerate
type Framerate struct {
	Numerator   uint32
	Denominator uint32
}

// DeviceDetector provides platform-specific device detection
type DeviceDetector interface {
	// FindDevices returns all currently available V4L2 devices
	FindDevices() ([]DeviceInfo, error)

	// GetDeviceFormats returns supported formats for a device
	GetDeviceFormats(devicePath string) ([]FormatInfo, error)

	// GetDevicePathByID returns the device path for a given device ID
	GetDevicePathByID(deviceID string) (string, error)

	// GetDeviceResolutions returns supported resolutions for a format
	GetDeviceResolutions(devicePath string, pixelFormat uint32) ([]Resolution, error)

	// GetDeviceFramerates returns supported framerates for a resolution
	GetDeviceFramerates(devicePath string, pixelFormat uint32, width, height uint32) ([]Framerate, error)

	// StartMonitoring starts monitoring for device changes
	StartMonitoring(ctx context.Context, broadcaster EventBroadcaster) error

	// StopMonitoring stops the device monitoring
	StopMonitoring()
}

// NewDetector creates a platform-specific device detector
func NewDetector() DeviceDetector {
	return newDetector()
}

// ConvertToAPIDeviceInfo converts internal DeviceInfo to API model
func ConvertToAPIDeviceInfo(device DeviceInfo) interface{} {
	// This will be used to convert to api/models.DeviceInfo
	// We'll update this when we refactor the imports
	return struct {
		DevicePath string
		DeviceName string
		DeviceId   string
		Caps       uint32
	}{
		DevicePath: device.DevicePath,
		DeviceName: device.DeviceName,
		DeviceId:   device.DeviceId,
		Caps:       device.Caps,
	}
}
