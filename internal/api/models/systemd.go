package models

// SystemdServiceStatus contains the status information for a systemd service.
type SystemdServiceStatus struct {
	Service string `json:"service" example:"mediamtx" doc:"Service name"`
	Status  string `json:"status" example:"active" doc:"Service status (active, inactive, failed, etc.)"`
}

// SystemdServiceStatusResponse wraps SystemdServiceStatus for API responses.
type SystemdServiceStatusResponse struct {
	Body SystemdServiceStatus
}

// SystemdServiceAction contains the result of a systemd service action.
type SystemdServiceAction struct {
	Service string `json:"service" example:"mediamtx" doc:"Service name"`
	Action  string `json:"action" example:"restart" doc:"Action performed (start, stop, restart)"`
	Success bool   `json:"success" example:"true" doc:"Whether the action succeeded"`
}

// SystemdServiceActionResponse wraps SystemdServiceAction for API responses.
type SystemdServiceActionResponse struct {
	Body SystemdServiceAction
}
