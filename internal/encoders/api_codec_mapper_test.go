package encoders

import (
	"testing"
)

func TestMatchesAPICodec(t *testing.T) {
	tests := []struct {
		encoderName string
		apiCodec    string
		expected    bool
	}{
		{"h264_vaapi", "h264", true},
		{"libx264", "h264", true},
		{"hevc_vaapi", "h265", true},
		{"libx265", "h265", true},
		{"h264_nvenc", "h264", true},
		{"hevc_nvenc", "h265", true},
		{"h264_vaapi", "h265", false},
		{"hevc_vaapi", "h264", false},
		{"mjpeg", "h264", false},
	}

	for _, tt := range tests {
		t.Run(tt.encoderName+"_"+tt.apiCodec, func(t *testing.T) {
			result := matchesAPICodec(tt.encoderName, tt.apiCodec)
			if result != tt.expected {
				t.Errorf("matchesAPICodec(%s, %s) = %v, want %v",
					tt.encoderName, tt.apiCodec, result, tt.expected)
			}
		})
	}
}
