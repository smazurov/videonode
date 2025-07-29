package server

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	streamconfig "github.com/smazurov/videonode/internal/config"
	"github.com/smazurov/videonode/internal/ffmpeg"
	"github.com/smazurov/videonode/internal/mediamtx"
	"github.com/smazurov/videonode/internal/sse"
	"github.com/smazurov/videonode/v4l2_detector"
)

// MediaMTX configuration file path
const mediamtxConfigPath = "mediamtx.yml"

// Note: FFmpeg monitoring is now handled by the obs package via AddFFmpegStreamMonitoring

// getStableDeviceID converts device path to stable device ID
func getStableDeviceID(devicePath string) string {
	devices, err := v4l2_detector.FindDevices()
	if err != nil {
		log.Printf("Error finding devices for stable ID lookup: %v", err)
		return ""
	}

	for _, device := range devices {
		if device.DevicePath == devicePath {
			return device.DeviceId
		}
	}
	return ""
}

// deviceResolver maps stable device IDs to device paths (local version)
func deviceResolver(stableID string) string {
	devices, err := v4l2_detector.FindDevices()
	if err != nil {
		log.Printf("Error finding devices for resolution: %v", err)
		return ""
	}

	for _, device := range devices {
		if device.DeviceId == stableID {
			return device.DevicePath
		}
	}
	return ""
}

// regenerateMediaMTXConfig generates MediaMTX config from TOML stream configurations
func regenerateMediaMTXConfig() error {
	const mediamtxConfigPath = "mediamtx.yml"

	if GlobalStreamManager == nil {
		return fmt.Errorf("stream manager not initialized")
	}

	// Generate MediaMTX config from streams using device resolver
	mtxConfig, err := GlobalStreamManager.ToMediaMTXConfig(deviceResolver)
	if err != nil {
		return err
	}

	// Write to MediaMTX config file
	if err := mtxConfig.WriteToFile(mediamtxConfigPath); err != nil {
		return err
	}

	enabledCount := len(GlobalStreamManager.GetEnabledStreams())
	log.Printf("Generated MediaMTX config with %d enabled streams", enabledCount)
	return nil
}

// LoadEnabledStreamsToRuntime populates runtime storage with enabled streams from TOML config
func LoadEnabledStreamsToRuntime() {
	if GlobalStreamManager == nil {
		return
	}

	enabledStreams := GlobalStreamManager.GetEnabledStreams()
	deviceStreamsMutex.Lock()
	defer deviceStreamsMutex.Unlock()

	needsConfigUpdate := false

	for streamID, stream := range enabledStreams {
		// Resolve device stable ID to current device path
		devicePath := DeviceResolver(stream.Device)
		if devicePath == "" {
			log.Printf("Warning: Device %s (stable ID: %s) not found, skipping stream %s", stream.Device, stream.Device, stream.ID)
			continue
		}

		// Always generate new timestamped socket path on server restart to avoid conflicts
		timestamp := time.Now().Unix()
		oldSocketPath := stream.ProgressSocket
		stream.ProgressSocket = fmt.Sprintf("/tmp/ffmpeg-progress-%s-%d.sock", stream.ID, timestamp)

		if oldSocketPath != stream.ProgressSocket {
			log.Printf("Updated progress socket for stream %s: %s -> %s", stream.ID, oldSocketPath, stream.ProgressSocket)

			// Update the stream in TOML config
			if err := GlobalStreamManager.UpdateStream(streamID, stream); err != nil {
				log.Printf("Failed to update stream %s with progress socket: %v", streamID, err)
			} else {
				log.Printf("Updated TOML config with new progress socket for stream %s", stream.ID)
				needsConfigUpdate = true
			}
		}

		// Create stream response for runtime storage
		response := StreamResponse{
			StreamID:   stream.ID,
			DevicePath: devicePath,
			Codec:      stream.Codec,
			IsRunning:  true,
			StartTime:  stream.CreatedAt,
			WebRTCURL:  mediamtx.GetWebRTCURL(stream.ID),
			RTSPURL:    mediamtx.GetRTSPURL(stream.ID),
		}

		deviceStreams[stream.ID] = response
		log.Printf("Loaded stream %s for device %s into runtime storage", stream.ID, devicePath)

		// Start FFmpeg progress monitoring using the socket path from TOML config
		if stream.ProgressSocket != "" {
			// Add FFmpeg monitoring with obs package
			logPath := "" // No log path for existing streams, only socket monitoring
			if err := AddFFmpegStreamMonitoring(stream.ID, stream.ProgressSocket, logPath); err != nil {
				log.Printf("Failed to start monitoring for existing stream %s: %v", stream.ID, err)
			} else {
				log.Printf("Started FFmpeg progress monitoring for existing stream %s on socket: %s", stream.ID, stream.ProgressSocket)
			}
		} else {
			log.Printf("No progress socket configured for stream %s, skipping monitoring", stream.ID)
		}
	}

	// Regenerate MediaMTX config if we updated any socket paths
	if needsConfigUpdate {
		if err := regenerateMediaMTXConfig(); err != nil {
			log.Printf("Failed to regenerate MediaMTX config after updating progress sockets: %v", err)
		} else {
			log.Printf("Regenerated MediaMTX config with updated progress sockets")
		}
	}

	log.Printf("Loaded %d enabled streams into runtime storage", len(enabledStreams))
}

