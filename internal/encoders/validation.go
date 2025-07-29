package encoders

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/pelletier/go-toml/v2"
)

// ValidationInfo contains metadata about the validation test
type ValidationInfo struct {
	Timestamp      string `toml:"timestamp"`
	FFmpegVersion  string `toml:"ffmpeg_version"`
	TestDuration   int    `toml:"test_duration"`
	TestResolution string `toml:"test_resolution"`
}

// ValidationResults represents the complete validation results
type ValidationResults struct {
	ValidationInfo ValidationInfo  `toml:"validation_info"`
	H264           CodecValidation `toml:"h264"`
	H265           CodecValidation `toml:"h265"`
}

// CodecValidation represents validation results for a specific codec type
type CodecValidation struct {
	Working []string `toml:"working"`
	Failed  []string `toml:"failed"`
}

// ValidateEncoder tests a single encoder using the appropriate validator
func ValidateEncoder(encoderName string) (bool, error) {
	registry := CreateValidatorRegistry()
	validator := registry.FindValidator(encoderName)

	if validator == nil {
		return false, fmt.Errorf("no validator found for encoder: %s", encoderName)
	}

	return validator.Validate(encoderName)
}

// ValidationLogger interface for conditional logging
type ValidationLogger interface {
	Printf(format string, v ...interface{})
}

// SilentLogger discards all log output
type SilentLogger struct{}

func (l SilentLogger) Printf(format string, v ...interface{}) {}

// VerboseLogger outputs to standard logger
type VerboseLogger struct {
	*log.Logger
}

func NewVerboseLogger() *VerboseLogger {
	return &VerboseLogger{log.Default()}
}

func (l *VerboseLogger) Printf(format string, v ...interface{}) {
	l.Logger.Printf(format, v...)
}

// ValidateEncoders validates all encoders and returns results (silent)
func ValidateEncoders() (*ValidationResults, error) {
	return ValidateEncodersWithLogger(SilentLogger{})
}

// ValidateEncodersWithLogger validates all encoders with custom logger
func ValidateEncodersWithLogger(logger ValidationLogger) (*ValidationResults, error) {
	results := &ValidationResults{
		ValidationInfo: ValidationInfo{
			Timestamp:      time.Now().Format(time.RFC3339),
			FFmpegVersion:  getFFmpegVersion(),
			TestDuration:   2,
			TestResolution: "640x480",
		},
		H264: CodecValidation{
			Working: []string{},
			Failed:  []string{},
		},
		H265: CodecValidation{
			Working: []string{},
			Failed:  []string{},
		},
	}

	registry := CreateValidatorRegistry()

	// Get all available validators (those with compiled encoders)
	availableValidators := registry.GetAvailableValidators()

	logger.Printf("Found %d validator(s) with compiled encoders", len(availableValidators))

	for _, validator := range availableValidators {
		logger.Printf("=== %s ===", validator.GetDescription())

		// Get only the compiled encoders for this validator
		compiledEncoders := registry.GetCompiledEncoders(validator)

		for _, encoderName := range compiledEncoders {
			logger.Printf("Testing %s...", encoderName)

			if valid, err := validator.Validate(encoderName); valid {
				logger.Printf("%s: ✓ WORKING", encoderName)

				// Categorize by codec type (including software encoders)
				if strings.Contains(encoderName, "h264") || strings.Contains(encoderName, "x264") {
					results.H264.Working = append(results.H264.Working, encoderName)
				} else if strings.Contains(encoderName, "hevc") || strings.Contains(encoderName, "h265") || strings.Contains(encoderName, "x265") {
					results.H265.Working = append(results.H265.Working, encoderName)
				}
			} else {
				logger.Printf("%s: ✗ FAILED (%v)", encoderName, err)

				// Categorize by codec type (including software encoders)
				if strings.Contains(encoderName, "h264") || strings.Contains(encoderName, "x264") {
					results.H264.Failed = append(results.H264.Failed, encoderName)
				} else if strings.Contains(encoderName, "hevc") || strings.Contains(encoderName, "h265") || strings.Contains(encoderName, "x265") {
					results.H265.Failed = append(results.H265.Failed, encoderName)
				}
			}
		}
	}

	return results, nil
}

// SaveValidationResults saves validation results to a TOML file
func SaveValidationResults(results *ValidationResults, filename string) error {
	data, err := toml.Marshal(results)
	if err != nil {
		return fmt.Errorf("failed to marshal results: %w", err)
	}

	err = os.WriteFile(filename, data, 0644)
	if err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	return nil
}

// LoadValidationResults loads validation results from a TOML file
func LoadValidationResults(filename string) (*ValidationResults, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	var results ValidationResults
	err = toml.Unmarshal(data, &results)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal results: %w", err)
	}

	return &results, nil
}

// PrintValidationSummary prints a summary of validation results
func PrintValidationSummary(results *ValidationResults) {
	fmt.Println("\n=== VALIDATION SUMMARY ===")

	fmt.Printf("H.264 encoders working: %d\n", len(results.H264.Working))
	if len(results.H264.Working) > 0 {
		fmt.Printf("  Working: %s\n", strings.Join(results.H264.Working, ", "))
	}

	fmt.Printf("H.265 encoders working: %d\n", len(results.H265.Working))
	if len(results.H265.Working) > 0 {
		fmt.Printf("  Working: %s\n", strings.Join(results.H265.Working, ", "))
	}

	if len(results.H264.Failed) > 0 || len(results.H265.Failed) > 0 {
		fmt.Println("\nFailed encoders:")
		if len(results.H264.Failed) > 0 {
			fmt.Printf("  H.264: %s\n", strings.Join(results.H264.Failed, ", "))
		}
		if len(results.H265.Failed) > 0 {
			fmt.Printf("  H.265: %s\n", strings.Join(results.H265.Failed, ", "))
		}
	}
}

// RunValidateCommand runs the validation command logic
func RunValidateCommand(outputFile string) {
	RunValidateCommandWithOptions(outputFile, false)
}

// RunValidateCommandWithOptions runs the validation command with options
func RunValidateCommandWithOptions(outputFile string, quiet bool) {
	var logger ValidationLogger
	if quiet {
		logger = SilentLogger{}
	} else {
		logger = NewVerboseLogger()
	}

	// Run validation with appropriate logger
	results, err := ValidateEncodersWithLogger(logger)
	if err != nil {
		log.Fatalf("Error validating encoders: %v", err)
	}

	// Save results to TOML file
	err = SaveValidationResults(results, outputFile)
	if err != nil {
		log.Fatalf("Error saving validation results: %v", err)
	}

	// Print summary
	PrintValidationSummary(results)

	fmt.Printf("\nResults saved to: %s\n", outputFile)
}

// getFFmpegVersion gets the FFmpeg version string
func getFFmpegVersion() string {
	cmd := exec.Command("ffmpeg", "-version")
	output, err := cmd.Output()
	if err != nil {
		return "unknown"
	}

	lines := strings.Split(string(output), "\n")
	if len(lines) > 0 {
		// Extract version from first line like "ffmpeg version 7.1.1 Copyright..."
		parts := strings.Fields(lines[0])
		if len(parts) >= 3 {
			return parts[2]
		}
	}

	return "unknown"
}
