package cmd

import (
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/smazurov/videonode/internal/config"
	"github.com/smazurov/videonode/internal/devices"
	"github.com/smazurov/videonode/internal/encoders"
	"github.com/smazurov/videonode/internal/ffmpeg"
	"github.com/smazurov/videonode/internal/logging"
	"github.com/smazurov/videonode/internal/process"
	"github.com/smazurov/videonode/internal/streams"
	"github.com/smazurov/videonode/internal/streams/store"
	"github.com/smazurov/videonode/internal/types"
	valManager "github.com/smazurov/videonode/internal/validation"
	"github.com/spf13/cobra"
)

// CreateStreamCmd creates the stream command.
func CreateStreamCmd() *cobra.Command {
	var configFile string
	var encoderOverride string
	var logJSON bool

	cmd := &cobra.Command{
		Use:   "stream [stream-id]",
		Short: "Run stream process manager",
		Long: `Spawns and manages a streaming process (FFmpeg, GStreamer, etc.) for the specified stream ID. ` +
			`Loads configuration from streams.toml and handles process lifecycle including graceful shutdown.`,
		Args: cobra.ExactArgs(1),
		Run: func(_ *cobra.Command, args []string) {
			streamID := args[0]

			// Initialize minimal logging
			loggingConfig := logging.Config{
				Level:  "info",
				Format: "text",
			}
			if logJSON {
				loggingConfig.Format = "json"
			}
			logging.Initialize(loggingConfig)
			// Create logger with stream_id context for journal integration
			logger := logging.GetLogger("stream").With("stream_id", streamID)

			logger.Info("Starting stream command", "config", configFile)

			// Load stream store
			streamStore := store.NewTOML(configFile)
			if err := streamStore.Load(); err != nil {
				logger.Error("Failed to load streams configuration", "error", err, "config", configFile)
				os.Exit(1)
			}

			// Verify stream exists
			_, exists := streamStore.GetStream(streamID)
			if !exists {
				logger.Error("Stream not found")
				os.Exit(1)
			}

			// Create encoder selector
			validationService := streams.NewValidationService(streamStore)
			vm := valManager.NewManager(validationService)
			if err := vm.LoadValidation(); err != nil {
				logger.Warn("Failed to load validation data, using software encoders", "error", err)
			}
			encoderSelector := encoders.NewDefaultSelector(vm)

			// Create processor
			processor := createStreamProcessor(streamStore, encoderSelector, logger)

			// Process stream to generate command
			var processed *ProcessedStream
			var err error
			if encoderOverride != "" {
				processed, err = processor.ProcessStreamWithEncoder(streamID, encoderOverride)
			} else {
				processed, err = processor.ProcessStream(streamID)
			}

			if err != nil {
				logger.Error("Failed to process stream", "error", err)
				os.Exit(1)
			}

			logger.Info("Generated command")

			// Create process with ffmpeg log parsing
			mgr := process.NewProcess(streamID, processed.Command, logger)
			mgr.SetLogParser(logging.GetLogger("ffmpeg"), ffmpeg.ParseLogLevel)

			// Create typed config watcher with fresh config loading
			streamsLoader := func(path string) (map[string]streams.StreamSpec, error) {
				s := store.NewTOML(path)
				if err := s.Load(); err != nil {
					return nil, err
				}
				return s.GetAllStreams(), nil
			}

			watcher := config.NewConfigWatcher(
				configFile,
				streamsLoader,
				logger,
				config.WithDebounce[map[string]streams.StreamSpec](1500*time.Millisecond),
			)

			watcher.OnReload(func(allStreams map[string]streams.StreamSpec) {
				// Check if stream still exists in fresh config
				var streamSpec streams.StreamSpec
				streamSpec, exists = allStreams[streamID]
				if !exists {
					logger.Warn("Stream removed from config, shutting down")
					mgr.Shutdown()
					return
				}

				// Regenerate command with fresh stream spec
				var newProcessed *ProcessedStream
				newProcessed, err = processor.ProcessStreamSpec(streamID, streamSpec, encoderOverride)
				if err != nil {
					logger.Warn("Failed to regenerate command", "error", err)
					return
				}

				// Compare and restart if command changed
				if newProcessed.Command != mgr.GetCommand() {
					logger.Info("Command changed, requesting restart")
					mgr.RequestRestart(newProcessed.Command)
				} else {
					logger.Debug("Config reloaded, command unchanged")
				}
			})

			// Start config watcher (non-fatal if it fails)
			if err := watcher.Start(); err != nil {
				logger.Warn("Failed to start config watcher, hot-reload disabled", "error", err)
			} else {
				defer func() { _ = watcher.Stop() }()
			}

			// Run with restart support
			exitCode := mgr.RunWithRestart()

			logger.Info("Stream command exiting", "exit_code", exitCode)
			os.Exit(exitCode)
		},
	}

	cmd.Flags().StringVar(&configFile, "config", "streams.toml", "Path to streams configuration file")
	cmd.Flags().StringVar(&encoderOverride, "encoder-override", "",
		"Override encoder selection (e.g., h264_vaapi, libx264)")
	cmd.Flags().BoolVar(&logJSON, "log-json", false, "Use JSON log format")

	return cmd
}

