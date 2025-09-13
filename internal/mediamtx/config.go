package mediamtx

import "sync"

// Config holds configuration for MediaMTX integration
type Config struct {
	EnableLogging bool
}

var (
	globalConfig = &Config{
		EnableLogging: true, // Default to enabled
	}
	configOnce sync.Once
)

// SetConfig sets the global configuration for the mediamtx package
// This should be called once at startup from main.go
func SetConfig(cfg *Config) {
	configOnce.Do(func() {
		if cfg != nil {
			globalConfig = cfg
		}
	})
}
