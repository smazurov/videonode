package ffmpeg

import (
	"os"
	"strings"
)

// MapCaptureDevice rewrites a raw ALSA capture device to the videonode
// "vncap" plug -> dsnoop PCM, but only when ALSA_CONFIG_PATH is set —
// i.e. when the shipped asound.conf that defines "vncap" is actually in
// effect. Without it, vncap would be undefined and ffmpeg would fail, so
// the raw device is left untouched (status-quo behavior for dev runs that
// don't load the conf). The daemon sets ALSA_CONFIG_PATH at startup when it
// finds the bundled conf, so production capture always goes through dsnoop.
func MapCaptureDevice(dev string) string {
	if os.Getenv("ALSA_CONFIG_PATH") == "" {
		return dev
	}
	return DsnoopDevice(dev)
}

// DsnoopDevice rewrites a raw ALSA hardware device string into the
// videonode "vncap" PCM, which routes capture through a plug -> dsnoop
// chain (see packaging/videonode.asound.conf). dsnoop gives mmap-backed
// access to the hardware (legible capture on RK3588); plug converts the
// card-native sample format to whatever the encoder requests.
//
//	hw:3,0      -> vncap:CARD=3,DEV=0
//	hw:3        -> vncap:CARD=3,DEV=0
//	plughw:3,0  -> unchanged (already a conversion plugin)
//	dsnoop:...  -> unchanged (already routed)
//	""          -> unchanged
//
// This is the pure transform; callers should use MapCaptureDevice unless
// they have already verified the conf is loaded.
func DsnoopDevice(dev string) string {
	const hwPrefix = "hw:"
	if !strings.HasPrefix(dev, hwPrefix) {
		return dev
	}

	spec := dev[len(hwPrefix):]
	// CARD identifier may itself be "CARD=name" or a numeric index; the
	// device suffix, if present, follows a comma.
	card, device, hasDevice := strings.Cut(spec, ",")
	if card == "" {
		return dev
	}
	if !hasDevice {
		device = "0"
	}

	return "vncap:CARD=" + card + ",DEV=" + device
}