// createStreamHandler creates a new MediaMTX stream configuration
func createStreamHandler(sseManager *sse.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			StreamID      string   `json:"stream_id"`
			Device        string   `json:"device"`
			Format        string   `json:"format"`
			Resolution    string   `json:"resolution"`
			FPS           string   `json:"fps"`
			Codec         string   `json:"codec"`
			Preset        string   `json:"preset"`
			FFmpegOptions []string `json:"ffmpeg_options"`
		}

		// Parse form data first (HTMX sends form data)
		if err := r.ParseForm(); err != nil {
			log.Printf("Failed to parse form: %v", err)
			handleError(w, "Failed to parse request", err, http.StatusBadRequest)
			return
		}

		req.StreamID = r.FormValue("stream_id")
		req.Device = r.FormValue("device")
		req.Format = r.FormValue("format")
		req.Resolution = r.FormValue("resolution")
		req.FPS = r.FormValue("fps")
		req.Codec = r.FormValue("codec")
		req.Preset = r.FormValue("preset")
		req.FFmpegOptions = r.Form["ffmpeg_options"] // Get all selected checkboxes

		log.Printf("Create stream request: id=%s, device=%s, format=%s, resolution=%s, fps=%s, codec=%s, preset=%s, ffmpeg_options=%v",
			req.StreamID, req.Device, req.Format, req.Resolution, req.FPS, req.Codec, req.Preset, req.FFmpegOptions)

		if req.StreamID == "" {
			log.Printf("Stream ID is empty")
			handleError(w, "Stream ID is required", nil, http.StatusBadRequest)
			return
		}

		if req.Device == "" {
			log.Printf("Device path is empty")
			handleError(w, "Device path is required", nil, http.StatusBadRequest)
			return
		}

		// Use the provided stream ID as the path name for MediaMTX
		pathName := req.StreamID

		// Load or create MediaMTX configuration
		config, err := mediamtx.LoadFromFile(mediamtxConfigPath)
		if err != nil {
			log.Printf("Failed to load MediaMTX config: %v", err)
			handleError(w, "Failed to load MediaMTX configuration", err, http.StatusInternalServerError)
			return
		}

		// Generate socket path with timestamp to avoid conflicts
		timestamp := time.Now().Unix()
		socketPath := fmt.Sprintf("/tmp/ffmpeg-progress-%s-%d.sock", pathName, timestamp)

		// Convert string FFmpeg options to typed options
		var ffmpegOptions []ffmpeg.OptionType
		for _, optionStr := range req.FFmpegOptions {
			ffmpegOptions = append(ffmpegOptions, ffmpeg.OptionType(optionStr))
		}

		// Add stream to MediaMTX configuration
		streamConfig := mediamtx.StreamConfig{
			DevicePath:     req.Device,
			InputFormat:    req.Format,
			Resolution:     req.Resolution,
			FPS:            req.FPS,
			Codec:          req.Codec,
			Preset:         req.Preset,
			FFmpegOptions:  ffmpegOptions,
			ProgressSocket: socketPath,
		}

		err = config.AddStream(pathName, streamConfig)
		if err != nil {
			log.Printf("Failed to add stream to config: %v", err)
			handleError(w, "Failed to configure stream", err, http.StatusInternalServerError)
			return
		}

		// Write updated configuration to file
		err = config.WriteToFile(mediamtxConfigPath)
		if err != nil {
			log.Printf("Failed to write MediaMTX config: %v", err)
			handleError(w, "Failed to save MediaMTX configuration", err, http.StatusInternalServerError)
			return
		}

		log.Printf("Added stream %s to MediaMTX config for device %s", pathName, req.Device)

		// Save to TOML config for persistence and regenerate MediaMTX config properly
		if GlobalStreamManager != nil {
			// Convert device path to stable device ID for TOML config
			stableID := getStableDeviceID(req.Device)
			if stableID == "" {
				log.Printf("Warning: Could not find stable ID for device %s", req.Device)
				stableID = req.Device // fallback to device path
			}

			streamConfigTOML := streamconfig.StreamConfig{
				ID:             req.StreamID,
				Name:           req.StreamID, // Use stream ID as name, user can change later
				Device:         stableID,
				Enabled:        true,
				InputFormat:    req.Format,
				Resolution:     req.Resolution,
				FPS:            req.FPS,
				Codec:          req.Codec,
				Preset:         req.Preset,
				FFmpegOptions:  ffmpegOptions,
				ProgressSocket: socketPath, // Save the actual socket path used
			}

			// Send stream chart config for this specific stream
			streamChartConfig := map[string]interface{}{
				"id":         fmt.Sprintf("stream-%s-chart", pathName),
				"type":       "line",
				"title":      fmt.Sprintf("Stream %s Bytes Over Time", pathName),
				"yAxisLabel": "Bytes",
				"yAxisStart": "zero",
				"maxPoints":  60,
				"datasets": []map[string]interface{}{
					{
						"label":           "Bytes Received",
						"borderColor":     "#3B82F6",
						"backgroundColor": "#3B82F620",
					},
					{
						"label":           "Bytes Sent",
						"borderColor":     "#10B981",
						"backgroundColor": "#10B98120",
					},
				},
			}
			sseManager.BroadcastCustomEvent("chart-config", streamChartConfig)
			log.Printf("Sent stream chart config for stream %s", pathName)

			if err := GlobalStreamManager.AddStream(streamConfigTOML); err != nil {
				log.Printf("Warning: Failed to save stream to TOML config: %v", err)
			} else {
				log.Printf("Saved stream %s to persistent TOML config", pathName)

				// Regenerate MediaMTX config from TOML using stable device resolution
				if err := regenerateMediaMTXConfig(); err != nil {
					log.Printf("Warning: Failed to regenerate MediaMTX config: %v", err)
				} else {
					log.Printf("Regenerated MediaMTX config with stable device paths")
				}
			}
		}

		// Create a stream response with MediaMTX URLs
		response := StreamResponse{
			StreamID:   pathName,
			DevicePath: req.Device,
			Codec:      req.Codec,
			IsRunning:  true,
			StartTime:  time.Now(),
			WebRTCURL:  mediamtx.GetWebRTCURL(pathName),
			RTSPURL:    mediamtx.GetRTSPURL(pathName),
		}

		// Store the stream
		deviceStreamsMutex.Lock()
		deviceStreams[pathName] = response
		deviceStreamsMutex.Unlock()

		log.Printf("Created MediaMTX stream: %s for device %s", pathName, req.Device)
		log.Printf("WebRTC URL: %s", response.WebRTCURL)
		log.Printf("RTSP URL: %s", response.RTSPURL)

		// Start FFmpeg progress monitoring using the actual socket path
		logPath := "" // No log path for created streams, only socket monitoring
		if err := AddFFmpegStreamMonitoring(pathName, socketPath, logPath); err != nil {
			log.Printf("Failed to start monitoring for stream %s: %v", pathName, err)
		} else {
			log.Printf("Started FFmpeg progress monitoring for stream %s on socket: %s", pathName, socketPath)
		}


		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}
}

