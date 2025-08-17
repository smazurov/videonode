package server

import (
	"log"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/smazurov/videonode/internal/config"
	"github.com/smazurov/videonode/internal/sse"
	"github.com/smazurov/videonode/v4l2_detector"
)

// Global stream manager
var GlobalStreamManager *config.StreamManager

// DeviceResolver maps stable device IDs to device paths
func DeviceResolver(stableID string) string {
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

// GetDevicesDataForSSE provides device data for the SSE manager
func GetDevicesDataForSSE() (sse.DeviceResponse, error) {
	devices, err := v4l2_detector.FindDevices()
	if err != nil {
		return sse.DeviceResponse{}, err
	}

	return sse.DeviceResponse{
		Devices: devices,
		Count:   len(devices),
	}, nil
}

// updateMediaMTXConfigFromStreams generates MediaMTX config from stream configurations
func updateMediaMTXConfigFromStreams() error {
	const mediamtxConfigPath = "mediamtx.yml"

	// Generate MediaMTX config from streams
	mtxConfig, err := GlobalStreamManager.ToMediaMTXConfig(DeviceResolver)
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

// StartServer sets up and starts the HTTP server
func StartServer(serverPort string) {
	// Create SSE manager
	sseManager := sse.New(
		sse.DefaultConfig(),
		GetDevicesDataForSSE,
	)
	// Initialize observability system
	if err := InitializeObservability(sseManager); err != nil {
		log.Fatalf("Failed to initialize observability: %v", err)
	}

	// Initialize stream manager
	GlobalStreamManager = config.NewStreamManager("streams.toml")
	if err := GlobalStreamManager.Load(); err != nil {
		log.Printf("Warning: Failed to load streams config: %v", err)
	} else {
		log.Printf("Loaded %d stream configurations", len(GlobalStreamManager.GetStreams()))

		// Load enabled streams into runtime storage (this will regenerate MediaMTX config with updated socket paths)
		LoadEnabledStreamsToRuntime()
	}

	// Ensure SSE manager and observability system are shut down when the function exits
	defer func() {
		ShutdownObservability()
		if err := sseManager.Shutdown(5 * time.Second); err != nil {
			log.Printf("Error shutting down SSE manager: %v", err)
		}
	}()

	r := chi.NewRouter()

	// Middleware
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	// r.Use(middleware.Timeout(30 * time.Second)) // Removed global timeout

	// CORS configuration
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"*"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Content-Type", "X-CSRF-Token"},
		ExposedHeaders:   []string{"Link"},
		AllowCredentials: true,
		MaxAge:           300,
	}))

	// Setup routes with SSE manager
	SetupRoutes(r, sseManager)

	// Register SSE handler
	// Clients will connect to /events/{channel_name}
	// e.g., /events/updates for the default channel
	r.Handle("/events/*", sseManager.GetHandler())

	// Start SSE manager (includes device monitoring)
	if err := sseManager.Start(); err != nil {
		log.Printf("Warning: Failed to start SSE manager: %v", err)
	}

	log.Printf("Starting server on http://localhost%s", serverPort)
	log.Printf("Note: Start MediaMTX separately with: ./mediamtx %s", mediamtxConfigPath)
	if err := http.ListenAndServe(serverPort, r); err != nil {
		log.Fatalf("Error starting server: %v", err)
	}
}
