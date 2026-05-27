package store

import (
	"testing"
	"time"

	"github.com/smazurov/videonode/internal/streams/pipeline"
)

func TestComposerFromV2_CropConfig(t *testing.T) {
	tests := []struct {
		name     string
		v2Crop   *V2CropConfig
		wantCrop *pipeline.CropConfig
	}{
		{
			name:     "nil crop (stretch/fit)",
			v2Crop:   nil,
			wantCrop: nil,
		},
		{
			name:     "explicit top-left (zero preserved)",
			v2Crop:   &V2CropConfig{X: 0, Y: 0, Scale: 1},
			wantCrop: &pipeline.CropConfig{X: 0, Y: 0, Scale: 1},
		},
		{
			name:     "centered",
			v2Crop:   &V2CropConfig{X: 0.5, Y: 0.5, Scale: 1},
			wantCrop: &pipeline.CropConfig{X: 0.5, Y: 0.5, Scale: 1},
		},
		{
			name:     "arbitrary values",
			v2Crop:   &V2CropConfig{X: 0.3, Y: 0.8, Scale: 2.0},
			wantCrop: &pipeline.CropConfig{X: 0.3, Y: 0.8, Scale: 2.0},
		},
		{
			name:     "mixed with zero x",
			v2Crop:   &V2CropConfig{X: 0, Y: 0.7, Scale: 1.5},
			wantCrop: &pipeline.CropConfig{X: 0, Y: 0.7, Scale: 1.5},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v2 := V2Composer{
				ID:     "test",
				Canvas: V2CanvasDims{W: 1920, H: 1080},
				Layout: []V2LayoutSlot{{
					Input:           "source:cam",
					W:               1920,
					H:               1080,
					AspectRatioMode: "crop",
					Crop:            tt.v2Crop,
				}},
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			}

			got := composerFromV2(v2)
			if len(got.Layout) != 1 {
				t.Fatalf("got %d layout slots, want 1", len(got.Layout))
			}

			gotCrop := got.Layout[0].Crop
			if tt.wantCrop == nil {
				if gotCrop != nil {
					t.Errorf("got Crop=%+v, want nil", gotCrop)
				}
				return
			}
			if gotCrop == nil {
				t.Fatalf("got Crop=nil, want %+v", tt.wantCrop)
			}
			if gotCrop.X != tt.wantCrop.X || gotCrop.Y != tt.wantCrop.Y || gotCrop.Scale != tt.wantCrop.Scale {
				t.Errorf("got Crop=%+v, want %+v", *gotCrop, *tt.wantCrop)
			}
		})
	}
}

func TestCropConfig_RoundTrip(t *testing.T) {
	original := V2Composer{
		ID:     "rt",
		Canvas: V2CanvasDims{W: 1920, H: 1080},
		Layout: []V2LayoutSlot{{
			Input:           "source:cam",
			W:               1920,
			H:               1080,
			AspectRatioMode: "crop",
			Crop:            &V2CropConfig{X: 0, Y: 0, Scale: 1},
		}},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	entity := composerFromV2(original)
	roundTripped := composerToV2(entity)

	if len(roundTripped.Layout) != 1 {
		t.Fatalf("got %d layout slots, want 1", len(roundTripped.Layout))
	}
	got := roundTripped.Layout[0].Crop
	if got == nil {
		t.Fatal("round-tripped Crop is nil, expected non-nil")
	}
	if got.X != 0 || got.Y != 0 || got.Scale != 1 {
		t.Errorf("round-trip lost values: got %+v, want {0, 0, 1}", *got)
	}
}

func TestCropConfig_NilRoundTrip(t *testing.T) {
	original := V2Composer{
		ID:     "rt-nil",
		Canvas: V2CanvasDims{W: 1920, H: 1080},
		Layout: []V2LayoutSlot{{
			Input:           "source:cam",
			W:               1920,
			H:               1080,
			AspectRatioMode: "stretch",
		}},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	entity := composerFromV2(original)
	roundTripped := composerToV2(entity)

	if roundTripped.Layout[0].Crop != nil {
		t.Errorf("expected nil Crop for stretch mode, got %+v", roundTripped.Layout[0].Crop)
	}
}