// ProcessedStream represents a processed stream ready to execute.
type ProcessedStream struct {
	StreamID string
	Command  string
}

// StreamProcessor wraps the internal processor for command generation.
type StreamProcessor struct {
	store           streams.Store
	encoderSelector func(
		codec string,
		inputFormat string,
		qualityParams *types.QualityParams,
		encoderOverride string,
	) *ffmpeg.Params
	deviceResolver func(deviceID string) string
}

// createStreamProcessor creates a minimal processor for command generation.
func createStreamProcessor(
	streamStore streams.Store,
	encoderSelector encoders.Selector,
	logger *slog.Logger,
) *StreamProcessor {
	// Create encoder selector function
	encoderSelectorFunc := func(
		codec string,
		inputFormat string,
		qualityParams *types.QualityParams,
		encoderOverride string,
	) *ffmpeg.Params {
		// Convert codec string to CodecType
		var codecType encoders.CodecType
		if codec == "h265" {
			codecType = encoders.CodecH265
		} else {
			codecType = encoders.CodecH264
		}

		// Select optimal encoder
		params, err := encoderSelector.SelectEncoder(codecType, inputFormat, qualityParams, encoderOverride)
		if err != nil {
			logger.Warn("Failed to select encoder, using software fallback", "error", err)
			// Return software fallback
			params = &ffmpeg.Params{}
			switch {
			case encoderOverride != "":
				params.Encoder = encoderOverride
			case codecType == encoders.CodecH265:
				params.Encoder = "libx265"
			default:
				params.Encoder = "libx264"
			}
		}

		return params
	}

	// Create device resolver function
	deviceResolverFunc := func(deviceID string) string {
		devicePath, err := devices.ResolveDevicePath(deviceID)
		if err != nil {
			logger.Warn("Device resolution failed", "device_id", deviceID, "error", err)
			return ""
		}
		return devicePath
	}

	return &StreamProcessor{
		store:           streamStore,
		encoderSelector: encoderSelectorFunc,
		deviceResolver:  deviceResolverFunc,
	}
}

// ProcessStream processes a stream and returns the command.
func (p *StreamProcessor) ProcessStream(streamID string) (*ProcessedStream, error) {
	return p.ProcessStreamWithEncoder(streamID, "")
}

// ProcessStreamWithEncoder processes a stream with optional encoder override.
func (p *StreamProcessor) ProcessStreamWithEncoder(streamID string, encoderOverride string) (*ProcessedStream, error) {
	streamConfig, exists := p.store.GetStream(streamID)
	if !exists {
		return nil, fmt.Errorf("stream %s not found", streamID)
	}
	return p.ProcessStreamSpec(streamID, streamConfig, encoderOverride)
}

