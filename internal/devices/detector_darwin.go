//go:build darwin

package devices

import (
	"context"
	"fmt"
	"log"
)

// Mock device constants for testing on macOS
var mockDevices = []DeviceInfo{
	{
		DevicePath: "/dev/video0",
		DeviceName: "Mock USB Webcam HD",
		DeviceId:   "usb-mock-webcam-001",
		Caps:       0x84000001, // VIDEO_CAPTURE | STREAMING | DEVICE_CAPS
	},
	{
		DevicePath: "/dev/video1",
		DeviceName: "Mock HDMI Capture Device",
		DeviceId:   "usb-mock-hdmi-capture",
		Caps:       0x84000001, // VIDEO_CAPTURE | STREAMING | DEVICE_CAPS
	},
}

// Mock formats for each device
var mockFormats = map[string][]FormatInfo{
	"/dev/video0": {
		{PixelFormat: 1196444237, FormatName: "MJPEG", Emulated: false},
		{PixelFormat: 1448695129, FormatName: "YUYV 4:2:2", Emulated: false},
	},
	"/dev/video1": {
		{PixelFormat: 842094158, FormatName: "NV12", Emulated: false},
		{PixelFormat: 1448695129, FormatName: "YUYV 4:2:2", Emulated: false},
		{PixelFormat: 861030210, FormatName: "BGR3", Emulated: false},
	},
}

// Mock resolutions for each device/format combination
var mockResolutions = map[string]map[uint32][]Resolution{
	"/dev/video0": {
		1196444237: { // MJPEG
			{Width: 640, Height: 480},
			{Width: 1280, Height: 720},
			{Width: 1920, Height: 1080},
		},
		1448695129: { // YUYV422
			{Width: 640, Height: 480},
			{Width: 1280, Height: 720},
			{Width: 1920, Height: 1080},
		},
	},
	"/dev/video1": {
		842094158: { // NV12
			{Width: 1920, Height: 1080},
			{Width: 3840, Height: 2160},
		},
		1448695129: { // YUYV422
			{Width: 1920, Height: 1080},
			{Width: 3840, Height: 2160},
		},
		861030210: { // BGR24
			{Width: 1920, Height: 1080},
		},
	},
}

// Mock framerates for each device/format/resolution combination
var mockFramerates = map[string]map[uint32]map[string][]Framerate{
	"/dev/video0": {
		1196444237: { // MJPEG
			"640x480":   {{Numerator: 1, Denominator: 30}, {Numerator: 1, Denominator: 60}},
			"1280x720":  {{Numerator: 1, Denominator: 30}, {Numerator: 1, Denominator: 60}},
			"1920x1080": {{Numerator: 1, Denominator: 30}},
		},
		1448695129: { // YUYV422
			"640x480":   {{Numerator: 1, Denominator: 30}, {Numerator: 1, Denominator: 60}},
			"1280x720":  {{Numerator: 1, Denominator: 30}, {Numerator: 1, Denominator: 60}},
			"1920x1080": {{Numerator: 1, Denominator: 30}},
		},
	},
	"/dev/video1": {
		842094158: { // NV12
			"1920x1080": {{Numerator: 1, Denominator: 30}, {Numerator: 1, Denominator: 60}},
			"3840x2160": {{Numerator: 1, Denominator: 30}},
		},
		1448695129: { // YUYV422
			"1920x1080": {{Numerator: 1, Denominator: 30}, {Numerator: 1, Denominator: 60}},
			"3840x2160": {{Numerator: 1, Denominator: 30}},
		},
		861030210: { // BGR24
			"1920x1080": {{Numerator: 1, Denominator: 30}, {Numerator: 1, Denominator: 60}},
		},
	},
}

type darwinDetector struct{}

func newDetector() DeviceDetector {
	log.Println("INFO: Using mock V4L2 devices for testing on macOS")
	return &darwinDetector{}
}

// FindDevices returns mock devices for testing on macOS
func (d *darwinDetector) FindDevices() ([]DeviceInfo, error) {
	return mockDevices, nil
}

// GetDeviceFormats returns mock formats for the device
func (d *darwinDetector) GetDeviceFormats(devicePath string) ([]FormatInfo, error) {
	formats, exists := mockFormats[devicePath]
	if !exists {
		return nil, fmt.Errorf("device not found: %s", devicePath)
	}
	return formats, nil
}

// GetDevicePathByID returns the device path for a given mock device ID
func (d *darwinDetector) GetDevicePathByID(deviceID string) (string, error) {
	for _, device := range mockDevices {
		if device.DeviceId == deviceID {
			return device.DevicePath, nil
		}
	}
	return "", fmt.Errorf("device ID not found: %s", deviceID)
}

// GetDeviceResolutions returns mock resolutions for the format
func (d *darwinDetector) GetDeviceResolutions(devicePath string, pixelFormat uint32) ([]Resolution, error) {
	deviceResolutions, exists := mockResolutions[devicePath]
	if !exists {
		return nil, fmt.Errorf("device not found: %s", devicePath)
	}
	
	resolutions, exists := deviceResolutions[pixelFormat]
	if !exists {
		return nil, fmt.Errorf("format not supported: %d", pixelFormat)
	}
	
	return resolutions, nil
}

// GetDeviceFramerates returns mock framerates for the resolution
func (d *darwinDetector) GetDeviceFramerates(devicePath string, pixelFormat uint32, width, height uint32) ([]Framerate, error) {
	deviceFramerates, exists := mockFramerates[devicePath]
	if !exists {
		return nil, fmt.Errorf("device not found: %s", devicePath)
	}
	
	formatFramerates, exists := deviceFramerates[pixelFormat]
	if !exists {
		return nil, fmt.Errorf("format not supported: %d", pixelFormat)
	}
	
	resKey := fmt.Sprintf("%dx%d", width, height)
	framerates, exists := formatFramerates[resKey]
	if !exists {
		return nil, fmt.Errorf("resolution not supported: %s", resKey)
	}
	
	return framerates, nil
}

// StartMonitoring is a no-op on macOS
func (d *darwinDetector) StartMonitoring(ctx context.Context, broadcaster EventBroadcaster) error {
	log.Println("Device monitoring not available on macOS - V4L2 is Linux-only")
	return nil
}

// StopMonitoring is a no-op on macOS
func (d *darwinDetector) StopMonitoring() {
	// Nothing to stop
}