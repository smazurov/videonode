package api

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/smazurov/videonode/internal/api/models"
)

func (s *Server) registerSystemdRoutes() {
	if s.options.SystemdManager == nil {
		return
	}

	serviceName := s.options.MediaMTXServiceName

	huma.Register(s.api, huma.Operation{
		OperationID: "get-mediamtx-status",
		Method:      http.MethodGet,
		Path:        "/api/systemd/mediamtx/status",
		Summary:     "MediaMTX Service Status",
		Description: "Get MediaMTX systemd service status",
		Tags:        []string{"systemd"},
		Security:    withAuth(),
	}, func(ctx context.Context, _ *struct{}) (*models.SystemdServiceStatusResponse, error) {
		status, err := s.options.SystemdManager.GetServiceStatus(ctx, serviceName)
		if err != nil {
			return nil, huma.Error500InternalServerError("Failed to get service status", err)
		}
		return &models.SystemdServiceStatusResponse{
			Body: models.SystemdServiceStatus{
				Service: "mediamtx",
				Status:  status,
			},
		}, nil
	})

	huma.Register(s.api, huma.Operation{
		OperationID: "restart-mediamtx",
		Method:      http.MethodPost,
		Path:        "/api/systemd/mediamtx/restart",
		Summary:     "Restart MediaMTX",
		Description: "Restart MediaMTX systemd service",
		Tags:        []string{"systemd"},
		Security:    withAuth(),
	}, func(ctx context.Context, _ *struct{}) (*models.SystemdServiceActionResponse, error) {
		err := s.options.SystemdManager.RestartService(ctx, serviceName)
		if err != nil {
			return nil, huma.Error500InternalServerError("Failed to restart service", err)
		}
		return &models.SystemdServiceActionResponse{
			Body: models.SystemdServiceAction{
				Service: "mediamtx",
				Action:  "restart",
				Success: true,
			},
		}, nil
	})

	huma.Register(s.api, huma.Operation{
		OperationID: "stop-mediamtx",
		Method:      http.MethodPost,
		Path:        "/api/systemd/mediamtx/stop",
		Summary:     "Stop MediaMTX",
		Description: "Stop MediaMTX systemd service",
		Tags:        []string{"systemd"},
		Security:    withAuth(),
	}, func(ctx context.Context, _ *struct{}) (*models.SystemdServiceActionResponse, error) {
		err := s.options.SystemdManager.StopService(ctx, serviceName)
		if err != nil {
			return nil, huma.Error500InternalServerError("Failed to stop service", err)
		}
		return &models.SystemdServiceActionResponse{
			Body: models.SystemdServiceAction{
				Service: "mediamtx",
				Action:  "stop",
				Success: true,
			},
		}, nil
	})

	huma.Register(s.api, huma.Operation{
		OperationID: "start-mediamtx",
		Method:      http.MethodPost,
		Path:        "/api/systemd/mediamtx/start",
		Summary:     "Start MediaMTX",
		Description: "Start MediaMTX systemd service",
		Tags:        []string{"systemd"},
		Security:    withAuth(),
	}, func(ctx context.Context, _ *struct{}) (*models.SystemdServiceActionResponse, error) {
		err := s.options.SystemdManager.StartService(ctx, serviceName)
		if err != nil {
			return nil, huma.Error500InternalServerError("Failed to start service", err)
		}
		return &models.SystemdServiceActionResponse{
			Body: models.SystemdServiceAction{
				Service: "mediamtx",
				Action:  "start",
				Success: true,
			},
		}, nil
	})
}
