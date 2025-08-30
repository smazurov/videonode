package encoders

import (
	"fmt"
	"log"
	"os/exec"
	"strings"
	"time"

	"github.com/smazurov/videonode/internal/config"
	"github.com/smazurov/videonode/internal/types"
)

// Validator manages encoder validation with access to StreamManager
type Validator struct {
	streamManager *config.StreamManager
	logger        ValidationLogger
}

// NewValidator creates a new Validator with the given StreamManager
func NewValidator(sm *config.StreamManager) *Validator {
	return &Validator{
		streamManager: sm,
		logger:        SilentLogger{},
	}
}

// SetLogger sets the logger for validation output
func (v *Validator) SetLogger(logger ValidationLogger) {
	v.logger = logger
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

// ValidateEncoders validates all encoders and returns results
func (v *Validator) ValidateEncoders() (*types.ValidationResults, error) {
	results := &types.ValidationResults{
		Timestamp:      time.Now().Format(time.RFC3339),
		FFmpegVersion:  getFFmpegVersion(),
		TestDuration:   2,
		TestResolution: "640x480",
		H264: types.CodecValidation{
			Working: []string{},
			Failed:  []string{},
		},
		H265: types.CodecValidation{
			Working: []string{},
			Failed:  []string{},
		},
	}

	registry := CreateValidatorRegistry()

	// Get all available validators (those with compiled encoders)
	availableValidators := registry.GetAvailableValidators()

	v.logger.Printf("Found %d validator(s) with compiled encoders", len(availableValidators))

	for _, validator := range availableValidators {
		v.logger.Printf("=== %s ===", validator.GetDescription())

		// Get only the compiled encoders for this validator
		compiledEncoders := registry.GetCompiledEncoders(validator)

		for _, encoderName := range compiledEncoders {
			v.logger.Printf("Testing %s...", encoderName)

			if valid, err := validator.Validate(encoderName); valid {
				v.logger.Printf("%s: ✓ WORKING", encoderName)

				// Categorize by codec type (including software encoders)
				if strings.Contains(encoderName, "h264") || strings.Contains(encoderName, "x264") {
					results.H264.Working = append(results.H264.Working, encoderName)
				} else if strings.Contains(encoderName, "hevc") || strings.Contains(encoderName, "h265") || strings.Contains(encoderName, "x265") {
					results.H265.Working = append(results.H265.Working, encoderName)
				}
			} else {
				v.logger.Printf("%s: ✗ FAILED (%v)", encoderName, err)

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

// SaveValidationResults saves validation results using StreamManager
func (v *Validator) SaveValidationResults(results *types.ValidationResults) error {
	// Update validation data directly through StreamManager
	return v.streamManager.UpdateValidation(results)
}

// LoadValidationResults loads validation results from StreamManager
func (v *Validator) LoadValidationResults() (*types.ValidationResults, error) {
	// Get validation data from StreamManager
	validation := v.streamManager.GetValidation()
	if validation == nil {
		return nil, fmt.Errorf("no validation data found")
	}

	return validation, nil
}

// Deprecated: Use Validator.LoadValidationResults() instead
func LoadValidationResults(sm *config.StreamManager) (*types.ValidationResults, error) {
	v := NewValidator(sm)
	return v.LoadValidationResults()
}

// PrintValidationSummary prints a summary of validation results
func PrintValidationSummary(results *types.ValidationResults) {
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
func (v *Validator) RunValidateCommand(quiet bool) error {
	if quiet {
		v.SetLogger(SilentLogger{})
	} else {
		v.SetLogger(NewVerboseLogger())
	}

	// Run validation
	results, err := v.ValidateEncoders()
	if err != nil {
		return fmt.Errorf("error validating encoders: %w", err)
	}

	// Save results
	err = v.SaveValidationResults(results)
	if err != nil {
		return fmt.Errorf("error saving validation results: %w", err)
	}

	// Print summary
	PrintValidationSummary(results)
	fmt.Println("\nResults saved")

	return nil
}

// RunValidateCommandWithOptions runs validation with StreamManager - for backward compatibility
func RunValidateCommandWithOptions(sm *config.StreamManager, quiet bool) {
	v := NewValidator(sm)
	if err := v.RunValidateCommand(quiet); err != nil {
		log.Fatal(err)
	}
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
