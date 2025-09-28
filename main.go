package main

import (
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/danielgtaylor/huma/v2/humacli"
	"github.com/smazurov/videonode/cmd"
	"github.com/smazurov/videonode/internal/api"
	"github.com/smazurov/videonode/internal/config"
	"github.com/smazurov/videonode/internal/logging"
	"github.com/smazurov/videonode/internal/mediamtx"
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

	// MediaMTX settings
	MediaMTXEnableLogging bool `help:"Enable systemd logging for ffmpeg commands" default:"true" toml:"mediamtx.enable_logging" env:"MEDIAMTX_ENABLE_LOGGING"`

	// Observability settings
	ObsRetentionDuration  string `help:"Metrics retention" default:"12h" toml:"obs.retention_duration" env:"OBS_RETENTION_DURATION"`
	ObsMaxPointsPerSeries int    `help:"Max points per series" default:"43200" toml:"obs.max_points_per_series" env:"OBS_MAX_POINTS_PER_SERIES"`
	ObsMaxSeries          int    `help:"Max series count" default:"5000" toml:"obs.max_series" env:"OBS_MAX_SERIES"`
	ObsWorkerCount        int    `help:"Worker threads" default:"2" toml:"obs.worker_count" env:"OBS_WORKER_COUNT"`

	ObsPrometheusEnabled bool `help:"Enable Prometheus" default:"true" toml:"obs.prometheus_enabled" env:"OBS_PROMETHEUS_ENABLED"`
	ObsSSEEnabled        bool `help:"Enable SSE" default:"true" toml:"obs.sse_enabled" env:"OBS_SSE_ENABLED"`

	// Capture settings
	CaptureDefaultDelayMs int `help:"Default capture delay in milliseconds" default:"3000" toml:"capture.default_delay_ms" env:"CAPTURE_DEFAULT_DELAY_MS"`

	// Auth settings
	AuthUsername string `help:"Basic auth username" default:"admin" toml:"auth.username" env:"AUTH_USERNAME"`
	AuthPassword string `help:"Basic auth password" default:"password" toml:"auth.password" env:"AUTH_PASSWORD"`

	// Logging settings
	LoggingLevel      string `help:"Global logging level (debug, info, warn, error)" default:"info" toml:"logging.level" env:"LOGGING_LEVEL"`
	LoggingFormat     string `help:"Logging format (text, json)" default:"text" toml:"logging.format" env:"LOGGING_FORMAT"`
	LoggingObs        string `help:"Observability logging level (debug, info, warn, error)" default:"info" toml:"logging.obs" env:"LOGGING_OBS"`
	LoggingStreams    string `help:"Streams logging level" default:"info" toml:"logging.streams" env:"LOGGING_STREAMS"`
	LoggingMediaMTX   string `help:"MediaMTX logging level" default:"info" toml:"logging.mediamtx" env:"LOGGING_MEDIAMTX"`
	LoggingDevices    string `help:"Devices logging level" default:"info" toml:"logging.devices" env:"LOGGING_DEVICES"`
	LoggingEncoders   string `help:"Encoders logging level" default:"info" toml:"logging.encoders" env:"LOGGING_ENCODERS"`
	LoggingCapture    string `help:"Capture logging level" default:"info" toml:"logging.capture" env:"LOGGING_CAPTURE"`
	LoggingAPI        string `help:"API logging level" default:"info" toml:"logging.api" env:"LOGGING_API"`
}

func main() {
	// Create Huma CLI
	cli := humacli.New(func(hooks humacli.Hooks, opts *Options) {
		// Load configuration automatically
		if err := config.LoadConfig(opts); err != nil {
			slog.Warn("Failed to load config", "error", err)
		}

		// Initialize logging system
		loggingConfig := logging.Config{
			Level:  opts.LoggingLevel,
			Format: opts.LoggingFormat,
			Modules: map[string]string{
				"streams":  opts.LoggingStreams,
				"mediamtx": opts.LoggingMediaMTX,
				"obs":      opts.LoggingObs,
				"devices":  opts.LoggingDevices,
				"encoders": opts.LoggingEncoders,
				"capture":  opts.LoggingCapture,
				"api":      opts.LoggingAPI,
			},
		}
		logging.Initialize(loggingConfig)

		logger := logging.GetLogger("main")

		// Set MediaMTX global configuration
		mediamtx.SetConfig(&mediamtx.Config{
			EnableLogging: opts.MediaMTXEnableLogging,
		})

		// Initialize observability system if enabled
		var obsManager *obs.Manager
		var promExporter *exporters.PromExporter
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
				DataChanSize: 10000,
				WorkerCount:  opts.ObsWorkerCount,
				LogLevel:     opts.LoggingObs,
			}

			obsManager = obs.NewManager(obsConfig)

			// Add collectors

			// MediaMTX metrics are collected via Prometheus scraping

			// Add MPP metrics collector (Rockchip only)
			if _, err := os.Stat("/proc/mpp_service/load"); err == nil {
				mppCollector := collectors.NewMPPCollector(obs.Labels{
					"service":  "videonode",
					"instance": "default",
				})
				if err := obsManager.AddCollector(mppCollector); err != nil {
					logger.Warn("Failed to add MPP collector", "error", err)
				}
			}

			// Add exporters based on config
			if opts.ObsPrometheusEnabled {
				promExporter = exporters.NewPromExporter()
				obsManager.AddExporter(promExporter)
			}

		}

		// Default command starts the server using existing API server
		// Create stream service with OBS integration
		serviceOpts := &streams.ServiceOptions{
			OBSManager: obsManager,
		}

		streamService := streams.NewStreamService(serviceOpts)

		// Load existing streams from TOML config into memory
		// This must happen after stream service is created so OBS callbacks are registered
		if err := streamService.LoadStreamsFromConfig(); err != nil {
			logger.Warn("Failed to load existing streams from config", "error", err)
		}

		apiOpts := &api.Options{
			AuthUsername:          opts.AuthUsername,
			AuthPassword:          opts.AuthPassword,
			CaptureDefaultDelayMs: opts.CaptureDefaultDelayMs,
			StreamService:         streamService,
		}

		// Add Prometheus handler if available
		if promExporter != nil {
			apiOpts.PrometheusHandler = promExporter.GetHandler()
		}

		server := api.NewServer(apiOpts)

		// Wire up SSE exporter if OBS is enabled and SSE is configured
		if obsManager != nil && opts.ObsSSEEnabled {
			sseBroadcaster := server.GetSSEBroadcaster()
			sseExporter := exporters.NewSSEExporter(sseBroadcaster)
			sseExporter.SetLogLevel(opts.LoggingObs)
			obsManager.AddExporter(sseExporter)
		}

		hooks.OnStart(func() {
			// NOW start the OBS manager after all exporters are added (only when running server)
			if obsManager != nil {
				if err := obsManager.Start(); err != nil {
					logger.Warn("Failed to start observability manager", "error", err)
				}
			}

			logger.Info("Starting server", "port", opts.Port)
			if err := server.Start(opts.Port); err != nil && err != http.ErrServerClosed {
				logger.Error("Failed to start server", "error", err)
				os.Exit(1)
			}
		})

		hooks.OnStop(func() {
			logger.Info("Shutting down server")
			if err := server.Stop(); err != nil {
				logger.Error("Error stopping server", "error", err)
			}
			if obsManager != nil {
				obsManager.Stop()
			}
		})
	})

	// Add validate-encoders command
	validateCmd := cmd.CreateValidateEncodersCmd()
	cli.Root().AddCommand(validateCmd)

	// Run the CLI
	cli.Run()
}
