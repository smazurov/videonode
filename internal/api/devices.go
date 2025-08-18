package api

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/smazurov/videonode/internal/api/models"
	"github.com/smazurov/videonode/internal/capture"
	"github.com/smazurov/videonode/v4l2_detector"
)

// Device path parameter input
type DevicePathInput struct {
	DeviceID string `path:"device_id" example:"usb-0000:00:14.0-1" doc:"Stable device identifier"`
}

// Device format query input
type DeviceFormatInput struct {
	DevicePathInput
	FormatName models.VideoFormat `query:"format_name" example:"yuyv422" doc:"Human-readable format name"`
}

// Device resolution query input
type DeviceResolutionInput struct {
	DeviceFormatInput
	Width  string `query:"width" example:"1920" doc:"Video width in pixels"`
	Height string `query:"height" example:"1080" doc:"Video height in pixels"`
}

// DeviceCaptureBody represents the request body for device capture
type DeviceCaptureBody struct {
	Resolution string  `json:"resolution,omitempty" example:"1920x1080" doc:"Optional resolution in format widthxheight"`
	Delay      float64 `json:"delay,omitempty" example:"2.0" doc:"Optional delay in seconds before capturing"`
}

// DeviceCaptureInput combines path parameters and request body
type DeviceCaptureInput struct {
	DevicePathInput
	Body DeviceCaptureBody
}

