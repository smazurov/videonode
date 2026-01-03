//go:build linux

package audio

/*
#cgo LDFLAGS: -lasound
#include <alsa/asoundlib.h>
#include <stdlib.h>
#include <string.h>
*/
import "C"

import (
	"fmt"
	"unsafe"
)

type linuxDetector struct{}

func newPlatformDetector() Detector {
	return &linuxDetector{}
}

// ListDevices enumerates all available ALSA audio capture devices.
func (d *linuxDetector) ListDevices() ([]Device, error) {
	var devices []Device

	// Get the list of cards using ALSA API
	cardNum := C.int(-1)
	for C.snd_card_next(&cardNum) >= 0 && cardNum >= 0 {
		// Get card info
		cardInfo := d.getCardInfo(int(cardNum))
		if cardInfo == nil {
			continue
		}

		// Enumerate PCM devices for this card
		cardDevices := d.enumeratePCMDevices(int(cardNum), cardInfo)
		devices = append(devices, cardDevices...)
	}

	return devices, nil
}

type cardInfo struct {
	number   int
	id       string
	name     string
	driver   string
	longname string
}

func (d *linuxDetector) getCardInfo(cardNum int) *cardInfo {
	var ctl *C.snd_ctl_t
	cardName := fmt.Sprintf("hw:%d", cardNum)
	cCardName := C.CString(cardName)
	defer C.free(unsafe.Pointer(cCardName))

	// Open control interface for the card
	if C.snd_ctl_open(&ctl, cCardName, 0) < 0 { //nolint:gocritic // CGO false positive
		return nil
	}
	defer C.snd_ctl_close(ctl)

	// Allocate and get card info
	var info *C.snd_ctl_card_info_t
	C.snd_ctl_card_info_malloc(&info) //nolint:gocritic // CGO false positive
	defer C.snd_ctl_card_info_free(info)

	if C.snd_ctl_card_info(ctl, info) < 0 {
		return nil
	}

	return &cardInfo{
		number:   cardNum,
		id:       C.GoString(C.snd_ctl_card_info_get_id(info)),
		name:     C.GoString(C.snd_ctl_card_info_get_name(info)),
		driver:   C.GoString(C.snd_ctl_card_info_get_driver(info)),
		longname: C.GoString(C.snd_ctl_card_info_get_longname(info)),
	}
}

func (d *linuxDetector) enumeratePCMDevices(cardNum int, card *cardInfo) []Device {
	var devices []Device
	var ctl *C.snd_ctl_t

	cardName := fmt.Sprintf("hw:%d", cardNum)
	cCardName := C.CString(cardName)
	defer C.free(unsafe.Pointer(cCardName))

	// Open control interface
	if C.snd_ctl_open(&ctl, cCardName, 0) < 0 { //nolint:gocritic // CGO false positive
		return devices
	}
	defer C.snd_ctl_close(ctl)

	// Iterate through PCM devices
	deviceNum := C.int(-1)
	for C.snd_ctl_pcm_next_device(ctl, &deviceNum) >= 0 && deviceNum >= 0 {
		// Only check for capture devices
		captureInfo := d.getPCMInfo(ctl, int(deviceNum), C.SND_PCM_STREAM_CAPTURE)

		// Skip if this device doesn't support capture
		if captureInfo == nil {
			continue
		}

		device := Device{
			CardNumber:   cardNum,
			CardID:       card.id,
			CardName:     card.longname,
			DeviceNumber: int(deviceNum),
			DeviceName:   captureInfo.name,
			Type:         "capture",
			ALSADevice:   FormatALSADevice(cardNum, int(deviceNum)),
		}

		// Query capabilities for capture
		d.queryDeviceCapabilities(&device, C.SND_PCM_STREAM_CAPTURE)

		devices = append(devices, device)
	}

	return devices
}

type pcmInfo struct {
	name string
	id   string
}

func (d *linuxDetector) getPCMInfo(ctl *C.snd_ctl_t, deviceNum int, stream C.snd_pcm_stream_t) *pcmInfo {
	var info *C.snd_pcm_info_t
	C.snd_pcm_info_malloc(&info) //nolint:gocritic // CGO false positive
	defer C.snd_pcm_info_free(info)

	C.snd_pcm_info_set_device(info, C.uint(deviceNum))
	C.snd_pcm_info_set_subdevice(info, 0)
	C.snd_pcm_info_set_stream(info, stream)

	if C.snd_ctl_pcm_info(ctl, info) < 0 {
		return nil
	}

	return &pcmInfo{
		name: C.GoString(C.snd_pcm_info_get_name(info)),
		id:   C.GoString(C.snd_pcm_info_get_id(info)),
	}
}

