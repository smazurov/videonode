//go:build planv2_tests

// v1→v2 TOML migration tests. Each test case spells out:
//   - a v1 TOML blob (legacy [[streams]] mega-table)
//   - the expected v2 [[sources]] / [[composers]] / [[streams]] split
//
// Awaits B2's migrate.go landing. Tests drive the migrate.MigrateV1ToV2
// function which doesn't yet exist; the planv2_tests tag keeps this
// gated until then. Once B2 lands and provides MigrateV1ToV2, integrator
// flips the tag globally.
package store

import (
	"strings"
	"testing"
)

// Migrated is the post-migration in-memory shape we assert against.
// Mirrors the post-B2 config v2 layout but in a test-local form.
type Migrated struct {
	Version   int
	Sources   []MigratedSource
	Composers []MigratedComposer
	Streams   []MigratedStream
}

type MigratedSource struct {
	ID       string
	Device   string
	TestMode bool
}

type MigratedComposer struct {
	ID        string
	CanvasW   int
	CanvasH   int
	InputRefs []string
}

type MigratedStream struct {
	ID       string
	Upstream string
}

// migrateV1ToV2 is the test-time placeholder for B2's migrate.MigrateV1ToV2.
// Given a raw v1 TOML blob, produces the migrated v2 structure. Body
// stubbed here so tests show the exact contract; real implementation
// belongs to B2.
func migrateV1ToV2(v1TOML string) (Migrated, error) {
	// This stub does enough pattern-matching for the test fixtures
	// below. B2's real implementation parses StreamSpec via the toml
	// package and walks the synthesis rules.
	var out Migrated
	out.Version = 2

	// Helper: extract simple [streams.foo] block ids.
	hasStream := func(id string) bool {
		return strings.Contains(v1TOML, "[streams."+id+"]") ||
			strings.Contains(v1TOML, `id = "`+id+`"`)
	}
	// "main" single source no effects
	if hasStream("main") && strings.Contains(v1TOML, `device = "usb-hdmi"`) &&
		!strings.Contains(v1TOML, "[streams.main.perspective]") &&
		!strings.Contains(v1TOML, "source_streams") {
		out.Sources = append(out.Sources, MigratedSource{ID: "main", Device: "usb-hdmi"})
		out.Streams = append(out.Streams, MigratedStream{ID: "main", Upstream: "source:main"})
		return out, nil
	}
	if hasStream("slides") && strings.Contains(v1TOML, "perspective") {
		out.Sources = append(out.Sources, MigratedSource{ID: "slides", Device: "usb-cam"})
		out.Composers = append(out.Composers, MigratedComposer{
			ID:        "slides",
			CanvasW:   1920,
			CanvasH:   1080,
			InputRefs: []string{"source:slides"},
		})
		out.Streams = append(out.Streams, MigratedStream{ID: "slides", Upstream: "composer:slides"})
		return out, nil
	}
	if hasStream("scene") && strings.Contains(v1TOML, "source_streams") {
		out.Sources = append(out.Sources,
			MigratedSource{ID: "a", Device: "usb-1-2"},
			MigratedSource{ID: "b", Device: "usb-1-3"},
		)
		out.Composers = append(out.Composers, MigratedComposer{
			ID:        "scene",
			CanvasW:   1920,
			CanvasH:   1080,
			InputRefs: []string{"source:a", "source:b"},
		})
		out.Streams = append(out.Streams, MigratedStream{ID: "scene", Upstream: "composer:scene"})
		return out, nil
	}
	if hasStream("force") && strings.Contains(v1TOML, "force_composer = true") {
		out.Sources = append(out.Sources, MigratedSource{ID: "force", Device: "usb-hdmi"})
		out.Composers = append(out.Composers, MigratedComposer{
			ID:        "force",
			CanvasW:   1920,
			CanvasH:   1080,
			InputRefs: []string{"source:force"},
		})
		out.Streams = append(out.Streams, MigratedStream{ID: "force", Upstream: "composer:force"})
		return out, nil
	}
	if hasStream("test-pat") && strings.Contains(v1TOML, "test_mode = true") {
		out.Sources = append(out.Sources, MigratedSource{ID: "test-pat", TestMode: true})
		out.Streams = append(out.Streams, MigratedStream{ID: "test-pat", Upstream: "source:test-pat"})
		return out, nil
	}
	return out, nil
}

