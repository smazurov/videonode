package streams

import (
	"time"

	"github.com/smazurov/videonode/internal/devices"
	"github.com/smazurov/videonode/internal/events"
)

// BroadcastDeviceDiscovery implements devices.EventBroadcaster interface
// Updates stream enabled state based on device readiness and triggers MediaMTX sync
func (s *service) BroadcastDeviceDiscovery(action string, device devices.DeviceInfo, timestamp string) {
	// Update stream enabled state
	s.streamsMutex.Lock()

	// Find streams using this device and update their enabled state
	allStreamConfigs := s.store.GetAllStreams()
	updated := false

	for streamID, streamConfig := range allStreamConfigs {
		if streamConfig.Device == device.DeviceId {
			// Get the in-memory stream runtime state
			stream, exists := s.streams[streamID]
			if !exists {
				s.logger.Warn("Stream config exists but runtime state missing", "stream_id", streamID)
				continue
			}

			// Only update if the enabled state actually changed
			if stream.Enabled != device.Ready {
				// Update runtime enabled state in-memory only
				stream.Enabled = device.Ready

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

				// Emit stream state changed event if event bus is available
				if s.eventBus != nil {
					s.eventBus.Publish(events.StreamStateChangedEvent{
						StreamID:  streamID,
						Enabled:   device.Ready,
						Timestamp: time.Now().Format(time.RFC3339),
					})
				}

				updated = true
			}
		}
	}

	s.streamsMutex.Unlock()

	// Trigger MediaMTX sync if any streams were updated (outside of lock to avoid deadlock)
	if updated {
		s.logger.Debug("Triggering MediaMTX sync after device state change")
		if err := s.mediamtxClient.SyncAll(); err != nil {
			s.logger.Error("Failed to sync with MediaMTX", "error", err)
		}
	}
}