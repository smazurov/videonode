package models

import "time"

// UpdateCheckData contains information about available updates.
type UpdateCheckData struct {
	CurrentVersion  string    `json:"current_version" example:"1.0.0" doc:"Currently installed version"`
	LatestVersion   string    `json:"latest_version" example:"1.1.0" doc:"Latest available version"`
	ReleaseNotes    string    `json:"release_notes" doc:"Markdown release notes"`
	ReleaseURL      string    `json:"release_url" doc:"URL to the release page"`
	PublishedAt     time.Time `json:"published_at" doc:"When the release was published"`
	AssetSize       int       `json:"asset_size" example:"5242880" doc:"Size of the update in bytes"`
	UpdateAvailable bool      `json:"update_available" example:"true" doc:"Whether an update is available"`
}

// UpdateCheckResponse wraps UpdateCheckData for API responses.
type UpdateCheckResponse struct {
	Body UpdateCheckData
}

// UpdateStatusData contains the current update state.
type UpdateStatusData struct {
	State           string     `json:"state" example:"idle" doc:"Current update state"`
	CurrentVersion  string     `json:"current_version" example:"1.0.0" doc:"Current version"`
	TargetVersion   string     `json:"target_version,omitempty" example:"1.1.0" doc:"Version being updated to"`
	Progress        int        `json:"progress,omitempty" example:"45" doc:"Progress percentage (0-100)"`
	Error           string     `json:"error,omitempty" doc:"Error message if in error state"`
	LastChecked     *time.Time `json:"last_checked,omitempty" doc:"When updates were last checked"`
	BackupAvailable bool       `json:"backup_available" example:"true" doc:"Whether a backup is available"`
	BackupVersion   string     `json:"backup_version,omitempty" example:"1.0.0" doc:"Version of the backup"`
}

// UpdateStatusResponse wraps UpdateStatusData for API responses.
type UpdateStatusResponse struct {
	Body UpdateStatusData
}

// UpdateApplyResponse represents a successful update apply response.
type UpdateApplyResponse struct {
	Body struct {
		Message string `json:"message" example:"Update applied, restarting..." doc:"Status message"`
	}
}

// UpdateRollbackResponse represents a successful rollback response.
type UpdateRollbackResponse struct {
	Body struct {
		Message string `json:"message" example:"Rollback complete, restarting..." doc:"Status message"`
	}
}

// RestartResponse represents a successful restart response.
type RestartResponse struct {
	Body struct {
		Message string `json:"message" example:"Restarting..." doc:"Status message"`
	}
}

// ApplyDevBuildResponse represents a successful dev build apply response.
type ApplyDevBuildResponse struct {
	Body struct {
		Message string `json:"message" example:"Dev build applied, restarting..." doc:"Status message"`
	}
}