// ProcessStreamSpec processes a stream spec directly (for config reload with fresh data).
func (p *StreamProcessor) ProcessStreamSpec(
	streamID string,
	streamConfig streams.StreamSpec,
	encoderOverride string,
) (*ProcessedStream, error) {
	// For stream command, always assume enabled=true (device is available)
	// The command will fail if device is not available, which is expected behavior
	enabled := true

	// Check for custom command first
	if streamConfig.CustomFFmpegCommand != "" {
		return &ProcessedStream{
			StreamID: streamID,
			Command:  streamConfig.CustomFFmpegCommand,
		}, nil
	}

	// Determine if using test source
	useTestSource := streamConfig.TestMode

	// Resolve device path (skip if using test source)
	var devicePath string
	if !useTestSource {
		devicePath = p.deviceResolver(streamConfig.Device)
		if devicePath == "" {
			// Device not found - treat as offline
			enabled = false
			useTestSource = streamConfig.TestMode || !enabled
		}
	}

	// Skip progress socket for standalone operation
	// Progress socket is only needed when OBS monitoring is active
	socketPath := ""

	// Select encoder and get settings
	var ffmpegParams *ffmpeg.Params

	if streamConfig.FFmpeg.Codec != "" {
		// Use encoder selector
		inputFormat := streamConfig.FFmpeg.InputFormat
		if useTestSource {
			inputFormat = "testsrc"
		}

		ffmpegParams = p.encoderSelector(
			streamConfig.FFmpeg.Codec,
			inputFormat,
			streamConfig.FFmpeg.QualityParams,
			encoderOverride,
		)

		// Set preset for software encoders if not already set
		if ffmpegParams.Preset == "" &&
			(ffmpegParams.Encoder == "libx264" || ffmpegParams.Encoder == "libx265") {
			ffmpegParams.Preset = "fast"
		}
	} else {
		// Default fallback
		ffmpegParams = &ffmpeg.Params{
			Encoder: "libx264",
			Preset:  "fast",
			Bitrate: "2M",
		}

		if encoderOverride != "" {
			ffmpegParams.Encoder = encoderOverride
		}
	}

	// Apply stream settings to FFmpeg params
	ffmpegParams.DevicePath = devicePath
	ffmpegParams.InputFormat = streamConfig.FFmpeg.InputFormat
	ffmpegParams.Resolution = streamConfig.FFmpeg.Resolution
	ffmpegParams.FPS = streamConfig.FFmpeg.FPS
	ffmpegParams.AudioDevice = streamConfig.FFmpeg.AudioDevice

	// Set default audio resampling filter when audio device is present
	if streamConfig.FFmpeg.AudioDevice != "" {
		ffmpegParams.AudioFilters = "aresample=async=1:min_hard_comp=0.100000:first_pts=0"
	}

	ffmpegParams.ProgressSocket = socketPath
	ffmpegParams.Options = streamConfig.FFmpeg.Options
	ffmpegParams.OutputURL = fmt.Sprintf("srt://localhost:8890?streamid=publish:%s", streamID)

	// Determine test source mode and overlay text
	switch {
	case !enabled:
		ffmpegParams.IsTestSource = true
		ffmpegParams.TestOverlay = "NO SIGNAL"
	case streamConfig.TestMode:
		ffmpegParams.IsTestSource = true
		ffmpegParams.TestOverlay = "TEST MODE"
	default:
		ffmpegParams.IsTestSource = false
		ffmpegParams.TestOverlay = ""
	}

	// Build FFmpeg command
	ffmpegCmd := ffmpeg.BuildCommand(ffmpegParams)

	return &ProcessedStream{
		StreamID: streamID,
		Command:  ffmpegCmd,
	}, nil
}
