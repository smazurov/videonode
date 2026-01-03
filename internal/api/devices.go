package api

import (
	"context"
	"encoding/base64"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/smazurov/videonode/internal/api/models"
	"github.com/smazurov/videonode/internal/capture"
	"github.com/smazurov/videonode/internal/devices"
	"github.com/smazurov/videonode/internal/events"
)

// DevicePathInput represents device path parameter input.
type DevicePathInput struct {
	DeviceID string `path:"device_id" example:"usb-0000:00:14.0-1" doc:"Stable device identifier"`
}

// DeviceFormatInput represents device format query input.
type DeviceFormatInput struct {
	DevicePathInput
	FormatName models.VideoFormat `query:"format_name" example:"yuyv422" doc:"Human-readable format name"`
}

// DeviceResolutionInput represents device resolution query input.
type DeviceResolutionInput struct {
	DeviceFormatInput
	Width  string `query:"width" example:"1920" doc:"Video width in pixels"`
	Height string `query:"height" example:"1080" doc:"Video height in pixels"`
}

// DeviceCaptureBody represents the request body for device capture.
type DeviceCaptureBody struct {
	Resolution string  `json:"resolution,omitempty" example:"1920x1080" doc:"Optional resolution"`
	Delay      float64 `json:"delay,omitempty" example:"2.0" doc:"Delay before capture in seconds"`
}

// DeviceCaptureInput combines path parameters and request body.
type DeviceCaptureInput struct {
	DevicePathInput
	Body DeviceCaptureBody
}

// humanReadableToPixelFormat converts human-readable format names to V4L2 pixel format codes.
func humanReadableToPixelFormat(formatName models.VideoFormat) (uint32, error) {
	return formatName.ToPixelFormat()
}

// pixelFormatToFourCC converts a V4L2 pixel format code to its FourCC string representation.
func pixelFormatToFourCC(pixelFormat uint32) string {
	// Convert to 4-byte array in little-endian order
	bytes := []byte{
		byte(pixelFormat & 0xFF),
		byte((pixelFormat >> 8) & 0xFF),
		byte((pixelFormat >> 16) & 0xFF),
		byte((pixelFormat >> 24) & 0xFF),
	}

	// Convert to string, replacing non-printable chars with '?'
	for i, b := range bytes {
		if b < 32 || b > 126 {
			bytes[i] = '?'
		}
	}

	return string(bytes)
}

// resolveDevicePath is a wrapper around devices.ResolveDevicePath.
func resolveDevicePath(deviceID string) (string, error) {
	return devices.ResolveDevicePath(deviceID)
}

// V4L2ToFFmpegFormat maps V4L2 pixel format codes to FFmpeg input format names.
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
	case 861030210: // BGR3
		return "bgr24", nil
	case 859981650: // RGB3
		return "rgb24", nil
	case 875714126: // NV24
		return "nv24", nil
	case 909203022: // NV16
		return "nv16", nil
	default:
		fourcc := pixelFormatToFourCC(pixelFormat)
		return "", fmt.Errorf("unsupported V4L2 pixel format: %d (0x%08X, FourCC: '%s')", pixelFormat, pixelFormat, fourcc)
	}
}

// V4L2 capability constants (from linux/videodev2.h).
const (
	V4L2CapVideoCapture       = 0x00000001
	V4L2CapVideoOutput        = 0x00000002
	V4L2CapVideoOverlay       = 0x00000004
	V4L2CapVbiCapture         = 0x00000010
	V4L2CapVbiOutput          = 0x00000020
	V4L2CapSlicedVbiCapture   = 0x00000040
	V4L2CapSlicedVbiOutput    = 0x00000080
	V4L2CapRdsCapture         = 0x00000100
	V4L2CapVideoOutputOverlay = 0x00000200
	V4L2CapHwFreqSeek         = 0x00000400
	V4L2CapRdsOutput          = 0x00000800
	V4L2CapVideoCaptureMplane = 0x00001000
	V4L2CapVideoOutputMplane  = 0x00002000
	V4L2CapVideoM2mMplane     = 0x00004000
	V4L2CapVideoM2m           = 0x00008000
	V4L2CapTuner              = 0x00010000
	V4L2CapAudio              = 0x00020000
	V4L2CapRadio              = 0x00040000
	V4L2CapModulator          = 0x00080000
	V4L2CapSdrCapture         = 0x00100000
	V4L2CapExtPixFormat       = 0x00200000
	V4L2CapSdrOutput          = 0x00400000
	V4L2CapMetaCapture        = 0x00800000
	V4L2CapReadwrite          = 0x01000000
	V4L2CapAsyncio            = 0x02000000
	V4L2CapStreaming          = 0x04000000
	V4L2CapMetaOutput         = 0x08000000
	V4L2CapTouch              = 0x10000000
	V4L2CapIoMc               = 0x20000000
	V4L2CapDeviceCaps         = 0x80000000
)

