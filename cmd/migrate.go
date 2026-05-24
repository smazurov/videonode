package cmd

import (
	"errors"
	"fmt"
	"os"
	"sort"

	"github.com/pelletier/go-toml/v2"
	"github.com/spf13/cobra"
)

// V2Config is the post-split TOML shape (top-level [[sources]], [[composers]],
// [[streams]] with explicit upstream refs). Mirrors the canonical shape
// from plan-a-full-rewrite-linked-gray.md. Stubbed locally; once unit B2 lands
// the canonical migrator from internal/streams/store/migrate.go supersedes this.
type V2Config struct {
	Version   int          `toml:"version"`
	Sources   []V2Source   `toml:"sources,omitempty"`
	Composers []V2Composer `toml:"composers,omitempty"`
	Streams   []V2Stream   `toml:"streams,omitempty"`
}

// V2Source is a daemon-managed frame producer.
type V2Source struct {
	ID       string `toml:"id"`
	Device   string `toml:"device,omitempty"`
	TestMode bool   `toml:"test_mode,omitempty"`
}

// V2Composer aggregates N V2Sources onto a canvas.
type V2Composer struct {
	ID     string            `toml:"id"`
	Canvas V2CanvasDims      `toml:"canvas"`
	Inputs []V2ComposerInput `toml:"inputs"`
	Layout []V2LayoutSlot    `toml:"layout"`
}

// V2CanvasDims sizes the composer canvas in pixels.
type V2CanvasDims struct {
	W int `toml:"w"`
	H int `toml:"h"`
}

// V2ComposerInput references an upstream source plus optional per-input effect.
type V2ComposerInput struct {
	Ref    string    `toml:"ref"` // "source:<id>"
	Effect *V2Effect `toml:"effect,omitempty"`
}

// V2LayoutSlot positions one input on the composer canvas.
type V2LayoutSlot struct {
	Input string `toml:"input"` // matches V2ComposerInput.Ref
	X     int    `toml:"x"`
	Y     int    `toml:"y"`
	W     int    `toml:"w"`
	H     int    `toml:"h"`
}

// V2Effect is one composer effect; today only perspective is supported.
type V2Effect struct {
	Type    string    `toml:"type"`
	Corners [4][2]int `toml:"corners,omitempty"`
}

// V2Stream is encoder+audio+publish; the upstream ref points to a source or composer.
type V2Stream struct {
	ID                string            `toml:"id"`
	Name              string            `toml:"name,omitempty"`
	Upstream          string            `toml:"upstream"` // "source:<id>" or "composer:<id>"
	Audio             V2AudioConfig     `toml:"audio,omitempty"`
	Encoder           V2EncoderConfig   `toml:"encoder,omitempty"`
	Publish           []V2PublishTarget `toml:"publish,omitempty"`
	CustomEncoderArgs string            `toml:"custom_encoder_args,omitempty"`
}

// V2AudioConfig mirrors pipeline.AudioConfig at the TOML layer.
type V2AudioConfig struct {
	Devices []string `toml:"devices,omitempty"`
	Codec   string   `toml:"codec,omitempty"`
	Bitrate string   `toml:"bitrate,omitempty"`
	Filters string   `toml:"filters,omitempty"`
}

// V2EncoderConfig mirrors pipeline.EncoderConfig at the TOML layer.
type V2EncoderConfig struct {
	Codec       string `toml:"codec,omitempty"`
	Bitrate     string `toml:"bitrate,omitempty"`
	GOP         int    `toml:"gop,omitempty"`
	BFrames     int    `toml:"b_frames,omitempty"`
	RateControl string `toml:"rate_control,omitempty"`
	Preset      string `toml:"preset,omitempty"`
}

// V2PublishTarget is a single output destination.
type V2PublishTarget struct {
	Type string `toml:"type"`
	URL  string `toml:"url"`
}

// v1Config matches the pre-split TOML shape: a single [[streams]] list with
// embedded inputs/layout/effects. Used by the migrator to read legacy files.
type v1Config struct {
	Version int                 `toml:"version"`
	Streams map[string]v1Stream `toml:"streams"`
}

type v1Stream struct {
	ID                string                `toml:"id"`
	Name              string                `toml:"name"`
	Inputs            []v1InputRef          `toml:"inputs"`
	Layout            []v1SlotPlacement     `toml:"layout"`
	Effects           map[string][]v1Effect `toml:"effects"`
	Audio             V2AudioConfig         `toml:"audio"`
	Encoder           V2EncoderConfig       `toml:"encoder"`
	Publish           []V2PublishTarget     `toml:"publish"`
	TestMode          bool                  `toml:"test_mode"`
	CustomEncoderArgs string                `toml:"custom_encoder_args"`
	ForceComposer     bool                  `toml:"force_composer"`
}