func (d *linuxDetector) queryDeviceCapabilities(device *Device, stream C.snd_pcm_stream_t) {
	// Create the ALSA device name
	deviceName := C.CString(device.ALSADevice)
	defer C.free(unsafe.Pointer(deviceName))

	var handle *C.snd_pcm_t

	// Try to open the device
	if err := C.snd_pcm_open(&handle, deviceName, stream, C.SND_PCM_NONBLOCK); err < 0 { //nolint:gocritic // CGO false positive
		// If we can't open the device, just return without capabilities
		return
	}
	defer C.snd_pcm_close(handle)

	// Allocate hardware parameters structure
	var hwparams *C.snd_pcm_hw_params_t
	C.snd_pcm_hw_params_malloc(&hwparams) //nolint:gocritic // CGO false positive
	defer C.snd_pcm_hw_params_free(hwparams)

	// Fill params with a full configuration space for the PCM
	if err := C.snd_pcm_hw_params_any(handle, hwparams); err < 0 {
		return
	}

	// Query min/max channels
	var minCh, maxCh C.uint
	C.snd_pcm_hw_params_get_channels_min(hwparams, &minCh)
	C.snd_pcm_hw_params_get_channels_max(hwparams, &maxCh)
	device.MinChannels = int(minCh)
	device.MaxChannels = int(maxCh)

	// Query supported sample rates
	commonRates := []int{8000, 11025, 16000, 22050, 32000, 44100, 48000, 88200, 96000, 176400, 192000}
	for _, rate := range commonRates {
		if C.snd_pcm_hw_params_test_rate(handle, hwparams, C.uint(rate), 0) == 0 {
			device.SupportedRates = append(device.SupportedRates, rate)
		}
	}

	// Query supported formats
	formatTests := []struct {
		name   string
		format C.snd_pcm_format_t
	}{
		{"U8", C.SND_PCM_FORMAT_U8},
		{"S16_LE", C.SND_PCM_FORMAT_S16_LE},
		{"S16_BE", C.SND_PCM_FORMAT_S16_BE},
		{"S24_LE", C.SND_PCM_FORMAT_S24_LE},
		{"S24_BE", C.SND_PCM_FORMAT_S24_BE},
		{"S24_3LE", C.SND_PCM_FORMAT_S24_3LE},
		{"S24_3BE", C.SND_PCM_FORMAT_S24_3BE},
		{"S32_LE", C.SND_PCM_FORMAT_S32_LE},
		{"S32_BE", C.SND_PCM_FORMAT_S32_BE},
		{"FLOAT_LE", C.SND_PCM_FORMAT_FLOAT_LE},
		{"FLOAT_BE", C.SND_PCM_FORMAT_FLOAT_BE},
		{"FLOAT64_LE", C.SND_PCM_FORMAT_FLOAT64_LE},
		{"FLOAT64_BE", C.SND_PCM_FORMAT_FLOAT64_BE},
	}

	for _, ft := range formatTests {
		if C.snd_pcm_hw_params_test_format(handle, hwparams, ft.format) == 0 {
			device.SupportedFormats = append(device.SupportedFormats, ft.name)
		}
	}

	// Query buffer sizes
	var minBuffer, maxBuffer C.snd_pcm_uframes_t
	C.snd_pcm_hw_params_get_buffer_size_min(hwparams, &minBuffer)
	C.snd_pcm_hw_params_get_buffer_size_max(hwparams, &maxBuffer)
	device.MinBufferSize = int(minBuffer)
	device.MaxBufferSize = int(maxBuffer)

	// Query period sizes
	var minPeriod, maxPeriod C.snd_pcm_uframes_t
	var dir C.int
	C.snd_pcm_hw_params_get_period_size_min(hwparams, &minPeriod, &dir)
	C.snd_pcm_hw_params_get_period_size_max(hwparams, &maxPeriod, &dir)
	device.MinPeriodSize = int(minPeriod)
	device.MaxPeriodSize = int(maxPeriod)
}
