package streams

import (
	"github.com/smazurov/videonode/internal/devices"
)

// MakeDeviceResolver maps a device id to its /dev/videoN path. An unplugged
// device has no live symlink, so it falls back to the by-id path the node will
// reappear at — letting the source spawn and self-heal once it's plugged back.
func MakeDeviceResolver() func(string) string {
	return func(deviceID string) string {
		if devicePath, err := devices.ResolveDevicePath(deviceID); err == nil {
			return devicePath
		}
		return "/dev/v4l/by-id/" + deviceID
	}
}
