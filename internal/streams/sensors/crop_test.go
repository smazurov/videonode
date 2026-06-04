package sensors

import (
	"math"
	"testing"
)

func approx(a, b float64) bool { return math.Abs(a-b) < 1e-6 }

func TestBBoxToCrop(t *testing.T) {
	tests := []struct {
		name            string
		b               BBox
		margin          float64
		wantX, wantY    float64
		wantScaleApprox float64
	}{
		{"centered square no margin", BBox{0.25, 0.25, 0.5, 0.5}, 0, 0.5, 0.5, 2.0},
		{"centered with 10% margin", BBox{0.25, 0.25, 0.5, 0.5}, 0.10, 0.5, 0.5, 1.0 / 0.55},
		{"wide bbox uses larger dim", BBox{0.1, 0.4, 0.8, 0.2}, 0, 0.5, 0.5, 1.0 / 0.8},
		{"offcenter pan", BBox{0.0, 0.0, 0.4, 0.4}, 0, 0.2, 0.2, 2.5},
		{"full frame clamps scale to 1", BBox{0, 0, 1, 1}, 0.5, 0.5, 0.5, 1.0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BBoxToCrop(tt.b, tt.margin)
			if !approx(got.X, tt.wantX) || !approx(got.Y, tt.wantY) {
				t.Errorf("center = (%v,%v), want (%v,%v)", got.X, got.Y, tt.wantX, tt.wantY)
			}
			if !approx(got.Scale, tt.wantScaleApprox) {
				t.Errorf("scale = %v, want %v", got.Scale, tt.wantScaleApprox)
			}
			if got.Scale < 1.0 {
				t.Errorf("scale %v must be >= 1.0", got.Scale)
			}
		})
	}
}

func TestBBoxToCropClampsCenter(t *testing.T) {
	// A bbox spilling past the right edge still yields an in-range pan center.
	got := BBoxToCrop(BBox{0.9, 0.9, 0.3, 0.3}, 0)
	if got.X < 0 || got.X > 1 || got.Y < 0 || got.Y > 1 {
		t.Errorf("center (%v,%v) out of [0,1]", got.X, got.Y)
	}
}

func TestCropDist(t *testing.T) {
	a := Crop{X: 0.5, Y: 0.5, Scale: 2.0}
	if d := a.Dist(a); d != 0 {
		t.Errorf("self distance = %v, want 0", d)
	}
	pan := Crop{X: 0.6, Y: 0.5, Scale: 2.0}
	if d := a.Dist(pan); !approx(d, 0.1) {
		t.Errorf("pan distance = %v, want 0.1", d)
	}
}