// translateCapabilities converts V4L2 capability flags to readable strings.
func translateCapabilities(caps uint32) []string {
	var capabilities []string

	capMap := map[uint32]string{
		V4L2CapVideoCapture:       "Video Capture",
		V4L2CapVideoOutput:        "Video Output",
		V4L2CapVideoOverlay:       "Video Overlay",
		V4L2CapVbiCapture:         "VBI Capture",
		V4L2CapVbiOutput:          "VBI Output",
		V4L2CapSlicedVbiCapture:   "Sliced VBI Capture",
		V4L2CapSlicedVbiOutput:    "Sliced VBI Output",
		V4L2CapRdsCapture:         "RDS Capture",
		V4L2CapVideoOutputOverlay: "Video Output Overlay",
		V4L2CapHwFreqSeek:         "Hardware Frequency Seek",
		V4L2CapRdsOutput:          "RDS Output",
		V4L2CapVideoCaptureMplane: "Multi-planar Video Capture",
		V4L2CapVideoOutputMplane:  "Multi-planar Video Output",
		V4L2CapVideoM2mMplane:     "Multi-planar Memory-to-Memory",
		V4L2CapVideoM2m:           "Memory-to-Memory",
		V4L2CapTuner:              "Tuner",
		V4L2CapAudio:              "Audio",
		V4L2CapRadio:              "Radio",
		V4L2CapModulator:          "Modulator",
		V4L2CapSdrCapture:         "Software Defined Radio Capture",
		V4L2CapExtPixFormat:       "Extended Pixel Format",
		V4L2CapSdrOutput:          "Software Defined Radio Output",
		V4L2CapMetaCapture:        "Metadata Capture",
		V4L2CapReadwrite:          "Read/Write I/O",
		V4L2CapAsyncio:            "Asynchronous I/O",
		V4L2CapStreaming:          "Streaming I/O",
		V4L2CapMetaOutput:         "Metadata Output",
		V4L2CapTouch:              "Touch Device",
		V4L2CapIoMc:               "Media Controller I/O",
	}

	for flag, name := range capMap {
		if caps&flag != 0 {
			capabilities = append(capabilities, name)
		}
	}

	return capabilities
}

// GetDevicesData fetches the list of available video devices.
func GetDevicesData() (models.DeviceData, error) {
	detector := devices.NewDetector()
	deviceList, err := detector.FindDevices()
	if err != nil {
		return models.DeviceData{}, fmt.Errorf("failed to find devices: %w", err)
	}

	// Convert devices.DeviceInfo to our API DeviceInfo with capabilities
	apiDevices := make([]models.DeviceInfo, len(deviceList))
	for i, dev := range deviceList {
		apiDevices[i] = models.DeviceInfo{
			DevicePath:   dev.DevicePath,
			DeviceName:   dev.DeviceName,
			DeviceID:     dev.DeviceID,
			Caps:         dev.Caps,
			Capabilities: translateCapabilities(dev.Caps),
			Ready:        dev.Ready,
			Type:         models.DeviceType(dev.Type),
		}
	}

	return models.DeviceData{
		Devices: apiDevices,
		Count:   len(apiDevices),
	}, nil
}

