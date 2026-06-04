// Package sensors wires AI perception sidecars (videonode-sensor) into
// the daemon: it reconciles an analysis-composer tap + sensor process from
// an auto_crop input effect, subscribes to the sensor's findings, and maps
// those findings to a crop on the display composer via the follow/commit
// policy. The bbox→crop conversion and the policy are pure and live here;
// the process/gRPC plumbing is in reconcile.go.
package sensors

import (
	"fmt"
	"math"
)

func bboxStr(b *BBox) string {
	if b == nil {
		return "none"
	}
	return fmt.Sprintf("[%.3f,%.3f,%.3f,%.3f]", b.X, b.Y, b.W, b.H)
}

func cropStr(c Crop) string {
	return fmt.Sprintf("[x=%.3f,y=%.3f,scale=%.2f]", c.X, c.Y, c.Scale)
}

// BBox is a normalized [0,1] axis-aligned region — the playfield as the
// detector reports it, over the analysis frame (which, by the tap invariant,
// is a faithful full-frame view of the source).
type BBox struct {
	X float64
	Y float64
	W float64
	H float64
}

// Crop is the composer's pan+zoom crop parameterization (aspect_ratio_mode=
// crop): a normalized pan center (X,Y) and a uniform overfill Scale (>=1).
// The visible window's aspect ratio is the display slot's; Scale zooms so the
// larger bbox dimension fills the frame, which preserves content AR (the
// source is never anisotropically stretched) while framing the playfield.
type Crop struct {
	X     float64
	Y     float64
	Scale float64
}

// WideCrop is the safe full-frame fallback used when confidence is low
// (occlusion, glare) — a hand over the field zooms out, never mis-crops.
var WideCrop = Crop{X: 0.5, Y: 0.5, Scale: 1.0}

const minVisibleFraction = 0.05

// BBoxToCrop frames the bbox: pan to its center and zoom so the larger
// dimension (plus margin bleed) fills the frame. The margin is a fractional
// bleed (0.1 = 10% padding) so legitimate content at the edges is never
// clipped. It assumes the display slot AR matches the source AR (the common
// single-source passthrough case); arbitrary-AR tight crop is the deferred
// raw-rect mode.
func BBoxToCrop(b BBox, margin float64) Crop {
	if margin < 0 {
		margin = 0
	}
	cx := clamp01(b.X + b.W/2)
	cy := clamp01(b.Y + b.H/2)
	visible := math.Max(b.W, b.H) * (1 + margin)
	visible = clamp(visible, minVisibleFraction, 1.0)
	return Crop{X: cx, Y: cy, Scale: 1.0 / visible}
}

// Dist is the max-component distance between two crops in normalized units
// (pan delta and a scale-derived zoom delta), used by the commit policy's
// dead-band. Both terms are roughly in [0,1].
func (c Crop) Dist(o Crop) float64 {
	dx := math.Abs(c.X - o.X)
	dy := math.Abs(c.Y - o.Y)
	var ds float64
	if c.Scale > 0 && o.Scale > 0 {
		ds = math.Abs(1.0/c.Scale - 1.0/o.Scale)
	}
	return math.Max(math.Max(dx, dy), ds)
}

func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func clamp01(v float64) float64 { return clamp(v, 0, 1) }
