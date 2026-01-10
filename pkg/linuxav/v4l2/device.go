//go:build linux

package v4l2

import (
	"bytes"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unsafe"
)

// FindDevices finds all V4L2 video capture devices on the system.
func FindDevices() ([]DeviceInfo, error) {
	entries, err := os.ReadDir("/sys/class/video4linux")
	if err != nil {
		if os.IsNotExist(err) {
			return []DeviceInfo{}, nil
		}
		return nil, fmt.Errorf("failed to read video4linux directory: %w", err)
	}

	devices := make([]DeviceInfo, 0, len(entries))

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		devicePath := "/dev/" + entry.Name()

		fd, openErr := open(devicePath)
		if openErr != nil {
			slog.With("component", "linuxav").Debug("failed to open video device", "path", devicePath, "error", openErr)
			continue
		}

		capability := v4l2Capability{}
		if err := ioctl(fd, vidiocQuerycap, unsafe.Pointer(&capability)); err != nil {
			slog.With("component", "linuxav").Debug("failed to query device capabilities", "path", devicePath, "error", err)
			_ = closefd(fd)
			continue
		}
		_ = closefd(fd)

		// Get the effective capabilities
		caps := capability.capabilities
		if caps&v4l2CapDeviceCaps != 0 {
			caps = capability.deviceCaps
		}

		// Only include video capture devices
		if caps&v4l2CapVideoCapture == 0 {
			continue
		}

		// Get device index from sysfs
		indexPath := filepath.Join("/sys/class/video4linux", entry.Name(), "index")
		indexValue := readSysfsInt(indexPath)

		// Find stable ID from /dev/v4l/by-id/
		stableID := findStableID(entry.Name(), indexValue)
		if stableID == "" {
			// Fallback: synthetic ID from bus_info + index
			busInfo := cstr(capability.busInfo[:])
			if strings.HasPrefix(busInfo, "usb-") {
				stableID = fmt.Sprintf("%s-video-index%d", busInfo, indexValue)
			} else {
				stableID = fmt.Sprintf("platform-%s-video-index%d", busInfo, indexValue)
			}
		}

		devices = append(devices, DeviceInfo{
			DevicePath: devicePath,
			DeviceName: cstr(capability.card[:]),
			DeviceID:   stableID,
			Caps:       caps,
		})
	}

	return devices, nil
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

// findStableID looks for a stable ID symlink in /dev/v4l/by-id/.
func findStableID(deviceName string, indexValue int) string {
	byIDDir := "/dev/v4l/by-id"
	entries, err := os.ReadDir(byIDDir)
	if err != nil {
		return ""
	}

	expectedSuffix := fmt.Sprintf("-video-index%d", indexValue)

	for _, entry := range entries {
		if entry.Type()&os.ModeSymlink == 0 {
			continue
		}

		linkPath := filepath.Join(byIDDir, entry.Name())
		target, readlinkErr := os.Readlink(linkPath)
		if readlinkErr != nil {
			continue
		}

		// Get the video device name from the target
		targetBase := filepath.Base(target)
		if targetBase == deviceName && strings.HasSuffix(entry.Name(), expectedSuffix) {
			return entry.Name()
		}
	}

	return ""
}

// readSysfsInt reads an integer value from a sysfs file.
func readSysfsInt(path string) int {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	val, _ := strconv.Atoi(strings.TrimSpace(string(data)))
	return val
}

// cstr converts a null-terminated byte slice to a Go string.
func cstr(b []byte) string {
	before, _, _ := bytes.Cut(b, []byte{0})
	return string(before)
}
