package models

import "time"

// ComposerData is the canonical API representation of a Composer — a GLES
// BGRA compositor that fans N source frames into a single canvas dma-buf.
// Streams reference composers via "composer:<id>" upstream refs.
type ComposerData struct {
	ID        string              `json:"id" example:"main-scene" doc:"Unique composer identifier"`
	Canvas    CanvasDimsData      `json:"canvas" doc:"Output canvas dimensions"`
	Inputs    []ComposerInputData `json:"inputs" doc:"Input refs and per-input effects"`
	Layout    []LayoutSlotData    `json:"layout" doc:"Layout slot rectangles keyed by input ref"`
	CreatedAt time.Time           `json:"created_at,omitzero" doc:"When the composer was created"`
	UpdatedAt time.Time           `json:"updated_at,omitzero" doc:"When the composer was last updated"`
}

// CanvasDimsData is the output canvas resolution in pixels.
type CanvasDimsData struct {
	W int `json:"w" example:"1920" doc:"Canvas width"`
	H int `json:"h" example:"1080" doc:"Canvas height"`
}

// ComposerInputData is one upstream reference to a Source plus an optional
// per-input effect (e.g. perspective correction).
type ComposerInputData struct {
	Ref    string      `json:"ref" example:"source:hdmi-slides" doc:"Upstream source ref"`
	Effect *EffectData `json:"effect,omitempty" doc:"Optional per-input effect"`
}

// LayoutSlotData is one rectangle in canvas pixel space keyed by input ref
// (not positional — matches ComposerInputData.Ref by name).
type LayoutSlotData struct {
	Input string `json:"input" example:"source:hdmi-slides" doc:"Input ref this slot belongs to"`
	X     int    `json:"x" doc:"Slot rectangle X in canvas pixels"`
	Y     int    `json:"y" doc:"Slot rectangle Y in canvas pixels"`
	W     int    `json:"w" doc:"Slot rectangle width in canvas pixels"`
	H     int    `json:"h" doc:"Slot rectangle height in canvas pixels"`
}

// EffectData describes a per-input compositing effect.
type EffectData struct {
	Type    string    `json:"type" example:"perspective" doc:"Effect type discriminator"`
	Corners [4][2]int `json:"corners,omitempty" doc:"Perspective corners (top-left, top-right, bottom-right, bottom-left) when type == \"perspective\""`
}

// ComposerResponse wraps ComposerData for API responses.
type ComposerResponse struct {
	Body ComposerData
}

// ComposerListData contains a list of all configured composers.
type ComposerListData struct {
	Composers []ComposerData `json:"composers" doc:"All configured composers"`
	Count     int            `json:"count" doc:"Number of composers"`
}

// ComposerListResponse wraps ComposerListData for API responses.
type ComposerListResponse struct {
	Body ComposerListData
}

// ComposerRequestData is the payload for creating or replacing a Composer.
type ComposerRequestData struct {
	ID     string              `json:"id" pattern:"^[a-zA-Z0-9_-]+$" minLength:"1" maxLength:"50" example:"main-scene" doc:"Composer identifier"`
	Canvas CanvasDimsData      `json:"canvas" doc:"Output canvas dimensions"`
	Inputs []ComposerInputData `json:"inputs" doc:"Input refs and per-input effects"`
	Layout []LayoutSlotData    `json:"layout" doc:"Layout slot rectangles keyed by input ref"`
}

// ComposerRequest wraps ComposerRequestData for API requests.
type ComposerRequest struct {
	Body ComposerRequestData
}

// ComposerUpdateRequestData is the payload for patching a Composer.
type ComposerUpdateRequestData struct {
	Canvas *CanvasDimsData      `json:"canvas,omitempty" doc:"Output canvas dimensions"`
	Inputs *[]ComposerInputData `json:"inputs,omitempty" doc:"Input refs and per-input effects"`
	Layout *[]LayoutSlotData    `json:"layout,omitempty" doc:"Layout slot rectangles keyed by input ref"`
}

// ComposerUpdateRequest wraps ComposerUpdateRequestData for API requests.
type ComposerUpdateRequest struct {
	Body ComposerUpdateRequestData
}

