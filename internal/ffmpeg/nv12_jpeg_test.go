package ffmpeg

import (
	"os/exec"
	"testing"
)

// TestEncodeNV12ToJPEG_ProducesJPEGMarker confirms the helper produces JPEG
// bytes whose first two bytes are the JPEG SOI marker (0xFFD8). Skipped when
// ffmpeg is not on PATH.
func TestEncodeNV12ToJPEG_ProducesJPEGMarker(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not on PATH")
	}

	const w, h = 32, 32
	frame := make([]byte, w*h*3/2)
	for i := range frame {
		frame[i] = 128
	}

	out, err := EncodeNV12ToJPEG(frame, w, h, "bt709")
	if err != nil {
		t.Fatalf("EncodeNV12ToJPEG returned error: %v", err)
	}
	if len(out) < 2 || out[0] != 0xFF || out[1] != 0xD8 {
		t.Fatalf("output does not start with JPEG SOI marker, got %x", out[:min(8, len(out))])
	}
}
