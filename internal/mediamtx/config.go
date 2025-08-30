package mediamtx

import (
	"fmt"
	"os"

	"github.com/smazurov/videonode/internal/ffmpeg"
	"gopkg.in/yaml.v3"
)

// Config represents a MediaMTX configuration
type Config struct {
	API               bool           `yaml:"api"`
	APIAddress        string         `yaml:"apiAddress"`
	RTSPAddress       string         `yaml:"rtspAddress"`
	WebRTCAddress     string         `yaml:"webrtcAddress"`
	Metrics           bool           `yaml:"metrics"`
	MetricsAddress    string         `yaml:"metricsAddress"`
	AuthMethod        string         `yaml:"authMethod"`
	AuthInternalUsers []InternalUser `yaml:"authInternalUsers"`

	Paths map[string]PathConfig `yaml:"paths"`
}

// InternalUser represents an internal authentication user
type InternalUser struct {
	User        string       `yaml:"user"`
	Pass        string       `yaml:"pass"`
	IPs         []string     `yaml:"ips"`
	Permissions []Permission `yaml:"permissions"`
}

// Permission represents a user permission
type Permission struct {
	Action string `yaml:"action"`
	Path   string `yaml:"path,omitempty"`
}

// PathConfig represents a MediaMTX path configuration
type PathConfig struct {
	RunOnInit        string `yaml:"runOnInit,omitempty"`
	RunOnInitRestart bool   `yaml:"runOnInitRestart,omitempty"`
}

// StreamConfig represents the parameters for creating a stream (deprecated: use ffmpeg.StreamConfig)
type StreamConfig = ffmpeg.StreamConfig

// NewConfig creates a basic MediaMTX configuration
func NewConfig() *Config {
	return &Config{
		API:            true,
		APIAddress:     ":9997",
		RTSPAddress:    ":8554",
		WebRTCAddress:  ":8889",
		Metrics:        true,
		MetricsAddress: ":9998",
		AuthMethod:     "internal",
		AuthInternalUsers: []InternalUser{
			{
				User: "any",
				Pass: "",
				IPs:  []string{},
				Permissions: []Permission{
					{Action: "publish"},
					{Action: "read"},
					{Action: "playback"},
					{Action: "api"},
					{Action: "metrics"},
					{Action: "pprof"},
				},
			},
		},

		Paths: make(map[string]PathConfig),
	}
}

// AddStream adds a new stream path to the configuration
func (c *Config) AddStream(pathName string, streamConfig StreamConfig) error {
	if pathName == "" {
		return fmt.Errorf("path name cannot be empty")
	}

	// Use the progress socket path provided (TOML config is source of truth)

	// Generate FFmpeg command for MediaMTX
	ffmpegCmd, err := ffmpeg.GenerateCommand(streamConfig)
	if err != nil {
		return fmt.Errorf("failed to generate FFmpeg command: %w", err)
	}

	c.Paths[pathName] = PathConfig{
		RunOnInit:        ffmpegCmd,
		RunOnInitRestart: true,
	}

	return nil
}

// RemoveStream removes a stream path from the configuration
func (c *Config) RemoveStream(pathName string) {
	delete(c.Paths, pathName)
}

// WriteToFile writes the configuration to a YAML file
func (c *Config) WriteToFile(filename string) error {
	data, err := yaml.Marshal(c)
	if err != nil {
		return fmt.Errorf("failed to marshal config to YAML: %w", err)
	}

	err = os.WriteFile(filename, data, 0644)
	if err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

// LoadFromFile loads configuration from a YAML file
func LoadFromFile(filename string) (*Config, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		// If file doesn't exist, return a new config
		if os.IsNotExist(err) {
			return NewConfig(), nil
		}
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var config Config
	err = yaml.Unmarshal(data, &config)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal YAML config: %w", err)
	}

	// Ensure paths map is initialized
	if config.Paths == nil {
		config.Paths = make(map[string]PathConfig)
	}

	// Set default values for missing fields
	if config.APIAddress == "" {
		config.APIAddress = ":9997"
	}
	if config.RTSPAddress == "" {
		config.RTSPAddress = ":8554"
	}
	if config.WebRTCAddress == "" {
		config.WebRTCAddress = ":8889"
	}
	if config.MetricsAddress == "" {
		config.MetricsAddress = ":9998"
		config.Metrics = true
	}

	return &config, nil
}

// GetWebRTCURL returns the WebRTC URL for a given path (without hostname)
func GetWebRTCURL(pathName string) string {
	return fmt.Sprintf(":8889/%s", pathName)
}

// GetRTSPURL returns the RTSP URL for a given path (without hostname)
func GetRTSPURL(pathName string) string {
	return fmt.Sprintf(":8554/%s", pathName)
}
