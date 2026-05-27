package models

import "time"

// CanvasDimsData carries the composer output canvas dimensions and
// render rate. FPS is optional in create/patch requests; the daemon
// substitutes a default (60) when zero.
type CanvasDimsData struct {
	W   int `json:"w" example:"1920" doc:"Canvas width in pixels"`
	H   int `json:"h" example:"1080" doc:"Canvas height in pixels"`
	FPS int `json:"fps,omitempty" minimum:"0" maximum:"240" example:"60" doc:"Canvas frame rate (0 = daemon default)"`
}

// EffectData describes a per-input visual effect. Currently only the
// "perspective" type is wired, with a four-corner quad in the source's
// pixel-coordinate space. SnapshotW/SnapshotH tag that coord space
// (typically the source's native resolution as seen by the snapshot
// UI) so the composer can normalize corners to UV.
type EffectData struct {
	Type      string    `json:"type" example:"perspective" doc:"Effect type identifier"`
	Corners   [4][2]int `json:"corners,omitempty" doc:"Corner coordinates [tl, tr, br, bl] in source pixel space"`
	SnapshotW int       `json:"snapshot_w,omitempty" example:"1920" doc:"Source pixel width the corners are expressed in"`
	SnapshotH int       `json:"snapshot_h,omitempty" example:"1080" doc:"Source pixel height the corners are expressed in"`
}

// ComposerInputData is one composer input entry, referencing an upstream
// source by ref string ("source:<id>") with an optional effect.
type ComposerInputData struct {
	Ref    string      `json:"ref" example:"source:hdmi-slides" doc:"Upstream ref — source:<id>"`
	Effect *EffectData `json:"effect,omitempty" doc:"Optional per-input effect"`
}

// CropConfigData holds crop-mode positioning for the API layer.
type CropConfigData struct {
	X     float64 `json:"x" doc:"Normalized horizontal crop offset (0-1, 0.5 = centered)"`
	Y     float64 `json:"y" doc:"Normalized vertical crop offset (0-1, 0.5 = centered)"`
	Scale float64 `json:"scale" doc:"Source overfill factor (>= 1.0, 1.0 = minimum fill)"`
}

// LayoutSlotData places a composer input on the canvas. The Input field
// matches a ComposerInputData.Ref by name (not positional index).
type LayoutSlotData struct {
	Input           string          `json:"input" example:"source:hdmi-slides" doc:"Input ref this slot draws (matches inputs[].ref)"`
	X               int             `json:"x" example:"0" doc:"Slot top-left X in canvas pixels"`
	Y               int             `json:"y" example:"0" doc:"Slot top-left Y in canvas pixels"`
	W               int             `json:"w" example:"1920" doc:"Slot width in canvas pixels"`
	H               int             `json:"h" example:"1080" doc:"Slot height in canvas pixels"`
	Rotation        int             `json:"rotation,omitempty" example:"0" doc:"Clockwise rotation in degrees (0, 90, 180, 270)"`
	AspectRatioMode string          `json:"aspect_ratio_mode,omitempty" example:"stretch" enum:"stretch,fit,crop" doc:"How to scale source into slot (stretch, fit, crop)"`
	Crop            *CropConfigData `json:"crop,omitempty" doc:"Crop positioning (only meaningful when aspect_ratio_mode=crop)"`
}

// ComposerData is the full wire shape for a composer entity.
type ComposerData struct {
	ID                  string              `json:"id" example:"main-scene" doc:"Composer identifier"`
	Canvas              CanvasDimsData      `json:"canvas" doc:"Output canvas dimensions"`
	Inputs              []ComposerInputData `json:"inputs" doc:"Composer inputs (refs + optional effects)"`
	Layout              []LayoutSlotData    `json:"layout" doc:"Layout slots placing each input on the canvas"`
	DownstreamStreamIDs []string            `json:"downstream_stream_ids,omitempty" example:"[\"main-720p\",\"main-1080p\"]" republish:"stream" doc:"Server-denormalized list of stream IDs whose upstream is composer:<this>. Auto-republished via dependency graph when streams change."`
	Status              ProcessStatus       `json:"status,omitempty" example:"running" enum:"idle,starting,running,stopping,error" doc:"Process pool state"`
	CreatedAt           time.Time           `json:"created_at,omitzero" doc:"Creation timestamp"`
	UpdatedAt           time.Time           `json:"updated_at,omitzero" doc:"Last update timestamp"`
}

// ComposerListData wraps a list of composers with a count.
type ComposerListData struct {
	Composers []ComposerData `json:"composers" doc:"Configured composers"`
	Count     int            `json:"count" example:"1" doc:"Total composer count"`
}

// ComposerListResponse is the GET /api/composers response wrapper.
type ComposerListResponse struct {
	Body ComposerListData
}

// ComposerResponse is the single-composer response wrapper.
type ComposerResponse struct {
	Body ComposerData
}

// ComposerCreateRequest is the POST /api/composers body.
type ComposerCreateRequest struct {
	Body ComposerCreateRequestData
}

// ComposerCreateRequestData carries the create payload for a composer.
type ComposerCreateRequestData struct {
	ID     string              `json:"id" minLength:"1" maxLength:"64" pattern:"^[a-zA-Z0-9_-]+$" example:"main-scene" doc:"Composer identifier"`
	Canvas CanvasDimsData      `json:"canvas" doc:"Output canvas dimensions"`
	Inputs []ComposerInputData `json:"inputs" minItems:"1" doc:"Composer inputs"`
	Layout []LayoutSlotData    `json:"layout,omitempty" doc:"Initial layout slots (optional; defaults to empty)"`
}

// ComposerUpdateRequest is the PATCH /api/composers/{id} body.
type ComposerUpdateRequest struct {
	Body ComposerUpdateRequestData
}

// ComposerUpdateRequestData carries optional fields to patch on a composer.
// Nil fields are left untouched.
type ComposerUpdateRequestData struct {
	Canvas *CanvasDimsData     `json:"canvas,omitempty" doc:"New canvas dimensions"`
	Inputs []ComposerInputData `json:"inputs,omitempty" doc:"Replacement inputs list"`
	Layout []LayoutSlotData    `json:"layout,omitempty" doc:"Replacement layout (also validated against inputs)"`
}

// ComposerLayoutRequest is the PATCH /api/composers/{id}/layout body — the
// full replacement layout array, validated against the composer's inputs.
type ComposerLayoutRequest struct {
	Body ComposerLayoutRequestData
}

// ComposerLayoutRequestData wraps the replacement layout array.
type ComposerLayoutRequestData struct {
	Layout []LayoutSlotData `json:"layout" doc:"Full replacement layout array"`
}

// ComposerEffectRequest is the PATCH /api/composers/{id}/inputs/{ref}/effect body.
type ComposerEffectRequest struct {
	Body ComposerEffectRequestData
}

// ComposerEffectRequestData sets or clears the effect on a specific input.
// The Effect field is a three-state Nullable: omitted leaves it untouched,
// explicit null clears it, a value replaces it.
type ComposerEffectRequestData struct {
	Effect Nullable[EffectData] `json:"effect" doc:"New effect; null clears the existing effect"`
}

// ComposerDeleteConflictBody is the 409 body returned when a composer
// cannot be deleted because streams still reference it.
type ComposerDeleteConflictBody struct {
	Message            string   `json:"message" example:"composer in use" doc:"Conflict description"`
	ReferencingStreams []string `json:"referencing_streams" doc:"IDs of streams still referencing this composer"`
}
