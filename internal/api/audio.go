package api

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/smazurov/videonode/internal/api/models"
	"github.com/smazurov/videonode/internal/audio"
)

// registerAudioRoutes registers all audio-related API endpoints under /api/devices/audio.
func (s *Server) registerAudioRoutes() {
	// GET /api/devices/audio - List all audio devices with capabilities
	huma.Register(s.api, huma.Operation{
		OperationID: "list-audio-devices",
		Method:      http.MethodGet,
		Path:        "/api/devices/audio",
		Summary:     "List Audio Devices",
		Description: "List all available audio devices with their capabilities including supported " +
			"sample rates, formats, and channel configurations",
		Tags:     []string{"devices"},
		Security: withAuth(),
	}, func(_ context.Context, _ *struct{}) (*models.AudioDevicesResponse, error) {
		detector := audio.NewDetector()
		devices, err := detector.ListDevices()
		if err != nil {
			return nil, huma.NewError(http.StatusInternalServerError, "Failed to enumerate audio devices", err)
		}

		// Convert internal audio.Device to API models.AudioDevice
		apiDevices := make([]models.AudioDevice, len(devices))
		for i, device := range devices {
			apiDevices[i] = models.AudioDevice{
				CardNumber:       device.CardNumber,
				CardID:           device.CardID,
				CardName:         device.CardName,
				DeviceNumber:     device.DeviceNumber,
				DeviceName:       device.DeviceName,
				Type:             device.Type,
				ALSADevice:       device.ALSADevice,
				SupportedRates:   device.SupportedRates,
				MinChannels:      device.MinChannels,
				MaxChannels:      device.MaxChannels,
				SupportedFormats: device.SupportedFormats,
				MinBufferSize:    device.MinBufferSize,
				MaxBufferSize:    device.MaxBufferSize,
				MinPeriodSize:    device.MinPeriodSize,
				MaxPeriodSize:    device.MaxPeriodSize,
			}
		}

		return &models.AudioDevicesResponse{
			Body: models.AudioDevicesData{
				Devices: apiDevices,
				Count:   len(apiDevices),
			},
		}, nil
	})
}
