package sensors

// Committer is the per-input follow/commit policy that turns a stream of
// detections into a stable committed crop. It never chases per-frame noise:
// a new crop is committed only when the detector is confident AND the move is
// large (dead-band) AND it persists (hysteresis). Low-confidence detections
// (occlusion) widen to the safe full frame after a grace period, then
// re-tighten when a confident, stable detection returns.
//
// Committer is not safe for concurrent use; the BindingRouter owns one per
// input and feeds it from a single goroutine.
type Committer struct {
	MinConfidence float64 // detections below this don't tighten the crop
	Margin        float64 // bleed passed to BBoxToCrop
	MoveThreshold float64 // Crop.Dist dead-band before a re-commit is considered
	StableFrames  int     // consecutive consistent candidates before committing
	WidenFrames   int     // consecutive low-confidence frames before widening

	committed     *Crop
	pending       Crop
	pendingActive bool
	pendingCount  int
	lowCount      int
}

// DefaultCommitter returns a Committer tuned for a slow periodic-re-detect
// playfield: ~10% bleed, a 5% dead-band, three consistent detections to
// move, three low-confidence frames to widen.
func DefaultCommitter() *Committer {
	return &Committer{
		MinConfidence: 0.8,
		Margin:        0.10,
		MoveThreshold: 0.05,
		StableFrames:  3,
		WidenFrames:   3,
	}
}

// Observe feeds one detection. It returns the crop to apply and whether it
// changed from the currently committed crop (callers apply only on change,
// easing toward it downstream). The bbox is ignored when confident is false.
func (c *Committer) Observe(b BBox, confidence float64) (Crop, bool) {
	if confidence < c.MinConfidence {
		return c.observeLow()
	}
	c.lowCount = 0
	return c.observeConfident(BBoxToCrop(b, c.Margin))
}

func (c *Committer) observeLow() (Crop, bool) {
	c.pendingActive = false
	c.pendingCount = 0
	c.lowCount++
	if c.lowCount < c.WidenFrames {
		return c.current()
	}
	if c.committed != nil && *c.committed == WideCrop {
		return WideCrop, false
	}
	w := WideCrop
	c.committed = &w
	return w, true
}

func (c *Committer) observeConfident(cand Crop) (Crop, bool) {
	if c.committed == nil {
		c.committed = &cand
		return cand, true
	}
	if c.committed.Dist(cand) <= c.MoveThreshold {
		c.pendingActive = false
		c.pendingCount = 0
		return *c.committed, false
	}
	if !c.pendingActive || c.pending.Dist(cand) > c.MoveThreshold {
		c.pending = cand
		c.pendingActive = true
		c.pendingCount = 1
	} else {
		c.pendingCount++
	}
	if c.pendingCount < c.StableFrames {
		return *c.committed, false
	}
	c.committed = &cand
	c.pendingActive = false
	c.pendingCount = 0
	return cand, true
}

func (c *Committer) current() (Crop, bool) {
	if c.committed == nil {
		return WideCrop, false
	}
	return *c.committed, false
}
