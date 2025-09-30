package streams

import (
	"time"

	"github.com/smazurov/videonode/internal/devices"
)

// BroadcastDeviceDiscovery implements devices.EventBroadcaster interface
// Updates stream enabled state based on device readiness and triggers MediaMTX sync
func (s *service) BroadcastDeviceDiscovery(action string, device devices.DeviceInfo, timestamp string) {
	s.streamsMutex.Lock()
	defer s.streamsMutex.Unlock()

	// Find streams using this device and update their enabled state
	allStreams := s.store.GetAllStreams()
	updated := false

	for streamID, streamConfig := range allStreams {
		if streamConfig.Device == device.DeviceId {
			// Only update if the enabled state actually changed
			if streamConfig.Enabled != device.Ready {
				// Update enabled state (not persisted to disk due to toml:"-" tag)
				streamConfig.Enabled = device.Ready
				if err := s.store.UpdateStream(streamID, streamConfig); err != nil {
					s.logger.Error("Failed to update stream enabled state", "stream_id", streamID, "error", err)
					continue
				}

				// Log the state change
				if device.Ready {
					s.logger.Info("Device ready, stream enabled",
						"stream_id", streamID,
						"device_id", device.DeviceId,
						"device_name", device.DeviceName)
				} else {
					s.logger.Info("Device not ready, stream disabled",
						"stream_id", streamID,
						"device_id", device.DeviceId,
						"device_name", device.DeviceName)
				}

				// Emit stream state changed event if broadcaster is available
				if s.eventBroadcaster != nil {
					s.eventBroadcaster(streamID, device.Ready, time.Now().Format(time.RFC3339))
				}

				updated = true
			}
		}
	}

	// Trigger MediaMTX sync if any streams were updated
	if updated {
		s.logger.Debug("Triggering MediaMTX sync after device state change")
		if err := s.mediamtxClient.SyncAll(); err != nil {
			s.logger.Error("Failed to sync with MediaMTX", "error", err)
		}
	}
}