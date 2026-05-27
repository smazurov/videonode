package encoders

import (
	"fmt"
	"log/slog"
	"os/exec"
	"sort"
	"strings"
	"time"

	"github.com/smazurov/videonode/internal/logging"
	"github.com/smazurov/videonode/internal/types"
)

// Validator manages encoder validation with access to ValidationProvider.
type Validator struct {
	provider types.ValidationProvider
	logger   ValidationLogger
}

// NewValidator creates a new Validator with the given ValidationProvider.
func NewValidator(provider types.ValidationProvider) *Validator {
	return &Validator{
		provider: provider,
		logger:   SilentLogger{},
	}
}

// SetLogger sets the logger for validation output.
func (v *Validator) SetLogger(logger ValidationLogger) {
	v.logger = logger
}

// ValidateEncoder tests a single encoder using the appropriate validator.
func ValidateEncoder(encoderName string) (bool, error) {
	registry := CreateValidatorRegistry()
	validator := registry.FindValidator(encoderName)

	if validator == nil {
		return false, fmt.Errorf("no validator found for encoder: %s", encoderName)
	}

	return validator.Validate(encoderName)
}

// ValidationLogger interface for conditional logging.
type ValidationLogger interface {
	Printf(format string, v ...any)
}

// SilentLogger discards all log output.
type SilentLogger struct{}

// Printf implements ValidationLogger interface by discarding all output.
func (l SilentLogger) Printf(_ string, _ ...any) {}

// VerboseLogger outputs using slog.
type VerboseLogger struct {
	logger *slog.Logger
}

// NewVerboseLogger creates a verbose logger that outputs via slog.
func NewVerboseLogger() *VerboseLogger {
	return &VerboseLogger{
		logger: slog.With(logging.KeyComponent, "encoder_validation"),
	}
}

// Printf implements ValidationLogger interface.
func (l *VerboseLogger) Printf(format string, v ...any) {
	l.logger.Info(fmt.Sprintf(format, v...))
}

// ValidateEncoders validates all encoders and returns results.
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
		Backends: map[string]types.BackendValidation{},
	}

	registry := CreateValidatorRegistry()

	// Get all available validators (those with compiled encoders)
	availableValidators := registry.GetAvailableValidators()

	v.logger.Printf("Found %d validator(s) with compiled encoders", len(availableValidators))

	for _, validator := range availableValidators {
		v.logger.Printf("=== %s ===", validator.GetDescription())

		compiledEncoders := registry.GetCompiledEncoders(validator)

		for _, encoderName := range compiledEncoders {
			v.logger.Printf("Testing %s...", encoderName)

			if valid, err := validator.Validate(encoderName); valid {
				v.logger.Printf("%s: ✓ WORKING", encoderName)

				if strings.Contains(encoderName, "h264") || strings.Contains(encoderName, "x264") {
					results.H264.Working = append(results.H264.Working, encoderName)
				} else if strings.Contains(encoderName, "hevc") || strings.Contains(encoderName, "h265") || strings.Contains(encoderName, "x265") {
					results.H265.Working = append(results.H265.Working, encoderName)
				}
			} else {
				v.logger.Printf("%s: ✗ FAILED (%v)", encoderName, err)

				if strings.Contains(encoderName, "h264") || strings.Contains(encoderName, "x264") {
					results.H264.Failed = append(results.H264.Failed, encoderName)
				} else if strings.Contains(encoderName, "hevc") || strings.Contains(encoderName, "h265") || strings.Contains(encoderName, "x265") {
					results.H265.Failed = append(results.H265.Failed, encoderName)
				}
			}
		}

		backendName := validator.GetBackendName()
		if backendName == "" || backendName == "sw" {
			continue
		}
		v.logger.Printf("--- %s backend probes ---", backendName)
		decW, decF := validator.ValidateDecoders(v.logger)
		fltW, fltF := validator.ValidateFilters(v.logger)
		if len(decW)+len(decF)+len(fltW)+len(fltF) == 0 {
			continue
		}
		results.Backends[backendName] = types.BackendValidation{
			Decoders: types.CodecValidation{Working: decW, Failed: decF},
			Filters:  types.CodecValidation{Working: fltW, Failed: fltF},
		}
	}

	return results, nil
}

// SaveValidationResults saves validation results using ValidationProvider.
func (v *Validator) SaveValidationResults(results *types.ValidationResults) error {
	return v.provider.UpdateValidation(results)
}

// LoadValidationResults loads validation results from ValidationProvider.
func (v *Validator) LoadValidationResults() (*types.ValidationResults, error) {
	validation := v.provider.GetValidation()
	if validation == nil {
		return nil, fmt.Errorf("no validation data found")
	}

	return validation, nil
}

// LoadValidationResults loads validation results via a ValidationProvider. Deprecated: use Validator.LoadValidationResults.
func LoadValidationResults(provider types.ValidationProvider) (*types.ValidationResults, error) {
	v := NewValidator(provider)
	return v.LoadValidationResults()
}

// PrintValidationSummary prints a summary of validation results.
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

	if len(results.Backends) > 0 {
		fmt.Println("\nHardware backends:")
		names := make([]string, 0, len(results.Backends))
		for k := range results.Backends {
			names = append(names, k)
		}
		sort.Strings(names)
		for _, name := range names {
			b := results.Backends[name]
			fmt.Printf("  %s:\n", name)
			fmt.Printf("    decoders working: %s\n", joinOrEmpty(b.Decoders.Working))
			if len(b.Decoders.Failed) > 0 {
				fmt.Printf("    decoders failed:  %s\n", strings.Join(b.Decoders.Failed, ", "))
			}
			fmt.Printf("    filters working:  %s\n", joinOrEmpty(b.Filters.Working))
			if len(b.Filters.Failed) > 0 {
				fmt.Printf("    filters failed:   %s\n", strings.Join(b.Filters.Failed, ", "))
			}
		}
	}
}

func joinOrEmpty(s []string) string {
	if len(s) == 0 {
		return "(none)"
	}
	return strings.Join(s, ", ")
}

// RunValidateCommand runs the validation command logic.
func (v *Validator) RunValidateCommand(quiet bool) error {
	if quiet {
		v.SetLogger(SilentLogger{})
	} else {
		v.SetLogger(NewVerboseLogger())
	}

	results, err := v.ValidateEncoders()
	if err != nil {
		return fmt.Errorf("error validating encoders: %w", err)
	}

	saveErr := v.SaveValidationResults(results)
	if saveErr != nil {
		return fmt.Errorf("error saving validation results: %w", saveErr)
	}

	PrintValidationSummary(results)
	fmt.Println("\nResults saved")

	return nil
}

// RunValidateCommandWithOptions runs validation with a ValidationProvider.
func RunValidateCommandWithOptions(provider types.ValidationProvider, quiet bool) {
	v := NewValidator(provider)
	if err := v.RunValidateCommand(quiet); err != nil {
		logger := slog.With(logging.KeyComponent, "encoder_validation")
		logger.Error("Validation command failed", logging.KeyError, err)
		panic(err)
	}
}

// getFFmpegVersion gets the FFmpeg version string.
func getFFmpegVersion() string {
	cmd := exec.Command("ffmpeg", "-version")
	output, err := cmd.Output()
	if err != nil {
		return "unknown"
	}

	lines := strings.Split(string(output), "\n")
	if len(lines) > 0 {
		parts := strings.Fields(lines[0])
		if len(parts) >= 3 {
			return parts[2]
		}
	}

	return "unknown"
}
