package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"

	"github.com/danielgtaylor/huma/v2/humacli"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/smazurov/videonode/cmd"
	"github.com/smazurov/videonode/internal/api"
	"github.com/smazurov/videonode/internal/config"
	"github.com/smazurov/videonode/internal/events"
	"github.com/smazurov/videonode/internal/led"
	"github.com/smazurov/videonode/internal/logging"
	"github.com/smazurov/videonode/internal/metrics/collectors"
	"github.com/smazurov/videonode/internal/metrics/exporters"
	"github.com/smazurov/videonode/internal/streaming"
	"github.com/smazurov/videonode/internal/streams"
	"github.com/smazurov/videonode/internal/streams/store"
	"github.com/smazurov/videonode/internal/updater"
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

	// Metrics settings
	SSEEnabled bool `help:"Enable SSE metrics" default:"true" toml:"metrics.sse_enabled" env:"METRICS_SSE_ENABLED"`

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
	LoggingStreams   string `help:"Streams logging level" default:"info" toml:"logging.streams" env:"LOGGING_STREAMS"`
	LoggingStreaming string `help:"Streaming server logging level" default:"info" toml:"logging.streaming" env:"LOGGING_STREAMING"`
	LoggingDevices   string `help:"Devices logging level" default:"info" toml:"logging.devices" env:"LOGGING_DEVICES"`
	LoggingEncoders  string `help:"Encoders logging level" default:"info" toml:"logging.encoders" env:"LOGGING_ENCODERS"`
	LoggingCapture   string `help:"Capture logging level" default:"info" toml:"logging.capture" env:"LOGGING_CAPTURE"`
	LoggingAPI       string `help:"API logging level" default:"info" toml:"logging.api" env:"LOGGING_API"`
	LoggingWebRTC    string `help:"WebRTC logging level" default:"info" toml:"logging.webrtc" env:"LOGGING_WEBRTC"`

	// Update settings
	UpdateEnabled    bool `help:"Enable self-update functionality" default:"true" toml:"update.enabled" env:"UPDATE_ENABLED"`
	UpdatePrerelease bool `help:"Include prereleases in updates" default:"false" toml:"update.prerelease" env:"UPDATE_PRERELEASE"`
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
				"streams":   opts.LoggingStreams,
				"streaming": opts.LoggingStreaming,
				"devices":   opts.LoggingDevices,
				"encoders":  opts.LoggingEncoders,
				"capture":   opts.LoggingCapture,
				"api":       opts.LoggingAPI,
				"webrtc":    opts.LoggingWebRTC,
			},
		}
		logging.Initialize(loggingConfig)

		logger := logging.GetLogger("main")

		// Start MPP collector if available (Rockchip hardware encoder metrics)
		var mppCollector *collectors.MPPCollector
		if _, statErr := os.Stat("/proc/mpp_service/load"); statErr == nil {
			mppCollector = collectors.NewMPPCollector()
			if err := mppCollector.Start(context.Background()); err != nil {
				logger.Warn("Failed to start MPP collector", "error", err)
			}
		}

		// Create SSE exporter if enabled
		var sseExporter *exporters.SSEExporter

		// Create event bus for in-process event handling
		eventBus := events.New()

		// Set up log callback to publish log entries to event bus for SSE streaming
		logging.SetLogCallback(func(entry logging.LogEntry) {
			eventBus.Publish(events.LogEntryEvent{
				Timestamp:  entry.Timestamp.Format("2006-01-02T15:04:05.000Z07:00"),
				Level:      entry.Level,
				Module:     entry.Module,
				Message:    entry.Message,
				Attributes: entry.Attributes,
			})
		})

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

		// Create stream service
		serviceOpts := &streams.ServiceOptions{
			Store:    streamStore,
			EventBus: eventBus,
		}

		streamService := streams.NewStreamService(serviceOpts)

		// Load existing streams from TOML config into memory at startup
		// This must happen after stream service is created so OBS callbacks are registered
		// Runtime stream management should use CRUD APIs (not reload)
		if err := streamService.LoadStreamsFromConfig(); err != nil {
			logger.Warn("Failed to load existing streams from config", "error", err)
		}

		// Initialize update service if enabled
		var updateService updater.Service
		if opts.UpdateEnabled {
			svc, err := updater.NewService(&updater.Options{
				Repository: "smazurov/videonode",
				Prerelease: opts.UpdatePrerelease,
			})
			if err != nil {
				logger.Warn("Failed to initialize update service", "error", err)
			} else {
				updateService = svc
				if !svc.IsEnabled() {
					logger.Warn("Update service disabled", "reason", svc.DisabledReason())
				}
			}
		}

		apiOpts := &api.Options{
			AuthUsername:          opts.AuthUsername,
			AuthPassword:          opts.AuthPassword,
			CaptureDefaultDelayMs: opts.CaptureDefaultDelayMs,
			StreamService:         streamService,
			EventBus:              eventBus,
			WebRTCManager:         webrtcManager,
			PrometheusHandler:     promhttp.Handler(), // Prometheus metrics via promauto
			UpdateService:         updateService,
		}

		// Add LED controller if available
		if ledController != nil {
			apiOpts.LEDController = ledController
		}

		server := api.NewServer(apiOpts)

		// Create SSE exporter if enabled
		if opts.SSEEnabled {
			sseExporter = exporters.NewSSEExporter(eventBus)
		}

		hooks.OnStart(func() {
			// Start RTSP streaming server first (must be ready for FFmpeg)
			if err := streamingServer.Start(opts.StreamingRTSPPort); err != nil {
				logger.Error("Failed to start RTSP server", "error", err)
				os.Exit(1)
			}

			// Start SSE exporter if enabled
			if sseExporter != nil {
				sseExporter.Start(context.Background())
			}

			// Start LED manager if enabled
			if ledManager != nil {
				ledManager.Start()
			}

			logger.Info("Starting HTTP server", "port", opts.Port)
			if err := server.Start(opts.Port); err != nil && !errors.Is(err, http.ErrServerClosed) {
				logger.Error("Failed to start HTTP server", "error", err)
				os.Exit(1)
			}
		})

		hooks.OnStop(func() {
			logger.Info("Shutting down server")
			if err := server.Stop(); err != nil {
				logger.Error("Error stopping HTTP server", "error", err)
			}

			// Stop all FFmpeg processes (after HTTP server stops accepting new requests)
			if pm := streamService.GetProcessManager(); pm != nil {
				logger.Info("Stopping all stream processes")
				pm.StopAll()
			}

			// Stop streaming server after FFmpeg processes
			if err := streamingServer.Stop(); err != nil {
				logger.Error("Error stopping RTSP server", "error", err)
			}

			// Stop WebRTC peers
			webrtcManager.Stop()

			if ledManager != nil {
				ledManager.Stop()
			}
			if sseExporter != nil {
				sseExporter.Stop()
			}
			if mppCollector != nil {
				_ = mppCollector.Stop()
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
