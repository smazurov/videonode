package auth

import (
	"github.com/smazurov/videonode/internal/logging"
)

// New creates an authenticator based on configuration. The result is wrapped
// in a short-TTL success cache so the API polling firehose does not re-run the
// (deliberately slow) password verification on every request.
// Falls back to basic auth if Linux auth is unavailable.
func New(cfg Config, logger logging.Logger) Authenticator {
	return WithCache(newUncached(cfg, logger), DefaultCacheTTL)
}

func newUncached(cfg Config, logger logging.Logger) Authenticator {
	switch cfg.Type {
	case "linux", "":
		auth := NewLinux(logger)
		if !auth.Available() {
			if logger != nil {
				logger.Warn("Linux auth unavailable (/etc/shadow not readable), falling back to basic")
			}
			return NewBasic(cfg.Username, cfg.Password)
		}
		if logger != nil {
			logger.Info("Using Linux authentication", logging.KeyServiceUser, auth.ServiceUser())
		}
		return auth

	case "basic":
		if logger != nil {
			logger.Info("Using basic authentication")
		}
		return NewBasic(cfg.Username, cfg.Password)

	default:
		if logger != nil {
			logger.Warn("Unknown auth type, falling back to basic", logging.KeyType, cfg.Type)
		}
		return NewBasic(cfg.Username, cfg.Password)
	}
}