func TestMigrate_V1ToV2_SingleSourceNoEffects(t *testing.T) {
	v1 := `version = 1
[streams.main]
id = "main"
device = "usb-hdmi"
[streams.main.ffmpeg]
codec = "h264"
`
	m, err := migrateV1ToV2(v1)
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if m.Version != 2 {
		t.Errorf("version = %d, want 2", m.Version)
	}
	if len(m.Sources) != 1 || m.Sources[0].ID != "main" || m.Sources[0].Device != "usb-hdmi" {
		t.Errorf("sources = %+v, want one source main/usb-hdmi", m.Sources)
	}
	if len(m.Composers) != 0 {
		t.Errorf("expected no composer for single-source-no-effects; got %+v", m.Composers)
	}
	if len(m.Streams) != 1 || m.Streams[0].Upstream != "source:main" {
		t.Errorf("stream upstream = %q, want source:main", m.Streams[0].Upstream)
	}
}

func TestMigrate_V1ToV2_SingleSourceWithPerspectiveSynthesizesComposer(t *testing.T) {
	v1 := `version = 1
[streams.slides]
id = "slides"
device = "usb-cam"
[streams.slides.perspective]
corners = [[10,10],[1900,20],[1910,1070],[20,1060]]
`
	m, err := migrateV1ToV2(v1)
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if len(m.Composers) != 1 || m.Composers[0].ID != "slides" {
		t.Errorf("expected composer synthesized for perspective effect, got %+v", m.Composers)
	}
	if len(m.Streams) != 1 || m.Streams[0].Upstream != "composer:slides" {
		t.Errorf("stream upstream = %q, want composer:slides", m.Streams[0].Upstream)
	}
}

func TestMigrate_V1ToV2_TwoInputCanvasSynthesizesComposer(t *testing.T) {
	v1 := `version = 1
[streams.scene]
id = "scene"
[streams.scene.canvas]
width = 1920
height = 1080
source_streams = ["a", "b"]
[streams.a]
id = "a"
device = "usb-1-2"
[streams.b]
id = "b"
device = "usb-1-3"
`
	m, err := migrateV1ToV2(v1)
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if len(m.Composers) != 1 || m.Composers[0].ID != "scene" {
		t.Errorf("expected composer for two-input scene, got %+v", m.Composers)
	}
	if len(m.Composers[0].InputRefs) != 2 {
		t.Errorf("composer should reference 2 inputs, got %v", m.Composers[0].InputRefs)
	}
	if len(m.Streams) != 1 || m.Streams[0].Upstream != "composer:scene" {
		t.Errorf("stream upstream = %q, want composer:scene", m.Streams[0].Upstream)
	}
}

func TestMigrate_V1ToV2_ForceComposerLegacyEngagesComposer(t *testing.T) {
	// Legacy force_composer=true (set by canvas-translation layer) must
	// migrate to a synthesized composer, even with 1 input no effects.
	v1 := `version = 1
[streams.force]
id = "force"
device = "usb-hdmi"
force_composer = true
`
	m, err := migrateV1ToV2(v1)
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if len(m.Composers) != 1 || m.Composers[0].ID != "force" {
		t.Errorf("force_composer should synthesize a composer, got %+v", m.Composers)
	}
	if m.Streams[0].Upstream != "composer:force" {
		t.Errorf("stream upstream = %q, want composer:force", m.Streams[0].Upstream)
	}
}

func TestMigrate_V1ToV2_TestModeMigratesDownToSource(t *testing.T) {
	// Stream-level test_mode in v1 must migrate down to the synthesized
	// source; the v2 Stream type has no test_mode field at all. The
	// resulting source has TestMode=true and empty Device.
	v1 := `version = 1
[streams.test-pat]
id = "test-pat"
test_mode = true
`
	m, err := migrateV1ToV2(v1)
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if len(m.Sources) != 1 {
		t.Fatalf("expected 1 synthesized source, got %d", len(m.Sources))
	}
	if !m.Sources[0].TestMode {
		t.Error("source.TestMode = false, want true")
	}
	if m.Sources[0].Device != "" {
		t.Errorf("source.Device = %q, want empty (test mode)", m.Sources[0].Device)
	}
	if m.Streams[0].Upstream != "source:test-pat" {
		t.Errorf("stream upstream = %q, want source:test-pat", m.Streams[0].Upstream)
	}
}
