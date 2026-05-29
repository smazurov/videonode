package services

import (
	"testing"

	"github.com/smazurov/videonode/internal/streams/pipeline"
)

// sourceDimsChanged gates the dependent-encoder rebuild: it must fire only
// when the geometry/rate baked into ffmpeg's `-s`/`-framerate` actually
// changes, so a no-op or codec-only edit leaves connected readers undisturbed.
func TestSourceDimsChanged(t *testing.T) {
	f := func(w, h, fps uint32, cc string) *pipeline.SourceFormat {
		return &pipeline.SourceFormat{FourCC: cc, Width: w, Height: h, FPS: fps}
	}
	tests := []struct {
		name string
		a, b *pipeline.SourceFormat
		want bool
	}{
		{"both nil", nil, nil, false},
		{"nil to set", nil, f(1920, 1080, 30, "NV12"), true},
		{"set to nil", f(1920, 1080, 30, "NV12"), nil, true},
		{"identical", f(1920, 1080, 30, "NV12"), f(1920, 1080, 30, "NV12"), false},
		{"width differs", f(3840, 2160, 30, "MJPG"), f(1920, 1080, 30, "NV12"), true},
		{"height differs", f(1920, 1080, 30, "NV12"), f(1920, 720, 30, "NV12"), true},
		{"fps differs", f(1920, 1080, 60, "NV12"), f(1920, 1080, 30, "NV12"), true},
		{"only fourcc differs", f(1920, 1080, 30, "MJPG"), f(1920, 1080, 30, "NV12"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := sourceDimsChanged(tt.a, tt.b); got != tt.want {
				t.Errorf("sourceDimsChanged(%v, %v) = %v, want %v", tt.a, tt.b, got, tt.want)
			}
		})
	}
}
