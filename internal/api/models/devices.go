// Package models provides API model types for device handling and video formats.
package models

import (
	"fmt"
	"log/slog"

	"github.com/danielgtaylor/huma/v2"
)

// VideoFormat represents supported video format names.
type VideoFormat string

// Single source of truth - all definitions here.
const (
	FormatYUYV422 VideoFormat = "yuyv422"
	FormatNV12    VideoFormat = "nv12"
	FormatH264    VideoFormat = "h264"
	FormatMJPEG   VideoFormat = "mjpeg"
	FormatYU12    VideoFormat = "yu12"
	FormatYV12    VideoFormat = "yv12"
	FormatBGR24   VideoFormat = "bgr24" // BGR3 - 24-bit BGR (HDMI native)
	FormatRGB24   VideoFormat = "rgb24" // RGB3 - 24-bit RGB (HDMI native)
	FormatNV24    VideoFormat = "nv24"  // Y/UV 4:4:4 (full chroma)
	FormatNV16    VideoFormat = "nv16"  // Y/UV 4:2:2 (half chroma)
)

// Pixel format mappings - single source of truth.
var videoFormatToPixelFormat = map[VideoFormat]uint32{
	FormatYUYV422: 1448695129, // YUYV
	FormatNV12:    842094158,  // NV12
	FormatH264:    875967048,  // H264
	FormatMJPEG:   1196444237, // MJPEG
	FormatYU12:    842093913,  // YU12/I420
	FormatYV12:    842094169,  // YV12
	FormatBGR24:   861030210,  // BGR3
	FormatRGB24:   859981650,  // RGB3
	FormatNV24:    875714126,  // NV24
	FormatNV16:    909203022,  // NV16
}

// Schema implements SchemaProvider for dynamic enum validation.
func (VideoFormat) Schema(_ huma.Registry) *huma.Schema {
	// Generate enum values dynamically from our map
	enumValues := make([]any, 0, len(videoFormatToPixelFormat))
	for format := range videoFormatToPixelFormat {
		enumValues = append(enumValues, string(format))
	}

	return &huma.Schema{
		Type:        huma.TypeString,
		Enum:        enumValues,
		Description: "Supported video format names",
	}
}

// ToPixelFormat converts VideoFormat to V4L2 pixel format code.
func (vf VideoFormat) ToPixelFormat() (uint32, error) {
	if pf, exists := videoFormatToPixelFormat[vf]; exists {
		return pf, nil
	}
	return 0, fmt.Errorf("unsupported format: %s", vf)
}

// IsValid checks if the VideoFormat is supported.
func (vf VideoFormat) IsValid() bool {
	_, exists := videoFormatToPixelFormat[vf]
	return exists
}

// PixelFormatToHumanReadable converts V4L2 pixel format codes to human-readable names.
func PixelFormatToHumanReadable(pixelFormat uint32) string {
	// Reverse lookup in our map
	for format, code := range videoFormatToPixelFormat {
		if code == pixelFormat {
			return string(format)
		}
	}

	logger := slog.With("component", "device_models")
	logger.Warn("Unknown pixel format code", "pixel_format", pixelFormat)
	return "unknown"
}

// DeviceType represents the type of V4L2 device.
type DeviceType int

// DeviceType constants for different V4L2 device types.
const (
	DeviceTypeWebcam  DeviceType = 0
	DeviceTypeHDMI    DeviceType = 1
	DeviceTypeUnknown DeviceType = -1
)

func (dt DeviceType) String() string {
	switch dt {
	case DeviceTypeWebcam:
		return "webcam"
	case DeviceTypeHDMI:
		return "hdmi"
	default:
		return "unknown"
	}
}

// DeviceInfo represents a video device with snake_case fields.
type DeviceInfo struct {
	DevicePath   string     `json:"device_path" example:"/dev/video0" doc:"System device path"`
	DeviceName   string     `json:"device_name" example:"USB Camera" doc:"Device name"`
	DeviceID     string     `json:"device_id" example:"usb-0000:00:14.0-1" doc:"Stable device identifier"`
	Caps         uint32     `json:"caps" example:"84000001" doc:"Raw V4L2 capability flags"`
	Capabilities []string   `json:"capabilities" example:"[\"Video Capture\", \"Streaming I/O\"]" doc:"Capabilities"`
	Ready        bool       `json:"ready" example:"true" doc:"Device ready status"`
	Type         DeviceType `json:"type" example:"1" doc:"Device type (0=webcam, 1=hdmi, -1=unknown)"`
}

// FormatInfo represents a video format with human-readable format names and snake_case fields.
type FormatInfo struct {
	FormatName   string `json:"format_name" example:"yuyv422" doc:"Human-readable format name"`
	OriginalName string `json:"original_name" example:"YUYV 4:2:2" doc:"Original V4L2 format name"`
	Emulated     bool   `json:"emulated" example:"false" doc:"Whether format is emulated"`
}

// Resolution represents video resolution with snake_case fields.
type Resolution struct {
	Width  uint32 `json:"width" example:"1920" doc:"Video width in pixels"`
	Height uint32 `json:"height" example:"1080" doc:"Video height in pixels"`
}

// Framerate represents video framerate with snake_case fields.
type Framerate struct {
	Numerator   uint32  `json:"numerator" example:"1" doc:"Framerate fraction numerator"`
	Denominator uint32  `json:"denominator" example:"30" doc:"Framerate fraction denominator"`
	Fps         float64 `json:"fps" example:"30.0" doc:"Frames per second"`
}

// DeviceData contains device listing information.
type DeviceData struct {
	Devices []DeviceInfo `json:"devices" doc:"List of available video devices"`
	Count   int          `json:"count" example:"2" doc:"Number of devices found"`
}

// DeviceResponse is the HTTP response wrapper for DeviceData.
type DeviceResponse struct {
	Body DeviceData
}

// DeviceCapabilitiesData contains device capabilities information.
type DeviceCapabilitiesData struct {
	DevicePath string       `json:"device_path" example:"/dev/video0" doc:"Path to the video device"`
	Formats    []FormatInfo `json:"formats" doc:"Supported video formats"`
}

// DeviceCapabilitiesResponse is the HTTP response wrapper for DeviceCapabilitiesData.
type DeviceCapabilitiesResponse struct {
	Body DeviceCapabilitiesData
}

// DeviceResolutionsData contains device resolution information.
type DeviceResolutionsData struct {
	Resolutions []Resolution `json:"resolutions" doc:"Supported resolutions for the format"`
}

// DeviceResolutionsResponse is the HTTP response wrapper for DeviceResolutionsData.
type DeviceResolutionsResponse struct {
	Body DeviceResolutionsData
}

// DeviceFrameratesData contains device framerate information.
type DeviceFrameratesData struct {
	Framerates []Framerate `json:"framerates" doc:"Supported framerates"`
}

// DeviceFrameratesResponse is the HTTP response wrapper for DeviceFrameratesData.
type DeviceFrameratesResponse struct {
	Body DeviceFrameratesData
}

// Note: V4L2 conversion functions were removed - conversion now happens in devices package
