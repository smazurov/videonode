// Package models — Sensor API request/response types.
//
// SensorData mirrors the canonical pipeline.Sensor type: a first-class
// perception entity that observes one upstream ref, runs a detector, and emits
// findings. It carries its own detector + commit policy and runs whenever the
// pipeline switch is on — unattached it still emits findings + status; a
// composer input's auto_crop effect binds it to a crop target by selecting it.
package models

import "time"

// SensorData represents a perception sensor.
//
// Bindings is the denormalized cross-entity rollup — every composer input
// whose auto_crop effect currently selects this sensor. Computed server-side on
// every Get/List so the UI can show what a sensor drives without a client-side
// join.
type SensorData struct {
	SensorID      string            `json:"id" example:"playfield" doc:"Stable sensor identifier (kebab-case)"`
	Source        string            `json:"source" example:"source:overhead-cam" doc:"Observed upstream ref (source:<id> or composer:<id>)"`
	Detector      string            `json:"detector,omitempty" example:"uv run sensors/playfield/detect.py" doc:"Detector child command (the swappable Python/native runtime; empty = daemon default)"`
	ModelID       string            `json:"model_id,omitempty" example:"playfield-classical-v0" doc:"Model id that tags emitted findings; empty = daemon default"`
	Mode          string            `json:"mode,omitempty" enum:"propose,auto" example:"auto" doc:"propose (emit candidates for confirm) or auto (apply crop directly)"`
	Margin        float64           `json:"margin,omitempty" example:"0.1" doc:"Fractional bleed kept around the detected region (0.1 = 10%)"`
	MinConfidence float64           `json:"min_confidence,omitempty" example:"0.8" doc:"Detection confidence floor below which the crop holds / widens"`
	TickMs        int               `json:"tick_ms,omitempty" example:"200" doc:"Periodic re-detect cadence in ms; 0 = binary default"`
	Bindings      []SensorReference `json:"bindings,omitempty" doc:"Composer inputs whose auto_crop effect selects this sensor. Server-denormalized."`
	Status        ProcessStatus     `json:"status,omitempty" example:"running" enum:"idle,starting,running,stopping,error" doc:"Process pool state"`
	CreatedAt     time.Time         `json:"created_at,omitzero" doc:"When the sensor record was created"`
	UpdatedAt     time.Time         `json:"updated_at,omitzero" doc:"When the sensor record was last updated"`
}

// SensorListData wraps a list of sensors for the index endpoint.
type SensorListData struct {
	Sensors []SensorData `json:"sensors" doc:"List of configured sensors"`
	Count   int          `json:"count" example:"2" doc:"Number of sensors returned"`
}

// SensorListResponse is the HTTP response wrapper for SensorListData.
type SensorListResponse struct {
	Body SensorListData
}

// SensorResponse is the HTTP response wrapper for a single SensorData.
type SensorResponse struct {
	Body SensorData
}

// SensorCreateBody is the create-sensor request payload.
type SensorCreateBody struct {
	SensorID      string  `json:"id" minLength:"1" maxLength:"64" pattern:"^[a-z0-9][a-z0-9-]*$" example:"playfield" doc:"Stable sensor identifier (kebab-case)"`
	Source        string  `json:"source" example:"source:overhead-cam" doc:"Observed upstream ref (source:<id> or composer:<id>)"`
	Detector      string  `json:"detector,omitempty" example:"uv run sensors/playfield/detect.py" doc:"Detector child command; empty = daemon default"`
	ModelID       string  `json:"model_id,omitempty" doc:"Model id that tags emitted findings; empty = daemon default"`
	Mode          string  `json:"mode,omitempty" enum:"propose,auto" example:"auto" doc:"propose or auto"`
	Margin        float64 `json:"margin,omitempty" example:"0.1" doc:"Fractional bleed around the detected region"`
	MinConfidence float64 `json:"min_confidence,omitempty" example:"0.8" doc:"Detection confidence floor"`
	TickMs        int     `json:"tick_ms,omitempty" example:"200" doc:"Periodic re-detect cadence in ms; 0 = binary default"`
}

// SensorCreateRequest wraps SensorCreateBody for Huma input parsing.
type SensorCreateRequest struct {
	Body SensorCreateBody
}

// SensorUpdateBody is the partial-update payload. Fields are pointers so the
// handler can distinguish "not sent" from "set to zero value".
type SensorUpdateBody struct {
	Source        *string  `json:"source,omitempty" example:"source:overhead-cam" doc:"New observed upstream ref"`
	Detector      *string  `json:"detector,omitempty" doc:"New detector child command; empty string resets to daemon default"`
	ModelID       *string  `json:"model_id,omitempty" doc:"New model id"`
	Mode          *string  `json:"mode,omitempty" enum:"propose,auto" doc:"New commit mode"`
	Margin        *float64 `json:"margin,omitempty" doc:"New margin"`
	MinConfidence *float64 `json:"min_confidence,omitempty" doc:"New confidence floor"`
	TickMs        *int     `json:"tick_ms,omitempty" doc:"New re-detect cadence in ms"`
}

// SensorUpdateRequest wraps SensorUpdateBody plus the path parameter.
type SensorUpdateRequest struct {
	SensorID string           `path:"sensor_id" example:"playfield" doc:"Sensor identifier"`
	Body     SensorUpdateBody `body:"body"`
}

// SensorReference identifies a composer input that selects this sensor.
// Surfaced through the huma error envelope as ErrorDetail entries when DELETE
// returns 409, and via the denormalized SensorData.Bindings field.
type SensorReference struct {
	Kind  string `json:"kind" enum:"composer" example:"composer" doc:"Always composer (the only thing that binds a sensor)"`
	ID    string `json:"id" example:"main-scene" doc:"Referencing composer identifier"`
	Input string `json:"input,omitempty" example:"source:overhead-cam" doc:"The composer input whose auto_crop effect selects this sensor"`
}
