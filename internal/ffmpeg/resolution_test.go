package ffmpeg

import "testing"

func TestParseResolution(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantW     int
		wantH     int
		wantError bool
	}{
		{"hd", "1920x1080", 1920, 1080, false},
		{"4k", "3840x2160", 3840, 2160, false},
		{"sd", "640x480", 640, 480, false},
		{"empty", "", 0, 0, true},
		{"only width", "1920", 0, 0, true},
		{"garbage", "not-a-resolution", 0, 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w, h, err := ParseResolution(tt.input)
			if (err != nil) != tt.wantError {
				t.Fatalf("error = %v, wantError = %v", err, tt.wantError)
			}
			if w != tt.wantW || h != tt.wantH {
				t.Errorf("got (%d, %d), want (%d, %d)", w, h, tt.wantW, tt.wantH)
			}
		})
	}
}
