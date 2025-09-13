package models

// AudioDevice represents an audio device with its capabilities
type AudioDevice struct {
	// Basic info
	CardNumber   int    `json:"card_number" example:"0" doc:"Sound card index"`
	CardID       string `json:"card_id" example:"PCH" doc:"Card identifier"`
	CardName     string `json:"card_name" example:"HDA Intel PCH" doc:"Full card name"`
	DeviceNumber int    `json:"device_number" example:"0" doc:"Device index on card"`
	DeviceName   string `json:"device_name" example:"ALC892 Analog" doc:"Device name"`
	Type         string `json:"type" example:"both" doc:"Device type: playback, capture, or both"`
	ALSADevice   string `json:"alsa_device" example:"hw:0,0" doc:"ALSA device string for FFmpeg"`

	// Capabilities (from ALSA hw_params)
	SupportedRates   []int    `json:"supported_rates" example:"[44100,48000,96000]" doc:"Supported sample rates in Hz"`
	MinChannels      int      `json:"min_channels" example:"1" doc:"Minimum number of channels"`
	MaxChannels      int      `json:"max_channels" example:"2" doc:"Maximum number of channels"`
	SupportedFormats []string `json:"supported_formats" example:"[\"S16_LE\",\"S24_LE\"]" doc:"Supported PCM formats"`
	MinBufferSize    int      `json:"min_buffer_size" example:"64" doc:"Minimum buffer size in frames"`
	MaxBufferSize    int      `json:"max_buffer_size" example:"65536" doc:"Maximum buffer size in frames"`
	MinPeriodSize    int      `json:"min_period_size" example:"32" doc:"Minimum period size in frames"`
	MaxPeriodSize    int      `json:"max_period_size" example:"32768" doc:"Maximum period size in frames"`
}

// AudioDevicesData represents the response data for audio device enumeration
type AudioDevicesData struct {
	Devices []AudioDevice `json:"devices" doc:"List of available audio devices"`
	Count   int           `json:"count" example:"2" doc:"Number of devices found"`
}

// AudioDevicesResponse represents the HTTP response for audio device enumeration
type AudioDevicesResponse struct {
	Body AudioDevicesData
}
