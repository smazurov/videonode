package updater

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"sync"
	"syscall"
	"time"

	"github.com/creativeprojects/go-selfupdate"
	"github.com/smazurov/videonode/internal/logging"
	"github.com/smazurov/videonode/internal/version"
)

type service struct {
	repository     selfupdate.Repository
	repositorySlug string // e.g., "smazurov/videonode"
	updater        *selfupdate.Updater
	backupManager  *backupManager

	// State management
	mu            sync.RWMutex
	state         State
	latestRelease *selfupdate.Release
	lastChecked   *time.Time
	lastError     error

	// Disabled state
	enabled        bool
	disabledReason string

	// Restart state
	restartPending bool

	logger *slog.Logger
}

// NewService creates a new updater service.
// Returns nil if update is disabled due to permission issues.
func NewService(opts *Options) (Service, error) {
	logger := logging.GetLogger("updater")

	// Check write permission first
	canWrite, reason := checkWritePermission()
	if !canWrite {
		logger.Warn("Update service disabled", "reason", reason)
		return &service{
			enabled:        false,
			disabledReason: reason,
			state:          StateIdle,
			logger:         logger,
		}, nil
	}

	// Create GitHub source
	source, err := selfupdate.NewGitHubSource(selfupdate.GitHubConfig{})
	if err != nil {
		return nil, fmt.Errorf("failed to create GitHub source: %w", err)
	}

	// Parse repository slug
	repo := selfupdate.ParseSlug(opts.Repository)

	// Create updater
	updater, err := selfupdate.NewUpdater(selfupdate.Config{
		Source:     source,
		Prerelease: opts.Prerelease,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create updater: %w", err)
	}

	// Create backup manager
	backupMgr, err := newBackupManager(logger)
	if err != nil {
		logger.Warn("Failed to create backup manager", "error", err)
	}

	svc := &service{
		repository:     repo,
		repositorySlug: opts.Repository,
		updater:        updater,
		backupManager:  backupMgr,
		state:          StateIdle,
		enabled:        true,
		logger:         logger,
	}

	return svc, nil
}

func checkWritePermission() (bool, string) {
	exe, err := os.Executable()
	if err != nil {
		return false, fmt.Sprintf("failed to get executable path: %v", err)
	}

	// Resolve symlinks
	exe, err = filepath.EvalSymlinks(exe)
	if err != nil {
		return false, fmt.Sprintf("failed to resolve symlinks: %v", err)
	}

	dir := filepath.Dir(exe)

	// Try creating temp file in same directory
	tmp := filepath.Join(dir, ".videonode.update.test")
	f, err := os.Create(tmp)
	if err != nil {
		return false, fmt.Sprintf("no write permission to %s: %v", dir, err)
	}
	f.Close()
	os.Remove(tmp)
	return true, ""
}

// IsEnabled returns whether the update service is operational.
// Returns false if permission checks failed during initialization.
func (s *service) IsEnabled() bool {
	return s.enabled
}

// DisabledReason returns why the update service is disabled.
// Returns empty string if the service is enabled.
func (s *service) DisabledReason() string {
	return s.disabledReason
}

// CheckForUpdate queries GitHub for the latest release and compares
// it against the current version. Returns update info without downloading.
func (s *service) CheckForUpdate(ctx context.Context) (*UpdateInfo, error) {
	if !s.enabled {
		return nil, newError(ErrCodeDisabled, s.disabledReason, nil)
	}

	if !s.transitionTo(StateChecking, StateIdle, StateAvailable, StateError) {
		return nil, newError(ErrCodeInvalidState,
			fmt.Sprintf("cannot check for updates in state %s", s.getState()), nil)
	}

	currentVersion := version.Version

	release, found, err := s.updater.DetectLatest(ctx, s.repository)
	if err != nil {
		s.setError(err)
		return nil, newError(ErrCodeCheckFailed, "failed to check for updates", err)
	}

	now := time.Now()
	s.mu.Lock()
	s.lastChecked = &now
	s.mu.Unlock()

	if !found {
		s.setError(fmt.Errorf("repository not found or has no releases"))
		return nil, newError(ErrCodeNotFound, "repository not found or has no releases", nil)
	}

	// Compare versions - dev is always considered outdated
	isNewer := currentVersion == "dev" || release.GreaterThan(currentVersion)

	if !isNewer {
		s.transitionTo(StateIdle)
		return &UpdateInfo{
			CurrentVersion:  currentVersion,
			LatestVersion:   release.Version(),
			UpdateAvailable: false,
		}, nil
	}

	// Store release for later download
	s.mu.Lock()
	s.latestRelease = release
	s.mu.Unlock()
	s.transitionTo(StateAvailable)

	return &UpdateInfo{
		CurrentVersion:  currentVersion,
		LatestVersion:   release.Version(),
		ReleaseNotes:    release.ReleaseNotes,
		ReleaseURL:      release.URL,
		PublishedAt:     release.PublishedAt,
		AssetSize:       release.AssetByteSize,
		UpdateAvailable: true,
	}, nil
}

// ApplyUpdate downloads and applies the latest update. Creates a backup
// of the current binary first, then replaces it. Triggers SIGTERM to
// restart via systemd after successful application.
func (s *service) ApplyUpdate(ctx context.Context) error {
	if !s.enabled {
		return newError(ErrCodeDisabled, s.disabledReason, nil)
	}

	// If in idle state, check for updates first
	if s.getState() == StateIdle {
		info, err := s.CheckForUpdate(ctx)
		if err != nil {
			return err
		}
		if !info.UpdateAvailable {
			return newError(ErrCodeNoUpdate, "no update available", nil)
		}
	}

	if !s.transitionTo(StateDownloading, StateAvailable) {
		return newError(ErrCodeInvalidState,
			fmt.Sprintf("cannot apply update in state %s", s.getState()), nil)
	}

	// Create backup before applying
	if s.backupManager != nil {
		if err := s.backupManager.createBackup(); err != nil {
			s.setError(err)
			return newError(ErrCodeBackupFailed, "failed to create backup", err)
		}
	}

	s.transitionTo(StateApplying)

	// Get executable path
	exe, err := selfupdate.ExecutablePath()
	if err != nil {
		s.setError(err)
		s.attemptRollback()
		return newError(ErrCodeApplyFailed, "failed to get executable path", err)
	}

	s.mu.RLock()
	release := s.latestRelease
	s.mu.RUnlock()

	// Apply the update
	if err := s.updater.UpdateTo(ctx, release, exe); err != nil {
		s.setError(err)
		s.attemptRollback()
		return newError(ErrCodeApplyFailed, "failed to apply update", err)
	}

	s.transitionTo(StateRestarting)
	s.logger.Info("Update applied successfully, triggering restart",
		"version", release.Version())

	// Trigger restart after short delay to allow response to be sent
	go func() {
		time.Sleep(500 * time.Millisecond)
		s.triggerRestart()
	}()

	return nil
}

// Rollback restores the previously backed up binary version.
// Triggers SIGTERM to restart via systemd after restoration.
func (s *service) Rollback(_ context.Context) error {
	if !s.enabled {
		return newError(ErrCodeDisabled, s.disabledReason, nil)
	}

	if s.backupManager == nil || !s.backupManager.hasBackup() {
		return newError(ErrCodeNoBackup, "no backup available for rollback", nil)
	}

	if err := s.backupManager.restore(); err != nil {
		return newError(ErrCodeRollbackFailed, "failed to restore backup", err)
	}

	s.transitionTo(StateRolledBack)
	s.logger.Info("Rollback completed, triggering restart")

	// Trigger restart
	go func() {
		time.Sleep(500 * time.Millisecond)
		s.triggerRestart()
	}()

	return nil
}

// GetStatus returns the current update state including version info,
// progress, any errors, and backup availability.
func (s *service) GetStatus(_ context.Context) *Status {
	s.mu.RLock()
	defer s.mu.RUnlock()

	status := &Status{
		State:          s.state,
		CurrentVersion: version.Version,
		LastChecked:    s.lastChecked,
	}

	if s.latestRelease != nil {
		status.TargetVersion = s.latestRelease.Version()
	}

	if s.lastError != nil {
		status.Error = s.lastError.Error()
	}

	if s.backupManager != nil {
		status.BackupAvailable = s.backupManager.hasBackup()
		status.BackupVersion = s.backupManager.backupVersion()
	}

	return status
}

func (s *service) transitionTo(newState State, validFromStates ...State) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(validFromStates) > 0 && !slices.Contains(validFromStates, s.state) {
		return false
	}

	s.logger.Debug("State transition", "from", s.state, "to", newState)
	s.state = newState
	s.lastError = nil
	return true
}

