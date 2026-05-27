package store

import (
	"fmt"
	"log/slog"
	"sort"
	"time"

	"github.com/smazurov/videonode/internal/logging"
)

// V2 types stub the canonical shapes from B1 (pipeline.Source / Composer /
// Stream). They live here until B1 lands so the store can migrate and
// persist the new shape without depending on the pipeline package.

// V2Source is one frame producer.
type V2Source struct {
	ID        string          `toml:"id" json:"id"`
	Device    string          `toml:"device,omitempty" json:"device,omitempty"`
	TestMode  bool            `toml:"test_mode,omitempty" json:"test_mode,omitempty"`
	Format    *V2SourceFormat `toml:"format,omitempty" json:"format,omitempty"`
	CreatedAt time.Time       `toml:"created_at" json:"created_at"`
	UpdatedAt time.Time       `toml:"updated_at" json:"updated_at"`
}

// V2SourceFormat is the persisted V4L2 capture format for a source.
type V2SourceFormat struct {
	FourCC string `toml:"fourcc" json:"fourcc"`
	Width  uint32 `toml:"width" json:"width"`
	Height uint32 `toml:"height" json:"height"`
	FPS    uint32 `toml:"fps,omitempty" json:"fps,omitempty"`
}

// V2Composer is one BGRA canvas compositing N sources.
type V2Composer struct {
	ID        string            `toml:"id" json:"id"`
	Canvas    V2CanvasDims      `toml:"canvas" json:"canvas"`
	Inputs    []V2ComposerInput `toml:"inputs" json:"inputs"`
	Layout    []V2LayoutSlot    `toml:"layout" json:"layout"`
	CreatedAt time.Time         `toml:"created_at" json:"created_at"`
	UpdatedAt time.Time         `toml:"updated_at" json:"updated_at"`
}

// V2CanvasDims is the composer's output dimensions and render rate. FPS
// is omitted on disk when unset; the daemon fills in the default at
// spawn time.
type V2CanvasDims struct {
	W   int `toml:"w" json:"w"`
	H   int `toml:"h" json:"h"`
	FPS int `toml:"fps,omitempty" json:"fps,omitempty"`
}

// V2ComposerInput binds an upstream source ref to a composer slot, with
// an optional per-input effect (e.g. perspective).
type V2ComposerInput struct {
	Ref    string    `toml:"ref" json:"ref"`
	Effect *V2Effect `toml:"effect,omitempty" json:"effect,omitempty"`
}

// V2CropConfig holds crop-mode positioning for TOML persistence.
type V2CropConfig struct {
	X     float64 `toml:"x" json:"x"`
	Y     float64 `toml:"y" json:"y"`
	Scale float64 `toml:"scale" json:"scale"`
}

// V2LayoutSlot positions one input on the canvas, addressed by ref.
type V2LayoutSlot struct {
	Input           string        `toml:"input" json:"input"`
	X               int           `toml:"x" json:"x"`
	Y               int           `toml:"y" json:"y"`
	W               int           `toml:"w" json:"w"`
	H               int           `toml:"h" json:"h"`
	Rotation        int           `toml:"rotation,omitempty" json:"rotation,omitempty"`
	AspectRatioMode string        `toml:"aspect_ratio_mode,omitempty" json:"aspect_ratio_mode,omitempty"`
	Crop            *V2CropConfig `toml:"crop,omitempty" json:"crop,omitempty"`
}

// V2Effect is a tagged-union per-input transformation. Today only
// "perspective" is implemented; Corners is its payload along with the
// SnapshotW/SnapshotH dims that define the coord space Corners live in.
type V2Effect struct {
	Type      string    `toml:"type" json:"type"`
	Corners   [4][2]int `toml:"corners,omitempty" json:"corners,omitempty"`
	SnapshotW int       `toml:"snapshot_w,omitempty" json:"snapshot_w,omitempty"`
	SnapshotH int       `toml:"snapshot_h,omitempty" json:"snapshot_h,omitempty"`
}

