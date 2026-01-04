package streams

import (
	"time"

	"github.com/smazurov/videonode/internal/devices"
	"github.com/smazurov/videonode/internal/events"
)

// BroadcastDeviceDiscovery implements devices.EventBroadcaster interface
// Updates stream enabled state based on device readiness.
func (s *service) BroadcastDeviceDiscovery(_ string, device devices.DeviceInfo, _ string) {
	// Update stream enabled state
	s.streamsMutex.Lock()

	// Find streams using this device and update their enabled state
	allStreamConfigs := s.store.GetAllStreams()
	updated := false

	for streamID, streamConfig := range allStreamConfigs {
		if streamConfig.Device == device.DeviceID {
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

	// Collect stream IDs that need process start/stop while holding the lock
	var streamsToStart, streamsToStop []string
	if s.processManager != nil {
		for streamID, streamConfig := range allStreamConfigs {
			if streamConfig.Device == device.DeviceID {
				stream, exists := s.streams[streamID]
				if exists && stream.Enabled != device.Ready {
					if device.Ready {
						streamsToStart = append(streamsToStart, streamID)
					} else {
						streamsToStop = append(streamsToStop, streamID)
					}
				}
			}
		}
	}

	s.streamsMutex.Unlock()

	// Start/stop processes based on device state changes (outside of lock to avoid deadlock)
	for _, streamID := range streamsToStart {
		if startErr := s.processManager.Start(streamID); startErr != nil {
			s.logger.Warn("Failed to start stream process", "stream_id", streamID, "error", startErr)
		}
	}
	for _, streamID := range streamsToStop {
		if stopErr := s.processManager.Stop(streamID); stopErr != nil {
			s.logger.Warn("Failed to stop stream process", "stream_id", streamID, "error", stopErr)
		}
	}

	// Log summary if streams were updated
	if updated {
		s.logger.Debug("Updated stream states after device state change")
	}
}
