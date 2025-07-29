package main

import (
	"fmt"
	"log"
	"time"

	"github.com/danielgtaylor/huma/v2/humacli"
	"github.com/prometheus/client_golang/prometheus"
	
	"github.com/smazurov/videonode/cmd"
	"github.com/smazurov/videonode/internal/api"
	"github.com/smazurov/videonode/internal/config"
	"github.com/smazurov/videonode/internal/obs"
	"github.com/smazurov/videonode/internal/obs/exporters"
)

// Options for the CLI - flat structure with toml mapping
type Options struct {
	Config string `help:"Path to configuration file" short:"c" default:"config.toml"`
	
	// Server settings
	Port  string `help:"Port to listen on" short:"p" default:":8090" toml:"server.port" env:"SERVER_PORT"`
	
	// Streams settings
	StreamsConfigFile  string `help:"Stream definitions file" default:"streams.toml" toml:"streams.config_file" env:"STREAMS_CONFIG_FILE"`
	MediamtxConfig     string `help:"MediaMTX config file" default:"mediamtx.yml" toml:"streams.mediamtx_config" env:"STREAMS_MEDIAMTX_CONFIG"`
	
	// Observability settings
	ObsRetentionDuration     string `help:"Metrics retention" default:"12h" toml:"obs.retention_duration" env:"OBS_RETENTION_DURATION"`
	ObsMaxPointsPerSeries    int    `help:"Max points per series" default:"43200" toml:"obs.max_points_per_series" env:"OBS_MAX_POINTS_PER_SERIES"`
	ObsMaxSeries             int    `help:"Max series count" default:"5000" toml:"obs.max_series" env:"OBS_MAX_SERIES"`
	ObsWorkerCount           int    `help:"Worker threads" default:"2" toml:"obs.worker_count" env:"OBS_WORKER_COUNT"`
	ObsSystemMetricsInterval string `help:"System metrics interval" default:"30s" toml:"obs.system_metrics_interval" env:"OBS_SYSTEM_METRICS_INTERVAL"`
	ObsPrometheusEnabled     bool   `help:"Enable Prometheus" default:"true" toml:"obs.prometheus_enabled" env:"OBS_PROMETHEUS_ENABLED"`
	ObsSSEEnabled            bool   `help:"Enable SSE" default:"true" toml:"obs.sse_enabled" env:"OBS_SSE_ENABLED"`
	
	// Encoders settings
	EncodersValidationOutput string `help:"Validation output file" default:"validated_encoders.toml" toml:"encoders.validation_output" env:"ENCODERS_VALIDATION_OUTPUT"`
	
	// Capture settings
	CaptureDefaultDelayMs int `help:"Default capture delay in milliseconds" default:"3000" toml:"capture.default_delay_ms" env:"CAPTURE_DEFAULT_DELAY_MS"`
	
	// Auth settings
	AuthUsername string `help:"Basic auth username" default:"admin" toml:"auth.username" env:"AUTH_USERNAME"`
	AuthPassword string `help:"Basic auth password" default:"password" toml:"auth.password" env:"AUTH_PASSWORD"`
}


func main() {
	// Create Huma CLI
	cli := humacli.New(func(hooks humacli.Hooks, opts *Options) {
		// Load configuration automatically
		if err := config.LoadConfig(opts); err != nil {
			log.Printf("Warning: Failed to load config: %v", err)
		}
		
		// Initialize observability system if enabled
		var obsManager *obs.Manager
		if opts.ObsPrometheusEnabled || opts.ObsSSEEnabled {
			// Parse retention duration
			retentionDuration, err := time.ParseDuration(opts.ObsRetentionDuration)
			if err != nil {
				retentionDuration = 12 * time.Hour
			}
			
			// Create config with user settings
			obsConfig := obs.ManagerConfig{
				StoreConfig: obs.StoreConfig{
					MaxRetentionDuration: retentionDuration,
					MaxPointsPerSeries:   opts.ObsMaxPointsPerSeries,
					MaxSeries:            opts.ObsMaxSeries,
					FlushInterval:        30 * time.Second,
				},
				DataChanSize:  5000,
				WorkerCount:   opts.ObsWorkerCount,
				FlushInterval: 5 * time.Second,
			}
			
			obsManager = obs.NewManager(obsConfig)
			
			// Add exporters based on config
			if opts.ObsPrometheusEnabled {
				promExporter := exporters.NewPrometheusExporter(prometheus.NewRegistry())
				obsManager.AddExporter(promExporter)
			}
			
			if opts.ObsSSEEnabled {
				// TODO: Need SSE broadcaster from server
				// sseExporter := exporters.NewSSEExporter(sseBroadcaster)
				// obsManager.AddExporter(sseExporter)
			}
			
			if err := obsManager.Start(); err != nil {
				log.Printf("Warning: Failed to initialize observability: %v", err)
			}
		}
		
		// Default command starts the server using existing API server
		server := api.NewServer(&api.Options{
			StreamsConfigFile:     opts.StreamsConfigFile,
			MediamtxConfig:        opts.MediamtxConfig,
			AuthUsername:          opts.AuthUsername,
			AuthPassword:          opts.AuthPassword,
			CaptureDefaultDelayMs: opts.CaptureDefaultDelayMs,
		})
		
		// Wire up SSE exporter if OBS is enabled and SSE is configured
		// TODO: Need to implement GetSSEBroadcaster method
		// if obsManager != nil && opts.ObsSSEEnabled {
		//     if sseBroadcaster := server.GetSSEBroadcaster(); sseBroadcaster != nil {
		//         sseExporter := exporters.NewSSEExporter(sseBroadcaster)
		//         obsManager.AddExporter(sseExporter)
		//     }
		// }
		
		hooks.OnStart(func() {
			fmt.Printf("Starting server on %s\n", opts.Port)
			if err := server.Start(opts.Port); err != nil {
				log.Fatalf("Failed to start server: %v", err)
			}
		})
		
		hooks.OnStop(func() {
			fmt.Println("Shutting down server...")
			if obsManager != nil {
				obsManager.Stop()
			}
		})
		
		// Configure validate-encoders command with config value
		cmd.SetValidationOutput(opts.EncodersValidationOutput)
	})
	
	// Add the existing validate-encoders command
	cli.Root().AddCommand(cmd.ValidateEncodersCmd)
	
	// Run the CLI
	cli.Run()
}
