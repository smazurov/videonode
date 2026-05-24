// Package pipeline owns the source/composer/stream pipeline model.
//
// The pipeline carries three independent entity kinds:
//   - Source: per-device frame producer (`videonode-source`), referenced
//     by id (`source:<id>`).
//   - Composer: per-canvas GLES compositor (`videonode-composer`),
//     referenced by id (`composer:<id>`).
//   - Stream: an encoder (`vn-sink | ffmpeg`) keyed by stream-id; its
//     Upstream string names exactly one Source or Composer.
//
// One Stream → exactly one supervised Encoder process. Sources and
// Composers are warm regardless of stream lifecycle. Stream-id == encoder
// identity end-to-end (`encoder:<stream-id>`, peer tracking, metrics).
//
// Data-model types live in this file; Stage interface in stage.go;
// stage implementations in producer.go / composer.go / encoder.go; the
// assembler in pipeline.go.
package pipeline

import "time"

// Stream is a slim encode-and-publish spec. The upstream graph
// (sources + optional composer) lives in separate top-level entities;
// the stream references whichever upstream it wants by id.
type Stream struct {
	ID   string `toml:"id" json:"id"`
	Name string `toml:"name" json:"name"`
	// Upstream is `source:<id>` or `composer:<id>` — the encoder dials
	// whichever SCM socket the referenced entity binds.
	Upstream          string          `toml:"upstream" json:"upstream"`
	Audio             AudioConfig     `toml:"audio,omitempty" json:"audio,omitempty"`
	Encoder           EncoderConfig   `toml:"encoder,omitempty" json:"encoder,omitempty"`
	Publish           []PublishTarget `toml:"publish,omitempty" json:"publish,omitempty"`
	CustomEncoderArgs string          `toml:"custom_encoder_args,omitempty" json:"custom_encoder_args,omitempty"`
	CreatedAt         time.Time       `toml:"created_at" json:"created_at"`
	UpdatedAt         time.Time       `toml:"updated_at" json:"updated_at"`
}

// AudioConfig is the per-stream audio routing. Devices are ALSA device
// names; each entry produces one output audio track in the published
// stream. RTSP/SRT/MPEG-TS all carry multi-track audio; SDP advertises
// one m=audio line per track.
type AudioConfig struct {
	Devices []string `toml:"devices,omitempty" json:"devices,omitempty"`
	Codec   string   `toml:"codec,omitempty" json:"codec,omitempty"`
	Bitrate string   `toml:"bitrate,omitempty" json:"bitrate,omitempty"`
	Filters string   `toml:"filters,omitempty" json:"filters,omitempty"`
}

// EncoderConfig is the backend-agnostic encoder hint. EncoderName,
// GlobalArgs and VideoFilters are populated upstream (typically by
// the service layer via encoders.MapAPICodec) so HW encoders that need
// extra plumbing carry that wiring through to ffmpeg.Params. Empty
// EncoderName falls back to libx264 inside the EncoderStage.
type EncoderConfig struct {
	Codec        string   `toml:"codec,omitempty" json:"codec,omitempty"`
	EncoderName  string   `toml:"encoder_name,omitempty" json:"encoder_name,omitempty"`
	GlobalArgs   []string `toml:"global_args,omitempty" json:"global_args,omitempty"`
	VideoFilters string   `toml:"video_filters,omitempty" json:"video_filters,omitempty"`
	Bitrate      string   `toml:"bitrate,omitempty" json:"bitrate,omitempty"`
	GOP          int      `toml:"gop,omitempty" json:"gop,omitempty"`
	BFrames      int      `toml:"b_frames,omitempty" json:"b_frames,omitempty"`
	RateControl  string   `toml:"rate_control,omitempty" json:"rate_control,omitempty"`
	Preset       string   `toml:"preset,omitempty" json:"preset,omitempty"`
}

// PublishTarget is a single output destination. Type discriminates the
// URL scheme (rtsp/srt/hls/...); URL is what the encoder writes to.
type PublishTarget struct {
	Type string `toml:"type" json:"type"`
	URL  string `toml:"url" json:"url"`
}

