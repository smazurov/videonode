package pipeline

import "time"

// Sensor is a first-class perception entity, keyed by stable id. One
// `videonode-sensor` process per Sensor. A Sensor observes one upstream
// ref (`source:<id>` or `composer:<id>`), runs a detector child, and
// streams normalized Findings to the daemon. It holds its own detection +
// commit policy and runs whenever the pipeline switch is on — unattached
// (no composer binding) it still emits findings and live status; a composer
// input's `auto_crop` effect binds it to a crop target by selecting its ref.
type Sensor struct {
	ID            string    `toml:"id" json:"id"`
	Source        string    `toml:"source" json:"source"`
	Detector      string    `toml:"detector,omitempty" json:"detector,omitempty"`
	ModelID       string    `toml:"model_id,omitempty" json:"model_id,omitempty"`
	Mode          string    `toml:"mode,omitempty" json:"mode,omitempty"`
	Margin        float64   `toml:"margin,omitempty" json:"margin,omitempty"`
	MinConfidence float64   `toml:"min_confidence,omitempty" json:"min_confidence,omitempty"`
	TickMs        int       `toml:"tick_ms,omitempty" json:"tick_ms,omitempty"`
	CreatedAt     time.Time `toml:"created_at" json:"created_at"`
	UpdatedAt     time.Time `toml:"updated_at" json:"updated_at"`
}

// SensorRef returns the canonical reference form (`sensor:<id>`) a composer
// input's auto_crop effect uses to select this sensor — mirroring
// SourceIDFor's `source:<id>`.
func SensorRef(id string) string { return "sensor:" + id }
