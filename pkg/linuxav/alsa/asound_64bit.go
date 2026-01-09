//go:build linux && (amd64 || arm64)

package alsa

import "unsafe"

// Compile-time struct size assertions.
// These will cause build failures if struct sizes don't match kernel expectations.
var (
	_ [376]byte = [unsafe.Sizeof(snd_ctl_card_info{})]byte{}
	_ [288]byte = [unsafe.Sizeof(snd_pcm_info{})]byte{}
	_ [32]byte  = [unsafe.Sizeof(snd_mask{})]byte{}
	_ [12]byte  = [unsafe.Sizeof(snd_interval{})]byte{}
	_ [608]byte = [unsafe.Sizeof(snd_pcm_hw_params{})]byte{}
)

// IOCTL constants for 64-bit architectures
const (
	// Control interface IOCTLs
	SNDRV_CTL_IOCTL_CARD_INFO       = 0x81785501
	SNDRV_CTL_IOCTL_PCM_NEXT_DEVICE = 0x80045530
	SNDRV_CTL_IOCTL_PCM_INFO        = 0xc1205531

	// PCM IOCTLs
	SNDRV_PCM_IOCTL_INFO      = 0x81204101
	SNDRV_PCM_IOCTL_HW_REFINE = 0xc2604110
	SNDRV_PCM_IOCTL_HW_PARAMS = 0xc2604111
	SNDRV_PCM_IOCTL_SW_PARAMS = 0xc0884113
	SNDRV_PCM_IOCTL_PREPARE   = 0x00004140
)

// Hardware parameter constants
const (
	SNDRV_PCM_HW_PARAM_ACCESS         = 0
	SNDRV_PCM_HW_PARAM_FORMAT         = 1
	SNDRV_PCM_HW_PARAM_SUBFORMAT      = 2
	SNDRV_PCM_HW_PARAM_FIRST_MASK     = 0
	SNDRV_PCM_HW_PARAM_LAST_MASK      = 2
	SNDRV_PCM_HW_PARAM_SAMPLE_BITS    = 8
	SNDRV_PCM_HW_PARAM_FRAME_BITS     = 9
	SNDRV_PCM_HW_PARAM_CHANNELS       = 10
	SNDRV_PCM_HW_PARAM_RATE           = 11
	SNDRV_PCM_HW_PARAM_PERIOD_TIME    = 12
	SNDRV_PCM_HW_PARAM_PERIOD_SIZE    = 13
	SNDRV_PCM_HW_PARAM_PERIOD_BYTES   = 14
	SNDRV_PCM_HW_PARAM_PERIODS        = 15
	SNDRV_PCM_HW_PARAM_BUFFER_TIME    = 16
	SNDRV_PCM_HW_PARAM_BUFFER_SIZE    = 17
	SNDRV_PCM_HW_PARAM_BUFFER_BYTES   = 18
	SNDRV_PCM_HW_PARAM_TICK_TIME      = 19
	SNDRV_PCM_HW_PARAM_FIRST_INTERVAL = 8
	SNDRV_PCM_HW_PARAM_LAST_INTERVAL  = 19

	SNDRV_MASK_MAX = 256

	SNDRV_PCM_ACCESS_RW_INTERLEAVED = 3
)

// snd_ctl_card_info - size 376 bytes
type snd_ctl_card_info struct {
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

// snd_pcm_info - size 288 bytes
type snd_pcm_info struct {
	device           uint32   // offset 0
	subdevice        uint32   // offset 4
	stream           int32    // offset 8
	card             int32    // offset 12
	id               [64]byte // offset 16
	name             [80]byte // offset 80
	subname          [32]byte // offset 160
	dev_class        int32    // offset 192
	dev_subclass     int32    // offset 196
	subdevices_count uint32   // offset 200
	subdevices_avail uint32   // offset 204
	_                [16]byte // padding
	reserved         [64]byte // offset 224
}

// snd_mask - size 32 bytes
type snd_mask struct {
	bits [(SNDRV_MASK_MAX + 31) / 32]uint32
}

// snd_interval - size 12 bytes
type snd_interval struct {
	min uint32
	max uint32
	bit uint32
}

// snd_pcm_hw_params - size 608 bytes
type snd_pcm_hw_params struct {
	flags     uint32                                                                                 // offset 0
	masks     [SNDRV_PCM_HW_PARAM_LAST_MASK - SNDRV_PCM_HW_PARAM_FIRST_MASK + 1]snd_mask             // offset 4, size 96
	mres      [5]snd_mask                                                                            // offset 100, size 160
	intervals [SNDRV_PCM_HW_PARAM_LAST_INTERVAL - SNDRV_PCM_HW_PARAM_FIRST_INTERVAL + 1]snd_interval // offset 260, size 144
	ires      [9]snd_interval                                                                        // offset 404, size 108
	rmask     uint32                                                                                 // offset 512
	cmask     uint32                                                                                 // offset 516
	info      uint32                                                                                 // offset 520
	msbits    uint32                                                                                 // offset 524
	rate_num  uint32                                                                                 // offset 528
	rate_den  uint32                                                                                 // offset 532
	fifo_size uint64                                                                                 // offset 536, size 8 (snd_pcm_uframes_t)
	reserved  [64]byte                                                                               // offset 544
}

// Helper methods for snd_pcm_hw_params
func (p *snd_pcm_hw_params) init() {
	for i := range p.masks {
		p.masks[i].bits[0] = 0xFFFFFFFF
		p.masks[i].bits[1] = 0xFFFFFFFF
	}
	for i := range p.intervals {
		p.intervals[i].max = 0xFFFFFFFF
	}
	p.rmask = 0xFFFFFFFF
	p.cmask = 0
	p.info = 0xFFFFFFFF
}

func (p *snd_pcm_hw_params) setMask(param, val uint32) {
	p.masks[param].bits[0] = 0
	p.masks[param].bits[1] = 0
	p.masks[param].bits[val>>5] = 1 << (val & 0x1F)
}

func (p *snd_pcm_hw_params) checkMask(param, val uint32) bool {
	return p.masks[param].bits[val>>5]&(1<<(val&0x1F)) > 0
}

func (p *snd_pcm_hw_params) setInterval(param, val uint32) {
	idx := param - SNDRV_PCM_HW_PARAM_FIRST_INTERVAL
	p.intervals[idx].min = val
	p.intervals[idx].max = val
	p.intervals[idx].bit = 0b0100 // integer
}

func (p *snd_pcm_hw_params) getInterval(param uint32) (min, max uint32) {
	idx := param - SNDRV_PCM_HW_PARAM_FIRST_INTERVAL
	return p.intervals[idx].min, p.intervals[idx].max
}
