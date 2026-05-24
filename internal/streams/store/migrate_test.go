package store

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestMigrate_V1Intermediate covers the [[streams]] array shape with
// inputs/effects/layout/force_composer. Each subtest writes a v1 fixture,
// calls Load (which migrates + rewrites), and asserts the resulting v2
// entities.
func TestMigrate_V1Intermediate(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantSrc  int
		wantComp int
		wantStr  int
		check    func(t *testing.T, s *tomlStore)
	}{
		{
			name: "single-source no-effects: 1 source + 1 stream, no composer",
			input: `version = 1

[[streams]]
id = "cam-front"
name = "Front camera"
test_mode = false

  [[streams.inputs]]
  id = "inp1"
  device = "usb-1-2"

  [streams.encoder]
  codec = "h264"
  bitrate = "4M"
`,
			wantSrc:  1,
			wantComp: 0,
			wantStr:  1,
			check: func(t *testing.T, s *tomlStore) {
				if got := s.config.Sources[0].Device; got != "usb-1-2" {
					t.Errorf("source device = %q want %q", got, "usb-1-2")
				}
				if s.config.Sources[0].TestMode {
					t.Error("source.TestMode should be false")
				}
				up := s.config.Streams[0].Upstream
				if !strings.HasPrefix(up, "source:") {
					t.Errorf("stream.Upstream = %q want source:* prefix", up)
				}
				if s.config.Streams[0].Encoder.Codec != "h264" {
					t.Errorf("encoder codec = %q want h264", s.config.Streams[0].Encoder.Codec)
				}
			},
		},
		{
			name: "single-source with perspective: 1 source + 1 composer + 1 stream",
			input: `version = 1

[[streams]]
id = "desk-keystone"
name = "Desk camera"
test_mode = false

  [[streams.inputs]]
  id = "inp1"
  device = "usb-3-1"

  [[streams.effects.inp1]]
  type = "perspective"
  corners = [[120, 80], [1820, 60], [1900, 1020], [60, 1000]]

  [streams.encoder]
  codec = "h264"
`,
			wantSrc:  1,
			wantComp: 1,
			wantStr:  1,
			check: func(t *testing.T, s *tomlStore) {
				comp := s.config.Composers[0]
				if len(comp.Inputs) != 1 {
					t.Fatalf("composer.Inputs len = %d want 1", len(comp.Inputs))
				}
				if comp.Inputs[0].Effect == nil {
					t.Fatal("composer.Inputs[0].Effect should be set")
				}
				if comp.Inputs[0].Effect.Type != "perspective" {
					t.Errorf("effect.Type = %q want perspective", comp.Inputs[0].Effect.Type)
				}
				if comp.Inputs[0].Effect.Corners[0] != [2]int{120, 80} {
					t.Errorf("effect.Corners[0] = %v want [120 80]", comp.Inputs[0].Effect.Corners[0])
				}
				up := s.config.Streams[0].Upstream
				if !strings.HasPrefix(up, "composer:") {
					t.Errorf("stream.Upstream = %q want composer:* prefix", up)
				}
			},
		},
		{
			name: "two-input canvas: 2 sources + 1 composer + 1 stream",
			input: `version = 1

[[streams]]
id = "main-scene"
name = "Main scene"

  [[streams.inputs]]
  id = "slides"
  device = "hdmi-rx"

  [[streams.inputs]]
  id = "cam"
  device = "usb-1-2"

  [[streams.layout]]
  slot = 0
  x = 0
  y = 0
  w = 1920
  h = 1080

  [[streams.layout]]
  slot = 1
  x = 20
  y = 740
  w = 320
  h = 180

  [streams.encoder]
  codec = "h265"
`,
			wantSrc:  2,
			wantComp: 1,
			wantStr:  1,
			check: func(t *testing.T, s *tomlStore) {
				comp := s.config.Composers[0]
				if len(comp.Inputs) != 2 {
					t.Errorf("composer.Inputs len = %d want 2", len(comp.Inputs))
				}
				if len(comp.Layout) != 2 {
					t.Errorf("composer.Layout len = %d want 2", len(comp.Layout))
				}
				// Layout addresses input by ref, not positional.
				if comp.Layout[0].Input == "" || !strings.HasPrefix(comp.Layout[0].Input, "source:") {
					t.Errorf("layout[0].Input = %q want source:* prefix", comp.Layout[0].Input)
				}
				// Canvas dims derive from layout bbox.
				if comp.Canvas.W != 1920 || comp.Canvas.H != 1080 {
					t.Errorf("canvas dims = %dx%d want 1920x1080", comp.Canvas.W, comp.Canvas.H)
				}
				up := s.config.Streams[0].Upstream
				if !strings.HasPrefix(up, "composer:") {
					t.Errorf("stream.Upstream = %q want composer:*", up)
				}
			},
		},
		{
			name: "force_composer=true: composer synthesized at N=1 no-effects",
			input: `version = 1

[[streams]]
id = "forced"
name = "Forced composer"
force_composer = true

  [[streams.inputs]]
  id = "inp1"
  device = "usb-9-9"

  [streams.encoder]
  codec = "h264"
`,
			wantSrc:  1,
			wantComp: 1,
			wantStr:  1,
			check: func(t *testing.T, s *tomlStore) {
				up := s.config.Streams[0].Upstream
				if !strings.HasPrefix(up, "composer:") {
					t.Errorf("stream.Upstream = %q want composer:* (force_composer should engage composer)", up)
				}
			},
		},
		{
			name: "stream-level test_mode: source-level test_mode, empty device",
			input: `version = 1

[[streams]]
id = "test-pattern-stream"
name = "Test pattern"
test_mode = true

  [[streams.inputs]]
  id = "inp1"
  device = ""

  [streams.encoder]
  codec = "h264"
`,
			wantSrc:  1,
			wantComp: 0,
			wantStr:  1,
			check: func(t *testing.T, s *tomlStore) {
				src := s.config.Sources[0]
				if !src.TestMode {
					t.Error("source.TestMode should be true after migration")
				}
				if src.Device != "" {
					t.Errorf("source.Device = %q want empty", src.Device)
				}
				up := s.config.Streams[0].Upstream
				if !strings.HasPrefix(up, "source:") {
					t.Errorf("stream.Upstream = %q want source:*", up)
				}
			},
		},
		{
			name: "multi-stream sharing one device: dedupes to 1 source",
			input: `version = 1

[[streams]]
id = "encode-a"

  [[streams.inputs]]
  id = "inp1"
  device = "usb-1-2"

  [streams.encoder]
  codec = "h264"

[[streams]]
id = "encode-b"

  [[streams.inputs]]
  id = "inp1"
  device = "usb-1-2"

  [streams.encoder]
  codec = "h265"
`,
			wantSrc:  1,
			wantComp: 0,
			wantStr:  2,
			check: func(t *testing.T, s *tomlStore) {
				if s.config.Sources[0].Device != "usb-1-2" {
					t.Errorf("expected shared source device usb-1-2, got %q", s.config.Sources[0].Device)
				}
				// Both streams should reference the same source.
				if s.config.Streams[0].Upstream != s.config.Streams[1].Upstream {
					t.Errorf("streams should share upstream: %q vs %q",
						s.config.Streams[0].Upstream, s.config.Streams[1].Upstream)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "streams.toml")
			if err := os.WriteFile(path, []byte(tt.input), 0o644); err != nil {
				t.Fatalf("write fixture: %v", err)
			}

			s := NewTOML(path).(*tomlStore)
			if err := s.Load(); err != nil {
				t.Fatalf("Load: %v", err)
			}

			if got := len(s.config.Sources); got != tt.wantSrc {
				t.Errorf("sources = %d want %d (sources=%+v)", got, tt.wantSrc, s.config.Sources)
			}
			if got := len(s.config.Composers); got != tt.wantComp {
				t.Errorf("composers = %d want %d (composers=%+v)", got, tt.wantComp, s.config.Composers)
			}
			if got := len(s.config.Streams); got != tt.wantStr {
				t.Errorf("streams = %d want %d", got, tt.wantStr)
			}
			if s.config.Version != schemaVersion {
				t.Errorf("post-migrate version = %d want %d", s.config.Version, schemaVersion)
			}
			if tt.check != nil {
				tt.check(t, s)
			}
		})
	}
}

