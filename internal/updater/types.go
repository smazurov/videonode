package updater

import (
	"context"
	"time"
)

// State represents the current state of the update process.
type State string

// Update states.
const (
	StateIdle        State = "idle"
	StateChecking    State = "checking"
	StateAvailable   State = "available"
	StateDownloading State = "downloading"
	StateApplying    State = "applying"
	StateRestarting  State = "restarting"
	StateError       State = "error"
	StateRolledBack  State = "rolled_back"
)

// Service defines the interface for update operations.
type Service interface {
	// CheckForUpdate checks for available updates without downloading.
	CheckForUpdate(ctx context.Context) (*UpdateInfo, error)

	// ApplyUpdate downloads and applies an update, then triggers restart.
	ApplyUpdate(ctx context.Context) error

	// Rollback reverts to the previous version.
	Rollback(ctx context.Context) error

	// GetStatus returns current update state and info.
	GetStatus(ctx context.Context) *Status

	// IsEnabled returns whether the update service is enabled.
	// Returns false if permission check failed on startup.
	IsEnabled() bool

	// DisabledReason returns why the service is disabled, empty if enabled.
	DisabledReason() string
}

// UpdateInfo contains information about an available update.
type UpdateInfo struct {
	CurrentVersion  string    `json:"current_version"`
	LatestVersion   string    `json:"latest_version"`
	ReleaseNotes    string    `json:"release_notes"`
	ReleaseURL      string    `json:"release_url"`
	PublishedAt     time.Time `json:"published_at"`
	AssetSize       int       `json:"asset_size"`
	UpdateAvailable bool      `json:"update_available"`
}

// Status contains the current state of the updater.
type Status struct {
	State           State      `json:"state"`
	CurrentVersion  string     `json:"current_version"`
	TargetVersion   string     `json:"target_version,omitempty"`
	Progress        int        `json:"progress,omitempty"`
	Error           string     `json:"error,omitempty"`
	LastChecked     *time.Time `json:"last_checked,omitempty"`
	BackupAvailable bool       `json:"backup_available"`
	BackupVersion   string     `json:"backup_version,omitempty"`
}

// Options contains configuration for the updater service.
type Options struct {
	Repository string // GitHub repo slug (e.g., "smazurov/videonode")
	Prerelease bool   // Whether to include prereleases
}
