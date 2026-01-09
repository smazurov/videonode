//go:build linux && arm && !arm64

package alsa

// IOCTL constants for 32-bit ARM
// Note: Some values differ from 64-bit due to struct size differences
const (
	// Control interface IOCTLs (same as 64-bit - no pointers in structs)
	SNDRV_CTL_IOCTL_CARD_INFO       = 0x81785501
	SNDRV_CTL_IOCTL_PCM_NEXT_DEVICE = 0x80045530
	SNDRV_CTL_IOCTL_PCM_INFO        = 0xc1205531

	// PCM IOCTLs (hw_params differs due to snd_pcm_uframes_t size)
	SNDRV_PCM_IOCTL_INFO      = 0x81204101
	SNDRV_PCM_IOCTL_HW_REFINE = 0xc25c4110 // 604 bytes on 32-bit vs 608 on 64-bit
	SNDRV_PCM_IOCTL_HW_PARAMS = 0xc25c4111
	SNDRV_PCM_IOCTL_SW_PARAMS = 0xc0684113 // 104 bytes on 32-bit vs 136 on 64-bit
	SNDRV_PCM_IOCTL_PREPARE   = 0x00004140
)

// Hardware parameter constants (same across architectures)
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

// snd_ctl_card_info - size 376 bytes (same as 64-bit, no pointers)
type snd_ctl_card_info struct {
	card       int32
	_          [4]byte
	id         [16]byte
	driver     [16]byte
	name       [32]byte
	longname   [80]byte
	reserved   [16]byte
	mixername  [80]byte
	components [128]byte
}

// snd_pcm_info - size 288 bytes (same as 64-bit, no pointers)
type snd_pcm_info struct {
	device           uint32
	subdevice        uint32
	stream           int32
	card             int32
	id               [64]byte
	name             [80]byte
	subname          [32]byte
	dev_class        int32
	dev_subclass     int32
	subdevices_count uint32
	subdevices_avail uint32
	_                [16]byte
	reserved         [64]byte
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

// snd_pcm_hw_params - size 604 bytes on 32-bit (vs 608 on 64-bit)
// The difference is fifo_size which is snd_pcm_uframes_t (4 bytes on 32-bit, 8 on 64-bit)
type snd_pcm_hw_params struct {
	flags     uint32
	masks     [SNDRV_PCM_HW_PARAM_LAST_MASK - SNDRV_PCM_HW_PARAM_FIRST_MASK + 1]snd_mask
	mres      [5]snd_mask
	intervals [SNDRV_PCM_HW_PARAM_LAST_INTERVAL - SNDRV_PCM_HW_PARAM_FIRST_INTERVAL + 1]snd_interval
	ires      [9]snd_interval
	rmask     uint32
	cmask     uint32
	info      uint32
	msbits    uint32
	rate_num  uint32
	rate_den  uint32
	fifo_size uint32 // snd_pcm_uframes_t - 4 bytes on 32-bit
	reserved  [64]byte
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
