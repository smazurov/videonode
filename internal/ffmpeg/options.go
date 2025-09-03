package ffmpeg

import (
	"fmt"
	"strings"
)

// OptionType represents a strongly typed FFmpeg option
type OptionType string

// FFmpeg option constants
const (
	OptionIgnoreErrors        OptionType = "ignore_err"
	OptionWallclockWithGenpts OptionType = "wallclock_with_genpts"
	OptionThreadQueue1024     OptionType = "thread_queue_1024"
	OptionThreadQueue4096     OptionType = "thread_queue_4096"
	OptionLowLatency          OptionType = "low_latency"
	OptionCopytsWithGenpts    OptionType = "copyts_with_genpts"
	OptionVsyncPassthrough    OptionType = "vsync_passthrough"
)

// FFmpegBase returns the ffmpeg command with standard flags
func FFmpegBase() string {
	return "ffmpeg -hide_banner"
}

// FFprobeBase returns the ffprobe command with standard flags
func FFprobeBase() string {
	return "ffprobe -hide_banner"
}

// Helper functions for internal use
func ffmpegBase() string {
	return FFmpegBase()
}

func ffprobeBase() string {
	return FFprobeBase()
}

// OptionCategory represents option categories
type OptionCategory string

const (
	CategoryTiming      OptionCategory = "Timing"
	CategoryErrorHandle OptionCategory = "Error Handling"
	CategoryPerformance OptionCategory = "Performance"
)

// ExclusiveGroup represents a group of mutually exclusive options
type ExclusiveGroup string

const (
	GroupThreadQueue       ExclusiveGroup = "thread_queue"
	GroupTimestampHandling ExclusiveGroup = "timestamp_handling"
)

// Option represents available FFmpeg feature flags with metadata
type Option struct {
	Key            OptionType      `json:"key"`
	Name           string          `json:"name"`
	Description    string          `json:"description"`
	Category       OptionCategory  `json:"category"`
	AppDefault     bool            `json:"app_default"`               // Application default
	ExclusiveGroup *ExclusiveGroup `json:"exclusive_group,omitempty"` // Group for mutually exclusive options
	ConflictsWith  []OptionType    `json:"conflicts_with,omitempty"`  // Options that may conflict
}

// AllOptions contains all available FFmpeg feature flags with comprehensive metadata
var AllOptions = []Option{

	{
		Key:         OptionIgnoreErrors,
		Name:        "Ignore Errors",
		Description: "Continue processing despite errors",
		Category:    CategoryErrorHandle,
		AppDefault:  false,
	},
	{
		Key:            OptionWallclockWithGenpts,
		Name:           "Wall Clock Timestamps with PTS Generation",
		Description:    "Use system time as timestamps with PTS regeneration (for live sources, fixes DTS issues)",
		Category:       CategoryTiming,
		AppDefault:     false,
		ExclusiveGroup: func() *ExclusiveGroup { g := GroupTimestampHandling; return &g }(),
	},

	{
		Key:            OptionThreadQueue1024,
		Name:           "Large Thread Queue",
		Description:    "Use 1024 thread queue size (helps with buffer corruption)",
		Category:       CategoryPerformance,
		AppDefault:     true,
		ExclusiveGroup: func() *ExclusiveGroup { g := GroupThreadQueue; return &g }(), // Mutually exclusive with other thread queue sizes
	},
	{
		Key:            OptionThreadQueue4096,
		Name:           "Extra Large Thread Queue",
		Description:    "Use 4096 thread queue size (for high bitrate streams)",
		Category:       CategoryPerformance,
		AppDefault:     false,
		ExclusiveGroup: func() *ExclusiveGroup { g := GroupThreadQueue; return &g }(), // Mutually exclusive with other thread queue sizes
	},
	{
		Key:         OptionLowLatency,
		Name:        "Low Latency Mode",
		Description: "Optimize for minimal latency",
		Category:    CategoryPerformance,
		AppDefault:  false,
	},
	{
		Key:            OptionCopytsWithGenpts,
		Name:           "Copy Timestamps with PTS Generation",
		Description:    "Preserve original timestamps with PTS regeneration (fixes V4L2 and DTS issues)",
		Category:       CategoryTiming,
		AppDefault:     true, // MAKE THIS DEFAULT FOR ALL STREAMS
		ExclusiveGroup: func() *ExclusiveGroup { g := GroupTimestampHandling; return &g }(),
	},
	{
		Key:         OptionVsyncPassthrough,
		Name:        "Vsync Passthrough",
		Description: "Pass frames exactly as they arrive from input without dropping or duplicating (fps_mode passthrough)",
		Category:    CategoryTiming,
		AppDefault:  false,
	},
}