func (s *service) getState() State {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.state
}

func (s *service) setError(err error) {
	s.mu.Lock()
	s.lastError = err
	s.state = StateError
	s.mu.Unlock()
}

func (s *service) attemptRollback() {
	if s.backupManager == nil || !s.backupManager.hasBackup() {
		s.logger.Error("No backup available for automatic rollback")
		return
	}

	if err := s.backupManager.restore(); err != nil {
		s.logger.Error("Failed to restore backup", "error", err)
		return
	}

	s.transitionTo(StateRolledBack)
	s.logger.Info("Automatic rollback completed")
}

func (s *service) triggerRestart() {
	s.mu.Lock()
	s.restartPending = true
	s.mu.Unlock()

	proc, err := os.FindProcess(os.Getpid())
	if err != nil {
		s.logger.Error("Failed to find own process", "error", err)
		return
	}

	s.logger.Info("Sending SIGTERM to trigger restart")
	if err := proc.Signal(syscall.SIGTERM); err != nil {
		s.logger.Error("Failed to send SIGTERM", "error", err)
	}
}

// IsRestartPending returns whether a restart was triggered by this service.
func (s *service) IsRestartPending() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.restartPending
}

// Restart triggers a service restart.
func (s *service) Restart(_ context.Context) error {
	s.logger.Info("Restart requested")
	go func() {
		time.Sleep(500 * time.Millisecond)
		s.triggerRestart()
	}()
	return nil
}

