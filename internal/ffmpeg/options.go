package ffmpeg

import (
	"fmt"
	"strings"
)

// OptionType represents a strongly typed FFmpeg option.
type OptionType string

// FFmpeg option constants.
const (
	OptionIgnoreErrors        OptionType = "ignore_err"
	OptionWallclockWithGenpts OptionType = "wallclock_with_genpts"
	OptionThreadQueue1024     OptionType = "thread_queue_1024"
	OptionThreadQueue4096     OptionType = "thread_queue_4096"
	OptionLowLatency          OptionType = "low_latency"
	OptionCopytsWithGenpts    OptionType = "copyts_with_genpts"
	OptionVsyncPassthrough    OptionType = "vsync_passthrough"
	OptionVerboseLogging      OptionType = "verbose_logging"
)

// Base returns the ffmpeg command with standard flags.
func Base() string {
	return "ffmpeg -hide_banner -nostats -nostdin -loglevel level+info"
}

// Deprecated: use Base.
//
//nolint:revive // backward compat stutter
func FFmpegBase() string {
	return Base()
}

// FFprobeBase returns the ffprobe command with standard flags.
func FFprobeBase() string {
	return "ffprobe -hide_banner -nostats"
}

func ffmpegBase() string {
	return Base()
}

func ffprobeBase() string {
	return FFprobeBase()
}

// OptionCategory represents option categories.
type OptionCategory string

// Option category constants.
const (
	CategoryTiming      OptionCategory = "Timing"
	CategoryErrorHandle OptionCategory = "Error Handling"
	CategoryPerformance OptionCategory = "Performance"
)

// ExclusiveGroup represents a group of mutually exclusive options.
type ExclusiveGroup string

// Exclusive group constants for mutually exclusive options.
const (
	GroupThreadQueue       ExclusiveGroup = "thread_queue"
	GroupTimestampHandling ExclusiveGroup = "timestamp_handling"
)

// Option represents available FFmpeg feature flags with metadata.
type Option struct {
	Key            OptionType      `json:"key"`
	Name           string          `json:"name"`
	Description    string          `json:"description"`
	Category       OptionCategory  `json:"category"`
	AppDefault     bool            `json:"app_default"`
	ExclusiveGroup *ExclusiveGroup `json:"exclusive_group,omitempty"`
	ConflictsWith  []OptionType    `json:"conflicts_with,omitempty"`
}

// AllOptions contains all available FFmpeg feature flags with comprehensive metadata.
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
		ExclusiveGroup: func() *ExclusiveGroup { g := GroupThreadQueue; return &g }(),
	},
	{
		Key:            OptionThreadQueue4096,
		Name:           "Extra Large Thread Queue",
		Description:    "Use 4096 thread queue size (for high bitrate streams)",
		Category:       CategoryPerformance,
		AppDefault:     false,
		ExclusiveGroup: func() *ExclusiveGroup { g := GroupThreadQueue; return &g }(),
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
		AppDefault:     true,
		ExclusiveGroup: func() *ExclusiveGroup { g := GroupTimestampHandling; return &g }(),
	},
	{
		Key:         OptionVsyncPassthrough,
		Name:        "Vsync Passthrough",
		Description: "Pass frames exactly as they arrive from input without dropping or duplicating (fps_mode passthrough)",
		Category:    CategoryTiming,
		AppDefault:  false,
	},
	{
		Key:         OptionVerboseLogging,
		Name:        "Verbose Logging",
		Description: "Show detailed FFmpeg warnings (DTS/PTS issues, encoder errors, input failures)",
		Category:    CategoryErrorHandle,
		AppDefault:  false,
	},
}

// GetOptionByKey returns an option by its key.
func GetOptionByKey(key OptionType) *Option {
	for i := range AllOptions {
		if AllOptions[i].Key == key {
			return &AllOptions[i]
		}
	}
	return nil
}

// GetOptionsByCategory returns options grouped by category.
func GetOptionsByCategory() map[OptionCategory][]Option {
	categories := make(map[OptionCategory][]Option)
	for _, option := range AllOptions {
		categories[option.Category] = append(categories[option.Category], option)
	}
	return categories
}

// GetExclusiveGroups returns options grouped by their exclusive groups.
func GetExclusiveGroups() map[ExclusiveGroup][]Option {
	groups := make(map[ExclusiveGroup][]Option)
	for _, option := range AllOptions {
		if option.ExclusiveGroup != nil {
			groups[*option.ExclusiveGroup] = append(groups[*option.ExclusiveGroup], option)
		}
	}
	return groups
}

// ValidateOptions checks for conflicts and exclusive group violations.
func ValidateOptions(selectedOptions []OptionType) error {
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

// GetDefaultOptions returns the options that are enabled by default in the application.
func GetDefaultOptions() []OptionType {
	var defaults []OptionType
	for _, option := range AllOptions {
		if option.AppDefault {
			defaults = append(defaults, option.Key)
		}
	}
	return defaults
}

// CommandBuilder interface for generating FFmpeg commands.
type CommandBuilder interface {
	BuildProbeCommand(devicePath string) (string, error)
	BuildEncodersListCommand() (string, error)
}

// DefaultCommandBuilder implements CommandBuilder with manual command construction.
type DefaultCommandBuilder struct{}

// NewCommandBuilder creates a new default command builder.
func NewCommandBuilder() CommandBuilder {
	return &DefaultCommandBuilder{}
}

// ApplyOptionsToCommand applies FFmpeg options to a command string builder.
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
			// copyts/start_at_zero apply after input; only genpts goes here.
			fflags = append(fflags, "+genpts")
			appliedOptions = append(appliedOptions, OptionCopytsWithGenpts)
		case OptionVsyncPassthrough:
			// fps_mode applies after input; handled in BuildStreamCommand.
			appliedOptions = append(appliedOptions, OptionVsyncPassthrough)
		case OptionVerboseLogging:
			cmd.WriteString(" -loglevel warning")
			appliedOptions = append(appliedOptions, OptionVerboseLogging)
		}
	}

	if len(fflags) > 0 {
		fmt.Fprintf(cmd, " -fflags %s", strings.Join(fflags, ""))
	}

	return appliedOptions
}

// BuildProbeCommand creates an FFmpeg command for probing device capabilities.
func (cb *DefaultCommandBuilder) BuildProbeCommand(devicePath string) (string, error) {
	if devicePath == "" {
		return "", fmt.Errorf("device path is required")
	}

	return fmt.Sprintf("%s -f v4l2 -list_formats all -i %s", ffprobeBase(), devicePath), nil
}

// BuildEncodersListCommand creates an FFmpeg command for listing available encoders.
func (cb *DefaultCommandBuilder) BuildEncodersListCommand() (string, error) {
	return fmt.Sprintf("%s -encoders", ffmpegBase()), nil
}

// isHardwareEncoder checks if the given codec name represents a hardware encoder.
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

// isHevcOrH264 reports whether the encoder is in the H.264/HEVC family.
func isHevcOrH264(encoder string) bool {
	return strings.Contains(encoder, "h264") ||
		strings.Contains(encoder, "x264") ||
		strings.Contains(encoder, "hevc") ||
		strings.Contains(encoder, "x265")
}
