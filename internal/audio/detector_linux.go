//go:build linux

package audio

import (
	"github.com/smazurov/videonode/pkg/linuxav/alsa"
)

type linuxDetector struct{}

func newPlatformDetector() Detector {
	return &linuxDetector{}
}

// ListDevices enumerates all available ALSA audio capture devices.
func (d *linuxDetector) ListDevices() ([]Device, error) {
	alsaDevices, err := alsa.ListDevices()
	if err != nil {
		return nil, err
	}

	// Convert alsa.Device to audio.Device
	devices := make([]Device, len(alsaDevices))
	for i, dev := range alsaDevices {
		devices[i] = Device{
			CardNumber:       dev.CardNumber,
			CardID:           dev.CardID,
			CardName:         dev.CardName,
			DeviceNumber:     dev.DeviceNumber,
			DeviceName:       dev.DeviceName,
			Type:             dev.Type,
			ALSADevice:       dev.ALSADevice,
			SupportedRates:   dev.SupportedRates,
			MinChannels:      dev.MinChannels,
			MaxChannels:      dev.MaxChannels,
			SupportedFormats: dev.SupportedFormats,
			MinBufferSize:    dev.MinBufferSize,
			MaxBufferSize:    dev.MaxBufferSize,
			MinPeriodSize:    dev.MinPeriodSize,
			MaxPeriodSize:    dev.MaxPeriodSize,
		}
	}

	return devices, nil
}