// GetOptionByKey returns an option by its key
func GetOptionByKey(key OptionType) *Option {
	for i := range AllOptions {
		if AllOptions[i].Key == key {
			return &AllOptions[i]
		}
	}
	return nil
}

// GetOptionsByCategory returns options grouped by category
func GetOptionsByCategory() map[OptionCategory][]Option {
	categories := make(map[OptionCategory][]Option)
	for _, option := range AllOptions {
		categories[option.Category] = append(categories[option.Category], option)
	}
	return categories
}

// GetExclusiveGroups returns options grouped by their exclusive groups
func GetExclusiveGroups() map[ExclusiveGroup][]Option {
	groups := make(map[ExclusiveGroup][]Option)
	for _, option := range AllOptions {
		if option.ExclusiveGroup != nil {
			groups[*option.ExclusiveGroup] = append(groups[*option.ExclusiveGroup], option)
		}
	}
	return groups
}

// ValidateOptions checks for conflicts and exclusive group violations
func ValidateOptions(selectedOptions []OptionType) error {
	// Check for exclusive group violations
	exclusiveGroups := make(map[ExclusiveGroup][]OptionType)

	for _, optionKey := range selectedOptions {
		option := GetOptionByKey(optionKey)
		if option == nil {
			continue
		}

		if option.ExclusiveGroup != nil {
			exclusiveGroups[*option.ExclusiveGroup] = append(exclusiveGroups[*option.ExclusiveGroup], optionKey)
		}
	}

	// Check if multiple options from same exclusive group are selected
	for group, options := range exclusiveGroups {
		if len(options) > 1 {
			var optionNames []string
			for _, opt := range options {
				if option := GetOptionByKey(opt); option != nil {
					optionNames = append(optionNames, option.Name)
				}
			}
			return fmt.Errorf("multiple options from exclusive group '%s' selected: %s", group, strings.Join(optionNames, ", "))
		}
	}

	// Check for conflicting options
	selectedSet := make(map[OptionType]bool)
	for _, opt := range selectedOptions {
		selectedSet[opt] = true
	}

	for _, optionKey := range selectedOptions {
		option := GetOptionByKey(optionKey)
		if option == nil {
			continue
		}

		for _, conflictOpt := range option.ConflictsWith {
			if selectedSet[conflictOpt] {
				conflictOption := GetOptionByKey(conflictOpt)
				conflictName := string(conflictOpt)
				if conflictOption != nil {
					conflictName = conflictOption.Name
				}
				return fmt.Errorf("option '%s' conflicts with '%s'", option.Name, conflictName)
			}
		}
	}

	return nil
}

// GetDefaultOptions returns the options that are enabled by default in the application
func GetDefaultOptions() []OptionType {
	var defaults []OptionType
	for _, option := range AllOptions {
		if option.AppDefault {
			defaults = append(defaults, option.Key)
		}
	}
	return defaults
}

// StreamConfig represents the parameters for creating a stream (moved from mediamtx)
type StreamConfig struct {
	DevicePath     string
	InputFormat    string // FFmpeg input format (e.g., "yuyv422", "mjpeg")
	Resolution     string
	FPS            string
	Codec          string
	Preset         string
	Bitrate        string            // Video bitrate (e.g., "2M", "1000k")
	FFmpegOptions  []OptionType      // Strongly typed FFmpeg feature flags/options
	ProgressSocket string            // Optional socket path for FFmpeg progress monitoring
	GlobalArgs     []string          // Global FFmpeg arguments (e.g., -vaapi_device)
	EncoderParams  map[string]string // Encoder-specific parameters (e.g., qp, cq)
	VideoFilters   string            // Video filter chain (e.g., format=nv12,hwupload)
	AudioDevice    string            // ALSA audio device (e.g., "hw:4,0") - if set, enables audio passthrough
}

