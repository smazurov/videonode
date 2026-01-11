package api

import (
	"context"
	"errors"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/smazurov/videonode/internal/api/models"
	"github.com/smazurov/videonode/internal/updater"
	"github.com/smazurov/videonode/internal/version"
)

// registerUpdateRoutes registers all update-related endpoints.
func (s *Server) registerUpdateRoutes() {
	// Version endpoint - no auth required, always available
	huma.Register(s.api, huma.Operation{
		OperationID: "get-version",
		Method:      http.MethodGet,
		Path:        "/api/update/version",
		Summary:     "Version",
		Description: "Get application version information",
		Tags:        []string{"update"},
		Security:    []map[string][]string{}, // Empty security = no auth required
	}, func(_ context.Context, _ *struct{}) (*models.VersionResponse, error) {
		versionInfo := version.Get()
		return &models.VersionResponse{
			Body: models.VersionData{
				Version:   versionInfo.Version,
				GitCommit: versionInfo.GitCommit,
				BuildDate: versionInfo.BuildDate,
				BuildID:   versionInfo.BuildID,
				GoVersion: versionInfo.GoVersion,
				Compiler:  versionInfo.Compiler,
				Platform:  versionInfo.Platform,
			},
		}, nil
	})

	if s.options.UpdateService == nil {
		return
	}

	svc := s.options.UpdateService

	// Check if service is disabled
	if !svc.IsEnabled() {
		s.registerDisabledUpdateRoutes(svc.DisabledReason())
		return
	}

	// Check for updates
	huma.Register(s.api, huma.Operation{
		OperationID: "check-updates",
		Method:      http.MethodGet,
		Path:        "/api/update/check",
		Summary:     "Check for Updates",
		Description: "Check if a newer version is available without downloading",
		Tags:        []string{"update"},
		Errors:      []int{401, 409, 500},
		Security:    withAuth(),
	}, func(ctx context.Context, _ *struct{}) (*models.UpdateCheckResponse, error) {
		info, err := svc.CheckForUpdate(ctx)
		if err != nil {
			return nil, mapUpdateError(err)
		}
		return &models.UpdateCheckResponse{
			Body: models.UpdateCheckData{
				CurrentVersion:  info.CurrentVersion,
				LatestVersion:   info.LatestVersion,
				ReleaseNotes:    info.ReleaseNotes,
				ReleaseURL:      info.ReleaseURL,
				PublishedAt:     info.PublishedAt,
				AssetSize:       info.AssetSize,
				UpdateAvailable: info.UpdateAvailable,
			},
		}, nil
	})

	// Get update status
	huma.Register(s.api, huma.Operation{
		OperationID: "get-update-status",
		Method:      http.MethodGet,
		Path:        "/api/update/status",
		Summary:     "Get Update Status",
		Description: "Get the current update state and progress",
		Tags:        []string{"update"},
		Errors:      []int{401, 500},
		Security:    withAuth(),
	}, func(ctx context.Context, _ *struct{}) (*models.UpdateStatusResponse, error) {
		status := svc.GetStatus(ctx)
		return &models.UpdateStatusResponse{
			Body: models.UpdateStatusData{
				State:           string(status.State),
				CurrentVersion:  status.CurrentVersion,
				TargetVersion:   status.TargetVersion,
				Progress:        status.Progress,
				Error:           status.Error,
				LastChecked:     status.LastChecked,
				BackupAvailable: status.BackupAvailable,
				BackupVersion:   status.BackupVersion,
			},
		}, nil
	})

	// Apply update
	huma.Register(s.api, huma.Operation{
		OperationID: "apply-update",
		Method:      http.MethodPost,
		Path:        "/api/update/apply",
		Summary:     "Apply Update",
		Description: "Download and apply the available update. Will trigger a restart.",
		Tags:        []string{"update"},
		Errors:      []int{400, 401, 409, 500},
		Security:    withAuth(),
	}, func(ctx context.Context, _ *struct{}) (*models.UpdateApplyResponse, error) {
		if err := svc.ApplyUpdate(ctx); err != nil {
			return nil, mapUpdateError(err)
		}
		return &models.UpdateApplyResponse{
			Body: struct {
				Message string `json:"message" example:"Update applied, restarting..." doc:"Status message"`
			}{
				Message: "Update applied, restarting...",
			},
		}, nil
	})

	// Rollback to previous version
	huma.Register(s.api, huma.Operation{
		OperationID: "rollback-update",
		Method:      http.MethodPost,
		Path:        "/api/update/rollback",
		Summary:     "Rollback Update",
		Description: "Revert to the previously backed up version. Will trigger a restart.",
		Tags:        []string{"update"},
		Errors:      []int{400, 401, 404, 500},
		Security:    withAuth(),
	}, func(ctx context.Context, _ *struct{}) (*models.UpdateRollbackResponse, error) {
		if err := svc.Rollback(ctx); err != nil {
			return nil, mapUpdateError(err)
		}
		return &models.UpdateRollbackResponse{
			Body: struct {
				Message string `json:"message" example:"Rollback complete, restarting..." doc:"Status message"`
			}{
				Message: "Rollback complete, restarting...",
			},
		}, nil
	})

	// Restart service
	huma.Register(s.api, huma.Operation{
		OperationID: "restart-service",
		Method:      http.MethodPost,
		Path:        "/api/update/restart",
		Summary:     "Restart Service",
		Description: "Trigger a service restart.",
		Tags:        []string{"update"},
		Errors:      []int{401, 500},
		Security:    withAuth(),
	}, func(ctx context.Context, _ *struct{}) (*models.RestartResponse, error) {
		if err := svc.Restart(ctx); err != nil {
			return nil, huma.Error500InternalServerError(err.Error())
		}
		return &models.RestartResponse{
			Body: struct {
				Message string `json:"message" example:"Restarting..." doc:"Status message"`
			}{
				Message: "Restarting...",
			},
		}, nil
	})
}