// V2Stream is an encoder + audio + publish targets pointing at one
// upstream (source or composer).
type V2Stream struct {
	ID                string            `toml:"id" json:"id"`
	Name              string            `toml:"name,omitempty" json:"name,omitempty"`
	Upstream          string            `toml:"upstream" json:"upstream"`
	Audio             V2AudioConfig     `toml:"audio,omitzero" json:"audio,omitzero"`
	Encoder           V2EncoderConfig   `toml:"encoder,omitzero" json:"encoder,omitzero"`
	Publish           []V2PublishTarget `toml:"publish,omitempty" json:"publish,omitempty"`
	CustomEncoderArgs string            `toml:"custom_encoder_args,omitempty" json:"custom_encoder_args,omitempty"`
	CreatedAt         time.Time         `toml:"created_at" json:"created_at"`
	UpdatedAt         time.Time         `toml:"updated_at" json:"updated_at"`
}

// V2AudioConfig is the per-stream audio routing.
type V2AudioConfig struct {
	Devices []string `toml:"devices,omitempty" json:"devices,omitempty"`
	Codec   string   `toml:"codec,omitempty" json:"codec,omitempty"`
	Bitrate string   `toml:"bitrate,omitempty" json:"bitrate,omitempty"`
	Filters string   `toml:"filters,omitempty" json:"filters,omitempty"`
}

// V2EncoderConfig is the user-facing encoder hint.
type V2EncoderConfig struct {
	Codec       string `toml:"codec,omitempty" json:"codec,omitempty"`
	Bitrate     string `toml:"bitrate,omitempty" json:"bitrate,omitempty"`
	GOP         int    `toml:"gop,omitempty" json:"gop,omitempty"`
	BFrames     int    `toml:"b_frames,omitempty" json:"b_frames,omitempty"`
	RateControl string `toml:"rate_control,omitempty" json:"rate_control,omitempty"`
	Preset      string `toml:"preset,omitempty" json:"preset,omitempty"`
}

// V2PublishTarget is a single output destination.
type V2PublishTarget struct {
	Type string `toml:"type" json:"type"`
	URL  string `toml:"url" json:"url"`
}

// v1RawStream mirrors the intermediate pipeline.Stream TOML shape (the
// [[streams]] array form with inputs/effects/layout). Decoded raw so the
// migrator can run regardless of whether the pipeline package matches the
// on-disk file's field set.
type v1RawStream struct {
	ID                string               `toml:"id"`
	Name              string               `toml:"name"`
	Inputs            []v1RawInput         `toml:"inputs"`
	Layout            []v1RawSlot          `toml:"layout"`
	Effects           map[string][]v1RawFx `toml:"effects"`
	Audio             V2AudioConfig        `toml:"audio"`
	Encoder           V2EncoderConfig      `toml:"encoder"`
	Publish           []V2PublishTarget    `toml:"publish"`
	TestMode          bool                 `toml:"test_mode"`
	ForceComposer     bool                 `toml:"force_composer"`
	CustomEncoderArgs string               `toml:"custom_encoder_args"`
	CreatedAt         time.Time            `toml:"created_at"`
	UpdatedAt         time.Time            `toml:"updated_at"`
}

type v1RawInput struct {
	ID     string `toml:"id"`
	Device string `toml:"device"`
}

type v1RawSlot struct {
	Slot int `toml:"slot"`
	X    int `toml:"x"`
	Y    int `toml:"y"`
	W    int `toml:"w"`
	H    int `toml:"h"`
}

type v1RawFx struct {
	Type    string    `toml:"type"`
	Corners [4][2]int `toml:"corners"`
}

// migrationResult bundles the three v2 entity slices produced by a v1→v2
// run. Sources are deduplicated across all input streams.
type migrationResult struct {
	Sources   []V2Source
	Composers []V2Composer
	Streams   []V2Stream
}

