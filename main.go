package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"slices"
	"time"

	"github.com/danielgtaylor/huma/v2/humacli"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/smazurov/videonode/cmd"
	"github.com/smazurov/videonode/internal/api"
	"github.com/smazurov/videonode/internal/auth"
	"github.com/smazurov/videonode/internal/config"
	"github.com/smazurov/videonode/internal/events"
	"github.com/smazurov/videonode/internal/led"
	"github.com/smazurov/videonode/internal/logging"
	"github.com/smazurov/videonode/internal/metrics/collectors"
	"github.com/smazurov/videonode/internal/metrics/exporters"
	"github.com/smazurov/videonode/internal/streaming"
	"github.com/smazurov/videonode/internal/streams"
	"github.com/smazurov/videonode/internal/streams/pipeline"
	"github.com/smazurov/videonode/internal/streams/pipelinectl"
	"github.com/smazurov/videonode/internal/streams/store"
	"github.com/smazurov/videonode/internal/updater"
)

// subcommandNames is the central registry of subcommand names. Add each
// subcommand here when registering it below; the lightweight-boot check
// uses this list to short-circuit the heavy server init.
var subcommandNames = []string{
	"openapi",
	"validate-encoders",
	"stream",
	"version",
}

// isSubcommandInvocation reports whether os.Args names one of the registered
// subcommands. The default (no-subcommand) invocation falls through to the
// full server boot path.
func isSubcommandInvocation(args []string) bool {
	if len(args) < 2 {
		return false
	}
	return slices.Contains(subcommandNames, args[1])
}

// Options for the CLI - flat structure with toml mapping.
type Options struct {
	Config string `help:"Path to configuration file" short:"c" default:"config.toml"`

	// Server settings
	Port string `help:"Port to listen on" short:"p" default:":8090" toml:"server.port" env:"SERVER_PORT"`

	// Streams settings
	StreamsConfigFile string `help:"Stream definitions file" default:"streams.toml" toml:"streams.config_file" env:"STREAMS_CONFIG_FILE"`

	// Streaming server settings
	StreamingRTSPPort string `help:"RTSP server port" default:":8554" toml:"streaming.rtsp_port" env:"STREAMING_RTSP_PORT"`

	// SRT server settings
	SRTEnabled bool   `help:"Enable SRT server" default:"true" toml:"srt.enabled" env:"SRT_ENABLED"`
	SRTAddr    string `help:"SRT listen address" default:":6001" toml:"srt.addr" env:"SRT_ADDR"`
	SRTLatency int    `help:"SRT latency in milliseconds" default:"20" toml:"srt.latency" env:"SRT_LATENCY"`

	// Metrics settings
	SSEEnabled bool `help:"Enable SSE metrics" default:"true" toml:"metrics.sse_enabled" env:"METRICS_SSE_ENABLED"`

	// Auth settings
	AuthType     string `help:"Auth type (basic, linux)" default:"linux" toml:"auth.type" env:"AUTH_TYPE"`
	AuthUsername string `help:"Fallback username for basic auth" default:"videonode" toml:"auth.username" env:"AUTH_USERNAME"`
	AuthPassword string `help:"Fallback password for basic auth" default:"videonode" toml:"auth.password" env:"AUTH_PASSWORD"`

	// Features settings
	FeaturesLEDControl bool `help:"Enable LED control" default:"false" toml:"features.led_control_enabled" env:"FEATURES_LED_CONTROL"`

	// Recording settings
	RecordingDataDir string `help:"Recording data directory" default:"data/recording" toml:"recording.data_dir" env:"RECORDING_DATA_DIR"`

	// Update settings
	UpdateEnabled    bool `help:"Enable self-update functionality" default:"true" toml:"update.enabled" env:"UPDATE_ENABLED"`
	UpdatePrerelease bool `help:"Include prereleases in updates" default:"false" toml:"update.prerelease" env:"UPDATE_PRERELEASE"`

	// Vision settings
	VisionDefaultFPS int `help:"Default FPS for vision raw-frame pipes" default:"10" toml:"vision.default_fps" env:"VISION_DEFAULT_FPS"`

	// Native pipeline binaries. When present + executable, single V4L2
	// streams and GPU canvases route through these instead of the legacy
	// ffmpeg-direct path. Empty path == component unavailable; legacy
	// pipeline kicks in. Defaults match the local-user CMake install
	// (cmake --install --prefix $HOME/.local → ~/.local/bin/<name>).
	NativeV4L2Source string `help:"Path to videonode-source binary"   default:"~/.local/bin/videonode-source"   toml:"native_pipeline.source"   env:"NATIVE_PIPELINE_SOURCE"`
	NativeVNSink     string `help:"Path to videonode-sink binary"     default:"~/.local/bin/videonode-sink"     toml:"native_pipeline.sink"        env:"NATIVE_PIPELINE_SINK"`
	NativeComposer   string `help:"Path to videonode-composer binary" default:"~/.local/bin/videonode-composer" toml:"native_pipeline.composer"    env:"NATIVE_PIPELINE_COMPOSER"`
}