// TestMigrate_PersistsToDisk verifies that the migration result is written
// back to the file so subsequent loads are pure v2 reads with no migration.
func TestMigrate_PersistsToDisk(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "streams.toml")

	v1 := `version = 1

[[streams]]
id = "s1"

  [[streams.inputs]]
  id = "inp1"
  device = "usb-1-2"

  [streams.encoder]
  codec = "h264"
`
	if err := os.WriteFile(path, []byte(v1), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	s := NewTOML(path).(*tomlStore)
	if err := s.Load(); err != nil {
		t.Fatalf("first Load: %v", err)
	}

	rewritten, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	body := string(rewritten)
	if !strings.Contains(body, "version = 2") {
		t.Errorf("rewritten file should contain version = 2, got:\n%s", body)
	}
	if !strings.Contains(body, "[[sources]]") {
		t.Errorf("rewritten file should contain [[sources]], got:\n%s", body)
	}

	// Second load: pure v2, no migration.
	s2 := NewTOML(path).(*tomlStore)
	if err := s2.Load(); err != nil {
		t.Fatalf("second Load: %v", err)
	}
	if len(s2.config.Sources) != 1 || len(s2.config.Streams) != 1 {
		t.Errorf("post-rewrite reload mismatch: sources=%d streams=%d",
			len(s2.config.Sources), len(s2.config.Streams))
	}
}