// migrateV1Streams converts the intermediate [[streams]] array shape (with
// inputs/effects/layout/force_composer) into the v2 sources/composers/
// streams triple. The output sources/composers are ordered deterministically
// (by id) so successive migrations of an unchanged input produce identical
// TOML output.
func migrateV1Streams(v1 []v1RawStream) (migrationResult, error) {
	out := migrationResult{}

	// sourceKey -> source-id, so streams referencing the same device collapse to one Source.
	srcByKey := make(map[string]string)
	srcByID := make(map[string]V2Source)

	for _, s := range v1 {
		if s.ID == "" {
			return out, fmt.Errorf("v1 stream missing id")
		}

		// Resolve each input to a v2 source id, synthesizing new sources as needed.
		streamSrcIDs := make([]string, 0, len(s.Inputs))
		inputIDToSrcID := make(map[string]string, len(s.Inputs))
		streamHasRealDevice := false
		for _, in := range s.Inputs {
			if in.Device != "" {
				streamHasRealDevice = true
			}
		}

		// v1 stream-level test_mode wins over a real device on its inputs:
		// the operator explicitly asked for the test pattern. Warn so the
		// override is visible in logs, since the v2 source will lose the
		// device string.
		if s.TestMode && streamHasRealDevice {
			devs := make([]string, 0, len(s.Inputs))
			for _, in := range s.Inputs {
				if in.Device != "" {
					devs = append(devs, in.Device)
				}
			}
			slog.Warn("v1→v2 migration: stream-level test_mode overrides input devices; sources will use test pattern",
				logging.KeyStreamID, s.ID,
				logging.KeyDroppedDevices, devs,
			)
		}

		for _, in := range s.Inputs {
			// Stream-level test_mode forces all synthesized sources to
			// test-pattern, discarding any device override.
			testMode := false
			device := in.Device
			if s.TestMode {
				testMode = true
				device = ""
			}

			key := sourceKey(device, testMode)
			srcID, ok := srcByKey[key]
			if !ok {
				srcID = synthesizeSourceID(device, testMode, s.ID, srcByID)
				srcByKey[key] = srcID
				srcByID[srcID] = V2Source{
					ID:        srcID,
					Device:    device,
					TestMode:  testMode,
					CreatedAt: s.CreatedAt,
					UpdatedAt: s.UpdatedAt,
				}
			}
			streamSrcIDs = append(streamSrcIDs, srcID)
			inputIDToSrcID[in.ID] = srcID
		}

		hasEffects := len(s.Effects) > 0
		needsComposer := len(s.Inputs) > 1 || hasEffects || s.ForceComposer

		newStream := V2Stream{
			ID:                s.ID,
			Name:              s.Name,
			Audio:             s.Audio,
			Encoder:           s.Encoder,
			Publish:           s.Publish,
			CustomEncoderArgs: s.CustomEncoderArgs,
			CreatedAt:         s.CreatedAt,
			UpdatedAt:         s.UpdatedAt,
		}

		switch {
		case needsComposer:
			comp := synthesizeComposer(s, inputIDToSrcID, streamSrcIDs)
			out.Composers = append(out.Composers, comp)
			newStream.Upstream = "composer:" + comp.ID
		case len(streamSrcIDs) == 1:
			newStream.Upstream = "source:" + streamSrcIDs[0]
		default:
			return out, fmt.Errorf("v1 stream %q has no inputs and no composer", s.ID)
		}

		out.Streams = append(out.Streams, newStream)
	}

	// Stable ordering of sources by id keeps generated TOML diffs minimal.
	srcIDs := make([]string, 0, len(srcByID))
	for id := range srcByID {
		srcIDs = append(srcIDs, id)
	}
	sort.Strings(srcIDs)
	for _, id := range srcIDs {
		out.Sources = append(out.Sources, srcByID[id])
	}

	return out, nil
}

// sourceKey is the dedup key for sources: same device+testmode collapses
// to a single producer, distinct device strings stay separate. Test-mode
// sources without device get a synthetic key per stream (handled by caller).
func sourceKey(device string, testMode bool) string {
	if testMode && device == "" {
		// Unique per call site — caller already routed this through synthesizeSourceID;
		// returning a key derived from the test-mode flag plus an empty device collapses
		// repeated test-mode sources across a v1 multi-stream file into one shared source.
		return "testmode:"
	}
	return "device:" + device
}

// synthesizeSourceID picks a stable id for a new Source. Preference order:
// device string (sanitized) → "test-pattern" for test mode → stream-id fallback.
// Collisions get a numeric suffix.
func synthesizeSourceID(device string, testMode bool, streamID string, existing map[string]V2Source) string {
	var base string
	switch {
	case device != "":
		base = sanitizeID(device)
	case testMode:
		base = "test-pattern"
	default:
		base = sanitizeID(streamID) + "-src"
	}
	if base == "" {
		base = "source"
	}
	candidate := base
	for i := 2; ; i++ {
		if _, taken := existing[candidate]; !taken {
			return candidate
		}
		candidate = fmt.Sprintf("%s-%d", base, i)
	}
}