func main() {
	// Create Huma CLI
	var cli humacli.CLI
	cli = humacli.New(func(hooks humacli.Hooks, opts *Options) {
		// Heavy server init (logging, MPP collector, stream service load,
		// updater, API wiring, SSE exporter) only runs for the default
		// (no-subcommand) server invocation. Every subcommand is lightweight
		// by default — they each do their own minimal setup and shouldn't
		// pay for, or interfere with, the running production server's state.
		if isSubcommandInvocation(os.Args) {
			return
		}

		// Load configuration with proper precedence: CLI > env > config file
		if err := config.LoadConfig(opts, cli.Root()); err != nil {
			slog.Warn("Failed to load config", "error", err)
		}

		// Initialize logging system from config file
		loggingConfig := config.LoadLoggingConfig(opts.Config)
		logging.Initialize(loggingConfig)

		logger := logging.GetLogger("api")

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

		// Initialize streaming server (RTSP + WebRTC + SRT)
		streamingLogger := logging.GetLogger("streams")
		streamingServer := streaming.NewServer(streamingLogger)
		webrtcManager := streaming.NewWebRTCManager(streamingServer, streaming.WebRTCConfig{}, logging.GetLogger("webrtc"))

		// Initialize SRT server if enabled
		var srtServer *streaming.SRTServer
		if opts.SRTEnabled {
			srtConfig := streaming.SRTConfig{
				Enabled: true,
				Addr:    opts.SRTAddr,
				Latency: opts.SRTLatency,
			}
			srtServer = streaming.NewSRTServer(streamingServer, srtConfig, logging.GetLogger("srt"))
		}

		// Close WebRTC and SRT consumers when stream producer is replaced (enables client reconnection)
		streamingServer.SetOnProducerReplaced(func(streamID string) {
			streamingLogger.Info("Producer replaced, closing consumers", "stream_id", streamID)
			webrtcManager.CloseStreamConsumers(streamID)
			if srtServer != nil {
				srtServer.CloseStreamConsumers(streamID)
			}
		})

		// Emit "running" event when a stream's RTSP producer connects
		streamingServer.SetOnProducerConnected(func(streamID string) {
			eventBus.Publish(events.StreamStateChangedEvent{
				StreamID:  streamID,
				Enabled:   true,
				Action:    "running",
				Timestamp: time.Now().Format(time.RFC3339),
			})
		})

		// Default command starts the server using existing API server
		// Create stream store
		streamStore := store.NewTOML(opts.StreamsConfigFile)

		// Resolve native-pipeline binary availability once. CanvasReady /
		// SingleStreamReady decide whether GPU canvas + V4L2 single streams
		// take the dma-buf path or fall back to legacy ffmpeg-direct.
		native := (&streams.NativePipelineConfig{
			V4L2Source: opts.NativeV4L2Source,
			VNSink:     opts.NativeVNSink,
			Composer:   opts.NativeComposer,
		}).Resolve(logger)

		// Daemon-side control plane for native sidecars. Must bind
		// BEFORE the stream service spawns any sidecars so they can
		// dial in on startup. Only enable when the native pipeline is
		// available — without a sidecar binary, no clients connect.
		var ctlServer *pipelinectl.Manager
		if native.V4L2Source != "" {
			ctlServer = pipelinectl.New("", nil)
			if err := ctlServer.Start(context.Background()); err != nil {
				logger.Warn("control plane disabled (start failed)",
					"error", err)
				ctlServer = nil
			} else {
				// Pump status notifications into the event bus.
				go func() {
					for st := range ctlServer.StatusFeed() {
						eventBus.Publish(events.SourceStatusEvent{
							DeviceID:  st.DeviceID,
							Status:    st,
							Timestamp: time.Now().Format(time.RFC3339),
						})
					}
				}()
			}
		}

		// Create stream service
		// Construct pipeline.Pipeline as the canonical process supervisor.
		// Replaces the legacy streamProcessManager + processor +
		// canvasProcessor + producerManager stack with a unified
		// Producer→Composer→Encoder model. The legacy stack is gone;
		// existing /api/streams CRUD flows through Pipeline via
		// pipelineProcessManager (the translation layer).
		nativePipeline := pipeline.New(pipeline.Config{
			VNSourceBin:    native.V4L2Source,
			VNComposerBin:  native.Composer,
			VNSinkBin:      native.VNSink,
			DRMDevice:      "/dev/dri/renderD128",
			DeviceResolver: streams.MakeDeviceResolver(logger),
			EventBus:       eventBus,
		}, logger)

		serviceOpts := &streams.ServiceOptions{
			Store:            streamStore,
			EventBus:         eventBus,
			VisionDefaultFPS: opts.VisionDefaultFPS,
			Native:           native,
			RTSPPort:         opts.StreamingRTSPPort,
			ProcessManager: streams.NewPipelineProcessManager(
				nativePipeline, streamStore, ctlServer, opts.StreamingRTSPPort,
			),
		}
		if ctlServer != nil {
			serviceOpts.ControlServer = ctlServer
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

		// Create authenticator
		authenticator := auth.New(auth.Config{
			Type:     opts.AuthType,
			Username: opts.AuthUsername,
			Password: opts.AuthPassword,
		}, logger)

		apiOpts := &api.Options{
			Authenticator:          authenticator,
			StreamService:          streamService,
			EventBus:               eventBus,
			WebRTCManager:          webrtcManager,
			StreamProvider:         streamingServer,
			SourceSnapshotProvider: streamService.GetProcessManager(),
			RecordingDir:           opts.RecordingDataDir,
			PrometheusHandler:      promhttp.Handler(), // Prometheus metrics via promauto
			UpdateService:          updateService,
			ControlServer:          ctlServer,
			ProcessesProvider:      nativePipeline,
			StreamingRTSPPort:      opts.StreamingRTSPPort,
			StreamingSRTPort:       opts.SRTAddr,
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
			// Validate recording directory is writable
			if err := os.MkdirAll(opts.RecordingDataDir, 0o755); err != nil {
				logger.Error("Failed to create recording directory", "path", opts.RecordingDataDir, "error", err)
				os.Exit(1)
			}
			testFile := opts.RecordingDataDir + "/.write_test"
			if err := os.WriteFile(testFile, []byte("ok"), 0o644); err != nil {
				logger.Error("Recording directory is not writable", "path", opts.RecordingDataDir, "error", err)
				os.Exit(1)
			}
			os.Remove(testFile)
			logger.Info("Recording directory ready", "path", opts.RecordingDataDir)

			// Start RTSP streaming server first (must be ready for FFmpeg)
			if err := streamingServer.Start(opts.StreamingRTSPPort); err != nil {
				logger.Error("Failed to start RTSP server", "error", err)
				os.Exit(1)
			}

			// Start SRT server if enabled
			if srtServer != nil {
				if startErr := srtServer.Start(); startErr != nil {
					logger.Error("Failed to start SRT server", "error", startErr)
					os.Exit(1)
				}
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

			// Stop SRT server
			if srtServer != nil {
				srtServer.Stop()
			}

			if ledManager != nil {
				ledManager.Stop()
			}
			if sseExporter != nil {
				sseExporter.Stop()
			}
			if mppCollector != nil {
				_ = mppCollector.Stop()
			}

			// Tear down the native control plane: closes every per-source
			// gRPC channel, cancels in-flight StreamStatus goroutines, and
			// closes the StatusFeed channel that the fan-out goroutine in
			// main reads. Without this the daemon hangs on SIGTERM because
			// the fan-out goroutine blocks on a never-closed channel.
			if ctlServer != nil {
				if err := ctlServer.Stop(); err != nil {
					logger.Error("Error stopping control manager", "error", err)
				}
			}

			// Exit with non-zero code if restart was requested (systemd will restart)
			if updateService != nil && updateService.IsRestartPending() {
				os.Exit(3)
			}
		})
	})

	// Add validate-encoders command. The path resolver shares the same precedence
	// the server uses (flag → env → default). We can't reuse opts.StreamsConfigFile
	// here because the lightweight subcommand short-circuits the humacli init that
	// populates it.
	validateCmd := cmd.CreateValidateEncodersCmd(cmd.ResolveStreamsConfigPath)
	cli.Root().AddCommand(validateCmd)

	// Add stream command
	streamCmd := cmd.CreateStreamCmd()
	cli.Root().AddCommand(streamCmd)

	// Add openapi command
	openapiCmd := cmd.CreateOpenAPICmd()
	cli.Root().AddCommand(openapiCmd)

	// Add version command
	versionCmd := cmd.CreateVersionCmd()
	cli.Root().AddCommand(versionCmd)

	// Run the CLI
	cli.Run()
}
