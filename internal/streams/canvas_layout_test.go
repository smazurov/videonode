package streams

import (
	"reflect"
	"testing"

	"github.com/smazurov/videonode/internal/ffmpeg"
)

// Helper to build a StreamSpec with just the fields the layout solver
// reads (resolution, rotation, perspective).
func specForLayout(resolution string, rotation int, persp *ffmpeg.PerspectiveConfig) *StreamSpec {
	return &StreamSpec{
		FFmpeg: FFmpegConfig{
			Resolution: resolution,
			Rotation:   rotation,
		},
		Perspective: persp,
	}
}

func TestPickLayout_TwoLandscapesTieBreaksSideBySide(t *testing.T) {
	// Two 16:9 inputs on a 16:9 canvas cannot exceed half the canvas in
	// either layout — each contained image ends up 960×540 regardless of
	// whether slots are 960×1080 (side-by-side) or 1920×540 (stacked).
	// Ties break in favor of the first candidate (side-by-side).
	sizes := [][2]int{{1920, 1080}, {1920, 1080}}
	slots, _, _ := pickLayout(1920, 1080, sizes, "")
	want := []canvasSlot{
		{0, 0, 960, 1080},
		{960, 0, 960, 1080},
	}
	if !reflect.DeepEqual(slots, want) {
		t.Errorf("two landscapes: got %+v, want side-by-side %+v", slots, want)
	}
}

func TestPickLayout_OneUltrawideLandscapePicksStacked(t *testing.T) {
	// A single ultrawide input (e.g. 21:9) at index 0 should prefer the
	// stacked layout because its slot becomes wider. Uses one 21:9 and
	// one 16:9 to break the tie toward stacked.
	sizes := [][2]int{{2560, 1080}, {2560, 1080}}
	slots, _, _ := pickLayout(1920, 1080, sizes, "")
	want := []canvasSlot{
		{0, 0, 1920, 540},
		{0, 540, 1920, 540},
	}
	if !reflect.DeepEqual(slots, want) {
		t.Errorf("two ultrawides: got %+v, want stacked %+v", slots, want)
	}
}

func TestPickLayout_PortraitsPickSideBySide(t *testing.T) {
	// Two 9:16 inputs in n=2 should keep side-by-side because it gives
	// each input more displayed pixels than stacked does.
	sizes := [][2]int{{1080, 1920}, {1080, 1920}}
	slots, _, _ := pickLayout(1920, 1080, sizes, "")
	want := []canvasSlot{
		{0, 0, 960, 1080},
		{960, 0, 960, 1080},
	}
	if !reflect.DeepEqual(slots, want) {
		t.Errorf("two portraits: got %+v, want side-by-side %+v", slots, want)
	}
}

func TestPickLayout_FourLandscapesPick2x2(t *testing.T) {
	sizes := make([][2]int, 4)
	for i := range sizes {
		sizes[i] = [2]int{1920, 1080}
	}
	slots, _, _ := pickLayout(1920, 1080, sizes, "")
	want := []canvasSlot{
		{0, 0, 960, 540},
		{960, 0, 960, 540},
		{0, 540, 960, 540},
		{960, 540, 960, 540},
	}
	if !reflect.DeepEqual(slots, want) {
		t.Errorf("four landscapes: got %+v, want 2x2 %+v", slots, want)
	}
}

func TestPickLayout_FourPortraitsPick1Col(t *testing.T) {
	sizes := make([][2]int, 4)
	for i := range sizes {
		sizes[i] = [2]int{1080, 1920}
	}
	slots, _, _ := pickLayout(1920, 1080, sizes, "")
	want := []canvasSlot{
		{0, 0, 480, 1080},
		{480, 0, 480, 1080},
		{960, 0, 480, 1080},
		{1440, 0, 480, 1080},
	}
	if !reflect.DeepEqual(slots, want) {
		t.Errorf("four portraits: got %+v, want 1-col %+v", slots, want)
	}
}