// GetDeviceCapabilities fetches all capabilities for a specific device.
func GetDeviceCapabilities(devicePath string) (models.DeviceCapabilitiesData, error) {
	detector := devices.NewDetector()
	deviceFormats, err := detector.GetDeviceFormats(devicePath)
	if err != nil {
		return models.DeviceCapabilitiesData{}, fmt.Errorf("failed to get device formats: %w", err)
	}

	// Convert device formats to our API format
	formats := make([]models.FormatInfo, 0, len(deviceFormats))
	for _, format := range deviceFormats {
		// Check if format is supported (has FFmpeg equivalent)
		_, formatErr := V4L2ToFFmpegFormat(format.PixelFormat)
		if formatErr != nil {
			// Skip unsupported formats instead of failing completely
			logger := slog.With("component", "devices_api")
			logger.Warn("Skipping unsupported format", "error", formatErr)
			continue
		}
		formats = append(formats, models.FormatInfo{
			FormatName:   models.PixelFormatToHumanReadable(format.PixelFormat),
			OriginalName: format.FormatName,
			Emulated:     format.Emulated,
		})
	}

	return models.DeviceCapabilitiesData{
		DevicePath: devicePath,
		Formats:    formats,
	}, nil
}

// registerDeviceRoutes registers all device-related endpoints.
func (s *Server) registerDeviceRoutes() {
	// List all devices
	huma.Register(s.api, huma.Operation{
		OperationID: "list-devices",
		Method:      http.MethodGet,
		Path:        "/api/devices",
		Summary:     "List Devices",
		Description: "List all available V4L2 video devices",
		Tags:        []string{"devices"},
		Security:    withAuth(),
		Errors:      []int{401, 500},
	}, func(_ context.Context, _ *struct{}) (*models.DeviceResponse, error) {
		data, err := GetDevicesData()
		if err != nil {
			return nil, huma.Error500InternalServerError("Failed to get devices", err)
		}

		return &models.DeviceResponse{Body: data}, nil
	})

	// Get device capabilities
	huma.Register(s.api, huma.Operation{
		OperationID: "device-formats",
		Method:      http.MethodGet,
		Path:        "/api/devices/{device_id}/formats",
		Summary:     "Formats",
		Description: "List supported formats for a specific device",
		Tags:        []string{"devices"},
		Security:    withAuth(),
		Errors:      []int{401, 500},
	}, func(_ context.Context, input *DevicePathInput) (*models.DeviceCapabilitiesResponse, error) {
		// Resolve device ID to device path
		devicePath, err := resolveDevicePath(input.DeviceID)
		if err != nil {
			return nil, huma.Error404NotFound("Device not found", err)
		}

		data, err := GetDeviceCapabilities(devicePath)
		if err != nil {
			return nil, huma.Error500InternalServerError("Failed to get device capabilities", err)
		}

		return &models.DeviceCapabilitiesResponse{Body: data}, nil
	})

	// Get device resolutions for a format
	huma.Register(s.api, huma.Operation{
		OperationID: "device-resolutions",
		Method:      http.MethodGet,
		Path:        "/api/devices/{device_id}/resolutions",
		Summary:     "Resolutions",
		Description: "List supported resolutions for a specific format",
		Tags:        []string{"devices"},
		Security:    withAuth(),
		Errors:      []int{400, 401, 500},
	}, func(_ context.Context, input *DeviceFormatInput) (*models.DeviceResolutionsResponse, error) {
		// Resolve device ID to device path
		devicePath, err := resolveDevicePath(input.DeviceID)
		if err != nil {
			return nil, huma.Error404NotFound("Device not found", err)
		}

		// Convert format name to pixel format
		pixelFormat, err := humanReadableToPixelFormat(input.FormatName)
		if err != nil {
			return nil, huma.Error400BadRequest("Invalid format name", err)
		}

		detector := devices.NewDetector()
		resolutions, err := detector.GetDeviceResolutions(devicePath, pixelFormat)
		if err != nil {
			return nil, huma.Error500InternalServerError("Failed to get device resolutions", err)
		}

		// Convert v4l2 resolutions to API resolutions
		apiResolutions := make([]models.Resolution, len(resolutions))
		for i, res := range resolutions {
			apiResolutions[i] = models.Resolution{
				Width:  res.Width,
				Height: res.Height,
			}
		}

		return &models.DeviceResolutionsResponse{
			Body: models.DeviceResolutionsData{Resolutions: apiResolutions},
		}, nil
	})

	// Get device framerates for a format and resolution
	huma.Register(s.api, huma.Operation{
		OperationID: "device-framerates",
		Method:      http.MethodGet,
		Path:        "/api/devices/{device_id}/framerates",
		Summary:     "Framerates",
		Description: "List supported framerates for a specific format and resolution",
		Tags:        []string{"devices"},
		Security:    withAuth(),
		Errors:      []int{400, 401, 500},
	}, func(_ context.Context, input *DeviceResolutionInput) (*models.DeviceFrameratesResponse, error) {
		// Resolve device ID to device path
		devicePath, err := resolveDevicePath(input.DeviceID)
		if err != nil {
			return nil, huma.Error404NotFound("Device not found", err)
		}

		// Convert format name to pixel format
		pixelFormat, err := humanReadableToPixelFormat(input.FormatName)
		if err != nil {
			return nil, huma.Error400BadRequest("Invalid format name", err)
		}

		width, err := strconv.ParseUint(input.Width, 10, 32)
		if err != nil {
			return nil, huma.Error400BadRequest("Invalid width parameter", err)
		}

		height, err := strconv.ParseUint(input.Height, 10, 32)
		if err != nil {
			return nil, huma.Error400BadRequest("Invalid height parameter", err)
		}

		detector := devices.NewDetector()
		framerates, err := detector.GetDeviceFramerates(devicePath, pixelFormat, uint32(width), uint32(height))
		if err != nil {
			return nil, huma.Error500InternalServerError("Failed to get device framerates", err)
		}

		// Convert v4l2 framerates to API framerates
		apiFramerates := make([]models.Framerate, len(framerates))
		for i, rate := range framerates {
			var fps float64
			if rate.Numerator != 0 {
				fps = float64(rate.Denominator) / float64(rate.Numerator)
			}
			apiFramerates[i] = models.Framerate{
				Numerator:   rate.Numerator,
				Denominator: rate.Denominator,
				Fps:         fps,
			}
		}

		return &models.DeviceFrameratesResponse{
			Body: models.DeviceFrameratesData{Framerates: apiFramerates},
		}, nil
	})

	// Capture screenshot from device
	huma.Register(s.api, huma.Operation{
		OperationID:   "device-capture-screenshot",
		Method:        http.MethodPost,
		Path:          "/api/devices/{device_id}/capture",
		Summary:       "Capture Screenshot",
		Description:   "Capture a screenshot from the device. Results are sent via SSE events.",
		Tags:          []string{"devices"},
		DefaultStatus: http.StatusAccepted, // 202 Accepted
		Security:      withAuth(),
		Errors:        []int{401, 404},
	}, func(_ context.Context, input *DeviceCaptureInput) (*models.CaptureResponse, error) {
		// Resolve device ID to device path
		devicePath, err := resolveDevicePath(input.DeviceID)
		if err != nil {
			return nil, huma.Error404NotFound("Device not found", err)
		}

		// Validate device exists
		if _, statErr := os.Stat(devicePath); os.IsNotExist(statErr) {
			return nil, huma.Error404NotFound(fmt.Sprintf("Device %s does not exist", devicePath), nil)
		}

		// Trigger capture asynchronously and send results via SSE
		go func() {
			// Use config default delay if none provided in request
			delay := input.Body.Delay
			if delay == 0 {
				delay = float64(s.options.CaptureDefaultDelayMs) / 1000.0
			}

			fmt.Printf("API capture with delay: %.1f seconds\n", delay)
			timestamp := time.Now().Format(time.RFC3339)

			imageBytes, captureErr := capture.ToBytes(devicePath, delay)

			if captureErr != nil {
				// Log the capture error
				fmt.Printf("Screenshot capture failed for %s: %s\n", devicePath, captureErr.Error())
			} else {
				// Broadcast success event with base64 image via Huma SSE
				base64Image := base64.StdEncoding.EncodeToString(imageBytes)
				if s.eventBus != nil {
					s.eventBus.Publish(events.CaptureSuccessEvent{
						DevicePath: devicePath,
						Message:    "Screenshot captured successfully",
						ImageData:  base64Image,
						Timestamp:  timestamp,
					})
				}
			}
		}()

		// Return immediate acknowledgment that capture was triggered
		return &models.CaptureResponse{
			Body: models.CaptureData{
				Status:  "accepted",
				Message: "Screenshot capture triggered. Results will be sent via SSE.",
			},
		}, nil
	})
}