// CaptureConfig represents parameters for screenshot capture
type CaptureConfig struct {
	DevicePath    string
	OutputPath    string
	InputFormat   string // FFmpeg input format (e.g., "yuyv422", "mjpeg")
	Resolution    string
	FPS           string
	DelayMs       int // Delay in milliseconds before capture
	FFmpegOptions []OptionType
}

// CommandBuilder interface for generating FFmpeg commands
type CommandBuilder interface {
	BuildStreamCommand(config StreamConfig) (string, error)
	BuildCaptureCommand(config CaptureConfig) (string, error)
	BuildProbeCommand(devicePath string) (string, error)
	BuildEncodersListCommand() (string, error)
}

// DefaultCommandBuilder implements CommandBuilder with manual command construction
type DefaultCommandBuilder struct{}

// NewCommandBuilder creates a new default command builder
func NewCommandBuilder() CommandBuilder {
	return &DefaultCommandBuilder{}
}

// ApplyOptionsToCommand applies FFmpeg options to a command string builder
func ApplyOptionsToCommand(options []OptionType, cmd *strings.Builder) []OptionType {
	var appliedOptions []OptionType
	var fflags []string

	for _, option := range options {
		switch option {
		case OptionIgnoreErrors:
			cmd.WriteString(" -err_detect ignore_err")
			appliedOptions = append(appliedOptions, OptionIgnoreErrors)
		case OptionWallclockWithGenpts:
			cmd.WriteString(" -use_wallclock_as_timestamps 1")
			fflags = append(fflags, "+genpts")
			appliedOptions = append(appliedOptions, OptionWallclockWithGenpts)
		case OptionThreadQueue1024:
			cmd.WriteString(" -thread_queue_size 1024")
			appliedOptions = append(appliedOptions, OptionThreadQueue1024)
		case OptionThreadQueue4096:
			cmd.WriteString(" -thread_queue_size 4096")
			appliedOptions = append(appliedOptions, OptionThreadQueue4096)
		case OptionLowLatency:
			cmd.WriteString(" -fflags +flush_packets")
			cmd.WriteString(" -flags +low_delay")
			appliedOptions = append(appliedOptions, OptionLowLatency)
		case OptionCopytsWithGenpts:
			// Note: copyts and start_at_zero need to be applied AFTER input
			// They will be handled separately in BuildStreamCommand
			// But we add genpts to fflags here
			fflags = append(fflags, "+genpts")
			appliedOptions = append(appliedOptions, OptionCopytsWithGenpts)
		case OptionVsyncPassthrough:
			// Note: fps_mode needs to be applied AFTER input
			// It will be handled separately in BuildStreamCommand
			appliedOptions = append(appliedOptions, OptionVsyncPassthrough)
		}
	}

	// Apply fflags if any were collected
	if len(fflags) > 0 {
		cmd.WriteString(fmt.Sprintf(" -fflags %s", strings.Join(fflags, "")))
	}

	return appliedOptions
}

