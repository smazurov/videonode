//go:build linux

package alsa

import (
	"fmt"
	"os"
	"syscall"
	"unsafe"
)

// ListDevices returns all available ALSA audio capture devices.
func ListDevices() ([]Device, error) {
	var devices []Device

	// Iterate through all sound cards
	for cardNum := 0; ; cardNum++ {
		// Try to open the control device
		ctlPath := fmt.Sprintf("/dev/snd/controlC%d", cardNum)
		ctlFd, err := syscall.Open(ctlPath, syscall.O_RDONLY, 0)
		if err != nil {
			if os.IsNotExist(err) || err == syscall.ENOENT {
				break // No more cards
			}
			continue // Skip this card
		}

		// Get card info
		cardInfo := snd_ctl_card_info{}
		if err := ioctl(uintptr(ctlFd), SNDRV_CTL_IOCTL_CARD_INFO, unsafe.Pointer(&cardInfo)); err != nil {
			syscall.Close(ctlFd)
			continue
		}

		// Enumerate PCM devices on this card
		deviceNum := int32(-1)
		for {
			if err := ioctl(uintptr(ctlFd), SNDRV_CTL_IOCTL_PCM_NEXT_DEVICE, unsafe.Pointer(&deviceNum)); err != nil {
				break
			}
			if deviceNum < 0 {
				break // No more devices
			}

			// Get PCM info for capture stream
			pcmInfo := snd_pcm_info{
				device:    uint32(deviceNum),
				subdevice: 0,
				stream:    StreamCapture,
			}

			if err := ioctl(uintptr(ctlFd), SNDRV_CTL_IOCTL_PCM_INFO, unsafe.Pointer(&pcmInfo)); err != nil {
				continue // Device doesn't support capture
			}

			alsaDevice := FormatALSADevice(cardNum, int(deviceNum))

			device := Device{
				CardNumber:   cardNum,
				CardID:       cstr(cardInfo.id[:]),
				CardName:     cstr(cardInfo.longname[:]),
				DeviceNumber: int(deviceNum),
				DeviceName:   cstr(pcmInfo.name[:]),
				Type:         "capture",
				ALSADevice:   alsaDevice,
			}

			// Query capabilities
			if caps, err := queryCapabilities(alsaDevice); err == nil {
				device.SupportedRates = caps.rates
				device.MinChannels = caps.minChannels
				device.MaxChannels = caps.maxChannels
				device.SupportedFormats = caps.formats
				device.MinBufferSize = caps.minBufferSize
				device.MaxBufferSize = caps.maxBufferSize
				device.MinPeriodSize = caps.minPeriodSize
				device.MaxPeriodSize = caps.maxPeriodSize
			}

			devices = append(devices, device)
		}

		syscall.Close(ctlFd)
	}

	return devices, nil
}

type capabilities struct {
	rates         []int
	minChannels   int
	maxChannels   int
	formats       []string
	minBufferSize int
	maxBufferSize int
	minPeriodSize int
	maxPeriodSize int
}

func queryCapabilities(alsaDevice string) (*capabilities, error) {
	// Open the PCM device
	pcmPath := fmt.Sprintf("/dev/snd/pcmC%sD%sc",
		string(alsaDevice[3]), // card number
		string(alsaDevice[5]), // device number
	)

	// Parse card and device from alsaDevice "hw:X,Y"
	var cardNum, devNum int
	_, err := fmt.Sscanf(alsaDevice, "hw:%d,%d", &cardNum, &devNum)
	if err != nil {
		return nil, err
	}
	pcmPath = fmt.Sprintf("/dev/snd/pcmC%dD%dc", cardNum, devNum)

	fd, err := syscall.Open(pcmPath, syscall.O_RDWR|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, err
	}
	defer syscall.Close(fd)

	// Remove nonblock for better read handling
	syscall.SetNonblock(fd, false)

	// Initialize and refine hw_params
	hwparams := snd_pcm_hw_params{}
	hwparams.init()

	// Set access mode
	hwparams.setMask(SNDRV_PCM_HW_PARAM_ACCESS, SNDRV_PCM_ACCESS_RW_INTERLEAVED)

	if err := ioctl(uintptr(fd), SNDRV_PCM_IOCTL_HW_REFINE, unsafe.Pointer(&hwparams)); err != nil {
		return nil, err
	}

	caps := &capabilities{}

	// Get channels range
	minCh, maxCh := hwparams.getInterval(SNDRV_PCM_HW_PARAM_CHANNELS)
	caps.minChannels = int(minCh)
	caps.maxChannels = int(maxCh)

	// Get rate range and test common rates
	minRate, maxRate := hwparams.getInterval(SNDRV_PCM_HW_PARAM_RATE)
	for _, rate := range CommonSampleRates {
		if uint32(rate) >= minRate && uint32(rate) <= maxRate {
			caps.rates = append(caps.rates, rate)
		}
	}

	// Test supported formats
	for _, format := range CommonFormats {
		if hwparams.checkMask(SNDRV_PCM_HW_PARAM_FORMAT, uint32(format)) {
			caps.formats = append(caps.formats, FormatName(format))
		}
	}

	// Get buffer size range
	minBuf, maxBuf := hwparams.getInterval(SNDRV_PCM_HW_PARAM_BUFFER_SIZE)
	caps.minBufferSize = int(minBuf)
	caps.maxBufferSize = int(maxBuf)

	// Get period size range
	minPer, maxPer := hwparams.getInterval(SNDRV_PCM_HW_PARAM_PERIOD_SIZE)
	caps.minPeriodSize = int(minPer)
	caps.maxPeriodSize = int(maxPer)

	return caps, nil
}