func TestPickLayout_ThreePortraitsPick3Col(t *testing.T) {
	// Regression: three 9:16 inputs must still pick 3-col after the
	// portrait-aware candidates were added — those candidates assume
	// only one portrait input and waste pixels on landscape slots when
	// fed three portraits.
	sizes := [][2]int{{1080, 1920}, {1080, 1920}, {1080, 1920}}
	slots, _, _ := pickLayout(1920, 1080, sizes, "")
	// 3-col expects thirdW=640 slots; last slot picks up the remainder.
	want := []canvasSlot{
		{0, 0, 640, 1080},
		{640, 0, 640, 1080},
		{1280, 0, 640, 1080},
	}
	if !reflect.DeepEqual(slots, want) {
		t.Errorf("three portraits: got %+v, want 3-col %+v", slots, want)
	}
}

func TestPickLayout_ThreeLandscapesPick2Top1Bottom(t *testing.T) {
	// Regression: three 16:9 inputs must still pick 2-top-1-bottom even
	// after the portrait-aware candidates were added — those candidates
	// score worse for all-landscape inputs because the portrait column
	// wastes pixels on a 16:9 source.
	sizes := [][2]int{{1920, 1080}, {1920, 1080}, {1920, 1080}}
	slots, _, _ := pickLayout(1920, 1080, sizes, "")
	want := []canvasSlot{
		{0, 0, 960, 540},
		{960, 0, 960, 540},
		{480, 540, 960, 540},
	}
	if !reflect.DeepEqual(slots, want) {
		t.Errorf("three landscapes: got %+v, want 2-top-1-bottom %+v", slots, want)
	}
}

func TestPickLayout_TwoLandscapesOnePortraitAtIndex2_PicksAsymPortRight(t *testing.T) {
	// {16:9, 16:9, 9:16}: asym-port-right scores 1,832,832 (88.4%) —
	// big landscape gets a perfect 1312×738 slot, small landscape
	// letterboxes inside 1312×342, portrait fills 608×1080 perfectly.
	sizes := [][2]int{{1920, 1080}, {1920, 1080}, {1080, 1920}}
	slots, _, _ := pickLayout(1920, 1080, sizes, "")
	want := []canvasSlot{
		{0, 0, 1312, 738},
		{0, 738, 1312, 342},
		{1312, 0, 608, 1080},
	}
	if !reflect.DeepEqual(slots, want) {
		t.Errorf("portrait at index 2: got %+v, want asym-port-right %+v", slots, want)
	}
}

func TestPickLayout_TwoLandscapesOnePortraitAtIndex1_PicksAsymPortMiddle(t *testing.T) {
	sizes := [][2]int{{1920, 1080}, {1080, 1920}, {1920, 1080}}
	slots, _, _ := pickLayout(1920, 1080, sizes, "")
	want := []canvasSlot{
		{0, 0, 1312, 738},
		{1312, 0, 608, 1080},
		{0, 738, 1312, 342},
	}
	if !reflect.DeepEqual(slots, want) {
		t.Errorf("portrait at index 1: got %+v, want asym-port-middle %+v", slots, want)
	}
}

func TestPickLayout_TwoLandscapesOnePortraitAtIndex0_PicksAsymPortLeft(t *testing.T) {
	sizes := [][2]int{{1080, 1920}, {1920, 1080}, {1920, 1080}}
	slots, _, _ := pickLayout(1920, 1080, sizes, "")
	want := []canvasSlot{
		{0, 0, 608, 1080},
		{608, 0, 1312, 738},
		{608, 738, 1312, 342},
	}
	if !reflect.DeepEqual(slots, want) {
		t.Errorf("portrait at index 0: got %+v, want asym-port-left %+v", slots, want)
	}
}

func TestPickLayout_TwoUltrawidesOnePortraitPicksStack(t *testing.T) {
	// Asym hurts ultrawide inputs (the small 1312×342 slot can't fit a
	// 21:9 source well), so for {21:9, 21:9, 9:16} the stacked variant
	// (equal landscape rows) outscores asym.
	sizes := [][2]int{{2560, 1080}, {2560, 1080}, {1080, 1920}}
	slots, _, _ := pickLayout(1920, 1080, sizes, "")
	want := []canvasSlot{
		{0, 0, 1312, 540},
		{0, 540, 1312, 540},
		{1312, 0, 608, 1080},
	}
	if !reflect.DeepEqual(slots, want) {
		t.Errorf("ultrawides + portrait: got %+v, want stack-port-right %+v", slots, want)
	}
}

