package cmd

// V2Config is the persisted (post-split) TOML shape: top-level [[sources]],
// [[composers]], and [[streams]] with explicit upstream refs. The
// validate-config command decodes streams.toml into this shape for
// structural validation.
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

// V2Stream is encoder+audio; the upstream ref points to a source or composer.
// The encoder output (local RTSP relay) is hardcoded at runtime, not persisted.
type V2Stream struct {
	ID                string          `toml:"id"`
	Name              string          `toml:"name,omitempty"`
	Upstream          string          `toml:"upstream"` // "source:<id>" or "composer:<id>"
	Audio             V2AudioConfig   `toml:"audio,omitempty"`
	Encoder           V2EncoderConfig `toml:"encoder,omitempty"`
	CustomEncoderArgs string          `toml:"custom_encoder_args,omitempty"`
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