// BuildStreamCommand creates an FFmpeg command for MediaMTX based on stream configuration
func (cb *DefaultCommandBuilder) BuildStreamCommand(streamConfig StreamConfig) (string, error) {
	if streamConfig.DevicePath == "" {
		return "", fmt.Errorf("device path is required")
	}

	var cmd strings.Builder
	cmd.WriteString(ffmpegBase())

	// Add progress monitoring if socket path is provided
	if streamConfig.ProgressSocket != "" {
		cmd.WriteString(fmt.Sprintf(" -progress unix://%s", streamConfig.ProgressSocket))
		// Log the socket path for debugging
		fmt.Printf("[FFMPEG] FFmpeg will attempt to connect to progress socket: %s\n", streamConfig.ProgressSocket)
	}

	// Add global args BEFORE input (e.g., -vaapi_device)
	for _, arg := range streamConfig.GlobalArgs {
		cmd.WriteString(" " + arg)
	}

	// Input parameters
	cmd.WriteString(" -f v4l2")

	// Apply configurable FFmpeg options
	appliedOptions := ApplyOptionsToCommand(streamConfig.FFmpegOptions, &cmd)
	if len(appliedOptions) > 0 {
		fmt.Printf("[FFMPEG] Applied FFmpeg options: %v\n", appliedOptions)
	}

	// Use the selected FFmpeg input format
	if streamConfig.InputFormat != "" {
		cmd.WriteString(fmt.Sprintf(" -input_format %s", streamConfig.InputFormat))
		fmt.Printf("[FFMPEG] Using input format: %s\n", streamConfig.InputFormat)
	} else {
		cmd.WriteString(" -input_format yuyv422") // Default to YUYV
	}

	// Resolution - only add if specified, let device decide if empty
	if streamConfig.Resolution != "" {
		cmd.WriteString(fmt.Sprintf(" -video_size %s", streamConfig.Resolution))
	}

	// Framerate - only add if specified, let device decide if empty
	if streamConfig.FPS != "" {
		cmd.WriteString(fmt.Sprintf(" -framerate %s", streamConfig.FPS))
	}

	// Input device
	cmd.WriteString(fmt.Sprintf(" -i %s", streamConfig.DevicePath))

	// Add audio input if audio device is specified
	if streamConfig.AudioDevice != "" {
		// Audio thread queue size (increased to prevent buffer underruns)
		cmd.WriteString(" -thread_queue_size 10240")
		// ALSA input
		cmd.WriteString(" -f alsa")
		// Sample format (16-bit signed integer for compatibility)
		cmd.WriteString(" -sample_fmt s16")
		// Sample rate (48kHz is standard)
		cmd.WriteString(" -ar 48000")
		// Stereo channels
		cmd.WriteString(" -ac 2")
		// Audio device
		cmd.WriteString(fmt.Sprintf(" -i %s", streamConfig.AudioDevice))
	}

	// Check if copyts option is enabled and apply it AFTER input
	hasCopyTS := false
	hasVsyncPassthrough := false
	for _, opt := range appliedOptions {
		if opt == OptionCopytsWithGenpts {
			hasCopyTS = true
		}
		if opt == OptionVsyncPassthrough {
			hasVsyncPassthrough = true
		}
	}
	if hasCopyTS {
		cmd.WriteString(" -copyts -start_at_zero")
	}
	if hasVsyncPassthrough {
		cmd.WriteString(" -fps_mode passthrough")
	}

	// Add stream mapping if audio is enabled
	if streamConfig.AudioDevice != "" {
		cmd.WriteString(" -map 0:v -map 1:a")
	}

	// Add video filters AFTER input, BEFORE codec
	if streamConfig.VideoFilters != "" {
		cmd.WriteString(fmt.Sprintf(" -vf %s", streamConfig.VideoFilters))
	}

	// Use configured codec or default to libx264
	codec := streamConfig.Codec
	if codec == "" {
		codec = "libx264" // Default software encoder
	}
	cmd.WriteString(fmt.Sprintf(" -c:v %s", codec))

	// Add encoder-specific params (qp, cq, preset, etc.)
	for key, value := range streamConfig.EncoderParams {
		cmd.WriteString(fmt.Sprintf(" -%s %s", key, value))
	}

	// Add preset if specified (legacy support)
	if streamConfig.Preset != "" {
		cmd.WriteString(fmt.Sprintf(" -preset %s", streamConfig.Preset))
	}

	// Add bitrate if specified (legacy support)
	if streamConfig.Bitrate != "" {
		cmd.WriteString(fmt.Sprintf(" -b:v %s", streamConfig.Bitrate))
	}

	// Low latency settings for software encoders only
	if !isHardwareEncoder(codec) {
		cmd.WriteString(" -tune zerolatency")
		// These parameters are only supported by software encoders
		// Hardware encoders should set these via EncoderParams if needed
		cmd.WriteString(" -g 30")           // GOP size
		cmd.WriteString(" -keyint_min 15")  // Minimum GOP size
		cmd.WriteString(" -sc_threshold 0") // Disable scene change detection
	}

	// Add audio codec if audio device is specified
	if streamConfig.AudioDevice != "" {
		// Use Opus codec for WebRTC compatibility (also works with RTSP)
		// Opus is the preferred audio codec for WebRTC and modern streaming
		cmd.WriteString(" -c:a libopus -b:a 128k -ar 48000")
	}

	// Output format and destination
	cmd.WriteString(" -rtsp_transport tcp") // Use TCP for more reliable streaming
	cmd.WriteString(" -f rtsp")
	cmd.WriteString(" rtsp://localhost:8554/$MTX_PATH") // MediaMTX will replace $MTX_PATH

	return cmd.String(), nil
}

