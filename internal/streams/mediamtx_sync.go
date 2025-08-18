package streams

import (
	"fmt"
	"log"
	"os"
	"time"

	streamconfig "github.com/smazurov/videonode/internal/config"
	"github.com/smazurov/videonode/v4l2_detector"
)

// SyncAllStreamsToMediaMTX regenerates the entire MediaMTX configuration from streams.toml
// Returns a map of streamID -> socketPath for the newly generated socket paths
func SyncAllStreamsToMediaMTX(manager *streamconfig.StreamManager, configPath string) (map[string]string, error) {
	if manager == nil {
		return nil, fmt.Errorf("stream manager is nil")
	}

	// Create device resolver function
	deviceResolver := func(deviceID string) string {
		devices, err := v4l2_detector.FindDevices()
		if err != nil {
			log.Printf("Error finding devices for resolution: %v", err)
			return deviceID
		}

		for _, device := range devices {
			if device.DeviceId == deviceID {
				log.Printf("Resolved device %s to %s", deviceID, device.DevicePath)
				return device.DevicePath
			}
		}

		// If not found by stable ID, return as-is (might be a direct path)
		return deviceID
	}

	// Generate fresh socket paths for all enabled streams
	socketPaths := make(map[string]string)
	for _, stream := range manager.GetEnabledStreams() {
		// Generate fresh socket path with timestamp and random component
		socketPath := fmt.Sprintf("/tmp/ffmpeg-progress-%s-%d-%d.sock",
			stream.ID, time.Now().Unix(), time.Now().UnixNano()%1000000)
		socketPaths[stream.ID] = socketPath

		// Clean up any old socket files that might exist
		if err := os.Remove(socketPath); err != nil && !os.IsNotExist(err) {
			log.Printf("Warning: Failed to clean up old socket file %s: %v", socketPath, err)
		}
	}

	// Generate MediaMTX config from all enabled streams with fresh socket paths
	config, err := manager.ToMediaMTXConfig(deviceResolver, socketPaths)
	if err != nil {
		return nil, fmt.Errorf("failed to generate MediaMTX config: %w", err)
	}

	// Write to file
	if err := config.WriteToFile(configPath); err != nil {
		return nil, fmt.Errorf("failed to write MediaMTX config: %w", err)
	}

	log.Printf("Synchronized MediaMTX config with %d paths", len(config.Paths))
	return socketPaths, nil
}
