package main

import (
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
	"github.com/smazurov/videonode/internal/obs"
	"github.com/smazurov/videonode/internal/obs/collectors"
	"github.com/smazurov/videonode/internal/obs/exporters"
	"github.com/smazurov/videonode/internal/streaming"
	"github.com/smazurov/videonode/internal/streams"
	"github.com/smazurov/videonode/internal/streams/store"
)

// Options for the CLI - flat structure with toml mapping.
type Options struct {
	Config string `help:"Path to configuration file" short:"c" default:"config.toml"`

	// Server settings
	Port string `help:"Port to listen on" short:"p" default:":8090" toml:"server.port" env:"SERVER_PORT"`

	// Streams settings
	StreamsConfigFile string `help:"Stream definitions file" default:"streams.toml" toml:"streams.config_file" env:"STREAMS_CONFIG_FILE"`

	// Streaming server settings
	StreamingRTSPPort string `help:"RTSP server port" default:":8554" toml:"streaming.rtsp_port" env:"STREAMING_RTSP_PORT"`

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
	FeaturesLEDControl bool `help:"Enable LED control" default:"false" toml:"features.led_control_enabled" env:"FEATURES_LED_CONTROL"`

	// Logging settings
	LoggingLevel     string `help:"Global logging level (debug, info, warn, error)" default:"info" toml:"logging.level" env:"LOGGING_LEVEL"`
	LoggingFormat    string `help:"Logging format (text, json)" default:"text" toml:"logging.format" env:"LOGGING_FORMAT"`
	LoggingObs       string `help:"Observability logging level (debug, info, warn, error)" default:"info" toml:"logging.obs" env:"LOGGING_OBS"`
	LoggingStreams   string `help:"Streams logging level" default:"info" toml:"logging.streams" env:"LOGGING_STREAMS"`
	LoggingStreaming string `help:"Streaming server logging level" default:"info" toml:"logging.streaming" env:"LOGGING_STREAMING"`
	LoggingDevices   string `help:"Devices logging level" default:"info" toml:"logging.devices" env:"LOGGING_DEVICES"`
	LoggingEncoders  string `help:"Encoders logging level" default:"info" toml:"logging.encoders" env:"LOGGING_ENCODERS"`
	LoggingCapture   string `help:"Capture logging level" default:"info" toml:"logging.capture" env:"LOGGING_CAPTURE"`
	LoggingAPI       string `help:"API logging level" default:"info" toml:"logging.api" env:"LOGGING_API"`
	LoggingWebRTC    string `help:"WebRTC logging level" default:"info" toml:"logging.webrtc" env:"LOGGING_WEBRTC"`
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
				"streams":   opts.LoggingStreams,
				"streaming": opts.LoggingStreaming,
				"obs":       opts.LoggingObs,
				"devices":   opts.LoggingDevices,
				"encoders":  opts.LoggingEncoders,
				"capture":   opts.LoggingCapture,
				"api":       opts.LoggingAPI,
				"webrtc":    opts.LoggingWebRTC,
			},
		}
		logging.Initialize(loggingConfig)

		logger := logging.GetLogger("main")

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

		// Initialize LED control if enabled
		var ledManager *led.Manager
		var ledController led.Controller
		if opts.FeaturesLEDControl {
			logger.Info("LED control enabled, initializing")
			ledController = led.New(logger)

			// Create LED manager that subscribes to stream state changes
			ledManager = led.NewManager(ledController, eventBus, logger)
		}

		// Initialize streaming server (RTSP + WebRTC)
		streamingLogger := logging.GetLogger("streaming")
		streamingHub := streaming.NewHub(streamingLogger)
		streamingServer := streaming.NewServer(streamingHub, streamingLogger)
		webrtcManager := streaming.NewWebRTCManager(streamingHub, streaming.WebRTCConfig{}, logging.GetLogger("webrtc"))

		// Close WebRTC consumers when stream producer is replaced (enables client reconnection)
		streamingHub.SetOnProducerReplaced(func(streamID string) {
			streamingLogger.Info("Producer replaced, closing WebRTC consumers", "stream_id", streamID)
			webrtcManager.CloseStreamConsumers(streamID)
		})

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
			WebRTCManager:         webrtcManager,
		}

		// Add Prometheus handler if available
		if promExporter != nil {
			apiOpts.PrometheusHandler = promExporter.GetHandler()
		}

		// Add LED controller if available
		if ledController != nil {
			apiOpts.LEDController = ledController
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
			// Start RTSP streaming server first (must be ready for FFmpeg)
			if startErr := streamingServer.Start(opts.StreamingRTSPPort); startErr != nil {
				logger.Error("Failed to start RTSP server", "error", startErr)
				os.Exit(1)
			}

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

			logger.Info("Starting HTTP server", "port", opts.Port)
			if startErr := server.Start(opts.Port); startErr != nil && !errors.Is(startErr, http.ErrServerClosed) {
				logger.Error("Failed to start HTTP server", "error", startErr)
				os.Exit(1)
			}
		})

		hooks.OnStop(func() {
			logger.Info("Shutting down server")
			if stopErr := server.Stop(); stopErr != nil {
				logger.Error("Error stopping HTTP server", "error", stopErr)
			}

			// Stop all FFmpeg processes (after HTTP server stops accepting new requests)
			if pm := streamService.GetProcessManager(); pm != nil {
				logger.Info("Stopping all stream processes")
				pm.StopAll()
			}

			// Stop streaming server after FFmpeg processes
			if stopErr := streamingServer.Stop(); stopErr != nil {
				logger.Error("Error stopping RTSP server", "error", stopErr)
			}

			// Stop WebRTC peers
			webrtcManager.Stop()

			if ledManager != nil {
				ledManager.Stop()
			}
			if obsManager != nil {
				obsManager.Stop()
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
