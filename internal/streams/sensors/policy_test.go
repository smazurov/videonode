package sensors

import "testing"

func newTestCommitter() *Committer {
	return &Committer{
		MinConfidence: 0.8,
		Margin:        0,
		MoveThreshold: 0.05,
		StableFrames:  3,
		WidenFrames:   2,
	}
}

func TestCommitterFirstConfidentCommits(t *testing.T) {
	c := newTestCommitter()
	crop, changed := c.Observe(BBox{0.25, 0.25, 0.5, 0.5}, 0.9)
	if !changed {
		t.Fatal("first confident detection must commit")
	}
	if !approx(crop.X, 0.5) || !approx(crop.Y, 0.5) {
		t.Errorf("committed center (%v,%v), want (0.5,0.5)", crop.X, crop.Y)
	}
}

func TestCommitterDeadBandHoldsSmallMoves(t *testing.T) {
	c := newTestCommitter()
	c.Observe(BBox{0.25, 0.25, 0.5, 0.5}, 0.9) // commit center 0.5
	// A tiny pan (well within MoveThreshold) must not re-commit.
	for i := range 5 {
		_, changed := c.Observe(BBox{0.26, 0.25, 0.5, 0.5}, 0.9)
		if changed {
			t.Fatalf("small move re-committed on iter %d", i)
		}
	}
}

func TestCommitterLargeSustainedMoveCommits(t *testing.T) {
	c := newTestCommitter()
	c.Observe(BBox{0.0, 0.0, 0.3, 0.3}, 0.9) // commit near top-left
	big := BBox{0.6, 0.6, 0.3, 0.3}
	var changed bool
	for i := 0; i < c.StableFrames; i++ {
		_, changed = c.Observe(big, 0.9)
	}
	if !changed {
		t.Fatal("large sustained move must commit after StableFrames")
	}
}

func TestCommitterUnstableMoveDoesNotCommit(t *testing.T) {
	c := newTestCommitter()
	c.Observe(BBox{0.0, 0.0, 0.3, 0.3}, 0.9)
	// Alternating distinct candidates never persist StableFrames in a row.
	for i := range 6 {
		var b BBox
		if i%2 == 0 {
			b = BBox{0.6, 0.6, 0.3, 0.3}
		} else {
			b = BBox{0.1, 0.7, 0.3, 0.3}
		}
		if _, changed := c.Observe(b, 0.9); changed {
			t.Fatalf("unstable candidate committed on iter %d", i)
		}
	}
}

func TestCommitterWidensOnSustainedLowConfidence(t *testing.T) {
	c := newTestCommitter()
	c.Observe(BBox{0.25, 0.25, 0.3, 0.3}, 0.9) // tight commit
	// First low-confidence frame: grace, no widen yet.
	if _, changed := c.Observe(BBox{}, 0.1); changed {
		t.Fatal("widened on first low-confidence frame (grace expected)")
	}
	// Second crosses WidenFrames=2 → widen.
	crop, changed := c.Observe(BBox{}, 0.1)
	if !changed {
		t.Fatal("must widen after WidenFrames low-confidence frames")
	}
	if crop != WideCrop {
		t.Errorf("widen crop = %+v, want %+v", crop, WideCrop)
	}
	// Staying low must not re-emit a change.
	if _, ch := c.Observe(BBox{}, 0.1); ch {
		t.Fatal("re-emitted widen while already wide")
	}
}
