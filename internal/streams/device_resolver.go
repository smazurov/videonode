package streams

import (
	"github.com/smazurov/videonode/internal/devices"
)

// MakeDeviceResolver maps opaque device ids (USB bus-port etc.) to canonical
// /dev/videoN paths, or "" on failure. Failures aren't logged here — every
// caller already logs or surfaces the resulting ApplySource error.
func MakeDeviceResolver() func(string) string {
	return func(deviceID string) string {
		devicePath, err := devices.ResolveDevicePath(deviceID)
		if err != nil {
			return ""
		}
		return devicePath
	}
}
