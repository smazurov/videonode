package server

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/smazurov/videonode/internal/sse"

	"github.com/go-chi/chi/v5"
	"github.com/smazurov/videonode/internal/capture"
	"github.com/smazurov/videonode/internal/encoders"
	"github.com/smazurov/videonode/v4l2_detector"
)


var globalEncoderCache = NewEncoderCache(15 * time.Minute)

// Simple in-memory store for device streams
var deviceStreams = make(map[string]StreamResponse)
var deviceStreamsMutex sync.RWMutex


func handleError(w http.ResponseWriter, message string, err error, statusCode int) {
	errorMsg := message
	if err != nil {
		errorMsg = fmt.Sprintf("%s: %v", message, err)
	}

	response := ApiResponse{
		Status:  "error",
		Message: errorMsg,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(response)
}

// GetDevicesData fetches the list of available video devices
func GetDevicesData() (DeviceResponse, error) {
	devices, err := v4l2_detector.FindDevices()
	if err != nil {
		return DeviceResponse{}, fmt.Errorf("failed to find devices: %w", err)
	}

	response := DeviceResponse{
		Devices: devices,
		Count:   len(devices),
	}
	return response, nil
}

// V4L2ToFFmpegFormat maps V4L2 pixel format codes to FFmpeg input format names
func V4L2ToFFmpegFormat(pixelFormat uint32) (string, error) {
	switch pixelFormat {
	case 1448695129: // YUYV
		return "yuyv422", nil
	case 842094158: // NV12 (Y/UV 4:2:0)
		return "nv12", nil
	case 875967048: // H264
		return "h264", nil
	case 1196444237: // MJPEG
		return "mjpeg", nil
	case 842093913: // YU12/I420
		return "yuv420p", nil
	case 842094169: // YV12
		return "yuv420p", nil
	default:
		return "", fmt.Errorf("unsupported V4L2 pixel format: %d", pixelFormat)
	}
}

// GetDeviceCapabilities fetches all capabilities for a specific device
func GetDeviceCapabilities(devicePath string) (DeviceCapabilitiesResponse, error) {
	v4l2Formats, err := v4l2_detector.GetDeviceFormats(devicePath)
	if err != nil {
		return DeviceCapabilitiesResponse{}, fmt.Errorf("failed to get device formats: %w", err)
	}

	// Convert V4L2 formats to FFmpeg formats
	formats := make([]v4l2_detector.FormatInfo, 0, len(v4l2Formats))
	for _, v4l2Format := range v4l2Formats {
		ffmpegFormat, err := V4L2ToFFmpegFormat(v4l2Format.PixelFormat)
		if err != nil {
			// Skip unsupported formats instead of failing completely
			fmt.Printf("Warning: %v\n", err)
			continue
		}
		formats = append(formats, v4l2_detector.FormatInfo{
			PixelFormat: v4l2Format.PixelFormat, // Keep original for resolution/framerate queries
			FormatName:  fmt.Sprintf("%s (%s)", v4l2Format.FormatName, ffmpegFormat),
			Emulated:    v4l2Format.Emulated,
		})
	}

	response := DeviceCapabilitiesResponse{
		DevicePath: devicePath,
		Formats:    formats,
	}

	return response, nil
}

// listDevicesHandler returns a list of all available video devices (HTMX fragment or JSON)
func listDevicesHandler(w http.ResponseWriter, r *http.Request) {
	response, err := GetDevicesData()
	if err != nil {
		handleError(w, "Failed to get devices", err, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// deviceCapabilitiesHandler returns all capabilities for a specific device
func deviceCapabilitiesHandler(w http.ResponseWriter, r *http.Request) {
	devicePath := chi.URLParam(r, "devicePath")
	if devicePath == "" {
		handleError(w, "Device path is required", nil, http.StatusBadRequest)
		return
	}

	// Decode the device path (it comes URL encoded)
	devicePath = strings.ReplaceAll(devicePath, "%2F", "/")

	capabilities, err := GetDeviceCapabilities(devicePath)
	if err != nil {
		handleError(w, "Failed to get device capabilities", err, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(capabilities)
}

// deviceResolutionsHandler returns resolutions for a specific device and format
func deviceResolutionsHandler(w http.ResponseWriter, r *http.Request) {
	devicePath := chi.URLParam(r, "devicePath")
	if devicePath == "" {
		handleError(w, "Device path is required", nil, http.StatusBadRequest)
		return
	}

	formatStr := r.URL.Query().Get("format")
	if formatStr == "" {
		handleError(w, "Format parameter is required", nil, http.StatusBadRequest)
		return
	}

	// Decode the device path
	devicePath = strings.ReplaceAll(devicePath, "%2F", "/")

	// Parse pixel format from string
	var pixelFormat uint32
	fmt.Sscanf(formatStr, "%d", &pixelFormat)

	resolutions, err := v4l2_detector.GetDeviceResolutions(devicePath, pixelFormat)
	if err != nil {
		handleError(w, "Failed to get device resolutions", err, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resolutions)
}

// deviceFrameratesHandler returns framerates for a specific device, format, and resolution
func deviceFrameratesHandler(w http.ResponseWriter, r *http.Request) {
	devicePath := chi.URLParam(r, "devicePath")
	if devicePath == "" {
		handleError(w, "Device path is required", nil, http.StatusBadRequest)
		return
	}

	formatStr := r.URL.Query().Get("format")
	widthStr := r.URL.Query().Get("width")
	heightStr := r.URL.Query().Get("height")

	if formatStr == "" || widthStr == "" || heightStr == "" {
		handleError(w, "Format, width, and height parameters are required", nil, http.StatusBadRequest)
		return
	}

	// Decode the device path
	devicePath = strings.ReplaceAll(devicePath, "%2F", "/")

	// Parse parameters
	var pixelFormat, width, height uint32
	fmt.Sscanf(formatStr, "%d", &pixelFormat)
	fmt.Sscanf(widthStr, "%d", &width)
	fmt.Sscanf(heightStr, "%d", &height)

	framerates, err := v4l2_detector.GetDeviceFramerates(devicePath, pixelFormat, width, height)
	if err != nil {
		handleError(w, "Failed to get device framerates", err, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(framerates)
}


// getValidatedEncoders returns only encoders that passed validation and are saved in validated_encoders.toml
func getValidatedEncoders() (*encoders.EncoderList, error) {
	// Load validation results from file
	validationFile := "validated_encoders.toml"
	results, err := encoders.LoadValidationResults(validationFile)
	if err != nil {
		// If no validation file exists, return error - system needs to be validated first
		return nil, fmt.Errorf("validation file not found - run encoder validation first: %w", err)
	}

	// Get all available encoders from system
	allEncoders, err := globalEncoderCache.Get()
	if err != nil {
		return nil, fmt.Errorf("failed to get system encoders: %w", err)
	}

	// Create a map of all available encoders for lookup
	availableMap := make(map[string]encoders.Encoder)
	for _, encoder := range allEncoders.VideoEncoders {
		availableMap[encoder.Name] = encoder
	}
	for _, encoder := range allEncoders.AudioEncoders {
		availableMap[encoder.Name] = encoder
	}
	for _, encoder := range allEncoders.SubtitleEncoders {
		availableMap[encoder.Name] = encoder
	}
	for _, encoder := range allEncoders.OtherEncoders {
		availableMap[encoder.Name] = encoder
	}

	// Create encoder list to hold only validated working encoders
	validatedList := &encoders.EncoderList{
		VideoEncoders:    []encoders.Encoder{},
		AudioEncoders:    []encoders.Encoder{},
		SubtitleEncoders: []encoders.Encoder{},
		OtherEncoders:    []encoders.Encoder{},
	}

	// Add working encoders from validation results - follow validation strictly
	for _, encoderName := range results.H264.Working {
		// Use existing encoder info if available, otherwise create a basic entry
		if encoder, exists := availableMap[encoderName]; exists {
			validatedList.VideoEncoders = append(validatedList.VideoEncoders, encoder)
		} else {
			// Create encoder entry for validated encoder even if not in current system list
			validatedList.VideoEncoders = append(validatedList.VideoEncoders, encoders.Encoder{
				Type:        "V",
				Name:        encoderName,
				Description: "H.264 encoder",
				HWAccel:     !strings.Contains(encoderName, "lib"), // lib* are software encoders
			})
		}
	}

	for _, encoderName := range results.H265.Working {
		// Use existing encoder info if available, otherwise create a basic entry
		if encoder, exists := availableMap[encoderName]; exists {
			validatedList.VideoEncoders = append(validatedList.VideoEncoders, encoder)
		} else {
			// Create encoder entry for validated encoder even if not in current system list
			validatedList.VideoEncoders = append(validatedList.VideoEncoders, encoders.Encoder{
				Type:        "V",
				Name:        encoderName,
				Description: "H.265/HEVC encoder",
				HWAccel:     !strings.Contains(encoderName, "lib"), // lib* are software encoders
			})
		}
	}

	// Add most popular audio encoders (2025) - no validation needed as they're reliable
	popularAudioEncoders := []string{"aac", "libopus", "libmp3lame", "ac3"}
	for _, encoderName := range popularAudioEncoders {
		if encoder, exists := availableMap[encoderName]; exists {
			validatedList.AudioEncoders = append(validatedList.AudioEncoders, encoder)
		} else {
			// Create basic entry for popular audio encoder
			var description string
			switch encoderName {
			case "aac":
				description = "AAC (Advanced Audio Coding)"
			case "libopus":
				description = "Opus audio codec"
			case "libmp3lame":
				description = "MP3 (MPEG Audio Layer 3)"
			case "ac3":
				description = "AC-3 (Dolby Digital)"
			}
			validatedList.AudioEncoders = append(validatedList.AudioEncoders, encoders.Encoder{
				Type:        "A",
				Name:        encoderName,
				Description: description,
				HWAccel:     false, // Audio encoders are typically software-based
			})
		}
	}

	return validatedList, nil
}

// GetEncodersData fetches the list of validated encoders with optional filtering
func GetEncodersData(r *http.Request) (EncoderResponse, error) {
	filter := EncoderFilter{
		Type:    r.URL.Query().Get("type"),
		Search:  r.URL.Query().Get("search"),
		Hwaccel: r.URL.Query().Get("hwaccel") == "true",
	}

	// Get validated encoders from validator registry instead of all available encoders
	encoders, err := getValidatedEncoders()
	if err != nil {
		return EncoderResponse{}, fmt.Errorf("failed to get validated encoders: %w", err)
	}

	if filter.Type != "" || filter.Search != "" || filter.Hwaccel {
		encoders = FilterEncoders(encoders, filter)
	}

	totalCount := len(encoders.VideoEncoders) +
		len(encoders.AudioEncoders) +
		len(encoders.SubtitleEncoders) +
		len(encoders.OtherEncoders)

	response := EncoderResponse{
		Encoders: *encoders,
		Count:    totalCount,
	}
	return response, nil
}

// listEncodersHandler returns a list of validated encoders (HTMX fragment or JSON)
func listEncodersHandler(w http.ResponseWriter, r *http.Request) {
	response, err := GetEncodersData(r)
	if err != nil {
		// Check if error is due to missing validation file
		if strings.Contains(err.Error(), "validation file not found") {
			handleError(w, "Validation required", err, http.StatusBadRequest)
		} else {
			handleError(w, "Failed to get encoders", err, http.StatusInternalServerError)
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}


// captureScreenshotHandler captures a screenshot from a video device
func captureScreenshotHandler(sseManager *sse.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var request CaptureRequest
		if r.Header.Get("Content-Type") == "application/json" {
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				handleError(w, "Invalid request format", err, http.StatusBadRequest)
				return
			}
		} else {
			if err := r.ParseForm(); err != nil {
				handleError(w, "Failed to parse form data", err, http.StatusBadRequest)
				return
			}

			// Parse delay parameter if provided
			var delay float64 = 0
			delayStr := r.FormValue("delay")
			if delayStr != "" {
				fmt.Sscanf(delayStr, "%f", &delay)
			}

			request = CaptureRequest{
				DevicePath: r.FormValue("devicePath"),
				Resolution: r.FormValue("resolution"),
				Delay:      delay,
			}
		}

		if request.DevicePath == "" {
			handleError(w, "DevicePath is required", nil, http.StatusBadRequest)
			return
		}

		if _, err := os.Stat(request.DevicePath); os.IsNotExist(err) {
			handleError(w, fmt.Sprintf("Device %s does not exist", request.DevicePath), nil, http.StatusBadRequest)
			return
		}

		// For API requests, capture synchronously and return base64
		fmt.Printf("API capture with delay: %.1f seconds\n", request.Delay)

		imageBytes, err := capture.CaptureToBytes(request.DevicePath, request.Delay)

		var apiResponse ApiResponse
		if err != nil {
			apiResponse = ApiResponse{
				Status:  "error",
				Message: fmt.Sprintf("Error capturing screenshot: %v", err),
			}
			w.WriteHeader(http.StatusInternalServerError)
		} else {
			base64Image := base64.StdEncoding.EncodeToString(imageBytes)
			apiResponse = ApiResponse{
				Status:  "success",
				Message: "Screenshot captured successfully",
				Data: map[string]string{
					"image": base64Image,
				},
			}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(apiResponse)
	}
}

// healthCheckHandler responds with a simple health check status
func healthCheckHandler(w http.ResponseWriter, r *http.Request) {
	response := ApiResponse{
		Status:  "ok",
		Message: "API is healthy",
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// Stream management handlers
func listStreamsHandler(w http.ResponseWriter, r *http.Request) {
	response, err := GetStreamsData()
	if err != nil {
		handleError(w, "Failed to get streams", err, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func newStreamFormHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusNotImplemented)
	json.NewEncoder(w).Encode(ApiResponse{
		Status:  "error",
		Message: "Not implemented",
	})
}

func closeStreamFormHandler(w http.ResponseWriter, r *http.Request) {
	// Close form handler
	w.WriteHeader(http.StatusNoContent)
}

// GetStreamsData returns stream data for templates
func GetStreamsData() (StreamListResponse, error) {
	streams := make([]StreamResponse, 0)

	// Add device streams
	deviceStreamsMutex.RLock()
	for _, stream := range deviceStreams {
		// Update uptime
		stream.Uptime = time.Since(stream.StartTime)
		streams = append(streams, stream)
	}
	deviceStreamsMutex.RUnlock()

	return StreamListResponse{
		Streams: streams,
		Count:   len(streams),
	}, nil
}

// Metrics handlers

// GetMetricsData returns current metrics for templates
func GetMetricsData() (CurrentMetrics, error) {
	return GetCurrentMetrics()
}

// metricsHandler serves the metrics page (HTMX template)
func metricsHandler(w http.ResponseWriter, r *http.Request) {
	metricsData, err := GetMetricsData()
	if err != nil {
		handleError(w, "Failed to get metrics", err, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(metricsData)
}

// prometheusMetricsHandler serves the Prometheus /metrics endpoint
func prometheusMetricsHandler(w http.ResponseWriter, r *http.Request) {
	handler := GetPrometheusHandler()
	handler.ServeHTTP(w, r)
}
