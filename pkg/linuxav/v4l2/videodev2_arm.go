//go:build linux && arm && !arm64

package v4l2

import "unsafe"

// Compile-time struct size assertions for 32-bit ARM.
// These will cause build failures if struct sizes don't match kernel expectations.
var (
	_ [104]byte = [unsafe.Sizeof(v4l2_capability{})]byte{}
	_ [64]byte  = [unsafe.Sizeof(v4l2_fmtdesc{})]byte{}
	_ [8]byte   = [unsafe.Sizeof(v4l2_frmsize_discrete{})]byte{}
	_ [24]byte  = [unsafe.Sizeof(v4l2_frmsize_stepwise{})]byte{}
	_ [44]byte  = [unsafe.Sizeof(v4l2_frmsizeenum{})]byte{}
	_ [8]byte   = [unsafe.Sizeof(v4l2_fract{})]byte{}
	_ [52]byte  = [unsafe.Sizeof(v4l2_frmivalenum{})]byte{}
	_ [124]byte = [unsafe.Sizeof(v4l2_bt_timings{})]byte{}
	_ [132]byte = [unsafe.Sizeof(v4l2_dv_timings{})]byte{}
	_ [32]byte  = [unsafe.Sizeof(v4l2_event_subscription{})]byte{}
	_ [124]byte = [unsafe.Sizeof(v4l2_event{})]byte{} // Smaller on 32-bit due to timespec
)

// IOCTL constants for 32-bit ARM
// Note: Most values are the same as 64-bit since the struct sizes are identical.
// The v4l2_event struct differs due to struct timespec size difference.
const (
	VIDIOC_QUERYCAP            = 0x80685600
	VIDIOC_ENUM_FMT            = 0xc0405602
	VIDIOC_ENUM_FRAMESIZES     = 0xc02c564a
	VIDIOC_ENUM_FRAMEINTERVALS = 0xc034564b
	VIDIOC_G_DV_TIMINGS        = 0xc0845658 // v4l2_dv_timings is 132 bytes on both
	VIDIOC_SUBSCRIBE_EVENT     = 0x4020565a // v4l2_event_subscription is 32 bytes on both
	VIDIOC_UNSUBSCRIBE_EVENT   = 0x4020565b
	VIDIOC_DQEVENT             = 0x807c5659 // v4l2_event is 124 bytes on 32-bit (vs 136 on 64-bit)
)

// v4l2_capability - size 104 bytes (same as 64-bit)
type v4l2_capability struct {
	driver       [16]byte
	card         [32]byte
	bus_info     [32]byte
	version      uint32
	capabilities uint32
	device_caps  uint32
	reserved     [3]uint32
}

// v4l2_fmtdesc - size 64 bytes (same as 64-bit)
type v4l2_fmtdesc struct {
	index       uint32
	typ         uint32
	flags       uint32
	description [32]byte
	pixelformat uint32
	mbus_code   uint32
	reserved    [3]uint32
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

// v4l2_frmsizeenum - size 44 bytes (same as 64-bit)
type v4l2_frmsizeenum struct {
	index        uint32
	pixel_format uint32
	typ          uint32
	discrete     v4l2_frmsize_discrete
	_            [16]byte
	reserved     [2]uint32
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

// v4l2_frmivalenum - size 52 bytes (same as 64-bit)
type v4l2_frmivalenum struct {
	index        uint32
	pixel_format uint32
	width        uint32
	height       uint32
	typ          uint32
	discrete     v4l2_fract
	_            [16]byte
	reserved     [2]uint32
}

// v4l2_bt_timings - size 124 bytes
type v4l2_bt_timings struct {
	width          uint32
	height         uint32
	interlaced     uint32
	_              uint32
	pixelclock     uint64
	hfrontporch    uint32
	hsync          uint32
	hbackporch     uint32
	vfrontporch    uint32
	vsync          uint32
	vbackporch     uint32
	il_vfrontporch uint32
	il_vsync       uint32
	il_vbackporch  uint32
	standards      uint32
	flags          uint32
	picture_aspect v4l2_fract
	cea861_vic     uint8
	hdmi_vic       uint8
	reserved       [46]byte
}

// v4l2_dv_timings - size 132 bytes
type v4l2_dv_timings struct {
	typ uint32
	bt  v4l2_bt_timings
	_   [4]byte
}

// v4l2_event_subscription - size 32 bytes (same as 64-bit)
type v4l2_event_subscription struct {
	typ      uint32
	id       uint32
	flags    uint32
	reserved [5]uint32
}

// v4l2_event_src_change - embedded in v4l2_event union
type v4l2_event_src_change struct {
	changes uint32
}

// v4l2_event - size 124 bytes on 32-bit (vs 136 on 64-bit)
// The difference is due to struct timespec being 8 bytes on 32-bit vs 16 on 64-bit.
type v4l2_event struct {
	typ       uint32
	_         [4]byte
	u         [64]byte // union
	pending   uint32
	sequence  uint32
	timestamp [8]byte // struct timespec - 8 bytes on 32-bit
	id        uint32
	reserved  [8]uint32
}

// getSrcChangeChanges extracts the changes field from the event union
func (e *v4l2_event) getSrcChangeChanges() uint32 {
	return uint32(e.u[0]) | uint32(e.u[1])<<8 | uint32(e.u[2])<<16 | uint32(e.u[3])<<24
}
