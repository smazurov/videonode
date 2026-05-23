package streams

import (
	"math"

	"github.com/smazurov/videonode/internal/ffmpeg"
)

// canvasSlot is a rectangular region on the canvas where a single source is drawn.
type canvasSlot struct {
	X, Y, W, H int
}

// CanvasLayout is the resolved slot + content geometry for a canvas in pixel coordinates.
type CanvasLayout struct {
	Slots            []CanvasLayoutSlot
	ChosenLayout     string   // empty only when canvas has no sources
	AvailableLayouts []string // ordered candidate names for the current source count
}

// CanvasLayoutSlot describes one source's placement on the canvas.
type CanvasLayoutSlot struct {
	SourceStreamID                         string
	SlotX, SlotY, SlotW, SlotH             int // region allotted by the layout solver
	ContentX, ContentY, ContentW, ContentH int // contain-fit rect inside the slot
	EffectiveAspectRatio                   float64
	RotationApplied                        int
}

// layoutCandidate is a named slot arrangement for a given n.
type layoutCandidate struct {
	name  string
	slots []canvasSlot
}

// candidateLayouts returns slot arrangements for n sources; first is the default for ties.
func candidateLayouts(canvasW, canvasH, n int) []layoutCandidate {
	halfW := canvasW / 2
	halfH := canvasH / 2
	quarterW := canvasW / 4
	quarterH := canvasH / 4
	thirdW := canvasW / 3

	switch n {
	case 1:
		return []layoutCandidate{{
			name:  "full",
			slots: []canvasSlot{{0, 0, canvasW, canvasH}},
		}}
	case 2:
		return []layoutCandidate{
			{
				name: "side-by-side",
				slots: []canvasSlot{
					{0, 0, halfW, canvasH},
					{halfW, 0, halfW, canvasH},
				},
			},
			{
				name: "stacked",
				slots: []canvasSlot{
					{0, 0, canvasW, halfH},
					{0, halfH, canvasW, halfH},
				},
			},
		}
	case 3:
		// Portrait-friendly: 9:16 column + two landscape slots; only "wins" with one 9:16 input.
		portW := (canvasH*9 + 8) / 16
		landW := canvasW - portW
		bigH := (landW*9 + 8) / 16
		smallH := canvasH - bigH

		return []layoutCandidate{
			{
				name: "2-top-1-bottom",
				slots: []canvasSlot{
					{0, 0, halfW, halfH},
					{halfW, 0, halfW, halfH},
					{quarterW, halfH, halfW, halfH},
				},
			},
			{
				name: "3-col",
				slots: []canvasSlot{
					{0, 0, thirdW, canvasH},
					{thirdW, 0, thirdW, canvasH},
					{2 * thirdW, 0, canvasW - 2*thirdW, canvasH},
				},
			},
			{
				name: "stack-port-right",
				slots: []canvasSlot{
					{0, 0, landW, halfH},
					{0, halfH, landW, canvasH - halfH},
					{landW, 0, portW, canvasH},
				},
			},
			{
				name: "stack-port-middle",
				slots: []canvasSlot{
					{0, 0, landW, halfH},
					{landW, 0, portW, canvasH},
					{0, halfH, landW, canvasH - halfH},
				},
			},
			{
				name: "stack-port-left",
				slots: []canvasSlot{
					{0, 0, portW, canvasH},
					{portW, 0, landW, halfH},
					{portW, halfH, landW, canvasH - halfH},
				},
			},
			{
				name: "asym-port-right",
				slots: []canvasSlot{
					{0, 0, landW, bigH},
					{0, bigH, landW, smallH},
					{landW, 0, portW, canvasH},
				},
			},
			{
				name: "asym-port-middle",
				slots: []canvasSlot{
					{0, 0, landW, bigH},
					{landW, 0, portW, canvasH},
					{0, bigH, landW, smallH},
				},
			},
			{
				name: "asym-port-left",
				slots: []canvasSlot{
					{0, 0, portW, canvasH},
					{portW, 0, landW, bigH},
					{portW, bigH, landW, smallH},
				},
			},
		}
	case 4:
		return []layoutCandidate{
			{
				name: "2x2",
				slots: []canvasSlot{
					{0, 0, halfW, halfH},
					{halfW, 0, halfW, halfH},
					{0, halfH, halfW, halfH},
					{halfW, halfH, halfW, halfH},
				},
			},
			{
				name: "1-row",
				slots: []canvasSlot{
					{0, 0, canvasW, quarterH},
					{0, quarterH, canvasW, quarterH},
					{0, 2 * quarterH, canvasW, quarterH},
					{0, 3 * quarterH, canvasW, canvasH - 3*quarterH},
				},
			},
			{
				name: "1-col",
				slots: []canvasSlot{
					{0, 0, quarterW, canvasH},
					{quarterW, 0, quarterW, canvasH},
					{2 * quarterW, 0, quarterW, canvasH},
					{3 * quarterW, 0, canvasW - 3*quarterW, canvasH},
				},
			},
		}
	}
	return nil
}

