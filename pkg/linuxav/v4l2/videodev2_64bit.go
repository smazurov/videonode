//go:build linux && (amd64 || arm64)

package v4l2

import "unsafe"

// Compile-time struct size and offset assertions.
// These fail at compile time if struct layout doesn't match kernel ABI.
// Pattern: [0]struct{} = [actual - expected]struct{} fails if actual != expected.
var (
	// Size assertions - must match kernel struct sizes exactly.
	_ [0]struct{} = [unsafe.Sizeof(v4l2Capability{}) - 104]struct{}{}
	_ [0]struct{} = [unsafe.Sizeof(v4l2Fmtdesc{}) - 64]struct{}{}
	_ [0]struct{} = [unsafe.Sizeof(v4l2FrmsizeDiscrete{}) - 8]struct{}{}
	_ [0]struct{} = [unsafe.Sizeof(v4l2FrmsizeStepwise{}) - 24]struct{}{}
	_ [0]struct{} = [unsafe.Sizeof(v4l2Frmsizeenum{}) - 44]struct{}{}
	_ [0]struct{} = [unsafe.Sizeof(v4l2Fract{}) - 8]struct{}{}
	_ [0]struct{} = [unsafe.Sizeof(v4l2Frmivalenum{}) - 52]struct{}{}
	_ [0]struct{} = [unsafe.Sizeof(v4l2BTTimings{}) - 128]struct{}{}
	_ [0]struct{} = [unsafe.Sizeof(v4l2DVTimings{}) - 132]struct{}{}
	_ [0]struct{} = [unsafe.Sizeof(v4l2EventSubscription{}) - 32]struct{}{}
	_ [0]struct{} = [unsafe.Sizeof(v4l2Event{}) - 132]struct{}{}

	// Offset assertions - must match kernel packed struct layout.
	_ [0]struct{} = [unsafe.Offsetof(v4l2DVTimings{}.btRaw) - 4]struct{}{}
)

// IOCTL constants for 64-bit architectures.
const (
	vidiocQuerycap           = 0x80685600
	vidiocEnumFmt            = 0xc0405602
	vidiocEnumFramesizes     = 0xc02c564a
	vidiocEnumFrameintervals = 0xc034564b
	vidiocGDVTimings         = 0xc0845658
	vidiocSDVTimings         = 0x40845657 // VIDIOC_S_DV_TIMINGS - set DV timings
	vidiocQueryDVTimings     = 0xc0845659 // VIDIOC_QUERY_DV_TIMINGS - query detected timings
	vidiocSubscribeEvent     = 0x4020565a
	vidiocUnsubscribeEvent   = 0x4020565b
	vidiocDqevent            = 0x80885659
)

// v4l2Capability has size 104 bytes.
type v4l2Capability struct {
	driver       [16]byte  // offset 0
	card         [32]byte  // offset 16
	busInfo      [32]byte  // offset 48
	version      uint32    // offset 80
	capabilities uint32    // offset 84
	deviceCaps   uint32    // offset 88
	reserved     [3]uint32 // offset 92
}

// v4l2Fmtdesc has size 64 bytes.
type v4l2Fmtdesc struct {
	index       uint32    // offset 0
	typ         uint32    // offset 4
	flags       uint32    // offset 8
	description [32]byte  // offset 12
	pixelformat uint32    // offset 44
	mbusCode    uint32    // offset 48
	reserved    [3]uint32 // offset 52
}

// v4l2FrmsizeDiscrete has size 8 bytes.
type v4l2FrmsizeDiscrete struct {
	width  uint32
	height uint32
}

// v4l2FrmsizeStepwise has size 24 bytes.
type v4l2FrmsizeStepwise struct {
	minWidth   uint32
	maxWidth   uint32
	stepWidth  uint32
	minHeight  uint32
	maxHeight  uint32
	stepHeight uint32
}

// v4l2Frmsizeenum has size 44 bytes.
type v4l2Frmsizeenum struct {
	index       uint32              // offset 0
	pixelFormat uint32              // offset 4
	typ         uint32              // offset 8
	discrete    v4l2FrmsizeDiscrete // offset 12 (union with stepwise)
	_           [16]byte            // padding for stepwise
	reserved    [2]uint32           // offset 36
}

// v4l2Fract has size 8 bytes.
type v4l2Fract struct {
	numerator   uint32
	denominator uint32
}

// v4l2Frmivalenum has size 52 bytes.
type v4l2Frmivalenum struct {
	index       uint32    // offset 0
	pixelFormat uint32    // offset 4
	width       uint32    // offset 8
	height      uint32    // offset 12
	typ         uint32    // offset 16
	discrete    v4l2Fract // offset 20 (union with stepwise)
	_           [16]byte  // padding for stepwise
	reserved    [2]uint32 // offset 44
}

// v4l2BTTimings has size 124 bytes (embedded in v4l2DVTimings).
type v4l2BTTimings struct {
	width         uint32    // offset 0
	height        uint32    // offset 4
	interlaced    uint32    // offset 8
	_             uint32    // padding
	pixelclock    uint64    // offset 16
	hfrontporch   uint32    // offset 24
	hsync         uint32    // offset 28
	hbackporch    uint32    // offset 32
	vfrontporch   uint32    // offset 36
	vsync         uint32    // offset 40
	vbackporch    uint32    // offset 44
	ilVfrontporch uint32    // offset 48
	ilVsync       uint32    // offset 52
	ilVbackporch  uint32    // offset 56
	standards     uint32    // offset 60
	flags         uint32    // offset 64
	pictureAspect v4l2Fract // offset 68
	cea861Vic     uint8     // offset 76
	hdmiVic       uint8     // offset 77
	reserved      [46]byte  // offset 78 to 124
}

// v4l2DVTimings has size 132 bytes to match kernel's packed struct.
// The btRaw field uses a raw byte array to prevent Go from inserting
// alignment padding between typ and bt (Go would pad to 8-byte alignment
// for the uint64 in v4l2BTTimings, but kernel expects bt at offset 4).
type v4l2DVTimings struct {
	typ   uint32    // offset 0
	btRaw [128]byte // offset 4 - raw bytes for v4l2BTTimings, no alignment padding
}

// bt returns the btRaw bytes interpreted as a v4l2BTTimings struct.
func (t *v4l2DVTimings) bt() *v4l2BTTimings {
	return (*v4l2BTTimings)(unsafe.Pointer(&t.btRaw[0]))
}

// v4l2EventSubscription has size 32 bytes.
type v4l2EventSubscription struct {
	typ      uint32    // offset 0
	id       uint32    // offset 4
	flags    uint32    // offset 8
	reserved [5]uint32 // offset 12
}

// v4l2Event has size 136 bytes.
type v4l2Event struct {
	typ       uint32    // offset 0
	_         [4]byte   // padding
	u         [64]byte  // offset 8 - union containing src_change at offset 0
	pending   uint32    // offset 72
	sequence  uint32    // offset 76
	timestamp [16]byte  // offset 80 - struct timespec
	id        uint32    // offset 96
	reserved  [8]uint32 // offset 100
}

// getSrcChangeChanges extracts the changes field from the event union.
func (e *v4l2Event) getSrcChangeChanges() uint32 {
	// The changes field is at the start of the union
	return uint32(e.u[0]) | uint32(e.u[1])<<8 | uint32(e.u[2])<<16 | uint32(e.u[3])<<24
}
