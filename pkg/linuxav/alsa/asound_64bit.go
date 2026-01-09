//go:build linux && (amd64 || arm64)

package alsa

import "unsafe"

// Compile-time struct size assertions.
// These will cause build failures if struct sizes don't match kernel expectations.
var (
	_ [376]byte = [unsafe.Sizeof(sndCtlCardInfo{})]byte{}
	_ [288]byte = [unsafe.Sizeof(sndPCMInfo{})]byte{}
	_ [32]byte  = [unsafe.Sizeof(sndMask{})]byte{}
	_ [12]byte  = [unsafe.Sizeof(sndInterval{})]byte{}
	_ [608]byte = [unsafe.Sizeof(sndPCMHwParams{})]byte{}
)

// IOCTL constants for 64-bit architectures.
const (
	// Control interface IOCTLs.
	sndrvCtlIoctlCardInfo      = 0x81785501
	sndrvCtlIoctlPCMNextDevice = 0x80045530
	sndrvCtlIoctlPCMInfo       = 0xc1205531

	// PCM IOCTLs.
	sndrvPCMIoctlInfo     = 0x81204101
	sndrvPCMIoctlHwRefine = 0xc2604110
	sndrvPCMIoctlHwParams = 0xc2604111
	sndrvPCMIoctlSwParams = 0xc0884113
	sndrvPCMIoctlPrepare  = 0x00004140
)

// Hardware parameter constants.
const (
	sndrvPCMHwParamAccess        = 0
	sndrvPCMHwParamFormat        = 1
	sndrvPCMHwParamSubformat     = 2
	sndrvPCMHwParamFirstMask     = 0
	sndrvPCMHwParamLastMask      = 2
	sndrvPCMHwParamSampleBits    = 8
	sndrvPCMHwParamFrameBits     = 9
	sndrvPCMHwParamChannels      = 10
	sndrvPCMHwParamRate          = 11
	sndrvPCMHwParamPeriodTime    = 12
	sndrvPCMHwParamPeriodSize    = 13
	sndrvPCMHwParamPeriodBytes   = 14
	sndrvPCMHwParamPeriods       = 15
	sndrvPCMHwParamBufferTime    = 16
	sndrvPCMHwParamBufferSize    = 17
	sndrvPCMHwParamBufferBytes   = 18
	sndrvPCMHwParamTickTime      = 19
	sndrvPCMHwParamFirstInterval = 8
	sndrvPCMHwParamLastInterval  = 19

	sndrvMaskMax = 256

	sndrvPCMAccessRwInterleaved = 3
)

// sndCtlCardInfo has size 376 bytes.
type sndCtlCardInfo struct {
	card       int32     // offset 0
	_          [4]byte   // padding
	id         [16]byte  // offset 8
	driver     [16]byte  // offset 24
	name       [32]byte  // offset 40
	longname   [80]byte  // offset 72
	reserved   [16]byte  // offset 152
	mixername  [80]byte  // offset 168
	components [128]byte // offset 248
}

// sndPCMInfo has size 288 bytes.
type sndPCMInfo struct {
	device          uint32   // offset 0
	subdevice       uint32   // offset 4
	stream          int32    // offset 8
	card            int32    // offset 12
	id              [64]byte // offset 16
	name            [80]byte // offset 80
	subname         [32]byte // offset 160
	devClass        int32    // offset 192
	devSubclass     int32    // offset 196
	subdevicesCount uint32   // offset 200
	subdevicesAvail uint32   // offset 204
	_               [16]byte // padding
	reserved        [64]byte // offset 224
}

// sndMask has size 32 bytes.
type sndMask struct {
	bits [(sndrvMaskMax + 31) / 32]uint32
}

// sndInterval has size 12 bytes.
type sndInterval struct {
	minVal uint32
	maxVal uint32
	bit    uint32
}

// sndPCMHwParams has size 608 bytes.
type sndPCMHwParams struct {
	flags     uint32                                                                      // offset 0
	masks     [sndrvPCMHwParamLastMask - sndrvPCMHwParamFirstMask + 1]sndMask             // offset 4, size 96
	mres      [5]sndMask                                                                  // offset 100, size 160
	intervals [sndrvPCMHwParamLastInterval - sndrvPCMHwParamFirstInterval + 1]sndInterval // offset 260, size 144
	ires      [9]sndInterval                                                              // offset 404, size 108
	rmask     uint32                                                                      // offset 512
	cmask     uint32                                                                      // offset 516
	info      uint32                                                                      // offset 520
	msbits    uint32                                                                      // offset 524
	rateNum   uint32                                                                      // offset 528
	rateDen   uint32                                                                      // offset 532
	fifoSize  uint64                                                                      // offset 536, size 8 (snd_pcm_uframes_t)
	reserved  [64]byte                                                                    // offset 544
}

// Helper methods for sndPCMHwParams.
func (p *sndPCMHwParams) init() {
	for i := range p.masks {
		p.masks[i].bits[0] = 0xFFFFFFFF
		p.masks[i].bits[1] = 0xFFFFFFFF
	}
	for i := range p.intervals {
		p.intervals[i].maxVal = 0xFFFFFFFF
	}
	p.rmask = 0xFFFFFFFF
	p.cmask = 0
	p.info = 0xFFFFFFFF
}

func (p *sndPCMHwParams) setMask(param, val uint32) {
	p.masks[param].bits[0] = 0
	p.masks[param].bits[1] = 0
	p.masks[param].bits[val>>5] = 1 << (val & 0x1F)
}

func (p *sndPCMHwParams) checkMask(param, val uint32) bool {
	return p.masks[param].bits[val>>5]&(1<<(val&0x1F)) > 0
}

func (p *sndPCMHwParams) getInterval(param uint32) (minVal, maxVal uint32) {
	idx := param - sndrvPCMHwParamFirstInterval
	return p.intervals[idx].minVal, p.intervals[idx].maxVal
}
