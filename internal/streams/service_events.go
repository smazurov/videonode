package streams

import (
	"github.com/smazurov/videonode/internal/devices"
)

// BroadcastDeviceDiscovery implements devices.EventBroadcaster.
//
// With a device pool configured (production path), this fans hotplug
// events out to it: the pool issues `Source.SetDevice` against the
// persistent source for the device_id. Stream-level `Enabled` no longer
// flips on device presence — sources stay alive and broadcast
// placeholders when their device is gone, encoders/composers see a
// stable SCM socket regardless. The canvas-input visibility map is
// still updated so the UI can show per-input presence.
//
// With no device pool (legacy / smoke), no per-stream restart is
// triggered. Hotplug events become diagnostics only.
func (s *service) BroadcastDeviceDiscovery(action string, device devices.DeviceInfo, _ string) {
	deviceReady := device.Ready
	if action == "removed" {
		deviceReady = false
	}
	devicePath := device.DevicePath
	if !deviceReady {
		devicePath = ""
	}

	s.logger.Debug("Device broadcast received",
		"action", action,
		"device_id", device.DeviceID,
		"device_ready", deviceReady,
		"device_path", devicePath)

	// Forward to the device pool. Pool decides whether to issue
	// SetDevice based on whether it manages this device_id. Called
	// outside any service lock to avoid deadlock with the pool's gRPC
	// round-trip (SetDevice can take up to SetDeviceTimeout).
	if s.devicePool != nil {
		s.devicePool.OnDeviceEvent(device.DeviceID, devicePath)
	}

	// Update per-canvas InputsEnabled visibility map. The composer keeps
	// running across device flaps, so we no longer restart anything from
	// this code path — but the UI still wants to see which inputs have
	// signal.
	s.streamsMutex.Lock()
	defer s.streamsMutex.Unlock()
	for streamID, cfg := range s.store.GetAllStreams() {
		if cfg.Canvas == nil {
			continue
		}
		stream, ok := s.streams[streamID]
		if !ok {
			continue
		}
		s.refreshCanvasInputVisibility(streamID, &cfg, stream, device.DeviceID, deviceReady)
	}
}

// refreshCanvasInputVisibility updates the per-canvas InputsEnabled flag
// for any input whose configured Device matches deviceID. UI-only —
// composer keeps running and the pool drives the underlying source.
// Caller holds streamsMutex.
func (s *service) refreshCanvasInputVisibility(streamID string, streamConfig *StreamSpec, stream *Stream, deviceID string, deviceReady bool) {
	if stream.InputsEnabled == nil {
		return
	}
	for _, srcID := range streamConfig.Canvas.SourceStreams {
		src, exists := s.store.GetStream(srcID)
		if !exists || src.Device != deviceID {
			continue
		}
		if stream.InputsEnabled[srcID] != deviceReady {
			stream.InputsEnabled[srcID] = deviceReady
			if deviceReady {
				s.logger.Info("Canvas source device ready",
					"stream_id", streamID, "source_id", srcID, "device_id", deviceID)
			} else {
				s.logger.Info("Canvas source device not ready",
					"stream_id", streamID, "source_id", srcID, "device_id", deviceID)
			}
		}
	}
}
