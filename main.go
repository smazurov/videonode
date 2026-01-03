package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/danielgtaylor/huma/v2/humacli"
	"github.com/smazurov/videonode/cmd"
	"github.com/smazurov/videonode/internal/api"
	"github.com/smazurov/videonode/internal/config"
	"github.com/smazurov/videonode/internal/events"
	"github.com/smazurov/videonode/internal/led"
	"github.com/smazurov/videonode/internal/logging"
	"github.com/smazurov/videonode/internal/mediamtx"
	videonats "github.com/smazurov/videonode/internal/nats"
	"github.com/smazurov/videonode/internal/obs"
	"github.com/smazurov/videonode/internal/obs/collectors"
	"github.com/smazurov/videonode/internal/obs/exporters"
	"github.com/smazurov/videonode/internal/streams"
	"github.com/smazurov/videonode/internal/streams/store"
	"github.com/smazurov/videonode/internal/systemd"
)

// Options for the CLI - flat structure with toml mapping.
type Options struct {
	Config string `help:"Path to configuration file" short:"c" default:"config.toml"`

	// Server settings
	Port string `help:"Port to listen on" short:"p" default:":8090" toml:"server.port" env:"SERVER_PORT"`

	// Streams settings
	StreamsConfigFile string `help:"Stream definitions file" default:"streams.toml" toml:"streams.config_file" env:"STREAMS_CONFIG_FILE"`
	MediamtxConfig    string `help:"MediaMTX config file" default:"mediamtx.yml" toml:"streams.mediamtx_config" env:"STREAMS_MEDIAMTX_CONFIG"`

	// MediaMTX settings
	MediaMTXUseSystemd  bool   `help:"Use systemd-run to wrap ffmpeg commands" default:"false" toml:"mediamtx.use_systemd" env:"MEDIAMTX_USE_SYSTEMD"`
	MediaMTXServiceName string `help:"MediaMTX systemd service name" default:"videonode_mediamtx.service" toml:"mediamtx.service_name" env:"MEDIAMTX_SERVICE_NAME"`

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

	// Features settings
	FeaturesLEDControl      bool `help:"Enable LED control" default:"false" toml:"features.led_control_enabled" env:"FEATURES_LED_CONTROL"`
	FeaturesMediaMTXControl bool `help:"Enable MediaMTX service control" default:"false" toml:"features.mediamtx_control_enabled" env:"FEATURES_MEDIAMTX_CONTROL"`

	// Logging settings
	LoggingLevel    string `help:"Global logging level (debug, info, warn, error)" default:"info" toml:"logging.level" env:"LOGGING_LEVEL"`
	LoggingFormat   string `help:"Logging format (text, json)" default:"text" toml:"logging.format" env:"LOGGING_FORMAT"`
	LoggingObs      string `help:"Observability logging level (debug, info, warn, error)" default:"info" toml:"logging.obs" env:"LOGGING_OBS"`
	LoggingStreams  string `help:"Streams logging level" default:"info" toml:"logging.streams" env:"LOGGING_STREAMS"`
	LoggingMediaMTX string `help:"MediaMTX logging level" default:"info" toml:"logging.mediamtx" env:"LOGGING_MEDIAMTX"`
	LoggingDevices  string `help:"Devices logging level" default:"info" toml:"logging.devices" env:"LOGGING_DEVICES"`
	LoggingEncoders string `help:"Encoders logging level" default:"info" toml:"logging.encoders" env:"LOGGING_ENCODERS"`
	LoggingCapture  string `help:"Capture logging level" default:"info" toml:"logging.capture" env:"LOGGING_CAPTURE"`
	LoggingAPI      string `help:"API logging level" default:"info" toml:"logging.api" env:"LOGGING_API"`

	// NATS settings
	NATSEnabled bool `help:"Enable embedded NATS server" default:"true" toml:"nats.enabled" env:"NATS_ENABLED"`
	NATSPort    int  `help:"NATS server port" default:"4222" toml:"nats.port" env:"NATS_PORT"`
}

