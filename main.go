package main

import (
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2/humacli"
	"github.com/smazurov/videonode/cmd"
	"github.com/smazurov/videonode/internal/api"
	"github.com/smazurov/videonode/internal/config"
	streamconfig "github.com/smazurov/videonode/internal/config"
	"github.com/smazurov/videonode/internal/obs"
	"github.com/smazurov/videonode/internal/obs/collectors"
	"github.com/smazurov/videonode/internal/obs/exporters"
	"github.com/smazurov/videonode/internal/streams"
)

// Options for the CLI - flat structure with toml mapping
type Options struct {
	Config string `help:"Path to configuration file" short:"c" default:"config.toml"`

	// Server settings
	Port string `help:"Port to listen on" short:"p" default:":8090" toml:"server.port" env:"SERVER_PORT"`

	// Streams settings
	StreamsConfigFile string `help:"Stream definitions file" default:"streams.toml" toml:"streams.config_file" env:"STREAMS_CONFIG_FILE"`
	MediamtxConfig    string `help:"MediaMTX config file" default:"mediamtx.yml" toml:"streams.mediamtx_config" env:"STREAMS_MEDIAMTX_CONFIG"`

	// Observability settings
	ObsRetentionDuration     string `help:"Metrics retention" default:"12h" toml:"obs.retention_duration" env:"OBS_RETENTION_DURATION"`
	ObsMaxPointsPerSeries    int    `help:"Max points per series" default:"43200" toml:"obs.max_points_per_series" env:"OBS_MAX_POINTS_PER_SERIES"`
	ObsMaxSeries             int    `help:"Max series count" default:"5000" toml:"obs.max_series" env:"OBS_MAX_SERIES"`
	ObsWorkerCount           int    `help:"Worker threads" default:"2" toml:"obs.worker_count" env:"OBS_WORKER_COUNT"`
	ObsSystemMetricsInterval string `help:"System metrics interval" default:"30s" toml:"obs.system_metrics_interval" env:"OBS_SYSTEM_METRICS_INTERVAL"`
	ObsNetworkInterfaces     string `help:"Network interfaces to monitor (comma-separated)" default:"lo,wlp12s0,enp14s0" toml:"obs.network_interfaces" env:"OBS_NETWORK_INTERFACES"`
	ObsPrometheusEnabled     bool   `help:"Enable Prometheus" default:"true" toml:"obs.prometheus_enabled" env:"OBS_PROMETHEUS_ENABLED"`
	ObsSSEEnabled            bool   `help:"Enable SSE" default:"true" toml:"obs.sse_enabled" env:"OBS_SSE_ENABLED"`

	// Encoders settings
	EncodersValidationOutput string `help:"Validation output file" default:"validated_encoders.toml" toml:"encoders.validation_output" env:"ENCODERS_VALIDATION_OUTPUT"`

	// Capture settings
	CaptureDefaultDelayMs int `help:"Default capture delay in milliseconds" default:"3000" toml:"capture.default_delay_ms" env:"CAPTURE_DEFAULT_DELAY_MS"`

	// Auth settings
	AuthUsername string `help:"Basic auth username" default:"admin" toml:"auth.username" env:"AUTH_USERNAME"`
	AuthPassword string `help:"Basic auth password" default:"password" toml:"auth.password" env:"AUTH_PASSWORD"`

	// Logging settings
	LoggingObs string `help:"Observability logging level (debug, info, warn, error)" default:"info" toml:"logging.obs" env:"LOGGING_OBS"`
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
				DataChanSize:  10000,
				WorkerCount:   opts.ObsWorkerCount,
				FlushInterval: 5 * time.Second,
				LogLevel:      opts.LoggingObs,
			}

			obsManager = obs.NewManager(obsConfig)

			// Add collectors
			// Parse system metrics interval
			systemMetricsInterval, err := time.ParseDuration(opts.ObsSystemMetricsInterval)
			if err != nil {
				systemMetricsInterval = 30 * time.Second
			}

			// Add system metrics collector
			systemCollector := collectors.NewSystemCollector()

			// Configure network interfaces
			if opts.ObsNetworkInterfaces != "" {
				interfaces := strings.Split(opts.ObsNetworkInterfaces, ",")
				for i, iface := range interfaces {
					interfaces[i] = strings.TrimSpace(iface)
				}
				systemCollector.SetNetworkInterfaces(interfaces)
			}

			systemCollector.UpdateConfig(obs.CollectorConfig{
				Name:     "system",
				Enabled:  true,
				Interval: systemMetricsInterval,
				Labels:   obs.Labels{"service": "videonode", "instance": "default"},
			})
			if err := obsManager.AddCollector(systemCollector); err != nil {
				log.Printf("Warning: Failed to add system collector: %v", err)
			}

			// MediaMTX metrics are collected via Prometheus scraping

			// Add Prometheus scraper for MediaMTX metrics
			promCollector := collectors.NewPrometheusCollector("http://localhost:9998/metrics")
			promCollector.SetMetricFilter("^(paths|paths_bytes|rtmp|webrtc).*")
			promCollector.UpdateConfig(obs.CollectorConfig{
				Name:     "prometheus_scraper",
				Enabled:  true,
				Interval: 15 * time.Second,
				Labels:   obs.Labels{"service": "videonode", "instance": "default"},
			})
			if err := obsManager.AddCollector(promCollector); err != nil {
				log.Printf("Warning: Failed to add Prometheus collector: %v", err)
			}

			// Add exporters based on config
			if opts.ObsPrometheusEnabled {
				promExporter := exporters.NewPrometheusExporterV2()
				obsManager.AddExporter(promExporter)
			}

		}

		// Default command starts the server using existing API server
		// Initialize stream manager
		streamManager := streamconfig.NewStreamManager(opts.StreamsConfigFile)
		if err := streamManager.Load(); err != nil {
			fmt.Printf("Warning: Failed to load stream config: %v\n", err)
		}

		// Create stream service with OBS integration
		var streamService streams.StreamService
		if obsManager != nil {
			streamService = streams.NewStreamServiceWithOBS(streamManager, opts.MediamtxConfig,
				func(streamID, socketPath, logPath string) error {
					// Create FFmpeg collector and add to OBS manager
					ffmpegCollector := collectors.NewFFmpegCollector(socketPath, logPath, streamID)
					ffmpegCollector.UpdateConfig(obs.CollectorConfig{
						Name:     "ffmpeg_" + streamID,
						Enabled:  true,
						Interval: 0, // Event-driven
						Labels:   obs.Labels{"stream_id": streamID},
					})
					return obsManager.AddCollector(ffmpegCollector)
				},
				func(streamID string) error {
					collectorName := "ffmpeg_" + streamID
					return obsManager.RemoveCollector(collectorName)
				})
		} else {
			streamService = streams.NewStreamService(streamManager, opts.MediamtxConfig)
		}

		server := api.NewServer(&api.Options{
			AuthUsername:          opts.AuthUsername,
			AuthPassword:          opts.AuthPassword,
			CaptureDefaultDelayMs: opts.CaptureDefaultDelayMs,
			StreamService:         streamService,
		})

		// Wire up SSE exporter if OBS is enabled and SSE is configured
		if obsManager != nil && opts.ObsSSEEnabled {
			if sseBroadcaster := server.GetSSEBroadcaster(); sseBroadcaster != nil {
				sseExporter := exporters.NewSSEExporter(sseBroadcaster)
				sseExporter.SetLogLevel(opts.LoggingObs)
				obsManager.AddExporter(sseExporter)
			}
			log.Printf("Added SSE exporter to OBS manager")
		}

		hooks.OnStart(func() {
			// NOW start the OBS manager after all exporters are added (only when running server)
			if obsManager != nil {
				if err := obsManager.Start(); err != nil {
					log.Printf("Warning: Failed to start observability manager: %v", err)
				} else {
					log.Printf("Started OBS manager with collectors and exporters")
				}
			}

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
