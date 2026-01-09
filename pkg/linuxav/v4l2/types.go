//go:build linux

package v4l2

// DeviceInfo contains information about a V4L2 device.
type DeviceInfo struct {
	DevicePath string
	DeviceName string
	DeviceID   string // Stable identifier (from /dev/v4l/by-id/ or synthetic)
	Caps       uint32
}

// FormatInfo contains information about a supported pixel format.
type FormatInfo struct {
	PixelFormat uint32
	FormatName  string
	Emulated    bool
}

// Resolution represents a supported video resolution.
type Resolution struct {
	Width  uint32
	Height uint32
}

// Framerate represents a supported framerate as a fraction.
type Framerate struct {
	Numerator   uint32
	Denominator uint32
}

// FPS returns the framerate as frames per second.
func (f Framerate) FPS() float64 {
	if f.Numerator == 0 {
		return 0
	}
	return float64(f.Denominator) / float64(f.Numerator)
}

// DeviceType represents the type of V4L2 device.
type DeviceType int

const (
	DeviceTypeWebcam  DeviceType = 0
	DeviceTypeHDMI    DeviceType = 1
	DeviceTypeUnknown DeviceType = -1
)

// SignalState represents the state of a video signal.
type SignalState int

const (
	SignalStateNoDevice     SignalState = -1
	SignalStateNoLink       SignalState = 0 // No cable connected
	SignalStateNoSignal     SignalState = 1 // Cable connected, no signal
	SignalStateUnstable     SignalState = 2 // Signal present but unstable
	SignalStateLocked       SignalState = 3 // Signal locked and stable
	SignalStateOutOfRange   SignalState = 4 // Signal out of supported range
	SignalStateNotSupported SignalState = 5 // Device doesn't support DV timings
)

// SignalStatus contains detailed signal information.
type SignalStatus struct {
	State      SignalState
	Width      uint32
	Height     uint32
	FPS        float64
	Interlaced bool
}

// DeviceStatus contains combined device type and ready status.
type DeviceStatus struct {
	DeviceType DeviceType
	Ready      bool
}

// Capability flags
const (
	V4L2_CAP_VIDEO_CAPTURE = 0x00000001
	V4L2_CAP_DEVICE_CAPS   = 0x80000000
)

// Format flags
const (
	V4L2_FMT_FLAG_EMULATED = 0x0002
)

// Common pixel formats
const (
	V4L2_PIX_FMT_YUYV  = 0x56595559 // 'YUYV'
	V4L2_PIX_FMT_MJPEG = 0x47504A4D // 'MJPG'
	V4L2_PIX_FMT_H264  = 0x34363248 // 'H264'
	V4L2_PIX_FMT_HEVC  = 0x43564548 // 'HEVC'
	V4L2_PIX_FMT_NV12  = 0x3231564E // 'NV12'
)

// Frame size types
const (
	V4L2_FRMSIZE_TYPE_DISCRETE   = 1
	V4L2_FRMSIZE_TYPE_CONTINUOUS = 2
	V4L2_FRMSIZE_TYPE_STEPWISE   = 3
)

// Frame interval types
const (
	V4L2_FRMIVAL_TYPE_DISCRETE   = 1
	V4L2_FRMIVAL_TYPE_CONTINUOUS = 2
	V4L2_FRMIVAL_TYPE_STEPWISE   = 3
)

// Buffer type
const (
	V4L2_BUF_TYPE_VIDEO_CAPTURE = 1
)

// Event types
const (
	V4L2_EVENT_SOURCE_CHANGE = 5
)
