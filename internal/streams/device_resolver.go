package streams

import (
	"github.com/smazurov/videonode/internal/devices"
	"github.com/smazurov/videonode/internal/logging"
)

// MakeDeviceResolver returns a function mapping opaque device ids (USB
// bus-port etc.) to canonical /dev/videoN paths, or "" when resolution
// fails. Used by the pipeline package's Pipeline.Config (which lives
// outside the streams package).
func MakeDeviceResolver(logger logging.Logger) func(string) string {
	return func(deviceID string) string {
		devicePath, err := devices.ResolveDevicePath(deviceID)
		if err != nil {
			logger.Warn("Device resolution failed", logging.KeyDeviceID, deviceID, logging.KeyError, err)
			return ""
		}
		return devicePath
	}
}