// TestMigrate_LegacyTableShape covers the [streams.<id>] map shape (the
// production StreamSpec layout pre-intermediate). Device → source,
// canvas.source_streams → multi-input composer, perspective → effect.
func TestMigrate_LegacyTableShape(t *testing.T) {
	v1 := `version = 1

[pipeline]
enabled = true

[streams]
[streams.c920]
id = "c920"
name = "C920"
device = "usb-046d-c920"
test_mode = false

[streams.c920.ffmpeg]
codec = "h264"
audio_device = "hw:CARD=USB,DEV=0"
`
	dir := t.TempDir()
	path := filepath.Join(dir, "streams.toml")
	if err := os.WriteFile(path, []byte(v1), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	s := NewTOML(path).(*tomlStore)
	if err := s.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}

	if len(s.config.Sources) != 1 {
		t.Fatalf("legacy table → sources len = %d want 1", len(s.config.Sources))
	}
	if s.config.Sources[0].Device != "usb-046d-c920" {
		t.Errorf("legacy source device = %q want usb-046d-c920", s.config.Sources[0].Device)
	}
	if len(s.config.Streams) != 1 {
		t.Errorf("legacy table → streams len = %d want 1", len(s.config.Streams))
	}
	if !strings.HasPrefix(s.config.Streams[0].Upstream, "source:") {
		t.Errorf("legacy stream Upstream = %q want source:* prefix", s.config.Streams[0].Upstream)
	}
	if s.config.Pipeline == nil || !s.config.Pipeline.Enabled {
		t.Errorf("pipeline table should round-trip through migration: %+v", s.config.Pipeline)
	}
}

// TestMigrate_PreservesValidation makes sure the validation block survives a v1→v2 rewrite.
func TestMigrate_PreservesValidation(t *testing.T) {
	v1 := `version = 1

[validation]
ffmpeg_version = "7.1.4"

[validation.h264]
working = ["libx264"]
failed = []

[validation.h265]
working = []
failed = ["hevc_qsv"]

[[streams]]
id = "s1"

  [[streams.inputs]]
  id = "inp1"
  device = "d1"

  [streams.encoder]
  codec = "h264"
`
	dir := t.TempDir()
	path := filepath.Join(dir, "streams.toml")
	if err := os.WriteFile(path, []byte(v1), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	s := NewTOML(path).(*tomlStore)
	if err := s.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}

	v := s.GetValidation()
	if v == nil {
		t.Fatal("validation lost during migration")
	}
	if len(v.H264.Working) != 1 || v.H264.Working[0] != "libx264" {
		t.Errorf("h264 working = %v want [libx264]", v.H264.Working)
	}
}

// TestMigrateV1Streams_NoInputs is a defensive check against the
// "no inputs and no composer" error path.
func TestMigrateV1Streams_NoInputs(t *testing.T) {
	_, err := migrateV1Streams([]v1RawStream{{ID: "bad"}})
	if err == nil {
		t.Error("expected error when stream has no inputs")
	}
}

func TestSanitizeID(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"usb-1-2", "usb-1-2"},
		{"USB_046d_C920", "usb-046d-c920"},
		{"/dev/video0", "dev-video0"},
		{"", ""},
		{"---", ""},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			if got := sanitizeID(tt.in); got != tt.want {
				t.Errorf("sanitizeID(%q) = %q want %q", tt.in, got, tt.want)
			}
		})
	}
}
