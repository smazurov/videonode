// Package models provides API model types for device handling and video formats.
package models

import (
	"fmt"
	"sort"

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
	// Sort so the generated OpenAPI enum order is stable across runs
	// (Go map iteration is randomized, which churns api.generated.ts).
	names := make([]string, 0, len(videoFormatToPixelFormat))
	for format := range videoFormatToPixelFormat {
		names = append(names, string(format))
	}
	sort.Strings(names)

	enumValues := make([]any, len(names))
	for i, n := range names {
		enumValues[i] = n
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

// ToFourCC returns the 4-char V4L2 fourcc string (e.g. "YUYV", "MJPG")
// that the source binary's SetFormat RPC expects. Derived from the same
// pixel-format code IsValid / ToPixelFormat use, so there's exactly one
// source of truth.
func (vf VideoFormat) ToFourCC() (string, error) {
	pf, err := vf.ToPixelFormat()
	if err != nil {
		return "", err
	}
	bytes := []byte{
		byte(pf & 0xFF),
		byte((pf >> 8) & 0xFF),
		byte((pf >> 16) & 0xFF),
		byte((pf >> 24) & 0xFF),
	}
	for i, b := range bytes {
		if b < 32 || b > 126 {
			bytes[i] = '?'
		}
	}
	return string(bytes), nil
}

// PixelFormatToVideoFormat converts V4L2 pixel format codes to VideoFormat.
func PixelFormatToVideoFormat(pixelFormat uint32) (VideoFormat, bool) {
	for format, code := range videoFormatToPixelFormat {
		if code == pixelFormat {
			return format, true
		}
	}
	return "", false
}

// PixelFormatToVideoFormatByFourCC maps a 4-char V4L2 fourcc string
// back to the lowercase VideoFormat the API returns to clients
// (e.g. "YUYV" -> "yuyv422"). Inverse of VideoFormat.ToFourCC.
func PixelFormatToVideoFormatByFourCC(fourcc string) (VideoFormat, bool) {
	if len(fourcc) != 4 {
		return "", false
	}
	code := uint32(fourcc[0]) | uint32(fourcc[1])<<8 | uint32(fourcc[2])<<16 | uint32(fourcc[3])<<24
	return PixelFormatToVideoFormat(code)
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
	FormatName   VideoFormat `json:"format_name" example:"yuyv422" doc:"Human-readable format name"`
	OriginalName string      `json:"original_name" example:"YUYV 4:2:2" doc:"Original V4L2 format name"`
	Emulated     bool        `json:"emulated" example:"false" doc:"Whether format is emulated"`
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

// DeviceSetFormatBody is the JSON body of a POST /api/devices/{id}/format request.
type DeviceSetFormatBody struct {
	FourCC string `json:"fourcc" example:"YUYV" doc:"4-character V4L2 pixel format code"`
	Width  uint32 `json:"width" example:"1920" doc:"Capture width in pixels"`
	Height uint32 `json:"height" example:"1080" doc:"Capture height in pixels"`
	FPS    uint32 `json:"fps,omitempty" example:"30" doc:"Capture framerate; 0 = driver default"`
}

// DeviceSetFormatInput is the HTTP request input for set_format.
type DeviceSetFormatInput struct {
	DeviceID string              `path:"device_id" doc:"Stable device identifier"`
	Body     DeviceSetFormatBody `body:"body"`
}

// DeviceSetFormatData is the success payload of set_format.
type DeviceSetFormatData struct {
	Applied bool `json:"applied" doc:"True if the source accepted and applied the new format"`
}

// DeviceSetFormatResponse is the HTTP response wrapper for DeviceSetFormatData.
type DeviceSetFormatResponse struct {
	Body DeviceSetFormatData
}

// Note: V4L2 conversion functions were removed - conversion now happens in devices package