// ComposerLayoutRequest is the payload for PATCH /api/composers/{id}/layout.
type ComposerLayoutRequest struct {
	Body struct {
		Layout []LayoutSlotData `json:"layout" doc:"Replacement layout slot rectangles"`
	}
}

// StreamSlimData is the v2 slim representation of a Stream — an encoder +
// audio + publish targets identified by stream-id. Unlike the legacy
// StreamData, it has no device/canvas/layout/effects fields. Upstream is
// resolved via a "source:<id>" or "composer:<id>" reference.
type StreamSlimData struct {
	StreamID          string              `json:"stream_id" example:"main-archive" doc:"Unique stream identifier"`
	Name              string              `json:"name,omitempty" example:"Main archive" doc:"Human-readable name"`
	Upstream          string              `json:"upstream" example:"composer:main-scene" doc:"Upstream ref (source:<id> or composer:<id>)"`
	Audio             AudioConfigData     `json:"audio" doc:"Audio routing configuration"`
	Encoder           EncoderConfigData   `json:"encoder" doc:"Encoder configuration"`
	Publish           []PublishTargetData `json:"publish,omitempty" doc:"Publish targets"`
	CustomEncoderArgs string              `json:"custom_encoder_args,omitempty" doc:"Custom encoder argument override"`
	Enabled           bool                `json:"enabled" doc:"Whether the stream is enabled"`
	RTSPURL           string              `json:"rtsp_url,omitempty" doc:"RTSP publish URL"`
	SRTURL            string              `json:"srt_url,omitempty" doc:"SRT publish URL"`
	CreatedAt         time.Time           `json:"created_at,omitzero" doc:"When the stream was created"`
	UpdatedAt         time.Time           `json:"updated_at,omitzero" doc:"When the stream was last updated"`
}

// AudioConfigData mirrors pipeline.AudioConfig at the API surface.
type AudioConfigData struct {
	Devices []string `json:"devices,omitempty" doc:"ALSA device names; one output track per entry"`
	Codec   string   `json:"codec,omitempty" example:"opus" doc:"Audio codec"`
	Bitrate string   `json:"bitrate,omitempty" example:"128k" doc:"Audio bitrate"`
	Filters string   `json:"filters,omitempty" doc:"Optional shared filter chain"`
}

// EncoderConfigData mirrors pipeline.EncoderConfig at the API surface.
type EncoderConfigData struct {
	Codec        string   `json:"codec,omitempty" example:"h265" doc:"Logical codec (h264/h265/av1)"`
	EncoderName  string   `json:"encoder_name,omitempty" doc:"Explicit ffmpeg encoder name"`
	GlobalArgs   []string `json:"global_args,omitempty" doc:"ffmpeg global args"`
	VideoFilters string   `json:"video_filters,omitempty" doc:"ffmpeg -vf filter chain"`
	Bitrate      string   `json:"bitrate,omitempty" example:"12M" doc:"Video bitrate"`
	GOP          int      `json:"gop,omitempty" example:"120" doc:"Keyframe interval"`
	BFrames      int      `json:"b_frames,omitempty" doc:"Number of B-frames"`
	RateControl  string   `json:"rate_control,omitempty" doc:"Rate control mode"`
	Preset       string   `json:"preset,omitempty" doc:"Encoder preset"`
}

// PublishTargetData mirrors pipeline.PublishTarget at the API surface.
type PublishTargetData struct {
	Type string `json:"type" example:"rtsp" doc:"Target type (rtsp/srt/hls/...)"`
	URL  string `json:"url" example:"rtsp://nas.lan:8554/archive/main" doc:"Destination URL"`
}

// StreamSlimResponse wraps StreamSlimData for API responses.
type StreamSlimResponse struct {
	Body StreamSlimData
}

// StreamSlimListData contains a list of v2 slim streams.
type StreamSlimListData struct {
	Streams []StreamSlimData `json:"streams" doc:"All configured streams"`
	Count   int              `json:"count" doc:"Number of streams"`
}

// StreamSlimListResponse wraps StreamSlimListData for API responses.
type StreamSlimListResponse struct {
	Body StreamSlimListData
}
