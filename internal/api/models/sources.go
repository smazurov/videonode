// Package models — Source API request/response types.
//
// SourceData mirrors the canonical pipeline.Source type defined by unit B1
// of the sources/composers/streams split: a top-level entity producing
// frames from either a V4L2 device or the RPC-driven test-pattern producer.
package models

import "time"

// SourceData represents a frame producer (V4L2 device or test pattern).
//
// Exactly one of Device or TestMode must be set; the daemon rejects both
// empty or both populated.
//
// Consumers is the denormalized cross-entity rollup — every composer
// and stream currently referencing this source. Computed server-side
// on every Get/List so the UI never needs to join across stores;
// auto-republished when a stream or composer ref changes via the
// events.OnLifecycle dependency graph.
type SourceData struct {
	SourceID  string            `json:"id" example:"hdmi-slides" doc:"Stable source identifier (kebab-case)"`
	Device    string            `json:"device,omitempty" example:"rk3588-hdmi-rx" doc:"Stable device identifier. Empty when test_mode is true."`
	TestMode  bool              `json:"test_mode,omitempty" example:"false" doc:"When true, swap the V4L2 producer for an RPC-driven test-pattern producer. Mutually exclusive with device."`
	Format    *SourceFormatBody `json:"format,omitempty" doc:"Operator-selected V4L2 capture format. Omit to let the source binary auto-negotiate."`
	Consumers []SourceReference `json:"consumers,omitempty" republish:"stream,composer" doc:"Composers and streams currently referencing this source. Server-denormalized; auto-republished when references change."`
	Status    ProcessStatus     `json:"status,omitempty" example:"running" enum:"idle,starting,running,stopping,error" doc:"Process pool state"`
	Liveness  SourceLiveness    `json:"liveness,omitempty" example:"live" enum:"live,transitioning,no_cable,no_signal,initializing,offline,unknown" doc:"Source-reported health, independent of the process pool state. offline when the process isn't running."`
	CreatedAt time.Time         `json:"created_at,omitzero" doc:"When the source record was created"`
	UpdatedAt time.Time         `json:"updated_at,omitzero" doc:"When the source record was last updated"`
}

// SourceFormatBody is the operator-selected V4L2 capture format the
// daemon pushes to videonode-source over gRPC SetFormat. FormatName is
// the lowercase VideoFormat (the same value /api/devices/{id}/formats
// returns); the API layer converts it to a 4-char V4L2 fourcc before
// dispatch.
type SourceFormatBody struct {
	FormatName VideoFormat `json:"format_name" example:"yuyv422" doc:"Lowercase video format name (matches /api/devices/{id}/formats)"`
	Width      uint32      `json:"width" example:"1920" doc:"Capture width in pixels"`
	Height     uint32      `json:"height" example:"1080" doc:"Capture height in pixels"`
	FPS        uint32      `json:"fps,omitempty" example:"30" doc:"Capture framerate; 0 = driver default"`
}

// SourceListData wraps a list of sources for the index endpoint.
type SourceListData struct {
	Sources []SourceData `json:"sources" doc:"List of configured sources"`
	Count   int          `json:"count" example:"3" doc:"Number of sources returned"`
}

// SourceListResponse is the HTTP response wrapper for SourceListData.
type SourceListResponse struct {
	Body SourceListData
}

// SourceResponse is the HTTP response wrapper for a single SourceData.
type SourceResponse struct {
	Body SourceData
}

// SourceCreateBody is the create-source request payload.
type SourceCreateBody struct {
	SourceID string            `json:"id" minLength:"1" maxLength:"64" pattern:"^[a-z0-9][a-z0-9-]*$" example:"hdmi-slides" doc:"Stable source identifier (kebab-case)"`
	Device   string            `json:"device,omitempty" example:"rk3588-hdmi-rx" doc:"Stable device identifier. Omit when test_mode is true."`
	TestMode bool              `json:"test_mode,omitempty" example:"false" doc:"When true, use the test-pattern producer instead of a V4L2 device."`
	Format   *SourceFormatBody `json:"format,omitempty" doc:"Initial V4L2 capture format. Omit to let the source auto-negotiate."`
}

// SourceCreateRequest wraps SourceCreateBody for Huma input parsing.
type SourceCreateRequest struct {
	Body SourceCreateBody
}

// SourceUpdateBody is the partial-update payload. Fields are pointers so
// the handler can distinguish "not sent" from "set to zero value".
type SourceUpdateBody struct {
	Device   *string           `json:"device,omitempty" example:"rk3588-hdmi-rx" doc:"New device identifier; clears when sent as empty string while test_mode is true"`
	TestMode *bool             `json:"test_mode,omitempty" example:"true" doc:"Toggle test-pattern mode"`
	Format   *SourceFormatBody `json:"format,omitempty" doc:"Replace the V4L2 capture format. Send null in a future revision to clear; today omitting leaves the prior format untouched."`
}

// SourceUpdateRequest wraps SourceUpdateBody plus the path parameter.
type SourceUpdateRequest struct {
	SourceID string           `path:"source_id" example:"hdmi-slides" doc:"Source identifier"`
	Body     SourceUpdateBody `body:"body"`
}

// SourceReferenceKind discriminates between composer- and stream-level
// references reported when a delete is refused.
type SourceReferenceKind string

// SourceReferenceKind constants.
const (
	SourceReferenceKindComposer SourceReferenceKind = "composer"
	SourceReferenceKindStream   SourceReferenceKind = "stream"
)

// SourceReference identifies an entity still using a source. Surfaced
// through the standard huma error envelope as ErrorDetail entries
// (Location = "<kind>:<id>") when DELETE returns 409, and via the
// denormalized SourceData.Consumers field on every Get/List response.
type SourceReference struct {
	Kind SourceReferenceKind `json:"kind" example:"composer" doc:"composer | stream"`
	ID   string              `json:"id" example:"main-scene" doc:"Referencing entity identifier"`
}