// containedSize returns the largest AR-preserving rect of inputW:inputH fitting in slotW×slotH.
// Unknown AR (zero input dim) returns the slot itself, neutral for placeholders.
func containedSize(slotW, slotH, inputW, inputH int) (int, int) {
	if inputW <= 0 || inputH <= 0 || slotW <= 0 || slotH <= 0 {
		return slotW, slotH
	}
	sx := float64(slotW) / float64(inputW)
	sy := float64(slotH) / float64(inputH)
	s := sx
	if sy < sx {
		s = sy
	}
	w := int(math.Round(float64(inputW) * s))
	h := int(math.Round(float64(inputH) * s))
	if w > slotW {
		w = slotW
	}
	if h > slotH {
		h = slotH
	}
	return w, h
}

// pickLayout maximizes total displayed area; input[i] → slot[i] (no permutation).
// Override bypasses scoring when it matches a candidate name; unknown overrides are ignored.
func pickLayout(canvasW, canvasH int, inputSizes [][2]int, override string) ([]canvasSlot, string, []string) {
	n := len(inputSizes)
	candidates := candidateLayouts(canvasW, canvasH, n)
	if len(candidates) == 0 {
		return nil, "", nil
	}

	available := make([]string, len(candidates))
	for i := range candidates {
		available[i] = candidates[i].name
	}

	if override != "" {
		for i := range candidates {
			if candidates[i].name == override {
				return candidates[i].slots, candidates[i].name, available
			}
		}
	}

	var best *layoutCandidate
	var bestScore int64 = -1
	for i := range candidates {
		c := &candidates[i]
		var score int64
		for j, slot := range c.slots {
			iw, ih := 0, 0
			if j < len(inputSizes) {
				iw, ih = inputSizes[j][0], inputSizes[j][1]
			}
			cw, ch := containedSize(slot.W, slot.H, iw, ih)
			score += int64(cw) * int64(ch)
		}
		if score > bestScore {
			best = c
			bestScore = score
		}
	}
	return best.slots, best.name, available
}

// resolveRotation returns rotation for source index i, honoring canvas-item overrides.
func resolveRotation(canvas *CanvasConfig, src *StreamSpec, i int) int {
	base := 0
	if src != nil {
		base = src.FFmpeg.Rotation
	}
	if i < 0 || i >= len(canvas.SourceOverrides) {
		return base
	}
	if canvas.SourceOverrides[i].Rotation != nil {
		return *canvas.SourceOverrides[i].Rotation
	}
	return base
}

// effectiveSizeForSource returns natural dims after rotation; (0,0)
// when unknown. Perspective doesn't change effective layout dims —
// the warp lives inside the composer, the bounding rect is unchanged.
// Inlined here after the legacy ffmpeg.EffectiveInputSize helper was
// removed with composite.go.
func effectiveSizeForSource(canvas *CanvasConfig, src *StreamSpec, i int) (int, int) {
	if src == nil {
		return 0, 0
	}
	w, h, err := ffmpeg.ParseResolution(src.FFmpeg.Resolution)
	if err != nil {
		return 0, 0
	}
	rot := resolveRotation(canvas, src, i)
	if rot == 90 || rot == 270 {
		return h, w
	}
	return w, h
}

// ComputeCanvasLayout resolves slots and content rects for a canvas spec. Pure function.
func ComputeCanvasLayout(canvas *CanvasConfig, sources map[string]*StreamSpec) CanvasLayout {
	if canvas == nil || len(canvas.SourceStreams) == 0 {
		return CanvasLayout{}
	}

	n := len(canvas.SourceStreams)
	sizes := make([][2]int, n)
	rotations := make([]int, n)
	for i, id := range canvas.SourceStreams {
		src := sources[id]
		w, h := effectiveSizeForSource(canvas, src, i)
		sizes[i] = [2]int{w, h}
		rotations[i] = resolveRotation(canvas, src, i)
	}

	slots, chosen, available := pickLayout(canvas.Width, canvas.Height, sizes, canvas.LayoutName)
	if slots == nil {
		return CanvasLayout{}
	}

	out := CanvasLayout{
		Slots:            make([]CanvasLayoutSlot, n),
		ChosenLayout:     chosen,
		AvailableLayouts: available,
	}
	for i := range n {
		slot := slots[i]
		iw, ih := sizes[i][0], sizes[i][1]
		cw, ch := containedSize(slot.W, slot.H, iw, ih)
		cx := slot.X + (slot.W-cw)/2
		cy := slot.Y + (slot.H-ch)/2

		var ar float64
		if ih > 0 {
			ar = float64(iw) / float64(ih)
		}

		out.Slots[i] = CanvasLayoutSlot{
			SourceStreamID:       canvas.SourceStreams[i],
			SlotX:                slot.X,
			SlotY:                slot.Y,
			SlotW:                slot.W,
			SlotH:                slot.H,
			ContentX:             cx,
			ContentY:             cy,
			ContentW:             cw,
			ContentH:             ch,
			EffectiveAspectRatio: ar,
			RotationApplied:      rotations[i],
		}
	}
	return out
}
