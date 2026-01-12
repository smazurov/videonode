package streams

import (
	"time"

	"github.com/smazurov/videonode/internal/devices"
	"github.com/smazurov/videonode/internal/events"
)

// BroadcastDeviceDiscovery implements devices.EventBroadcaster interface
// Updates stream enabled state based on device readiness.
func (s *service) BroadcastDeviceDiscovery(action string, device devices.DeviceInfo, _ string) {
	s.streamsMutex.Lock()

	// When device is removed, always treat as not ready
	deviceReady := device.Ready
	if action == "removed" {
		deviceReady = false
	}

	// Log the broadcast for debugging device-to-stream matching
	s.logger.Debug("Device broadcast received",
		"action", action,
		"device_id", device.DeviceID,
		"device_ready", deviceReady)

	allStreamConfigs := s.store.GetAllStreams()

	// Track streams that need process restart (state changed)
	var streamsToRestart []string
	updated := false
	matchFound := false

	for streamID, streamConfig := range allStreamConfigs {
		// Log each comparison for debugging
		if streamConfig.Device != "" {
			s.logger.Debug("Checking stream device match",
				"stream_id", streamID,
				"stream_device", streamConfig.Device,
				"discovered_device", device.DeviceID,
				"match", streamConfig.Device == device.DeviceID)
		}

		if streamConfig.Device == device.DeviceID {
			matchFound = true
			stream, exists := s.streams[streamID]
			if !exists {
				s.logger.Warn("Stream config exists but runtime state missing", "stream_id", streamID)
				continue
			}

			// Only update if the enabled state actually changed
			if stream.Enabled != deviceReady {
				// Capture that this stream needs restart BEFORE updating state
				if s.processManager != nil {
					streamsToRestart = append(streamsToRestart, streamID)
				}

				// Update runtime enabled state in-memory only
				stream.Enabled = deviceReady

				// Log the state change
				if deviceReady {
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
						Enabled:   deviceReady,
						Timestamp: time.Now().Format(time.RFC3339),
					})
				}

				updated = true
			}
		}
	}

	// Log if no streams matched this device
	if !matchFound && len(allStreamConfigs) > 0 {
		s.logger.Debug("No streams configured for device",
			"device_id", device.DeviceID,
			"stream_count", len(allStreamConfigs))
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