// ApplyDevBuild downloads and applies the latest dev build from the rolling "dev" release.
func (s *service) ApplyDevBuild(ctx context.Context) error {
	if !s.enabled {
		return newError(ErrCodeDisabled, s.disabledReason, nil)
	}

	if !s.transitionTo(StateDownloading, StateIdle, StateAvailable, StateError) {
		return newError(ErrCodeInvalidState,
			fmt.Sprintf("cannot apply dev build in state %s", s.getState()), nil)
	}

	// Create backup before applying
	if s.backupManager != nil {
		if err := s.backupManager.createBackup(); err != nil {
			s.setError(err)
			return newError(ErrCodeBackupFailed, "failed to create backup", err)
		}
	}

	s.transitionTo(StateApplying)

	// Get executable path
	exe, err := selfupdate.ExecutablePath()
	if err != nil {
		s.setError(err)
		s.attemptRollback()
		return newError(ErrCodeApplyFailed, "failed to get executable path", err)
	}

	// Construct URL for current architecture
	arch := runtime.GOARCH
	assetName := fmt.Sprintf("videonode_linux_%s.tar.gz", arch)
	url := fmt.Sprintf("https://github.com/%s/releases/download/dev/%s",
		s.repositorySlug, assetName)

	s.logger.Info("Downloading dev build", "url", url)

	// Download and apply using go-selfupdate
	if err := selfupdate.UpdateTo(ctx, url, assetName, exe); err != nil {
		s.setError(err)
		s.attemptRollback()
		return newError(ErrCodeApplyFailed, "failed to apply dev build", err)
	}

	s.transitionTo(StateRestarting)
	s.logger.Info("Dev build applied, triggering restart")

	go func() {
		time.Sleep(500 * time.Millisecond)
		s.triggerRestart()
	}()

	return nil
}
