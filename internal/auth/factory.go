package auth

import (
	"github.com/smazurov/videonode/internal/logging"
)

// New creates an authenticator based on configuration.
// Falls back to basic auth if Linux auth is unavailable.
func New(cfg Config, logger logging.Logger) Authenticator {
	switch cfg.Type {
	case "linux", "":
		auth := NewLinux(logger)
		if !auth.Available() {
			if logger != nil {
				logger.Warn("Linux auth unavailable (unix_chkpwd not found), falling back to basic")
			}
			return NewBasic(cfg.Username, cfg.Password)
		}
		if logger != nil {
			logger.Info("Using Linux authentication", "service_user", auth.ServiceUser())
		}
		return auth

	case "basic":
		if logger != nil {
			logger.Info("Using basic authentication")
		}
		return NewBasic(cfg.Username, cfg.Password)

	default:
		if logger != nil {
			logger.Warn("Unknown auth type, falling back to basic", "type", cfg.Type)
		}
		return NewBasic(cfg.Username, cfg.Password)
	}
}