type v1InputRef struct {
	ID     string `toml:"id"`
	Device string `toml:"device"`
}

type v1SlotPlacement struct {
	Slot int `toml:"slot"`
	X    int `toml:"x"`
	Y    int `toml:"y"`
	W    int `toml:"w"`
	H    int `toml:"h"`
}

type v1Effect struct {
	Type    string    `toml:"type"`
	Corners [4][2]int `toml:"corners"`
}

// sourceKey deduplicates synthesized sources by their (device, test_mode) tuple.
type sourceKey struct {
	device   string
	testMode bool
}

// MigrateV1ToV2 converts a v1 (pre-split) config to v2 (sources/composers/streams).
// Synthesizes one V2Source per stream input device (deduplicated by device path),
// one V2Composer per stream that needs one (multi-input OR effects OR force_composer),
// and rewrites each stream's upstream ref accordingly. Drops stream-level test_mode
// after migrating it down to the synthesized source (only when the stream had no
// real device).
func MigrateV1ToV2(in *v1Config) (*V2Config, error) {
	out := &V2Config{Version: 2}

	sourceID := map[sourceKey]string{}

	streamIDs := make([]string, 0, len(in.Streams))
	for id := range in.Streams {
		streamIDs = append(streamIDs, id)
	}
	sort.Strings(streamIDs)

	for _, sid := range streamIDs {
		s := in.Streams[sid]
		if s.ID == "" {
			s.ID = sid
		}

		// Build per-stream input-id → source-id mapping.
		inputToSource := map[string]string{}
		for _, in := range s.Inputs {
			device := in.Device
			testMode := false
			if device == "" && s.TestMode {
				testMode = true
			}
			key := sourceKey{device: device, testMode: testMode}
			srcID, ok := sourceID[key]
			if !ok {
				srcID = synthesizeSourceID(device, testMode, sourceID)
				sourceID[key] = srcID
				out.Sources = append(out.Sources, V2Source{
					ID:       srcID,
					Device:   device,
					TestMode: testMode,
				})
			}
			inputToSource[in.ID] = srcID
		}

		// Decide if this stream needs a composer.
		needsComposer := len(s.Inputs) > 1 || len(s.Effects) > 0 || s.ForceComposer

		if !needsComposer {
			// Single source, no effects → stream points directly at source.
			upstream := ""
			if len(s.Inputs) == 1 {
				upstream = "source:" + inputToSource[s.Inputs[0].ID]
			}
			out.Streams = append(out.Streams, V2Stream{
				ID:                s.ID,
				Name:              s.Name,
				Upstream:          upstream,
				Audio:             s.Audio,
				Encoder:           s.Encoder,
				Publish:           s.Publish,
				CustomEncoderArgs: s.CustomEncoderArgs,
			})
			continue
		}

		// Synthesize composer.
		compID := s.ID + "-composer"
		canvas := V2CanvasDims{W: 1920, H: 1080}
		// Derive canvas dims from layout bounding box if present.
		if len(s.Layout) > 0 {
			maxW, maxH := 0, 0
			for _, slot := range s.Layout {
				if slot.X+slot.W > maxW {
					maxW = slot.X + slot.W
				}
				if slot.Y+slot.H > maxH {
					maxH = slot.Y + slot.H
				}
			}
			if maxW > 0 && maxH > 0 {
				canvas = V2CanvasDims{W: maxW, H: maxH}
			}
		}

		comp := V2Composer{ID: compID, Canvas: canvas}
		for _, in := range s.Inputs {
			ref := "source:" + inputToSource[in.ID]
			ci := V2ComposerInput{Ref: ref}
			if effs, ok := s.Effects[in.ID]; ok && len(effs) > 0 {
				ci.Effect = &V2Effect{Type: effs[0].Type, Corners: effs[0].Corners}
			}
			comp.Inputs = append(comp.Inputs, ci)
		}
		for _, slot := range s.Layout {
			if slot.Slot < 0 || slot.Slot >= len(s.Inputs) {
				return nil, fmt.Errorf("stream %s layout slot %d out of range (%d inputs)",
					s.ID, slot.Slot, len(s.Inputs))
			}
			comp.Layout = append(comp.Layout, V2LayoutSlot{
				Input: "source:" + inputToSource[s.Inputs[slot.Slot].ID],
				X:     slot.X, Y: slot.Y, W: slot.W, H: slot.H,
			})
		}
		out.Composers = append(out.Composers, comp)

		out.Streams = append(out.Streams, V2Stream{
			ID:                s.ID,
			Name:              s.Name,
			Upstream:          "composer:" + compID,
			Audio:             s.Audio,
			Encoder:           s.Encoder,
			Publish:           s.Publish,
			CustomEncoderArgs: s.CustomEncoderArgs,
		})
	}

	// Sort outputs for deterministic file content.
	sort.Slice(out.Sources, func(i, j int) bool { return out.Sources[i].ID < out.Sources[j].ID })
	sort.Slice(out.Composers, func(i, j int) bool { return out.Composers[i].ID < out.Composers[j].ID })
	sort.Slice(out.Streams, func(i, j int) bool { return out.Streams[i].ID < out.Streams[j].ID })

	return out, nil
}