func main() {
	// Create Huma CLI
	cli := humacli.New(func(hooks humacli.Hooks, opts *Options) {
		// Load configuration automatically
		if loadErr := config.LoadConfig(opts); loadErr != nil {
			slog.Warn("Failed to load config", "error", loadErr)
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
			UseSystemd: opts.MediaMTXUseSystemd,
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
			if _, statErr := os.Stat("/proc/mpp_service/load"); statErr == nil {
				mppCollector := collectors.NewMPPCollector(obs.Labels{
					"service":  "videonode",
					"instance": "default",
				})
				if addErr := obsManager.AddCollector(mppCollector); addErr != nil {
					logger.Warn("Failed to add MPP collector", "error", addErr)
				}
			}

			// Add exporters based on config
			if opts.ObsPrometheusEnabled {
				promExporter = exporters.NewPromExporter()
				if addErr := obsManager.AddExporter(promExporter); addErr != nil {
					logger.Warn("Failed to add Prometheus exporter", "error", addErr)
				}
			}
		}

		// Create event bus for in-process event handling
		eventBus := events.New()

		// Initialize NATS server and bridge if enabled
		var natsServer *videonats.Server
		var natsBridge *videonats.Bridge
		var natsControlPublisher *videonats.ControlPublisher
		if opts.NATSEnabled {
			natsServer = videonats.NewServer(videonats.ServerOptions{
				Port:   opts.NATSPort,
				Name:   "videonode",
				Logger: logging.GetLogger("nats"),
			})
			if startErr := natsServer.Start(); startErr != nil {
				logger.Error("Failed to start NATS server", "error", startErr)
			} else {
				// Create bridge to forward NATS messages to event bus
				natsBridge = videonats.NewBridge(
					natsServer.ClientURL(),
					eventBus,
					logging.GetLogger("nats"),
				)
				if bridgeErr := natsBridge.Start(); bridgeErr != nil {
					logger.Warn("Failed to start NATS bridge", "error", bridgeErr)
				}

				// Create control publisher for restart commands
				var ctrlErr error
				natsControlPublisher, ctrlErr = videonats.NewControlPublisher(
					natsServer.ClientURL(),
					logging.GetLogger("nats"),
				)
				if ctrlErr != nil {
					logger.Warn("Failed to create NATS control publisher", "error", ctrlErr)
				}
			}
		}

		// Initialize LED control if enabled
		var ledManager *led.Manager
		var ledController led.Controller
		if opts.FeaturesLEDControl {
			logger.Info("LED control enabled, initializing")
			ledController = led.New(logger)

			// Create LED manager that subscribes to stream state changes
			ledManager = led.NewManager(ledController, eventBus, logger)
		}

		// Initialize systemd manager if enabled
		var systemdManager *systemd.Manager
		if opts.FeaturesMediaMTXControl {
			logger.Info("MediaMTX control enabled, initializing systemd manager")
			mgr, err := systemd.NewManager(context.Background())
			if err != nil {
				logger.Warn("Failed to initialize systemd manager", "error", err)
			} else {
				systemdManager = mgr
			}
		}

		// Default command starts the server using existing API server
		// Create stream store
		streamStore := store.NewTOML(opts.StreamsConfigFile)

		// Create stream service with OBS integration
		serviceOpts := &streams.ServiceOptions{
			Store:      streamStore,
			OBSManager: obsManager,
			EventBus:   eventBus,
		}

		streamService := streams.NewStreamService(serviceOpts)

		// Load existing streams from TOML config into memory at startup
		// This must happen after stream service is created so OBS callbacks are registered
		// Runtime stream management should use CRUD APIs (not reload)
		if loadErr := streamService.LoadStreamsFromConfig(); loadErr != nil {
			logger.Warn("Failed to load existing streams from config", "error", loadErr)
		}

		apiOpts := &api.Options{
			AuthUsername:          opts.AuthUsername,
			AuthPassword:          opts.AuthPassword,
			CaptureDefaultDelayMs: opts.CaptureDefaultDelayMs,
			StreamService:         streamService,
			EventBus:              eventBus,
		}

		// Add Prometheus handler if available
		if promExporter != nil {
			apiOpts.PrometheusHandler = promExporter.GetHandler()
		}

		// Add LED controller if available
		if ledController != nil {
			apiOpts.LEDController = ledController
		}

		// Add systemd manager if available
		if systemdManager != nil {
			apiOpts.SystemdManager = systemdManager
			apiOpts.MediaMTXServiceName = opts.MediaMTXServiceName
		}

		// Add NATS control publisher if available
		if natsControlPublisher != nil {
			apiOpts.NATSControlPublisher = natsControlPublisher
		}

		server := api.NewServer(apiOpts)

		// Wire up SSE exporter if OBS is enabled and SSE is configured
		if obsManager != nil && opts.ObsSSEEnabled {
			sseExporter := exporters.NewSSEExporter(eventBus)
			sseExporter.SetLogLevel(opts.LoggingObs)
			if addErr := obsManager.AddExporter(sseExporter); addErr != nil {
				logger.Warn("Failed to add SSE exporter", "error", addErr)
			}
		}

		hooks.OnStart(func() {
			// NOW start the OBS manager after all exporters are added (only when running server)
			if obsManager != nil {
				if startErr := obsManager.Start(); startErr != nil {
					logger.Warn("Failed to start observability manager", "error", startErr)
				}
			}

			// Start LED manager if enabled
			if ledManager != nil {
				ledManager.Start()
			}

			logger.Info("Starting server", "port", opts.Port)
			if startErr := server.Start(opts.Port); startErr != nil && !errors.Is(startErr, http.ErrServerClosed) {
				logger.Error("Failed to start server", "error", startErr)
				os.Exit(1)
			}
		})

		hooks.OnStop(func() {
			logger.Info("Shutting down server")
			if stopErr := server.Stop(); stopErr != nil {
				logger.Error("Error stopping server", "error", stopErr)
			}
			if ledManager != nil {
				ledManager.Stop()
			}
			if systemdManager != nil {
				systemdManager.Close()
			}
			if obsManager != nil {
				obsManager.Stop()
			}
			// Stop NATS components
			if natsControlPublisher != nil {
				natsControlPublisher.Close()
			}
			if natsBridge != nil {
				natsBridge.Stop()
			}
			if natsServer != nil {
				natsServer.Stop()
			}
		})
	})

	// Add validate-encoders command
	validateCmd := cmd.CreateValidateEncodersCmd()
	cli.Root().AddCommand(validateCmd)

	// Add stream command
	streamCmd := cmd.CreateStreamCmd()
	cli.Root().AddCommand(streamCmd)

	// Run the CLI
	cli.Run()
}