// BuildCaptureCommand creates an FFmpeg command for screenshot capture
func (cb *DefaultCommandBuilder) BuildCaptureCommand(config CaptureConfig) (string, error) {
	if config.DevicePath == "" {
		return "", fmt.Errorf("device path is required")
	}
	if config.OutputPath == "" {
		return "", fmt.Errorf("output path is required")
	}

	var cmd strings.Builder
	cmd.WriteString(ffmpegBase())

	// Add delay if specified (using input seeking)
	if config.DelayMs > 0 {
		delaySeconds := float64(config.DelayMs) / 1000.0
		cmd.WriteString(fmt.Sprintf(" -ss %.3f", delaySeconds))
	}

	// Input parameters
	cmd.WriteString(" -f v4l2")

	// Apply configurable FFmpeg options
	appliedOptions := ApplyOptionsToCommand(config.FFmpegOptions, &cmd)
	if len(appliedOptions) > 0 {
		fmt.Printf("[FFMPEG] Applied FFmpeg options for capture: %v\n", appliedOptions)
	}

	// Use the selected FFmpeg input format
	if config.InputFormat != "" {
		cmd.WriteString(fmt.Sprintf(" -input_format %s", config.InputFormat))
	} else {
		cmd.WriteString(" -input_format yuyv422") // Default to YUYV
	}

	// Resolution - only add if specified, let device decide if empty
	if config.Resolution != "" {
		cmd.WriteString(fmt.Sprintf(" -video_size %s", config.Resolution))
	}

	// Framerate - only add if specified, let device decide if empty
	if config.FPS != "" {
		cmd.WriteString(fmt.Sprintf(" -framerate %s", config.FPS))
	}

	// Input device
	cmd.WriteString(fmt.Sprintf(" -i %s", config.DevicePath))

	// Capture single frame
	cmd.WriteString(" -frames:v 1")

	// Output format and path
	cmd.WriteString(" -y") // Overwrite output file
	cmd.WriteString(fmt.Sprintf(" %s", config.OutputPath))

	return cmd.String(), nil
}

// BuildProbeCommand creates an FFmpeg command for probing device capabilities
func (cb *DefaultCommandBuilder) BuildProbeCommand(devicePath string) (string, error) {
	if devicePath == "" {
		return "", fmt.Errorf("device path is required")
	}

	return fmt.Sprintf("%s -f v4l2 -list_formats all -i %s", ffprobeBase(), devicePath), nil
}

// BuildEncodersListCommand creates an FFmpeg command for listing available encoders
func (cb *DefaultCommandBuilder) BuildEncodersListCommand() (string, error) {
	return fmt.Sprintf("%s -encoders", ffmpegBase()), nil
}

// GenerateCommand provides backward compatibility - delegates to BuildStreamCommand
func GenerateCommand(streamConfig StreamConfig) (string, error) {
	builder := NewCommandBuilder()
	return builder.BuildStreamCommand(streamConfig)
}

// isHardwareEncoder checks if the given codec name represents a hardware encoder
func isHardwareEncoder(codec string) bool {
	hardwareCodecs := []string{
		"nvenc", "amf", "vaapi", "qsv", "videotoolbox", "rkmpp", "v4l2m2m",
	}

	for _, hwCodec := range hardwareCodecs {
		if strings.Contains(codec, hwCodec) {
			return true
		}
	}
	return false
}