// synthesizeSourceID derives a stable, human-readable source id from a device
// ref. Falls back to "test-pattern-N" for TestMode sources or "source-N" when
// the device string is empty.
func synthesizeSourceID(device string, testMode bool, existing map[sourceKey]string) string {
	used := map[string]bool{}
	for _, id := range existing {
		used[id] = true
	}
	var base string
	switch {
	case testMode:
		base = "test-pattern"
	case device != "":
		base = sanitizeID(device)
	default:
		base = "source"
	}
	id := base
	for i := 2; used[id]; i++ {
		id = fmt.Sprintf("%s-%d", base, i)
	}
	return id
}

// sanitizeID converts an opaque device ref into a kebab-case identifier safe
// for use as a TOML key.
func sanitizeID(s string) string {
	out := make([]rune, 0, len(s))
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-':
			out = append(out, r)
		case r >= 'A' && r <= 'Z':
			out = append(out, r+('a'-'A'))
		case r == '_', r == '/', r == '.', r == ' ':
			out = append(out, '-')
		}
	}
	if len(out) == 0 {
		return "source"
	}
	return string(out)
}

// LoadV1Config reads a v1-shaped streams.toml from disk. Returns the parsed
// struct and a flag indicating whether the file uses the legacy shape (i.e.
// version unset / 1 with a map-of-stream-id streams table).
func LoadV1Config(path string) (*v1Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	cfg := &v1Config{Streams: map[string]v1Stream{}}
	if err := toml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return cfg, nil
}

// CreateMigrateConfigCmd creates the migrate-config command. Dry-run by default;
// pass --write to overwrite the file in place. The v1→v2 migration also runs
// automatically when the daemon loads a legacy file (unit B2); this command is
// the manual escape hatch for inspection and offline conversion.
func CreateMigrateConfigCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "migrate-config <path>",
		Short: "Migrate streams.toml from v1 to v2 (sources/composers/streams)",
		Long: `Convert a pre-split streams.toml (single [[streams]] table with embedded ` +
			`inputs/layout/effects) to the post-split shape with top-level [[sources]], ` +
			`[[composers]], and [[streams]] tables. Dry-run by default; pass --write to ` +
			`overwrite the file in place.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path := args[0]
			write, _ := cmd.Flags().GetBool("write")

			v1, err := LoadV1Config(path)
			if err != nil {
				return fmt.Errorf("load v1 config: %w", err)
			}
			if v1.Version >= 2 {
				return fmt.Errorf("%s already at version %d; nothing to migrate", path, v1.Version)
			}

			v2, err := MigrateV1ToV2(v1)
			if err != nil {
				return fmt.Errorf("migrate: %w", err)
			}

			data, err := toml.Marshal(v2)
			if err != nil {
				return fmt.Errorf("marshal v2: %w", err)
			}

			if !write {
				if _, err := fmt.Fprintln(cmd.OutOrStdout(), "# dry-run: migrated config (pass --write to apply)"); err != nil {
					return err
				}
				if _, err := fmt.Fprintln(cmd.OutOrStdout(), string(data)); err != nil {
					return err
				}
				return nil
			}

			if err := os.WriteFile(path, data, 0o644); err != nil {
				return fmt.Errorf("write %s: %w", path, err)
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(),
				"migrated %s: %d sources, %d composers, %d streams\n",
				path, len(v2.Sources), len(v2.Composers), len(v2.Streams))
			return err
		},
	}
	cmd.Flags().Bool("write", false, "Write the migrated config back to the file (otherwise dry-run)")
	return cmd
}

// ErrAlreadyV2 signals a no-op migration: the input file is already v2.
var ErrAlreadyV2 = errors.New("config already at version 2")