// humanReadableToPixelFormat converts human-readable format names to V4L2 pixel format codes
func humanReadableToPixelFormat(formatName models.VideoFormat) (uint32, error) {
	return formatName.ToPixelFormat()
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

// V4L2 capability constants (from linux/videodev2.h)
const (
	V4L2_CAP_VIDEO_CAPTURE        = 0x00000001
	V4L2_CAP_VIDEO_OUTPUT         = 0x00000002
	V4L2_CAP_VIDEO_OVERLAY        = 0x00000004
	V4L2_CAP_VBI_CAPTURE          = 0x00000010
	V4L2_CAP_VBI_OUTPUT           = 0x00000020
	V4L2_CAP_SLICED_VBI_CAPTURE   = 0x00000040
	V4L2_CAP_SLICED_VBI_OUTPUT    = 0x00000080
	V4L2_CAP_RDS_CAPTURE          = 0x00000100
	V4L2_CAP_VIDEO_OUTPUT_OVERLAY = 0x00000200
	V4L2_CAP_HW_FREQ_SEEK         = 0x00000400
	V4L2_CAP_RDS_OUTPUT           = 0x00000800
	V4L2_CAP_VIDEO_CAPTURE_MPLANE = 0x00001000
	V4L2_CAP_VIDEO_OUTPUT_MPLANE  = 0x00002000
	V4L2_CAP_VIDEO_M2M_MPLANE     = 0x00004000
	V4L2_CAP_VIDEO_M2M            = 0x00008000
	V4L2_CAP_TUNER                = 0x00010000
	V4L2_CAP_AUDIO                = 0x00020000
	V4L2_CAP_RADIO                = 0x00040000
	V4L2_CAP_MODULATOR            = 0x00080000
	V4L2_CAP_SDR_CAPTURE          = 0x00100000
	V4L2_CAP_EXT_PIX_FORMAT       = 0x00200000
	V4L2_CAP_SDR_OUTPUT           = 0x00400000
	V4L2_CAP_META_CAPTURE         = 0x00800000
	V4L2_CAP_READWRITE            = 0x01000000
	V4L2_CAP_ASYNCIO              = 0x02000000
	V4L2_CAP_STREAMING            = 0x04000000
	V4L2_CAP_META_OUTPUT          = 0x08000000
	V4L2_CAP_TOUCH                = 0x10000000
	V4L2_CAP_IO_MC                = 0x20000000
	V4L2_CAP_DEVICE_CAPS          = 0x80000000
)

// translateCapabilities converts V4L2 capability flags to readable strings
func translateCapabilities(caps uint32) []string {
	var capabilities []string

	capMap := map[uint32]string{
		V4L2_CAP_VIDEO_CAPTURE:        "Video Capture",
		V4L2_CAP_VIDEO_OUTPUT:         "Video Output",
		V4L2_CAP_VIDEO_OVERLAY:        "Video Overlay",
		V4L2_CAP_VBI_CAPTURE:          "VBI Capture",
		V4L2_CAP_VBI_OUTPUT:           "VBI Output",
		V4L2_CAP_SLICED_VBI_CAPTURE:   "Sliced VBI Capture",
		V4L2_CAP_SLICED_VBI_OUTPUT:    "Sliced VBI Output",
		V4L2_CAP_RDS_CAPTURE:          "RDS Capture",
		V4L2_CAP_VIDEO_OUTPUT_OVERLAY: "Video Output Overlay",
		V4L2_CAP_HW_FREQ_SEEK:         "Hardware Frequency Seek",
		V4L2_CAP_RDS_OUTPUT:           "RDS Output",
		V4L2_CAP_VIDEO_CAPTURE_MPLANE: "Multi-planar Video Capture",
		V4L2_CAP_VIDEO_OUTPUT_MPLANE:  "Multi-planar Video Output",
		V4L2_CAP_VIDEO_M2M_MPLANE:     "Multi-planar Memory-to-Memory",
		V4L2_CAP_VIDEO_M2M:            "Memory-to-Memory",
		V4L2_CAP_TUNER:                "Tuner",
		V4L2_CAP_AUDIO:                "Audio",
		V4L2_CAP_RADIO:                "Radio",
		V4L2_CAP_MODULATOR:            "Modulator",
		V4L2_CAP_SDR_CAPTURE:          "Software Defined Radio Capture",
		V4L2_CAP_EXT_PIX_FORMAT:       "Extended Pixel Format",
		V4L2_CAP_SDR_OUTPUT:           "Software Defined Radio Output",
		V4L2_CAP_META_CAPTURE:         "Metadata Capture",
		V4L2_CAP_READWRITE:            "Read/Write I/O",
		V4L2_CAP_ASYNCIO:              "Asynchronous I/O",
		V4L2_CAP_STREAMING:            "Streaming I/O",
		V4L2_CAP_META_OUTPUT:          "Metadata Output",
		V4L2_CAP_TOUCH:                "Touch Device",
		V4L2_CAP_IO_MC:                "Media Controller I/O",
	}

	for flag, name := range capMap {
		if caps&flag != 0 {
			capabilities = append(capabilities, name)
		}
	}

	return capabilities
}

// GetDevicesData fetches the list of available video devices
func GetDevicesData() (models.DeviceData, error) {
	v4l2Devices, err := v4l2_detector.FindDevices()
	if err != nil {
		return models.DeviceData{}, fmt.Errorf("failed to find devices: %w", err)
	}

	// Convert v4l2_detector.DeviceInfo to our API DeviceInfo with capabilities
	devices := make([]models.DeviceInfo, len(v4l2Devices))
	for i, v4l2Dev := range v4l2Devices {
		devices[i] = models.DeviceInfo{
			DevicePath:   v4l2Dev.DevicePath,
			DeviceName:   v4l2Dev.DeviceName,
			DeviceId:     v4l2Dev.DeviceId,
			Caps:         v4l2Dev.Caps,
			Capabilities: translateCapabilities(v4l2Dev.Caps),
		}
	}

	return models.DeviceData{
		Devices: devices,
		Count:   len(devices),
	}, nil
}

// GetDeviceCapabilities fetches all capabilities for a specific device
func GetDeviceCapabilities(devicePath string) (models.DeviceCapabilitiesData, error) {
	v4l2Formats, err := v4l2_detector.GetDeviceFormats(devicePath)
	if err != nil {
		return models.DeviceCapabilitiesData{}, fmt.Errorf("failed to get device formats: %w", err)
	}

	// Convert V4L2 formats to our API format
	formats := make([]models.FormatInfo, 0, len(v4l2Formats))
	for _, v4l2Format := range v4l2Formats {
		// Check if format is supported (has FFmpeg equivalent)
		_, err := V4L2ToFFmpegFormat(v4l2Format.PixelFormat)
		if err != nil {
			// Skip unsupported formats instead of failing completely
			fmt.Printf("Warning: %v\n", err)
			continue
		}
		formats = append(formats, models.ConvertV4L2FormatInfo(v4l2Format))
	}

	return models.DeviceCapabilitiesData{
		DevicePath: devicePath,
		Formats:    formats,
	}, nil
}

// registerDeviceRoutes registers all device-related endpoints
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
	}, func(ctx context.Context, input *struct{}) (*models.DeviceResponse, error) {
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
	}, func(ctx context.Context, input *DevicePathInput) (*models.DeviceCapabilitiesResponse, error) {
		// Look up device path from stable device ID
		devicePath, err := v4l2_detector.GetDevicePathByID(input.DeviceID)
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
	}, func(ctx context.Context, input *DeviceFormatInput) (*models.DeviceResolutionsResponse, error) {
		// Look up device path from stable device ID
		devicePath, err := v4l2_detector.GetDevicePathByID(input.DeviceID)
		if err != nil {
			return nil, huma.Error404NotFound("Device not found", err)
		}

		// Convert format name to pixel format
		pixelFormat, err := humanReadableToPixelFormat(input.FormatName)
		if err != nil {
			return nil, huma.Error400BadRequest("Invalid format name", err)
		}

		resolutions, err := v4l2_detector.GetDeviceResolutions(devicePath, pixelFormat)
		if err != nil {
			return nil, huma.Error500InternalServerError("Failed to get device resolutions", err)
		}

		// Convert v4l2 resolutions to API resolutions
		apiResolutions := make([]models.Resolution, len(resolutions))
		for i, res := range resolutions {
			apiResolutions[i] = models.ConvertV4L2Resolution(res)
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
	}, func(ctx context.Context, input *DeviceResolutionInput) (*models.DeviceFrameratesResponse, error) {
		// Look up device path from stable device ID
		devicePath, err := v4l2_detector.GetDevicePathByID(input.DeviceID)
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

		framerates, err := v4l2_detector.GetDeviceFramerates(devicePath, pixelFormat, uint32(width), uint32(height))
		if err != nil {
			return nil, huma.Error500InternalServerError("Failed to get device framerates", err)
		}

		// Convert v4l2 framerates to API framerates
		apiFramerates := make([]models.Framerate, len(framerates))
		for i, rate := range framerates {
			apiFramerates[i] = models.ConvertV4L2Framerate(rate)
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
	}, func(ctx context.Context, input *DeviceCaptureInput) (*models.CaptureResponse, error) {
		// Look up device path from stable device ID
		devicePath, err := v4l2_detector.GetDevicePathByID(input.DeviceID)
		if err != nil {
			return nil, huma.Error404NotFound("Device not found", err)
		}

		// Validate device exists
		if _, err := os.Stat(devicePath); os.IsNotExist(err) {
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

			imageBytes, err := capture.CaptureToBytes(devicePath, delay)

			if err != nil {
				// Log the capture error
				fmt.Printf("Screenshot capture failed for %s: %s\n", devicePath, err.Error())
			} else {
				// Broadcast success event with base64 image via Huma SSE
				base64Image := base64.StdEncoding.EncodeToString(imageBytes)
				BroadcastCaptureSuccess(devicePath, base64Image, timestamp)
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