func TestPickLayout_NeutralInputsPickFirstCandidate(t *testing.T) {
	// Zero-size inputs don't bias the solver — all candidates score the
	// same (full slot areas). Ties break in favor of the first candidate
	// returned by candidateLayouts, which is the "default" shape.
	sizes := [][2]int{{0, 0}, {0, 0}}
	slots, _, _ := pickLayout(1920, 1080, sizes, "")
	want := []canvasSlot{
		{0, 0, 960, 1080},
		{960, 0, 960, 1080},
	}
	if !reflect.DeepEqual(slots, want) {
		t.Errorf("neutral inputs n=2: got %+v, want default side-by-side %+v", slots, want)
	}
}

func TestPickLayout_UnsupportedN(t *testing.T) {
	if slots, _, _ := pickLayout(1920, 1080, [][2]int{}, ""); slots != nil {
		t.Errorf("n=0: got %+v, want nil", slots)
	}
	sizes5 := make([][2]int, 5)
	if slots, _, _ := pickLayout(1920, 1080, sizes5, ""); slots != nil {
		t.Errorf("n=5: got %+v, want nil", slots)
	}
}

func TestContainedSize(t *testing.T) {
	tests := []struct {
		name                   string
		slotW, slotH, inW, inH int
		wantW, wantH           int
	}{
		{"exact match", 960, 540, 1920, 1080, 960, 540},
		{"wide input letterbox", 960, 1080, 1920, 1080, 960, 540},
		{"tall input pillarbox", 1920, 540, 1080, 1920, 304, 540},
		{"zero input fills slot", 960, 540, 0, 0, 960, 540},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w, h := containedSize(tt.slotW, tt.slotH, tt.inW, tt.inH)
			if w != tt.wantW || h != tt.wantH {
				t.Errorf("containedSize(%d,%d,%d,%d) = %d×%d, want %d×%d",
					tt.slotW, tt.slotH, tt.inW, tt.inH, w, h, tt.wantW, tt.wantH)
			}
		})
	}
}

func TestComputeCanvasLayout_ContentRects(t *testing.T) {
	canvas := &CanvasConfig{
		Width:         1920,
		Height:        1080,
		SourceStreams: []string{"a", "b"},
	}
	sources := map[string]*StreamSpec{
		"a": specForLayout("1920x1080", 0, nil), // landscape
		"b": specForLayout("1920x1080", 0, nil), // landscape
	}
	layout := ComputeCanvasLayout(canvas, sources)
	if len(layout.Slots) != 2 {
		t.Fatalf("expected 2 slots, got %d", len(layout.Slots))
	}
	// Two 16:9 on a 16:9 canvas → side-by-side (tie-break). Each 960×1080
	// slot letterboxes a 16:9 input to 960×540 content centered vertically.
	for i, s := range layout.Slots {
		if s.SlotW != 960 || s.SlotH != 1080 {
			t.Errorf("slot %d: got %dx%d, want 960x1080", i, s.SlotW, s.SlotH)
		}
		if s.ContentW != 960 || s.ContentH != 540 {
			t.Errorf("slot %d: content %dx%d, want 960x540", i, s.ContentW, s.ContentH)
		}
		if s.ContentX != s.SlotX || s.ContentY != s.SlotY+270 {
			t.Errorf("slot %d: content offset (%d,%d) not centered in slot", i, s.ContentX, s.ContentY)
		}
	}
}

