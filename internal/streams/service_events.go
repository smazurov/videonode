package streams

import (
	"time"

	"github.com/smazurov/videonode/internal/devices"
	"github.com/smazurov/videonode/internal/events"
)

// BroadcastDeviceDiscovery implements devices.EventBroadcaster interface
// Updates stream enabled state based on device readiness.
func (s *service) BroadcastDeviceDiscovery(_ string, device devices.DeviceInfo, _ string) {
	s.streamsMutex.Lock()

	allStreamConfigs := s.store.GetAllStreams()

	// Track streams that need process restart (state changed)
	var streamsToRestart []string
	updated := false

	for streamID, streamConfig := range allStreamConfigs {
		if streamConfig.Device == device.DeviceID {
			stream, exists := s.streams[streamID]
			if !exists {
				s.logger.Warn("Stream config exists but runtime state missing", "stream_id", streamID)
				continue
			}

			// Only update if the enabled state actually changed
			if stream.Enabled != device.Ready {
				// Capture that this stream needs restart BEFORE updating state
				if s.processManager != nil {
					streamsToRestart = append(streamsToRestart, streamID)
				}

				// Update runtime enabled state in-memory only
				stream.Enabled = device.Ready

				// Log the state change
				if device.Ready {
					s.logger.Info("Device ready, stream enabled",
						"stream_id", streamID,
						"device_id", device.DeviceID,
						"device_name", device.DeviceName)
				} else {
					s.logger.Info("Device not ready, stream disabled",
						"stream_id", streamID,
						"device_id", device.DeviceID,
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

	// Restart processes to apply new enabled state (outside lock to avoid deadlock)
	for _, streamID := range streamsToRestart {
		if err := s.processManager.Restart(streamID); err != nil {
			s.logger.Warn("Failed to restart stream process", "stream_id", streamID, "error", err)
		}
	}

	// Log summary if streams were updated
	if updated {
		s.logger.Debug("Updated stream states after device state change")
	}
}
