package encoders

import (
	"bufio"
	"fmt"
	"os/exec"
	"regexp"
	"strings"

	"github.com/smazurov/videonode/internal/ffmpeg"
)

// EncoderType represents the type of encoder (video, audio, subtitle)
type EncoderType string

const (
	VideoEncoder    EncoderType = "V"
	AudioEncoder    EncoderType = "A"
	SubtitleEncoder EncoderType = "S"
	Unknown         EncoderType = "?"
)

// Encoder represents an FFmpeg encoder
type Encoder struct {
	Type        EncoderType `json:"type"`
	Name        string      `json:"name"`
	Description string      `json:"description"`
	HWAccel     bool        `json:"hwaccel"`
}

// EncoderList holds a categorized list of encoders
type EncoderList struct {
	VideoEncoders    []Encoder `json:"video_encoders"`
	AudioEncoders    []Encoder `json:"audio_encoders"`
	SubtitleEncoders []Encoder `json:"subtitle_encoders"`
	OtherEncoders    []Encoder `json:"other_encoders"`
}

// EncoderFilter represents filter options for encoders
type EncoderFilter struct {
	Type    string `json:"type"`    // Filter by encoder type (V, A, S)
	Search  string `json:"search"`  // Search term for name or description
	Hwaccel bool   `json:"hwaccel"` // Filter for hardware accelerated encoders
}

// GetFFmpegEncoders retrieves all available encoders from ffmpeg
func GetFFmpegEncoders() (*EncoderList, error) {
	// Check if ffmpeg is installed
	if !IsFFmpegInstalled() {
		return nil, fmt.Errorf("ffmpeg is not installed or not in PATH")
	}

	// Use our command builder to get the encoders list command
	builder := ffmpeg.NewCommandBuilder()
	cmdStr, err := builder.BuildEncodersListCommand()
	if err != nil {
		return nil, fmt.Errorf("failed to build encoders command: %w", err)
	}

	// Execute the command
	cmd := exec.Command("sh", "-c", cmdStr)
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to execute encoders command: %w", err)
	}

	// Parse the output
	return parseEncoderOutput(string(output))
}

// fallbackGetEncoders uses a direct ffmpeg command call as a fallback method
func fallbackGetEncoders() (*EncoderList, error) {
	cmd := exec.Command("ffmpeg", "-encoders")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to execute ffmpeg command: %v", err)
	}
	return parseEncoderOutput(string(output))
}

// IsFFmpegInstalled checks if ffmpeg is installed and available
func IsFFmpegInstalled() bool {
	_, err := exec.LookPath("ffmpeg")
	return err == nil
}

// parseEncoderOutput processes the output of ffmpeg -encoders command
func parseEncoderOutput(output string) (*EncoderList, error) {
	result := &EncoderList{
		VideoEncoders:    []Encoder{},
		AudioEncoders:    []Encoder{},
		SubtitleEncoders: []Encoder{},
		OtherEncoders:    []Encoder{},
	}

	scanner := bufio.NewScanner(strings.NewReader(output))

	// Skip header lines until we reach the encoders section
	encodersStarted := false
	encoderRegex := regexp.MustCompile(`^\s*([VASF\.]{6})\s+(\w+)\s+(.+)$`)
	hwaccelRegex := regexp.MustCompile(`(?i)(nvenc|qsv|amf|vaapi|videotoolbox|vdpau|cuda|dxva2|d3d11va|opencl|vulkan)`)

	for scanner.Scan() {
		line := scanner.Text()

		// Skip until we find the line that contains "Encoders:"
		if !encodersStarted {
			if strings.Contains(line, "Encoders:") {
				encodersStarted = true
			}
			continue
		}

		// Skip empty lines
		if len(strings.TrimSpace(line)) == 0 {
			continue
		}

		// Parse each encoder line
		matches := encoderRegex.FindStringSubmatch(line)
		if len(matches) == 4 {
			typeFlags := matches[1]
			name := matches[2]
			description := matches[3]

			var encoderType EncoderType
			if strings.Contains(typeFlags, "V") {
				encoderType = VideoEncoder
			} else if strings.Contains(typeFlags, "A") {
				encoderType = AudioEncoder
			} else if strings.Contains(typeFlags, "S") {
				encoderType = SubtitleEncoder
			} else {
				encoderType = Unknown
			}

			// Check if this is a hardware accelerated encoder
			isHWAccel := hwaccelRegex.MatchString(name) || hwaccelRegex.MatchString(description)

			encoder := Encoder{
				Type:        encoderType,
				Name:        name,
				Description: description,
				HWAccel:     isHWAccel,
			}

			// Add to the appropriate list
			switch encoderType {
			case VideoEncoder:
				result.VideoEncoders = append(result.VideoEncoders, encoder)
			case AudioEncoder:
				result.AudioEncoders = append(result.AudioEncoders, encoder)
			case SubtitleEncoder:
				result.SubtitleEncoders = append(result.SubtitleEncoders, encoder)
			default:
				result.OtherEncoders = append(result.OtherEncoders, encoder)
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading output: %v", err)
	}

	return result, nil
}

// FilterEncoders applies filters to a list of encoders
func FilterEncoders(encoders *EncoderList, filter EncoderFilter) *EncoderList {
	result := &EncoderList{
		VideoEncoders:    []Encoder{},
		AudioEncoders:    []Encoder{},
		SubtitleEncoders: []Encoder{},
		OtherEncoders:    []Encoder{},
	}

	// Define a function to check if an encoder matches the filter
	matchesFilter := func(encoder Encoder) bool {
		// Filter by encoder type
		if filter.Type != "" && string(encoder.Type) != filter.Type {
			return false
		}

		// Filter by hardware acceleration
		if filter.Hwaccel && !encoder.HWAccel {
			return false
		}

		// Filter by search term
		if filter.Search != "" {
			searchTerm := strings.ToLower(filter.Search)
			encoderName := strings.ToLower(encoder.Name)
			encoderDesc := strings.ToLower(encoder.Description)

			if !strings.Contains(encoderName, searchTerm) && !strings.Contains(encoderDesc, searchTerm) {
				return false
			}
		}

		// All filters passed
		return true
	}

	// Apply filter to each encoder list
	for _, encoder := range encoders.VideoEncoders {
		if matchesFilter(encoder) {
			result.VideoEncoders = append(result.VideoEncoders, encoder)
		}
	}

	for _, encoder := range encoders.AudioEncoders {
		if matchesFilter(encoder) {
			result.AudioEncoders = append(result.AudioEncoders, encoder)
		}
	}

	for _, encoder := range encoders.SubtitleEncoders {
		if matchesFilter(encoder) {
			result.SubtitleEncoders = append(result.SubtitleEncoders, encoder)
		}
	}

	for _, encoder := range encoders.OtherEncoders {
		if matchesFilter(encoder) {
			result.OtherEncoders = append(result.OtherEncoders, encoder)
		}
	}

	return result
}
