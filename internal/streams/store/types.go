package store

import "time"

// V2 types are the canonical on-disk shapes. They mirror the pipeline
// types so the store persists the v2 schema without depending on the
// pipeline package; convert.go bridges the two.

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
	W          int    `toml:"w" json:"w"`
	H          int    `toml:"h" json:"h"`
	FPS        int    `toml:"fps,omitempty" json:"fps,omitempty"`
	Background string `toml:"background,omitempty" json:"background,omitempty"`
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
	Corners   [4][2]int `toml:"corners,omitzero" json:"corners,omitempty"`
	SnapshotW int       `toml:"snapshot_w,omitempty" json:"snapshot_w,omitempty"`
	SnapshotH int       `toml:"snapshot_h,omitempty" json:"snapshot_h,omitempty"`
}

// V2Stream is an encoder + audio config pointing at one upstream (source
// or composer). The encoder's output is the daemon's local RTSP relay,
// hardcoded at the pipeline-build boundary — not persisted here.
type V2Stream struct {
	ID                string          `toml:"id" json:"id"`
	Name              string          `toml:"name,omitempty" json:"name,omitempty"`
	Upstream          string          `toml:"upstream" json:"upstream"`
	Audio             V2AudioConfig   `toml:"audio,omitzero" json:"audio,omitzero"`
	Encoder           V2EncoderConfig `toml:"encoder,omitzero" json:"encoder,omitzero"`
	CustomEncoderArgs string          `toml:"custom_encoder_args,omitempty" json:"custom_encoder_args,omitempty"`
	CreatedAt         time.Time       `toml:"created_at" json:"created_at"`
	UpdatedAt         time.Time       `toml:"updated_at" json:"updated_at"`
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