// stopStreamFromParamsHandler stops an existing MediaMTX stream
func stopStreamFromParamsHandler(sseManager *sse.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Parse form data (HTMX sends form data)
		if err := r.ParseForm(); err != nil {
			log.Printf("Failed to parse form: %v", err)
			handleError(w, "Failed to parse request", err, http.StatusBadRequest)
			return
		}

		streamID := r.FormValue("stream_id")

		if streamID == "" {
			handleError(w, "Stream ID is required", nil, http.StatusBadRequest)
			return
		}

		// Use the stream ID directly
		pathName := streamID

		// Load MediaMTX configuration
		config, err := mediamtx.LoadFromFile(mediamtxConfigPath)
		if err != nil {
			log.Printf("Failed to load MediaMTX config: %v", err)
			handleError(w, "Failed to load MediaMTX configuration", err, http.StatusInternalServerError)
			return
		}

		// Remove stream from MediaMTX configuration
		config.RemoveStream(pathName)

		// Write updated configuration to file
		err = config.WriteToFile(mediamtxConfigPath)
		if err != nil {
			log.Printf("Failed to write MediaMTX config: %v", err)
			handleError(w, "Failed to save MediaMTX configuration", err, http.StatusInternalServerError)
			return
		}

		log.Printf("Removed stream %s from MediaMTX config", pathName)

		// Remove from device streams
		deviceStreamsMutex.Lock()
		delete(deviceStreams, pathName)
		deviceStreamsMutex.Unlock()

		// Stop FFmpeg progress monitoring
		if err := RemoveFFmpegStreamMonitoring(pathName); err != nil {
			log.Printf("Failed to stop monitoring for stream %s: %v", pathName, err)
		} else {
			log.Printf("Stopped FFmpeg progress monitoring for stream %s", pathName)
		}

		// Also remove from TOML config for persistence
		if GlobalStreamManager != nil {
			// Send chart removal event for this specific stream
			sseManager.BroadcastCustomEvent("chart-remove", map[string]interface{}{
				"chartId": fmt.Sprintf("stream-%s-chart", pathName),
			})
			log.Printf("Sent stream chart removal event for stream %s", pathName)

			if err := GlobalStreamManager.RemoveStream(pathName); err != nil {
				log.Printf("Warning: Failed to remove stream from TOML config: %v", err)
			} else {
				log.Printf("Removed stream %s from persistent TOML config", pathName)

				// Regenerate MediaMTX config from TOML
				if err := regenerateMediaMTXConfig(); err != nil {
					log.Printf("Warning: Failed to regenerate MediaMTX config: %v", err)
				} else {
					log.Printf("Regenerated MediaMTX config after removing stream")
				}
			}
		}

		log.Printf("Stopped stream: %s", pathName)


		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(ApiResponse{
			Status:  "ok",
			Message: "Stream stopped",
		})
	}
}
