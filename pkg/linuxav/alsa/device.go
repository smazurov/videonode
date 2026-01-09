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
		cardInfo := sndCtlCardInfo{}
		if ioctlErr := ioctl(uintptr(ctlFd), sndrvCtlIoctlCardInfo, unsafe.Pointer(&cardInfo)); ioctlErr != nil {
			syscall.Close(ctlFd)
			continue
		}

		// Enumerate PCM devices on this card
		deviceNum := int32(-1)
		for {
			if ioctlErr := ioctl(uintptr(ctlFd), sndrvCtlIoctlPCMNextDevice, unsafe.Pointer(&deviceNum)); ioctlErr != nil {
				break
			}
			if deviceNum < 0 {
				break // No more devices
			}

			// Get PCM info for capture stream
			pcmInfo := sndPCMInfo{
				device:    uint32(deviceNum),
				subdevice: 0,
				stream:    StreamCapture,
			}

			if ioctlErr := ioctl(uintptr(ctlFd), sndrvCtlIoctlPCMInfo, unsafe.Pointer(&pcmInfo)); ioctlErr != nil {
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
			if caps, capsErr := queryCapabilities(alsaDevice); capsErr == nil {
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
	// Parse card and device from alsaDevice "hw:X,Y"
	var cardNum, devNum int
	_, err := fmt.Sscanf(alsaDevice, "hw:%d,%d", &cardNum, &devNum)
	if err != nil {
		return nil, err
	}
	pcmPath := fmt.Sprintf("/dev/snd/pcmC%dD%dc", cardNum, devNum)

	fd, err := syscall.Open(pcmPath, syscall.O_RDWR|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, err
	}
	defer syscall.Close(fd)

	// Remove nonblock for better read handling
	syscall.SetNonblock(fd, false)

	// Initialize and refine hw_params
	hwparams := sndPCMHwParams{}
	hwparams.init()

	// Set access mode
	hwparams.setMask(sndrvPCMHwParamAccess, sndrvPCMAccessRwInterleaved)

	if ioctlErr := ioctl(uintptr(fd), sndrvPCMIoctlHwRefine, unsafe.Pointer(&hwparams)); ioctlErr != nil {
		return nil, ioctlErr
	}

	caps := &capabilities{}

	// Get channels range
	minCh, maxCh := hwparams.getInterval(sndrvPCMHwParamChannels)
	caps.minChannels = int(minCh)
	caps.maxChannels = int(maxCh)

	// Get rate range and test common rates
	minRate, maxRate := hwparams.getInterval(sndrvPCMHwParamRate)
	for _, rate := range CommonSampleRates {
		if uint32(rate) >= minRate && uint32(rate) <= maxRate {
			caps.rates = append(caps.rates, rate)
		}
	}

	// Test supported formats
	for _, format := range CommonFormats {
		if hwparams.checkMask(sndrvPCMHwParamFormat, uint32(format)) {
			caps.formats = append(caps.formats, FormatName(format))
		}
	}

	// Get buffer size range
	minBuf, maxBuf := hwparams.getInterval(sndrvPCMHwParamBufferSize)
	caps.minBufferSize = int(minBuf)
	caps.maxBufferSize = int(maxBuf)

	// Get period size range
	minPer, maxPer := hwparams.getInterval(sndrvPCMHwParamPeriodSize)
	caps.minPeriodSize = int(minPer)
	caps.maxPeriodSize = int(maxPer)

	return caps, nil
}
