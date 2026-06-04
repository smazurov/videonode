package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pelletier/go-toml/v2"
)

func TestMigrateV1ToV2_SoloSource(t *testing.T) {
	in := &v1Config{
		Version: 1,
		Streams: map[string]v1Stream{
			"cam-front": {
				ID:      "cam-front",
				Name:    "Front camera",
				Inputs:  []v1InputRef{{ID: "inp1", Device: "usb-1-2"}},
				Encoder: V2EncoderConfig{Codec: "h264", Bitrate: "4M"},
			},
		},
	}

	out, err := MigrateV1ToV2(in)
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if out.Version != 2 {
		t.Errorf("want version 2, got %d", out.Version)
	}
	if len(out.Sources) != 1 {
		t.Fatalf("want 1 source, got %d", len(out.Sources))
	}
	if out.Sources[0].Device != "usb-1-2" {
		t.Errorf("want device usb-1-2, got %q", out.Sources[0].Device)
	}
	if len(out.Composers) != 0 {
		t.Errorf("solo source should not produce composer, got %d", len(out.Composers))
	}
	if len(out.Streams) != 1 {
		t.Fatalf("want 1 stream, got %d", len(out.Streams))
	}
	wantUpstream := "source:" + out.Sources[0].ID
	if out.Streams[0].Upstream != wantUpstream {
		t.Errorf("upstream = %q, want %q", out.Streams[0].Upstream, wantUpstream)
	}
}

func TestMigrateV1ToV2_TwoInputs(t *testing.T) {
	in := &v1Config{
		Version: 1,
		Streams: map[string]v1Stream{
			"split-show": {
				ID: "split-show",
				Inputs: []v1InputRef{
					{ID: "left", Device: "usb-1-2"},
					{ID: "right", Device: "usb-1-3"},
				},
				Layout: []v1SlotPlacement{
					{Slot: 0, X: 0, Y: 0, W: 1920, H: 1080},
					{Slot: 1, X: 1920, Y: 0, W: 1920, H: 1080},
				},
				Encoder: V2EncoderConfig{Codec: "h265", Bitrate: "12M"},
			},
		},
	}

	out, err := MigrateV1ToV2(in)
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if len(out.Sources) != 2 {
		t.Fatalf("want 2 sources, got %d", len(out.Sources))
	}
	if len(out.Composers) != 1 {
		t.Fatalf("want 1 composer, got %d", len(out.Composers))
	}
	comp := out.Composers[0]
	if comp.Canvas.W != 3840 || comp.Canvas.H != 1080 {
		t.Errorf("canvas = %dx%d, want 3840x1080 (layout bbox)", comp.Canvas.W, comp.Canvas.H)
	}
	if len(comp.Layout) != 2 {
		t.Fatalf("want 2 layout slots, got %d", len(comp.Layout))
	}
	if out.Streams[0].Upstream != "composer:"+comp.ID {
		t.Errorf("upstream = %q, want composer:%s", out.Streams[0].Upstream, comp.ID)
	}
}

func TestMigrateV1ToV2_PerspectiveForcesComposer(t *testing.T) {
	in := &v1Config{
		Version: 1,
		Streams: map[string]v1Stream{
			"keystone": {
				ID:     "keystone",
				Inputs: []v1InputRef{{ID: "inp1", Device: "usb-3-1"}},
				Effects: map[string][]v1Effect{
					"inp1": {{
						Type:    "perspective",
						Corners: [4][2]int{{120, 80}, {1820, 60}, {1900, 1020}, {60, 1000}},
					}},
				},
			},
		},
	}

	out, err := MigrateV1ToV2(in)
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if len(out.Composers) != 1 {
		t.Fatalf("effect should force composer; got %d composers", len(out.Composers))
	}
	if out.Composers[0].Inputs[0].Effect == nil {
		t.Fatal("effect not propagated")
	}
	if out.Composers[0].Inputs[0].Effect.Type != "perspective" {
		t.Errorf("effect type = %q, want perspective", out.Composers[0].Inputs[0].Effect.Type)
	}
}

