package models

import "time"

// SourceData is the canonical API representation of a Source — a producer of
// frames from a single device (or test pattern). Sources are referenced by
// composers and streams via "source:<id>" upstream refs.
type SourceData struct {
	ID        string    `json:"id" example:"hdmi-slides" doc:"Unique source identifier"`
	Device    string    `json:"device,omitempty" example:"rk3588-hdmi-rx" doc:"Stable device identifier"`
	TestMode  bool      `json:"test_mode,omitempty" doc:"Use RPC test-pattern producer instead of a real device"`
	CreatedAt time.Time `json:"created_at,omitzero" doc:"When the source was created"`
	UpdatedAt time.Time `json:"updated_at,omitzero" doc:"When the source was last updated"`
}

// SourceResponse wraps SourceData for API responses.
type SourceResponse struct {
	Body SourceData
}

// SourceListData contains a list of all configured sources.
type SourceListData struct {
	Sources []SourceData `json:"sources" doc:"All configured sources"`
	Count   int          `json:"count" doc:"Number of sources"`
}

// SourceListResponse wraps SourceListData for API responses.
type SourceListResponse struct {
	Body SourceListData
}

// SourceRequestData is the payload for creating or replacing a Source.
type SourceRequestData struct {
	ID       string `json:"id" pattern:"^[a-zA-Z0-9_-]+$" minLength:"1" maxLength:"50" example:"hdmi-slides" doc:"Source identifier"`
	Device   string `json:"device,omitempty" example:"rk3588-hdmi-rx" doc:"Stable device identifier"`
	TestMode bool   `json:"test_mode,omitempty" doc:"Use RPC test-pattern producer instead of a real device"`
}

// SourceRequest wraps SourceRequestData for API requests.
type SourceRequest struct {
	Body SourceRequestData
}

// SourceUpdateRequestData is the payload for patching a Source.
type SourceUpdateRequestData struct {
	Device   *string `json:"device,omitempty" doc:"Stable device identifier"`
	TestMode *bool   `json:"test_mode,omitempty" doc:"Use RPC test-pattern producer instead of a real device"`
}

// SourceUpdateRequest wraps SourceUpdateRequestData for API requests.
type SourceUpdateRequest struct {
	Body SourceUpdateRequestData
}