func TestComputeCanvasLayout_RotationOverrideFlipsAR(t *testing.T) {
	ninety := 90
	canvas := &CanvasConfig{
		Width:         1920,
		Height:        1080,
		SourceStreams: []string{"a", "b"},
		SourceOverrides: []CanvasSourceOverride{
			{Rotation: &ninety},
			{Rotation: &ninety},
		},
	}
	sources := map[string]*StreamSpec{
		"a": specForLayout("1920x1080", 0, nil),
		"b": specForLayout("1920x1080", 0, nil),
	}
	layout := ComputeCanvasLayout(canvas, sources)
	// With both inputs rotated 90°, effective AR becomes 9:16 — solver
	// should prefer side-by-side (1080-tall slots) over stacked.
	if len(layout.Slots) != 2 {
		t.Fatalf("expected 2 slots, got %d", len(layout.Slots))
	}
	if layout.Slots[0].SlotW != 960 || layout.Slots[0].SlotH != 1080 {
		t.Errorf("with 90° override expected side-by-side 960x1080 slots, got %+v",
			layout.Slots[0])
	}
	if layout.Slots[0].RotationApplied != 90 {
		t.Errorf("expected RotationApplied=90, got %d", layout.Slots[0].RotationApplied)
	}
	// Content should be letterboxed inside the 960-wide slot: w = 1080 * 9/16 = 607.
	if layout.Slots[0].ContentW >= layout.Slots[0].SlotW {
		t.Errorf("expected letterbox content < slot width, got content=%d slot=%d",
			layout.Slots[0].ContentW, layout.Slots[0].SlotW)
	}
}

func TestComputeCanvasLayout_MissingSourceNeutral(t *testing.T) {
	canvas := &CanvasConfig{
		Width:         1920,
		Height:        1080,
		SourceStreams: []string{"gone", "also-gone"},
	}
	layout := ComputeCanvasLayout(canvas, map[string]*StreamSpec{})
	if len(layout.Slots) != 2 {
		t.Fatalf("expected 2 slots, got %d", len(layout.Slots))
	}
	// Missing sources default to the first candidate (side-by-side).
	if layout.Slots[0].SlotW != 960 {
		t.Errorf("missing sources should get default layout, got %+v", layout.Slots[0])
	}
}

func TestComputeCanvasLayout_LayoutNameOverridesScorer(t *testing.T) {
	// Two 16:9 inputs would normally tie-break to side-by-side. With
	// LayoutName="stacked" the solver must return stacked slots verbatim.
	canvas := &CanvasConfig{
		Width:         1920,
		Height:        1080,
		SourceStreams: []string{"a", "b"},
		LayoutName:    "stacked",
	}
	sources := map[string]*StreamSpec{
		"a": specForLayout("1920x1080", 0, nil),
		"b": specForLayout("1920x1080", 0, nil),
	}
	layout := ComputeCanvasLayout(canvas, sources)
	if layout.ChosenLayout != "stacked" {
		t.Errorf("ChosenLayout: got %q, want %q", layout.ChosenLayout, "stacked")
	}
	wantAvailable := []string{"side-by-side", "stacked"}
	if !reflect.DeepEqual(layout.AvailableLayouts, wantAvailable) {
		t.Errorf("AvailableLayouts: got %v, want %v", layout.AvailableLayouts, wantAvailable)
	}
	if layout.Slots[0].SlotW != 1920 || layout.Slots[0].SlotH != 540 {
		t.Errorf("expected stacked first slot 1920x540, got %dx%d",
			layout.Slots[0].SlotW, layout.Slots[0].SlotH)
	}
	if layout.Slots[1].SlotY != 540 {
		t.Errorf("expected stacked second slot at y=540, got y=%d", layout.Slots[1].SlotY)
	}
}

func TestComputeCanvasLayout_UnknownLayoutNameFallsBackToScorer(t *testing.T) {
	// Pinning a layout name that doesn't exist for n=2 should silently fall
	// back to the scorer (e.g. user removed a source after pinning "2x2").
	canvas := &CanvasConfig{
		Width:         1920,
		Height:        1080,
		SourceStreams: []string{"a", "b"},
		LayoutName:    "2x2", // valid for n=4, not n=2
	}
	sources := map[string]*StreamSpec{
		"a": specForLayout("1920x1080", 0, nil),
		"b": specForLayout("1920x1080", 0, nil),
	}
	layout := ComputeCanvasLayout(canvas, sources)
	if layout.ChosenLayout != "side-by-side" {
		t.Errorf("expected fallback to side-by-side, got ChosenLayout=%q", layout.ChosenLayout)
	}
}
