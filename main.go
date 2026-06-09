package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	_ "net/http/pprof"
	"os"
	"slices"
	"time"

	"github.com/danielgtaylor/huma/v2/humacli"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/smazurov/videonode/cmd"
	"github.com/smazurov/videonode/internal/api"
	"github.com/smazurov/videonode/internal/auth"
	"github.com/smazurov/videonode/internal/config"
	"github.com/smazurov/videonode/internal/encoders"
	"github.com/smazurov/videonode/internal/events"
	"github.com/smazurov/videonode/internal/led"
	"github.com/smazurov/videonode/internal/logging"
	"github.com/smazurov/videonode/internal/metrics/collectors"
	"github.com/smazurov/videonode/internal/metrics/exporters"
	"github.com/smazurov/videonode/internal/snapshots"
	"github.com/smazurov/videonode/internal/streaming"
	"github.com/smazurov/videonode/internal/streams"
	"github.com/smazurov/videonode/internal/streams/pipeline"
	"github.com/smazurov/videonode/internal/streams/pipelinectl"
	"github.com/smazurov/videonode/internal/streams/services"
	"github.com/smazurov/videonode/internal/streams/store"
)

// subcommandNames is the central registry of subcommand names. Add each
// subcommand here when registering it below; the lightweight-boot check
// uses this list to short-circuit the heavy server init.
var subcommandNames = []string{
	"composer",
	"openapi",
	"source",
	"stream",
	"validate-config",
	"validate-encoders",
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

	// Profiling settings. The pprof listener only binds when enabled.
	FeaturesPprof bool   `help:"Enable net/http/pprof debug server" default:"false" toml:"features.pprof_enabled" env:"FEATURES_PPROF"`
	PprofAddr     string `help:"Address for the pprof debug server" default:"127.0.0.1:6060" toml:"features.pprof_addr" env:"FEATURES_PPROF_ADDR"`

	// Snapshot preview settings
	PreviewMaxFPS int `help:"Max fps for snapshot preview streams" default:"10" toml:"preview.max_fps" env:"PREVIEW_MAX_FPS"`

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
			slog.Warn("Failed to load config", logging.KeyError, err)
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
				logger.Warn("Failed to start MPP collector", logging.KeyError, err)
			}
		}

		// Create SSE exporter if enabled
		var sseExporter *exporters.SSEExporter

		// Create event bus for in-process event handling
		eventBus := events.New()

		// Entity registry: drives the uniform SSE entity envelope and
		// (Step 2) the auto-publish HTTP middleware. Constructed here
		// because every service that registers an entity needs to dial
		// into the same registry instance the API server reads from.
		eventRegistry := events.NewRegistry(eventBus)

		// Set up log callback to publish log entries to event bus for SSE streaming
		logging.SetLogCallback(func(entry logging.LogEntry) {
			events.Publish(eventBus, events.LogEntryEvent{
				Timestamp:  entry.Timestamp.Format("2006-01-02T15:04:05.000Z07:00"),
				Level:      entry.Level,
				Module:     entry.Module,
				Message:    entry.Message,
				Attributes: entry.Attributes,
			})
		})

		// Initialize LED control if enabled. The LED is driven solely by the
		// REST surface (POST /api/leds); it does not react to pipeline events.
		var ledController led.Controller
		if opts.FeaturesLEDControl {
			logger.Info("LED control enabled, initializing")
			ledController = led.New(logger)
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
			streamingLogger.Info("Producer replaced, closing consumers", logging.KeyStreamID, streamID)
			webrtcManager.CloseStreamConsumers(streamID)
			if srtServer != nil {
				srtServer.CloseStreamConsumers(streamID)
			}
		})

		// Publish "running" status on the entity envelope when a stream's
		// RTSP producer connects, so the UI's per-stream status pill flips
		// without polling.
		streamingServer.SetOnProducerConnected(func(streamID string) {
			if eventRegistry != nil {
				eventRegistry.Publish("stream", events.ActionStatus, streamID, streaming.StreamStatusPayload{
					State:     "running",
					EncoderUp: true,
				})
			}
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
			ctlServer = pipelinectl.New(nil)
			if err := ctlServer.Start(context.Background()); err != nil {
				logger.Warn("control plane disabled (start failed)",
					logging.KeyError, err)
				ctlServer = nil
			}
			// The StatusFeed pump goroutine is started AFTER
			// nativePipeline is constructed below, so it can stamp
			// StartedAtUs onto each status payload from the supervisor
			// pool.
		}

		// Shared v2 entity store; the pipeline reads source/composer specs
		// through it (single source of truth) and the ensure hook below
		// rejects consumers for streams that aren't configured at all.
		entityStore, _ := streamStore.(streams.EntityStore)

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
			RTSPPort:       opts.StreamingRTSPPort,
			DeviceResolver: streams.MakeDeviceResolver(),
			ControlServer:  ctlServer,
			Registry:       eventRegistry,
			EventBus:       eventBus,
			EntityStore:    entityStore,
			StartedAtUS:    time.Now().UnixMicro(),
			EncoderResolver: func(codec, inputPixFmt string) (pipeline.EncoderResolution, error) {
				cfg, err := encoders.MapAPICodec(codec, inputPixFmt, streamStore)
				if err != nil {
					return pipeline.EncoderResolution{}, err
				}
				res := pipeline.EncoderResolution{EncoderName: cfg.EncoderName}
				if cfg.Settings != nil {
					res.GlobalArgs = cfg.Settings.GlobalArgs
					res.VideoFilters = cfg.Settings.VideoFilters
				}
				return res, nil
			},
		}, logging.GetLogger("pipeline"))

		// Pump status notifications into the event bus AND the uniform
		// entity envelope so the UI's per-source status pill and
		// consumer count react in real time. RunStatusFanout decouples
		// the drain from the publish so a slow event subscriber can
		// never block the StatusFeed channel.
		if ctlServer != nil {
			// Source consumers change far less often than the ~1 Hz status
			// heartbeat, and the status payload already embeds the consumer
			// set. Strip consumers from the status event and publish a
			// dedicated consumers event only when the membership (count +
			// live fds) changes — no double-send, no per-tick churn.
			lastConsumerSig := make(map[string]string)
			consumerSig := func(c pipelinectl.SourceConsumersInfo) string {
				fds := make([]int, 0, len(c.Live))
				for _, e := range c.Live {
					fds = append(fds, e.FD)
				}
				slices.Sort(fds)
				return fmt.Sprintf("%d:%v", c.Count, fds)
			}
			pipelinectl.RunStatusFanout(
				ctlServer.StatusFeed(),
				func(st pipelinectl.StatusParams) {
					if eventRegistry == nil || st.DeviceID == "" {
						return
					}
					statusOnly := st
					statusOnly.Consumers = pipelinectl.SourceConsumersInfo{}
					eventRegistry.Publish("source", events.ActionStatus, st.DeviceID, statusOnly)
					if sig := consumerSig(st.Consumers); lastConsumerSig[st.DeviceID] != sig {
						lastConsumerSig[st.DeviceID] = sig
						eventRegistry.Publish("source", events.ActionConsumers, st.DeviceID, st.Consumers)
					}
				},
				func(poolKey string) int64 {
					info := nativePipeline.Pool().GetStatus(poolKey)
					if !info.StartedAt.IsZero() {
						return info.StartedAt.UnixMicro()
					}
					return 0
				},
			)
		}

		// Lazy encoder lifecycle: idle the encoder once the last consumer
		// disconnects, restart it on the next consumer attach. Mirror
		// encoder teardown on the entity envelope so the UI's per-stream
		// status pill flips back to "idle" without polling.
		streamingServer.SetOnLastReaderGone(func(streamID string) {
			_ = nativePipeline.StopEncoder(streamID)
			if eventRegistry != nil {
				eventRegistry.Publish("stream", events.ActionStatus, streamID, streaming.StreamStatusPayload{
					State:     "stopped",
					EncoderUp: false,
				})
			}
		})
		streamingServer.SetOnEnsureStream(func(streamID string) error {
			if entityStore != nil {
				if _, ok := entityStore.GetPipelineStream(streamID); !ok {
					return streaming.ErrStreamNotFound
				}
			}
			if !streamStore.GetPipeline().Enabled {
				return fmt.Errorf("pipeline is disabled")
			}
			return nativePipeline.EnsureEncoder(streamID)
		})

		// Load persisted entities + validation/pipeline-switch data once.
		if err := streamStore.Load(); err != nil {
			logger.Warn("Failed to load streams.toml", logging.KeyError, err)
		}
		var (
			sourceSvc   api.SourceService
			composerSvc api.ComposerService
			streamSvc   api.StreamService
		)
		if entityStore != nil {
			sourceOpts := services.SourceServiceOptions{
				Store:          entityStore,
				Pipeline:       nativePipeline,
				PipelineSwitch: streamStore,
			}
			if ctlServer != nil {
				sourceOpts.ColorMatrix = ctlServer
			}
			sourceSvc = services.NewSourceService(sourceOpts)
			composerSvc = services.NewComposerService(services.ComposerServiceOptions{
				Store:          entityStore,
				Pipeline:       nativePipeline,
				PipelineSwitch: streamStore,
			})
			streamSvc = services.NewStreamService(services.StreamServiceOptions{
				Store:          entityStore,
				Pipeline:       nativePipeline,
				PipelineSwitch: streamStore,
			})
		}

		validationProvider := streams.NewValidationService(streamStore)

		// Replay v2 entities into the pipeline at startup. When the
		// pipeline switch is on, all processes spawn. When off, entities
		// are registered in the pipeline registry (for upstream-ref
		// resolution) but no processes are spawned.
		if entityStore != nil {
			if err := streams.ReplayV2Entities(entityStore, nativePipeline, streamStore.GetPipeline().Enabled); err != nil {
				logger.Warn("Failed to replay v2 entities", logging.KeyError, err)
			}
		}

		// Create authenticator
		authenticator := auth.New(auth.Config{
			Type:     opts.AuthType,
			Username: opts.AuthUsername,
			Password: opts.AuthPassword,
		}, logging.GetLogger("auth"))

		snapshotCache := snapshots.NewCache(
			snapshots.Config{MaxFPS: opts.PreviewMaxFPS},
			nativePipeline,
			snapshots.FFmpegEncoder{},
		)

		apiOpts := &api.Options{
			Authenticator:      authenticator,
			StreamService:      streamSvc,
			SourceService:      sourceSvc,
			ComposerService:    composerSvc,
			ValidationProvider: validationProvider,
			EventBus:           eventBus,
			EventRegistry:      eventRegistry,
			WebRTCManager:      webrtcManager,
			SRTServer:          srtServer,
			StreamProvider:     streamingServer,
			SnapshotCache:      snapshotCache,
			PrometheusHandler:  promhttp.Handler(), // Prometheus metrics via promauto
			ControlServer:      ctlServer,
			ProcessesProvider:  nativePipeline,
			StreamingRTSPPort:  opts.StreamingRTSPPort,
			StreamingSRTPort:   opts.SRTAddr,
		}

		// Add LED controller if available
		if ledController != nil {
			apiOpts.LEDController = ledController
		}

		server := api.NewServer(apiOpts)

		// Self-check the entity registry against the actual Huma route
		// table. Fails fast on common wiring mistakes: a registered
		// entity with no CRUD routes (forgot RegisterEntityRoutes), or
		// a CRUD route with no events.Register call (forgot to wire a
		// new entity into the registry). The error message names the
		// missing call so future contributors don't have to grep.
		if err := eventRegistry.SelfCheck(context.Background(), server); err != nil {
			logger.Error("entity registry self-check failed", logging.KeyError, err)
			os.Exit(1)
		}

		// Create SSE exporter if enabled
		if opts.SSEEnabled {
			sseExporter = exporters.NewSSEExporter(eventRegistry, func() bool {
				return server.SSEClientCount() > 0
			})
		}

		hooks.OnStart(func() {
			// net/http/pprof on DefaultServeMux, localhost by default.
			if opts.FeaturesPprof {
				logger.Info("Starting pprof server", logging.KeyAddr, opts.PprofAddr)
				go func() {
					if err := http.ListenAndServe(opts.PprofAddr, nil); err != nil && !errors.Is(err, http.ErrServerClosed) {
						logger.Error("pprof server failed", logging.KeyError, err)
					}
				}()
			}

			// Start RTSP streaming server first (must be ready for FFmpeg)
			if err := streamingServer.Start(opts.StreamingRTSPPort); err != nil {
				logger.Error("Failed to start RTSP server", logging.KeyError, err)
				os.Exit(1)
			}

			// Start SRT server if enabled
			if srtServer != nil {
				if startErr := srtServer.Start(); startErr != nil {
					logger.Error("Failed to start SRT server", logging.KeyError, startErr)
					os.Exit(1)
				}
			}

			// Start SSE exporter if enabled
			if sseExporter != nil {
				sseExporter.Start(context.Background())
			}

			// Per-stream consumer-count broadcaster. Polls every second
			// and emits an `entity.stream.consumers` envelope so the UI's
			// per-stream reader-count column updates without piggybacking
			// on encoder restarts.
			if eventRegistry != nil && streamSvc != nil {
				go func() {
					ticker := time.NewTicker(1 * time.Second)
					defer ticker.Stop()
					lastConsumers := make(map[string]streaming.StreamConsumersPayload)
					var lastClients int64
					for range ticker.C {
						// Force a full re-emit when a new subscriber connects
						// so it sees current counts without waiting for a change.
						clients := server.SSEClientCount()
						forceEmit := clients > lastClients
						lastClients = clients
						if clients == 0 {
							continue
						}

						streamsCtx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
						list, err := streamSvc.List(streamsCtx)
						cancel()
						if err != nil {
							continue
						}
						for _, st := range list {
							sid := st.ID
							rtsp := streamingServer.StreamRTSPCount(sid)
							webrtcCount := webrtcManager.StreamPeerCount(sid)
							srtCount := 0
							if srtServer != nil {
								srtCount = srtServer.StreamConsumerCount(sid)
							}

							payload := streaming.StreamConsumersPayload{
								Total:         rtsp + webrtcCount + srtCount,
								RTSP:          rtsp,
								WebRTC:        webrtcCount,
								SRT:           srtCount,
								WebRTCClients: webrtcManager.StreamPeerInfo(sid),
								RTSPClients:   streamingServer.StreamRTSPInfo(sid),
							}
							if srtServer != nil {
								payload.SRTClients = srtServer.StreamConsumerInfo(sid)
							}

							// Client-info slices only change when counts do.
							if !forceEmit {
								if prev, ok := lastConsumers[sid]; ok &&
									prev.Total == payload.Total && prev.RTSP == payload.RTSP &&
									prev.WebRTC == payload.WebRTC && prev.SRT == payload.SRT {
									continue
								}
							}
							lastConsumers[sid] = payload
							eventRegistry.Publish("stream", events.ActionConsumers, sid, payload)
						}
					}
				}()
			}

			logger.Info("Starting HTTP server", logging.KeyPort, opts.Port)
			if err := server.Start(opts.Port); err != nil && !errors.Is(err, http.ErrServerClosed) {
				logger.Error("Failed to start HTTP server", logging.KeyError, err)
				os.Exit(1)
			}
		})

		hooks.OnStop(func() {
			logger.Info("Shutting down server")
			if err := server.Stop(); err != nil {
				logger.Error("Error stopping HTTP server", logging.KeyError, err)
			}

			// Tear down the native control plane FIRST: cancels in-flight
			// StreamStatus goroutines and closes the StatusFeed channel
			// that the fan-out goroutines drain. This must happen before
			// Pool().StopAll() kills the source processes, otherwise the
			// StreamStatus recv loop enters a retry loop against a dead
			// socket until the 30s StaleStreamTimeout evicts it.
			if ctlServer != nil {
				if err := ctlServer.Stop(); err != nil {
					logger.Error("Error stopping control manager", logging.KeyError, err)
				}
			}

			// Stop all supervised stream/source/composer processes.
			logger.Info("Stopping all stream processes")
			nativePipeline.Pool().StopAll()

			// Stop streaming server after FFmpeg processes
			if err := streamingServer.Stop(); err != nil {
				logger.Error("Error stopping RTSP server", logging.KeyError, err)
			}

			// Stop WebRTC peers
			webrtcManager.Stop()

			// Stop SRT server
			if srtServer != nil {
				srtServer.Stop()
			}

			if sseExporter != nil {
				sseExporter.Stop()
			}
			if mppCollector != nil {
				_ = mppCollector.Stop()
			}
		})
	})

	// Add validate-encoders command. The path resolver shares the same precedence
	// the server uses (flag → env → default). We can't reuse opts.StreamsConfigFile
	// here because the lightweight subcommand short-circuits the humacli init that
	// populates it.
	cli.Root().AddCommand(cmd.CreateValidateEncodersCmd(cmd.ResolveStreamsConfigPath))
	cli.Root().AddCommand(cmd.CreateValidateConfigCmd(cmd.ResolveStreamsConfigPath))

	// Add entity-CRUD subcommands. Each is a thin REST client; the daemon owns the schema.
	cli.Root().AddCommand(cmd.CreateSourceCmd())
	cli.Root().AddCommand(cmd.CreateComposerCmd())
	cli.Root().AddCommand(cmd.CreateStreamCmd())

	// Add openapi command
	cli.Root().AddCommand(cmd.CreateOpenAPICmd())

	// Add version command
	cli.Root().AddCommand(cmd.CreateVersionCmd())

	// Run the CLI
	cli.Run()
}