// sanitizeID lowercases and replaces non-[a-z0-9-] runs with '-' so device
// strings make readable source ids without breaking TOML inline-key rules.
func sanitizeID(s string) string {
	out := make([]rune, 0, len(s))
	prevDash := false
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-':
			out = append(out, r)
			prevDash = r == '-'
		case r >= 'A' && r <= 'Z':
			out = append(out, r+('a'-'A'))
			prevDash = false
		default:
			if !prevDash && len(out) > 0 {
				out = append(out, '-')
				prevDash = true
			}
		}
	}
	for len(out) > 0 && out[len(out)-1] == '-' {
		out = out[:len(out)-1]
	}
	return string(out)
}

// synthesizeComposer builds one V2Composer from a v1 stream. Input order
// preserves the v1 inputs slice; layout (if present) is mapped by slot
// index → input ref. Canvas dims default to 1920x1080 when not derivable
// (v1 has no explicit canvas field — composer dims were implicit).
func synthesizeComposer(s v1RawStream, inputIDToSrcID map[string]string, _ []string) V2Composer {
	comp := V2Composer{
		ID:        s.ID + "-composer",
		Canvas:    V2CanvasDims{W: 1920, H: 1080},
		CreatedAt: s.CreatedAt,
		UpdatedAt: s.UpdatedAt,
	}

	// Inputs: preserve v1 input order, attach effect when present.
	// The v2 composer keys slot bindings by source_id, so duplicate refs
	// (same device reused via multiple v1 inputs) collapse to one entry;
	// keep the first occurrence and warn so the operator notices the v1
	// "same source twice" trick isn't preserved in v2.
	seenRefs := make(map[string]string, len(s.Inputs))
	for _, in := range s.Inputs {
		ref := "source:" + inputIDToSrcID[in.ID]
		if firstInput, dup := seenRefs[ref]; dup {
			slog.Warn("v1→v2 migration: dropping duplicate composer input (same source referenced twice)",
				logging.KeyStreamID, s.ID,
				logging.KeyComposerID, comp.ID,
				logging.KeyRef, ref,
				logging.KeyFirstInput, firstInput,
				logging.KeyDuplicateInput, in.ID,
			)
			continue
		}
		seenRefs[ref] = in.ID
		ci := V2ComposerInput{Ref: ref}
		if fxList, ok := s.Effects[in.ID]; ok && len(fxList) > 0 {
			fx := fxList[0]
			ci.Effect = &V2Effect{Type: fx.Type, Corners: fx.Corners}
		}
		comp.Inputs = append(comp.Inputs, ci)
	}

	// Layout: v1 slot index → v2 input ref (by name, not position). Skip
	// slots whose referenced input was deduped out above.
	knownInputRefs := make(map[string]struct{}, len(comp.Inputs))
	for _, ci := range comp.Inputs {
		knownInputRefs[ci.Ref] = struct{}{}
	}
	for _, slot := range s.Layout {
		if slot.Slot < 0 || slot.Slot >= len(s.Inputs) {
			continue
		}
		ref := "source:" + inputIDToSrcID[s.Inputs[slot.Slot].ID]
		if _, ok := knownInputRefs[ref]; !ok {
			continue
		}
		comp.Layout = append(comp.Layout, V2LayoutSlot{
			Input: ref,
			X:     slot.X,
			Y:     slot.Y,
			W:     slot.W,
			H:     slot.H,
		})
	}

	// Derive canvas dims from the largest layout slot bbox when present.
	if maxW, maxH := layoutBounds(comp.Layout); maxW > 0 && maxH > 0 {
		comp.Canvas = V2CanvasDims{W: maxW, H: maxH}
	}

	return comp
}

func layoutBounds(layout []V2LayoutSlot) (int, int) {
	maxW, maxH := 0, 0
	for _, l := range layout {
		if l.X+l.W > maxW {
			maxW = l.X + l.W
		}
		if l.Y+l.H > maxH {
			maxH = l.Y + l.H
		}
	}
	return maxW, maxH
}
