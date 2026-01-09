//go:build linux && (amd64 || arm64)

package v4l2

import "unsafe"

// Compile-time struct size assertions.
// These will cause build failures if struct sizes don't match kernel expectations.
var (
	_ [104]byte = [unsafe.Sizeof(v4l2_capability{})]byte{}
	_ [64]byte  = [unsafe.Sizeof(v4l2_fmtdesc{})]byte{}
	_ [8]byte   = [unsafe.Sizeof(v4l2_frmsize_discrete{})]byte{}
	_ [24]byte  = [unsafe.Sizeof(v4l2_frmsize_stepwise{})]byte{}
	_ [44]byte  = [unsafe.Sizeof(v4l2_frmsizeenum{})]byte{}
	_ [8]byte   = [unsafe.Sizeof(v4l2_fract{})]byte{}
	_ [52]byte  = [unsafe.Sizeof(v4l2_frmivalenum{})]byte{}
	_ [128]byte = [unsafe.Sizeof(v4l2_bt_timings{})]byte{}
	_ [144]byte = [unsafe.Sizeof(v4l2_dv_timings{})]byte{}
	_ [32]byte  = [unsafe.Sizeof(v4l2_event_subscription{})]byte{}
	_ [132]byte = [unsafe.Sizeof(v4l2_event{})]byte{}
)

// IOCTL constants for 64-bit architectures
const (
	VIDIOC_QUERYCAP            = 0x80685600
	VIDIOC_ENUM_FMT            = 0xc0405602
	VIDIOC_ENUM_FRAMESIZES     = 0xc02c564a
	VIDIOC_ENUM_FRAMEINTERVALS = 0xc034564b
	VIDIOC_G_DV_TIMINGS        = 0xc0845658
	VIDIOC_SUBSCRIBE_EVENT     = 0x4020565a
	VIDIOC_UNSUBSCRIBE_EVENT   = 0x4020565b
	VIDIOC_DQEVENT             = 0x80885659
)

// v4l2_capability - size 104 bytes
type v4l2_capability struct {
	driver       [16]byte  // offset 0
	card         [32]byte  // offset 16
	bus_info     [32]byte  // offset 48
	version      uint32    // offset 80
	capabilities uint32    // offset 84
	device_caps  uint32    // offset 88
	reserved     [3]uint32 // offset 92
}

// v4l2_fmtdesc - size 64 bytes
type v4l2_fmtdesc struct {
	index       uint32    // offset 0
	typ         uint32    // offset 4
	flags       uint32    // offset 8
	description [32]byte  // offset 12
	pixelformat uint32    // offset 44
	mbus_code   uint32    // offset 48
	reserved    [3]uint32 // offset 52
}

// v4l2_frmsize_discrete - size 8 bytes
type v4l2_frmsize_discrete struct {
	width  uint32
	height uint32
}

// v4l2_frmsize_stepwise - size 24 bytes
type v4l2_frmsize_stepwise struct {
	min_width   uint32
	max_width   uint32
	step_width  uint32
	min_height  uint32
	max_height  uint32
	step_height uint32
}

// v4l2_frmsizeenum - size 44 bytes
type v4l2_frmsizeenum struct {
	index        uint32                // offset 0
	pixel_format uint32                // offset 4
	typ          uint32                // offset 8
	discrete     v4l2_frmsize_discrete // offset 12 (union with stepwise)
	_            [16]byte              // padding for stepwise
	reserved     [2]uint32             // offset 36
}

// v4l2_fract - size 8 bytes
type v4l2_fract struct {
	numerator   uint32
	denominator uint32
}

// v4l2_frmival_stepwise - size 24 bytes
type v4l2_frmival_stepwise struct {
	min  v4l2_fract
	max  v4l2_fract
	step v4l2_fract
}

// v4l2_frmivalenum - size 52 bytes
type v4l2_frmivalenum struct {
	index        uint32     // offset 0
	pixel_format uint32     // offset 4
	width        uint32     // offset 8
	height       uint32     // offset 12
	typ          uint32     // offset 16
	discrete     v4l2_fract // offset 20 (union with stepwise)
	_            [16]byte   // padding for stepwise
	reserved     [2]uint32  // offset 44
}

// v4l2_bt_timings - size 124 bytes (embedded in v4l2_dv_timings)
type v4l2_bt_timings struct {
	width          uint32     // offset 0
	height         uint32     // offset 4
	interlaced     uint32     // offset 8
	_              uint32     // padding
	pixelclock     uint64     // offset 16
	hfrontporch    uint32     // offset 24
	hsync          uint32     // offset 28
	hbackporch     uint32     // offset 32
	vfrontporch    uint32     // offset 36
	vsync          uint32     // offset 40
	vbackporch     uint32     // offset 44
	il_vfrontporch uint32     // offset 48
	il_vsync       uint32     // offset 52
	il_vbackporch  uint32     // offset 56
	standards      uint32     // offset 60
	flags          uint32     // offset 64
	picture_aspect v4l2_fract // offset 68
	cea861_vic     uint8      // offset 76
	hdmi_vic       uint8      // offset 77
	reserved       [46]byte   // offset 78 to 124
}

// v4l2_dv_timings - size 132 bytes
type v4l2_dv_timings struct {
	typ uint32          // offset 0
	bt  v4l2_bt_timings // offset 4
	_   [4]byte         // padding to 132
}

// v4l2_event_subscription - size 32 bytes
type v4l2_event_subscription struct {
	typ      uint32    // offset 0
	id       uint32    // offset 4
	flags    uint32    // offset 8
	reserved [5]uint32 // offset 12
}

// v4l2_event_src_change - embedded in v4l2_event union
type v4l2_event_src_change struct {
	changes uint32
}

// v4l2_event - size 136 bytes
type v4l2_event struct {
	typ       uint32    // offset 0
	_         [4]byte   // padding
	u         [64]byte  // offset 8 - union containing src_change at offset 0
	pending   uint32    // offset 72
	sequence  uint32    // offset 76
	timestamp [16]byte  // offset 80 - struct timespec
	id        uint32    // offset 96
	reserved  [8]uint32 // offset 100
}

// getSrcChangeChanges extracts the changes field from the event union
func (e *v4l2_event) getSrcChangeChanges() uint32 {
	// The changes field is at the start of the union
	return uint32(e.u[0]) | uint32(e.u[1])<<8 | uint32(e.u[2])<<16 | uint32(e.u[3])<<24
}