func TestMigrateV1ToV2_ForceComposer(t *testing.T) {
	in := &v1Config{
		Version: 1,
		Streams: map[string]v1Stream{
			"forced": {
				ID:            "forced",
				Inputs:        []v1InputRef{{ID: "inp1", Device: "usb-1-2"}},
				ForceComposer: true,
			},
		},
	}
	out, err := MigrateV1ToV2(in)
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if len(out.Composers) != 1 {
		t.Fatalf("force_composer should synthesize composer; got %d", len(out.Composers))
	}
}

func TestMigrateV1ToV2_SourceDedup(t *testing.T) {
	in := &v1Config{
		Version: 1,
		Streams: map[string]v1Stream{
			"a": {ID: "a", Inputs: []v1InputRef{{ID: "i", Device: "usb-1-2"}}},
			"b": {ID: "b", Inputs: []v1InputRef{{ID: "i", Device: "usb-1-2"}}},
		},
	}
	out, err := MigrateV1ToV2(in)
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if len(out.Sources) != 1 {
		t.Errorf("same device should dedupe to 1 source, got %d", len(out.Sources))
	}
}

func TestMigrateConfigCmd_DryRun(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "streams.toml")

	in := v1Config{
		Version: 1,
		Streams: map[string]v1Stream{
			"cam": {ID: "cam", Inputs: []v1InputRef{{ID: "i", Device: "usb-1-2"}}},
		},
	}
	data, err := toml.Marshal(in)
	if err != nil {
		t.Fatalf("marshal v1: %v", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	c := CreateMigrateConfigCmd()
	var stdout bytes.Buffer
	c.SetOut(&stdout)
	c.SetErr(&stdout)
	c.SetArgs([]string{path})
	if err := c.Execute(); err != nil {
		t.Fatalf("migrate-config: %v", err)
	}

	if !strings.Contains(stdout.String(), "dry-run") {
		t.Errorf("expected dry-run marker; got:\n%s", stdout.String())
	}

	// File should be unchanged.
	got, _ := os.ReadFile(path)
	if !bytes.Equal(got, data) {
		t.Errorf("dry-run modified the file")
	}
}

func TestMigrateConfigCmd_Write(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "streams.toml")

	in := v1Config{
		Version: 1,
		Streams: map[string]v1Stream{
			"cam": {ID: "cam", Inputs: []v1InputRef{{ID: "i", Device: "usb-1-2"}}},
		},
	}
	data, err := toml.Marshal(in)
	if err != nil {
		t.Fatalf("marshal v1: %v", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	c := CreateMigrateConfigCmd()
	var stdout bytes.Buffer
	c.SetOut(&stdout)
	c.SetErr(&stdout)
	c.SetArgs([]string{path, "--write"})
	if err := c.Execute(); err != nil {
		t.Fatalf("migrate-config --write: %v", err)
	}

	// File should now be v2.
	got, _ := os.ReadFile(path)
	var v2 V2Config
	if err := toml.Unmarshal(got, &v2); err != nil {
		t.Fatalf("parse migrated: %v", err)
	}
	if v2.Version != 2 {
		t.Errorf("post-migrate version = %d, want 2", v2.Version)
	}
	if len(v2.Sources) != 1 || len(v2.Streams) != 1 {
		t.Errorf("want 1 source + 1 stream, got %d sources / %d streams",
			len(v2.Sources), len(v2.Streams))
	}
}

func TestMigrateConfigCmd_AlreadyV2(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "streams.toml")

	v2 := V2Config{Version: 2}
	data, _ := toml.Marshal(v2)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	c := CreateMigrateConfigCmd()
	var stderr bytes.Buffer
	c.SetOut(&stderr)
	c.SetErr(&stderr)
	c.SetArgs([]string{path})
	err := c.Execute()
	if err == nil {
		t.Fatal("expected error for already-v2 file")
	}
	if !strings.Contains(err.Error(), "already at version") {
		t.Errorf("expected 'already at version' error, got: %v", err)
	}
}
