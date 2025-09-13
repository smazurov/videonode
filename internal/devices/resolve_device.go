package devices

import (
	"fmt"
	"os"
	"strings"
)

// ResolveDevicePath converts a device_id to a usable device path for FFmpeg
func ResolveDevicePath(deviceID string) (string, error) {
	// If it's already a full path, use it directly
	if strings.HasPrefix(deviceID, "/dev/") {
		return deviceID, nil
	}

	// Try by-id first (for USB devices)
	if strings.HasPrefix(deviceID, "usb-") {
		devicePath := "/dev/v4l/by-id/" + deviceID
		if _, err := os.Stat(devicePath); err == nil {
			return devicePath, nil
		}
	}

	// Try by-path (for platform devices and USB devices without by-id)
	if strings.HasPrefix(deviceID, "platform-") || strings.HasPrefix(deviceID, "usb-") {
		devicePath := "/dev/v4l/by-path/" + deviceID
		if _, err := os.Stat(devicePath); err == nil {
			return devicePath, nil
		}
	}

	return "", fmt.Errorf("no stable symlink found for device ID: %s", deviceID)
}
