package streams

import (
	"time"

	"github.com/smazurov/videonode/internal/devices"
	"github.com/smazurov/videonode/internal/events"
)

// BroadcastDeviceDiscovery implements devices.EventBroadcaster.
func (s *service) BroadcastDeviceDiscovery(action string, device devices.DeviceInfo, _ string) {
	s.streamsMutex.Lock()

	deviceReady := device.Ready
	if action == "removed" {
		deviceReady = false
	}

	s.logger.Debug("Device broadcast received",
		"action", action,
		"device_id", device.DeviceID,
		"device_ready", deviceReady)

	allStreamConfigs := s.store.GetAllStreams()

	var streamsToRestart []string
	updated := false
	matchFound := false

	for streamID, streamConfig := range allStreamConfigs {
		stream, exists := s.streams[streamID]
		if !exists {
			continue
		}

		if streamConfig.Canvas != nil {
			changed := s.handleCanvasDeviceEvent(streamID, &streamConfig, stream, device.DeviceID, deviceReady)
			if changed {
				matchFound = true
				updated = true
				if s.processManager != nil {
					streamsToRestart = append(streamsToRestart, streamID)
				}
			}
		} else {
			changed := s.handleSingleDeviceEvent(streamID, &streamConfig, stream, device, deviceReady)
			if changed {
				matchFound = true
				updated = true
				if s.processManager != nil {
					streamsToRestart = append(streamsToRestart, streamID)
				}
			}
		}
	}

	if !matchFound && len(allStreamConfigs) > 0 {
		s.logger.Debug("No streams configured for device",
			"device_id", device.DeviceID,
			"stream_count", len(allStreamConfigs))
	}

	s.streamsMutex.Unlock()

	// Restart outside the lock to avoid deadlock with the process manager.
	for _, streamID := range streamsToRestart {
		if err := s.processManager.Restart(streamID); err != nil {
			s.logger.Warn("Failed to restart stream process", "stream_id", streamID, "error", err)
		}
	}

	if updated {
		s.logger.Debug("Updated stream states after device state change")
	}
}

// handleSingleDeviceEvent updates a single-camera stream's enabled flag. Caller holds streamsMutex.
func (s *service) handleSingleDeviceEvent(streamID string, streamConfig *StreamSpec, stream *Stream, device devices.DeviceInfo, deviceReady bool) bool {
	if streamConfig.Device == "" {
		return false
	}

	s.logger.Debug("Checking stream device match",
		"stream_id", streamID,
		"stream_device", streamConfig.Device,
		"discovered_device", device.DeviceID,
		"match", streamConfig.Device == device.DeviceID)

	if streamConfig.Device != device.DeviceID {
		return false
	}

	if stream.Enabled == deviceReady {
		return false
	}

	stream.Enabled = deviceReady

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

	if s.eventBus != nil {
		s.eventBus.Publish(events.StreamStateChangedEvent{
			StreamID:  streamID,
			Enabled:   deviceReady,
			Timestamp: time.Now().Format(time.RFC3339),
		})
	}

	return true
}

// handleCanvasDeviceEvent flips InputsEnabled per source matching deviceID. Caller holds streamsMutex.
func (s *service) handleCanvasDeviceEvent(streamID string, streamConfig *StreamSpec, stream *Stream, deviceID string, deviceReady bool) bool {
	if stream.InputsEnabled == nil {
		return false
	}

	stateChanged := false
	for _, srcID := range streamConfig.Canvas.SourceStreams {
		src, exists := s.store.GetStream(srcID)
		if !exists || src.Device != deviceID {
			continue
		}

		if stream.InputsEnabled[srcID] != deviceReady {
			stream.InputsEnabled[srcID] = deviceReady
			stateChanged = true

			if deviceReady {
				s.logger.Info("Canvas source device ready",
					"stream_id", streamID, "source_id", srcID, "device_id", deviceID)
			} else {
				s.logger.Info("Canvas source device not ready",
					"stream_id", streamID, "source_id", srcID, "device_id", deviceID)
			}
		}
	}

	if stateChanged && s.eventBus != nil {
		s.eventBus.Publish(events.StreamStateChangedEvent{
			StreamID:  streamID,
			Enabled:   true,
			Timestamp: time.Now().Format(time.RFC3339),
		})
	}

	return stateChanged
}
