//go:build planv2_tests

// Package pipeline planv2 stubs: package-local stubs of the canonical
// Source/Composer/Stream types from the plan at
// /home/stepan/.claude/plans/plan-a-full-rewrite-linked-gray.md.
// These exist so B12 tests compile under the planv2_tests tag before
// foundation units B1/B2/B9 land. Integrator deduplicates against the
// real types in source.go/composer.go/stream.go once those land.
//
// Build with: go test -tags=planv2_tests ./...
package pipeline

import "time"

// PlanSource is the canonical [[sources]] entry. Mirrors the plan's
// Source type verbatim; named PlanSource here to avoid collision with
// any pre-existing Source decl in this worktree.
type PlanSource struct {
	ID        string    `toml:"id" json:"id"`
	Device    string    `toml:"device,omitempty" json:"device,omitempty"`
	TestMode  bool      `toml:"test_mode,omitempty" json:"test_mode,omitempty"`
	CreatedAt time.Time `toml:"created_at" json:"created_at"`
	UpdatedAt time.Time `toml:"updated_at" json:"updated_at"`
}

// PlanComposer mirrors the canonical Composer.
type PlanComposer struct {
	ID        string              `toml:"id" json:"id"`
	Canvas    PlanCanvasDims      `toml:"canvas" json:"canvas"`
	Inputs    []PlanComposerInput `toml:"inputs" json:"inputs"`
	Layout    []PlanLayoutSlot    `toml:"layout" json:"layout"`
	CreatedAt time.Time           `toml:"created_at" json:"created_at"`
	UpdatedAt time.Time           `toml:"updated_at" json:"updated_at"`
}

// PlanCanvasDims mirrors the canonical CanvasDims.
type PlanCanvasDims struct {
	W int `toml:"w" json:"w"`
	H int `toml:"h" json:"h"`
}

// PlanComposerInput mirrors the canonical ComposerInput.
type PlanComposerInput struct {
	Ref    string      `toml:"ref" json:"ref"`
	Effect *PlanEffect `toml:"effect,omitempty" json:"effect,omitempty"`
}

// PlanLayoutSlot mirrors the canonical LayoutSlot.
type PlanLayoutSlot struct {
	Input string `toml:"input" json:"input"`
	X     int    `toml:"x" json:"x"`
	Y     int    `toml:"y" json:"y"`
	W     int    `toml:"w" json:"w"`
	H     int    `toml:"h" json:"h"`
}

// PlanEffect mirrors the canonical Effect.
type PlanEffect struct {
	Type    string    `toml:"type" json:"type"`
	Corners [4][2]int `toml:"corners,omitempty" json:"corners,omitempty"`
}

// PlanStream is the canonical slim Stream — no Inputs/Layout/Effects/
// ForceComposer/TestMode (TestMode moves to Source).
type PlanStream struct {
	ID                string          `toml:"id" json:"id"`
	Name              string          `toml:"name" json:"name"`
	Upstream          string          `toml:"upstream" json:"upstream"`
	Audio             AudioConfig     `toml:"audio,omitempty" json:"audio,omitzero"`
	Encoder           EncoderConfig   `toml:"encoder,omitempty" json:"encoder,omitzero"`
	Publish           []PublishTarget `toml:"publish,omitempty" json:"publish,omitempty"`
	CustomEncoderArgs string          `toml:"custom_encoder_args,omitempty" json:"custom_encoder_args,omitempty"`
	CreatedAt         time.Time       `toml:"created_at" json:"created_at"`
	UpdatedAt         time.Time       `toml:"updated_at" json:"updated_at"`
}

// ParseUpstreamRef splits an upstream string of the form "source:<id>"
// or "composer:<id>" into kind and id. Returns ("", "", false) on
// malformed input. Mirrors what buildEncoder will do post-B1.
func ParseUpstreamRef(ref string) (kind, id string, ok bool) {
	for i := 0; i < len(ref); i++ {
		if ref[i] == ':' {
			return ref[:i], ref[i+1:], true
		}
	}
	return "", "", false
}
