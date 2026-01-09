//go:build linux

package alsa

// Device represents an ALSA audio capture device.
type Device struct {
	CardNumber       int
	CardID           string
	CardName         string
	DeviceNumber     int
	DeviceName       string
	Type             string // "capture"
	ALSADevice       string // ALSA device string (e.g., "hw:0,0")
	SupportedRates   []int
	MinChannels      int
	MaxChannels      int
	SupportedFormats []string
	MinBufferSize    int
	MaxBufferSize    int
	MinPeriodSize    int
	MaxPeriodSize    int
}

// FormatALSADevice creates an ALSA device string from card and device numbers.
func FormatALSADevice(cardNum, deviceNum int) string {
	return "hw:" + itoa(cardNum) + "," + itoa(deviceNum)
}

// itoa is a simple int to string conversion.
func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var b [20]byte
	pos := len(b)
	neg := i < 0
	if neg {
		i = -i
	}
	for i > 0 {
		pos--
		b[pos] = byte('0' + i%10)
		i /= 10
	}
	if neg {
		pos--
		b[pos] = '-'
	}
	return string(b[pos:])
}

// Stream types
const (
	StreamPlayback = 0
	StreamCapture  = 1
)

// PCM format constants
const (
	FormatS8        = 0
	FormatU8        = 1
	FormatS16LE     = 2
	FormatS16BE     = 3
	FormatU16LE     = 4
	FormatU16BE     = 5
	FormatS24LE     = 6
	FormatS24BE     = 7
	FormatU24LE     = 8
	FormatU24BE     = 9
	FormatS32LE     = 10
	FormatS32BE     = 11
	FormatU32LE     = 12
	FormatU32BE     = 13
	FormatFloatLE   = 14
	FormatFloatBE   = 15
	FormatFloat64LE = 16
	FormatFloat64BE = 17
	FormatMuLaw     = 20
	FormatALaw      = 21
)

// FormatName returns a human-readable name for a PCM format.
func FormatName(format int) string {
	switch format {
	case FormatS8:
		return "S8"
	case FormatU8:
		return "U8"
	case FormatS16LE:
		return "S16_LE"
	case FormatS16BE:
		return "S16_BE"
	case FormatU16LE:
		return "U16_LE"
	case FormatU16BE:
		return "U16_BE"
	case FormatS24LE:
		return "S24_LE"
	case FormatS24BE:
		return "S24_BE"
	case FormatU24LE:
		return "U24_LE"
	case FormatU24BE:
		return "U24_BE"
	case FormatS32LE:
		return "S32_LE"
	case FormatS32BE:
		return "S32_BE"
	case FormatU32LE:
		return "U32_LE"
	case FormatU32BE:
		return "U32_BE"
	case FormatFloatLE:
		return "FLOAT_LE"
	case FormatFloatBE:
		return "FLOAT_BE"
	case FormatFloat64LE:
		return "FLOAT64_LE"
	case FormatFloat64BE:
		return "FLOAT64_BE"
	case FormatMuLaw:
		return "MU_LAW"
	case FormatALaw:
		return "A_LAW"
	default:
		return "UNKNOWN"
	}
}

// Common sample rates to test
var CommonSampleRates = []int{
	8000, 11025, 16000, 22050, 32000, 44100, 48000, 88200, 96000, 176400, 192000,
}

// Common formats to test
var CommonFormats = []int{
	FormatU8, FormatS16LE, FormatS16BE, FormatS24LE, FormatS24BE,
	FormatS32LE, FormatS32BE, FormatFloatLE, FormatFloatBE,
	FormatFloat64LE, FormatFloat64BE,
}
