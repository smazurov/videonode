//go:build linux

package v4l2

import (
	"errors"
	"fmt"
	"syscall"
	"unsafe"
)

// GetFormats returns all supported pixel formats for a device.
func GetFormats(devicePath string) ([]FormatInfo, error) {
	fd, err := syscall.Open(devicePath, syscall.O_RDWR|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, fmt.Errorf("failed to open device: %w", err)
	}
	defer syscall.Close(fd)

	var formats []FormatInfo

	for i := uint32(0); ; i++ {
		fmtdesc := v4l2_fmtdesc{
			index: i,
			typ:   V4L2_BUF_TYPE_VIDEO_CAPTURE,
		}

		if ioctlErr := ioctl(fd, VIDIOC_ENUM_FMT, unsafe.Pointer(&fmtdesc)); ioctlErr != nil {
			if errors.Is(ioctlErr, syscall.EINVAL) {
				break // End of enumeration
			}
			return nil, fmt.Errorf("failed to enumerate format %d: %w", i, ioctlErr)
		}

		formats = append(formats, FormatInfo{
			PixelFormat: fmtdesc.pixelformat,
			FormatName:  cstr(fmtdesc.description[:]),
			Emulated:    fmtdesc.flags&V4L2_FMT_FLAG_EMULATED != 0,
		})
	}

	return formats, nil
}

// GetResolutions returns all supported resolutions for a device and pixel format.
func GetResolutions(devicePath string, pixelFormat uint32) ([]Resolution, error) {
	fd, err := syscall.Open(devicePath, syscall.O_RDWR|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, fmt.Errorf("failed to open device: %w", err)
	}
	defer syscall.Close(fd)

	var resolutions []Resolution

	for i := uint32(0); ; i++ {
		frmsize := v4l2_frmsizeenum{
			index:        i,
			pixel_format: pixelFormat,
		}

		if ioctlErr := ioctl(fd, VIDIOC_ENUM_FRAMESIZES, unsafe.Pointer(&frmsize)); ioctlErr != nil {
			if errors.Is(ioctlErr, syscall.EINVAL) {
				break // End of enumeration
			}
			// ENOTTY means device doesn't support frame size enumeration
			if errors.Is(ioctlErr, syscall.ENOTTY) {
				return []Resolution{}, nil
			}
			return nil, fmt.Errorf("failed to enumerate frame size %d: %w", i, ioctlErr)
		}

		switch frmsize.typ {
		case V4L2_FRMSIZE_TYPE_DISCRETE:
			resolutions = append(resolutions, Resolution{
				Width:  frmsize.discrete.width,
				Height: frmsize.discrete.height,
			})
		case V4L2_FRMSIZE_TYPE_CONTINUOUS, V4L2_FRMSIZE_TYPE_STEPWISE:
			// For stepwise/continuous, return common resolutions within the range
			resolutions = append(resolutions, getStepwiseResolutions(&frmsize)...)
			return resolutions, nil // Only one stepwise entry
		}
	}

	return resolutions, nil
}

// GetFramerates returns all supported framerates for a device, format, and resolution.
func GetFramerates(devicePath string, pixelFormat uint32, width, height uint32) ([]Framerate, error) {
	fd, err := syscall.Open(devicePath, syscall.O_RDWR|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, fmt.Errorf("failed to open device: %w", err)
	}
	defer syscall.Close(fd)

	var framerates []Framerate

	for i := uint32(0); ; i++ {
		frmival := v4l2_frmivalenum{
			index:        i,
			pixel_format: pixelFormat,
			width:        width,
			height:       height,
		}

		if ioctlErr := ioctl(fd, VIDIOC_ENUM_FRAMEINTERVALS, unsafe.Pointer(&frmival)); ioctlErr != nil {
			if errors.Is(ioctlErr, syscall.EINVAL) {
				break // End of enumeration
			}
			return nil, fmt.Errorf("failed to enumerate frame interval %d: %w", i, ioctlErr)
		}

		switch frmival.typ {
		case V4L2_FRMIVAL_TYPE_DISCRETE:
			framerates = append(framerates, Framerate{
				Numerator:   frmival.discrete.numerator,
				Denominator: frmival.discrete.denominator,
			})
		case V4L2_FRMIVAL_TYPE_CONTINUOUS, V4L2_FRMIVAL_TYPE_STEPWISE:
			// For stepwise/continuous, return common framerates
			framerates = append(framerates, getCommonFramerates()...)
			return framerates, nil
		}
	}

	return framerates, nil
}

// getStepwiseResolutions returns common resolutions within a stepwise range.
func getStepwiseResolutions(frmsize *v4l2_frmsizeenum) []Resolution {
	// Common resolutions to check
	commonResolutions := [][2]uint32{
		{320, 240},  // QVGA
		{640, 480},  // VGA
		{800, 600},  // SVGA
		{1024, 768}, // XGA
		{1280, 720}, // HD
		{1280, 960},
		{1280, 1024}, // SXGA
		{1920, 1080}, // Full HD
		{1920, 1200}, // WUXGA
		{2560, 1440}, // QHD
		{3840, 2160}, // 4K UHD
		{4096, 2160}, // 4K DCI
	}

	// Extract stepwise params from union (stepwise overlays discrete in memory)
	stepwise := (*v4l2_frmsize_stepwise)(unsafe.Pointer(&frmsize.discrete))

	var resolutions []Resolution
	for _, res := range commonResolutions {
		w, h := res[0], res[1]
		if w >= stepwise.min_width && w <= stepwise.max_width &&
			h >= stepwise.min_height && h <= stepwise.max_height {
			resolutions = append(resolutions, Resolution{Width: w, Height: h})
		}
	}

	return resolutions
}

// getCommonFramerates returns a list of common framerates.
func getCommonFramerates() []Framerate {
	return []Framerate{
		{1, 60}, // 60 fps
		{1, 50}, // 50 fps
		{1, 30}, // 30 fps
		{1, 25}, // 25 fps
		{1, 20}, // 20 fps
		{1, 15}, // 15 fps
		{1, 10}, // 10 fps
		{1, 5},  // 5 fps
	}
}

// FormatFourCC converts a 4-byte pixel format to a human-readable string.
func FormatFourCC(format uint32) string {
	b := make([]byte, 4)
	b[0] = byte(format & 0xFF)
	b[1] = byte((format >> 8) & 0xFF)
	b[2] = byte((format >> 16) & 0xFF)
	b[3] = byte((format >> 24) & 0xFF)
	return string(b)
}
