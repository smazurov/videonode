// Package pipeline is the post-rip stream pipeline model.
//
// One Stream → at most three supervised processes: Producer (per unique
// device, refcounted across streams) → Composer (optional, present when
// len(Inputs)>1 or any Effects) → Encoder (always). The Composer picker
// runs once at Pipeline.Apply time; see picker.go.
//
// This file holds only the data-model types. Stage interface lives in
// stage.go; the actual stage implementations live in producer.go,
// composer.go, encoder.go; the assembler lives in pipeline.go.
package pipeline

import "time"

// Stream is the unified canvas+source spec. Replaces StreamSpec.Canvas
// dichotomy with a single shape — N inputs, optional layout/effects,
// audio+encoder+publish always present.
type Stream struct {
	ID   string `toml:"id" json:"id"`
	Name string `toml:"name" json:"name"`

	// Inputs is the ordered list of device refs this stream consumes.
	// len==1 with no effects: Encoder dials the producer's SCM socket
	// directly. len>1 or effects present: Composer engages.
	Inputs []InputRef `toml:"inputs" json:"inputs"`

	// Layout is one entry per input; positional (slot 0 = inputs[0]).
	// Optional at N==1 (identity at canvas size); required for N>1.
	Layout []SlotPlacement `toml:"layout,omitempty" json:"layout,omitempty"`

	// Effects keyed by input ID. Presence of any effect engages Composer
	// even when len(Inputs)==1.
	Effects map[string][]Effect `toml:"effects,omitempty" json:"effects,omitempty"`

	Audio   AudioConfig     `toml:"audio,omitempty" json:"audio,omitempty"`
	Encoder EncoderConfig   `toml:"encoder,omitempty" json:"encoder,omitempty"`
	Publish []PublishTarget `toml:"publish,omitempty" json:"publish,omitempty"`

	// TestMode is preserved as a config + API surface. Currently a no-op
	// in Pipeline.Apply — follow-up work adds RPC-driven test-pattern
	// producer that engages when TestMode=true.
	TestMode bool `toml:"test_mode,omitempty" json:"test_mode,omitempty"`

	// CustomEncoderArgs, when non-empty, replaces the daemon-generated
	// encoder argv from `-c:v` onward. The daemon always prepends the
	// input fragment (`vn-sink --socket X | ffmpeg ...`) so user-supplied
	// args cannot break the data-plane plumbing.
	CustomEncoderArgs string `toml:"custom_encoder_args,omitempty" json:"custom_encoder_args,omitempty"`

	CreatedAt time.Time `toml:"created_at" json:"created_at"`
	UpdatedAt time.Time `toml:"updated_at" json:"updated_at"`
}

// InputRef binds a stream-local slot id to a device. Device is a
// daemon-resolvable opaque reference (USB bus-port, /dev/videoN, etc.);
// the Producer stage resolves it to a path at Start time.
type InputRef struct {
	ID     string `toml:"id" json:"id"`
	Device string `toml:"device" json:"device"`
}

// SlotPlacement positions one input on the composer canvas. Slot is the
// positional index into Stream.Inputs. X/Y/W/H are canvas pixels.
type SlotPlacement struct {
	Slot int `toml:"slot" json:"slot"`
	X    int `toml:"x" json:"x"`
	Y    int `toml:"y" json:"y"`
	W    int `toml:"w" json:"w"`
	H    int `toml:"h" json:"h"`
}

// Effect is one transformation applied per-input by the Composer. Today
// only "perspective" is implemented; new types are tagged-union by Type
// and extend the struct with their own params.
type Effect struct {
	Type string `toml:"type" json:"type"`
	// Perspective: source-image corners in clockwise order from TL.
	Corners [4][2]int `toml:"corners,omitempty" json:"corners,omitempty"`
}

// AudioConfig is the per-stream audio routing. Devices are ALSA device
// names; N>1 implies mixing (Filters supplies the amix expression).
// Codec/Bitrate select the AAC/Opus encode params.
type AudioConfig struct {
	Devices []string `toml:"devices,omitempty" json:"devices,omitempty"`
	Codec   string   `toml:"codec,omitempty" json:"codec,omitempty"`
	Bitrate string   `toml:"bitrate,omitempty" json:"bitrate,omitempty"`
	Filters string   `toml:"filters,omitempty" json:"filters,omitempty"`
}

// EncoderConfig is the backend-agnostic encoder hint. The concrete
// EncoderStage maps Codec ("h264"/"h265"/"av1") to a backend-specific
// encoder name (h264_rkmpp, libx264, etc.) based on probe results.
type EncoderConfig struct {
	Codec       string `toml:"codec,omitempty" json:"codec,omitempty"`
	Bitrate     string `toml:"bitrate,omitempty" json:"bitrate,omitempty"`
	GOP         int    `toml:"gop,omitempty" json:"gop,omitempty"`
	BFrames     int    `toml:"b_frames,omitempty" json:"b_frames,omitempty"`
	RateControl string `toml:"rate_control,omitempty" json:"rate_control,omitempty"`
	Preset      string `toml:"preset,omitempty" json:"preset,omitempty"`
}

// PublishTarget is a single output destination. Type discriminates the
// URL scheme (rtsp/srt/hls/...); URL is the destination the encoder
// writes to. Multiple PublishTargets imply `-f tee` for the ffmpeg
// backend (follow-up work).
type PublishTarget struct {
	Type string `toml:"type" json:"type"`
	URL  string `toml:"url" json:"url"`
}
