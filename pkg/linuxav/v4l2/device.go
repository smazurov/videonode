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
	"syscall"
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

	var devices []DeviceInfo

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		devicePath := "/dev/" + entry.Name()

		fd, err := open(devicePath)
		if err != nil {
			slog.With("component", "linuxav").Debug("failed to open video device", "path", devicePath, "error", err)
			continue
		}

		cap := v4l2_capability{}
		if err := ioctl(fd, VIDIOC_QUERYCAP, unsafe.Pointer(&cap)); err != nil {
			slog.With("component", "linuxav").Debug("failed to query device capabilities", "path", devicePath, "error", err)
			close(fd)
			continue
		}
		close(fd)

		// Get the effective capabilities
		caps := cap.capabilities
		if caps&V4L2_CAP_DEVICE_CAPS != 0 {
			caps = cap.device_caps
		}

		// Only include video capture devices
		if caps&V4L2_CAP_VIDEO_CAPTURE == 0 {
			continue
		}

		// Get device index from sysfs
		indexPath := filepath.Join("/sys/class/video4linux", entry.Name(), "index")
		indexValue := readSysfsInt(indexPath)

		// Find stable ID from /dev/v4l/by-id/
		stableID := findStableID(entry.Name(), indexValue)
		if stableID == "" {
			// Fallback: synthetic ID from bus_info + index
			busInfo := cstr(cap.bus_info[:])
			if strings.HasPrefix(busInfo, "usb-") {
				stableID = fmt.Sprintf("%s-video-index%d", busInfo, indexValue)
			} else {
				stableID = fmt.Sprintf("platform-%s-video-index%d", busInfo, indexValue)
			}
		}

		devices = append(devices, DeviceInfo{
			DevicePath: devicePath,
			DeviceName: cstr(cap.card[:]),
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

// findStableID looks for a stable ID symlink in /dev/v4l/by-id/
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
		target, err := os.Readlink(linkPath)
		if err != nil {
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
	if i := bytes.IndexByte(b, 0); i >= 0 {
		return string(b[:i])
	}
	return string(b)
}

// getDeviceCapability queries the device capabilities.
func getDeviceCapability(devicePath string) (*v4l2_capability, error) {
	fd, err := syscall.Open(devicePath, syscall.O_RDWR|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, err
	}
	defer syscall.Close(fd)

	cap := &v4l2_capability{}
	if err := ioctl(fd, VIDIOC_QUERYCAP, unsafe.Pointer(cap)); err != nil {
		return nil, err
	}

	return cap, nil
}