// registerDisabledUpdateRoutes registers endpoints that return 503 when update is disabled.
func (s *Server) registerDisabledUpdateRoutes(reason string) {
	disabledHandler := func(_ context.Context, _ *struct{}) (*struct{}, error) {
		return nil, huma.Error503ServiceUnavailable("Update service disabled: " + reason)
	}

	huma.Register(s.api, huma.Operation{
		OperationID: "check-updates",
		Method:      http.MethodGet,
		Path:        "/api/update/check",
		Summary:     "Check for Updates",
		Description: "Check if a newer version is available (disabled)",
		Tags:        []string{"update"},
		Errors:      []int{503},
		Security:    withAuth(),
	}, disabledHandler)

	huma.Register(s.api, huma.Operation{
		OperationID: "get-update-status",
		Method:      http.MethodGet,
		Path:        "/api/update/status",
		Summary:     "Get Update Status",
		Description: "Get the current update state (disabled)",
		Tags:        []string{"update"},
		Errors:      []int{503},
		Security:    withAuth(),
	}, disabledHandler)

	huma.Register(s.api, huma.Operation{
		OperationID: "apply-update",
		Method:      http.MethodPost,
		Path:        "/api/update/apply",
		Summary:     "Apply Update",
		Description: "Apply update (disabled)",
		Tags:        []string{"update"},
		Errors:      []int{503},
		Security:    withAuth(),
	}, disabledHandler)

	huma.Register(s.api, huma.Operation{
		OperationID: "rollback-update",
		Method:      http.MethodPost,
		Path:        "/api/update/rollback",
		Summary:     "Rollback Update",
		Description: "Rollback update (disabled)",
		Tags:        []string{"update"},
		Errors:      []int{503},
		Security:    withAuth(),
	}, disabledHandler)
}

// mapUpdateError converts updater errors to Huma HTTP errors.
func mapUpdateError(err error) error {
	var updateErr *updater.Error
	if errors.As(err, &updateErr) {
		switch updateErr.Code {
		case updater.ErrCodeInvalidState:
			return huma.Error409Conflict(updateErr.Message)
		case updater.ErrCodeNoUpdate:
			return huma.Error400BadRequest(updateErr.Message)
		case updater.ErrCodeNotFound:
			return huma.Error404NotFound(updateErr.Message)
		case updater.ErrCodeNoBackup:
			return huma.Error404NotFound(updateErr.Message)
		case updater.ErrCodeDisabled:
			return huma.Error503ServiceUnavailable(updateErr.Message)
		default:
			return huma.Error500InternalServerError(updateErr.Message)
		}
	}
	return huma.Error500InternalServerError(err.Error())
}
