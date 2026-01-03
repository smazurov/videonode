package validation

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/smazurov/videonode/internal/types"
)

// EncoderParams is a map of encoder-specific parameters.
type EncoderParams map[string]string

// EncoderSettings contains the specific FFmpeg settings needed for an encoder.
type EncoderSettings struct {
	GlobalArgs   []string          `json:"global_args"`   // Global FFmpeg arguments (e.g., -vaapi_device)
	OutputParams map[string]string `json:"output_params"` // Output parameters (e.g., qp, preset, cq)
	VideoFilters string            `json:"video_filters"` // Video filter chain (e.g., format=nv12,hwupload)
}

// EncoderValidator defines the interface for validating specific encoder types.
type EncoderValidator interface {
	// CanValidate returns true if this validator can handle the given encoder name
	CanValidate(encoderName string) bool

	// Validate tests the encoder and returns true if it works, along with any error
	Validate(encoderName string) (bool, error)

	// GetEncoderNames returns a list of encoder names this validator handles
	GetEncoderNames() []string

	// GetDescription returns a human-readable description of this validator
	GetDescription() string

	// GetProductionSettings returns the production FFmpeg settings for the encoder
	// These are the same settings used in validation tests
	// The inputFormat parameter specifies the input format (e.g., "mjpeg", "h264", "yuyv422")
	// to allow the validator to return appropriate video filters for format conversion
	GetProductionSettings(encoderName string, inputFormat string) (*EncoderSettings, error)

	// GetQualityParams translates quality settings to encoder-specific parameters
	GetQualityParams(encoderName string, params *types.QualityParams) (EncoderParams, error)
}

// ValidatorRegistry holds all registered validators.
type ValidatorRegistry struct {
	validators []EncoderValidator
}

// NewValidatorRegistry creates a new validator registry.
func NewValidatorRegistry() *ValidatorRegistry {
	return &ValidatorRegistry{
		validators: make([]EncoderValidator, 0),
	}
}

// Register adds a validator to the registry.
func (r *ValidatorRegistry) Register(validator EncoderValidator) {
	r.validators = append(r.validators, validator)
}

// FindValidator finds the appropriate validator for the given encoder name.
func (r *ValidatorRegistry) FindValidator(encoderName string) EncoderValidator {
	for _, validator := range r.validators {
		if validator.CanValidate(encoderName) {
			return validator
		}
	}
	return nil
}

// GetAvailableValidators returns validators for encoders that are compiled into ffmpeg.
func (r *ValidatorRegistry) GetAvailableValidators() []EncoderValidator {
	available := make([]EncoderValidator, 0)

	for _, validator := range r.validators {
		hasCompiledEncoder := slices.ContainsFunc(validator.GetEncoderNames(), isEncoderCompiled)
		if hasCompiledEncoder {
			available = append(available, validator)
		}
	}

	return available
}

// GetAllValidators returns all registered validators without checking if encoders are compiled
// This is used for encoder overrides where we want to force a specific encoder.
func (r *ValidatorRegistry) GetAllValidators() []EncoderValidator {
	return r.validators
}

// GetCompiledEncoders returns only the encoder names that are compiled into ffmpeg.
func (r *ValidatorRegistry) GetCompiledEncoders(validator EncoderValidator) []string {
	compiled := make([]string, 0)

	for _, encoderName := range validator.GetEncoderNames() {
		if isEncoderCompiled(encoderName) {
			compiled = append(compiled, encoderName)
		}
	}

	return compiled
}

// isEncoderCompiled checks if an encoder is compiled into ffmpeg.
func isEncoderCompiled(encoderName string) bool {
	cmd := exec.Command("ffmpeg", "-hide_banner", "-nostats", "-encoders")
	output, err := cmd.Output()
	if err != nil {
		return false
	}

	return strings.Contains(string(output), encoderName)
}

// createTempDir creates a temporary directory for validation tests.
func createTempDir() (string, func(), error) {
	tempDir, err := os.MkdirTemp("", "encoder_validate")
	if err != nil {
		return "", nil, err
	}

	cleanup := func() {
		os.RemoveAll(tempDir)
	}

	return tempDir, cleanup, nil
}

// ValidateEncoderWithSettings provides a common validation implementation for all validators.
func ValidateEncoderWithSettings(validator EncoderValidator, encoderName string) (bool, error) {
	tempDir, cleanup, err := createTempDir()
	if err != nil {
		return false, fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer cleanup()

	testFile := filepath.Join(tempDir, fmt.Sprintf("test_%s.mp4", encoderName))

	// Get production settings for this encoder
	// Use empty input format for validation tests (they use synthetic test data)
	settings, err := validator.GetProductionSettings(encoderName, "")
	if err != nil {
		return false, fmt.Errorf("failed to get production settings: %w", err)
	}

	// Build FFmpeg command manually
	cmdParts := []string{"ffmpeg"}

	// Add global arguments first
	if len(settings.GlobalArgs) > 0 {
		cmdParts = append(cmdParts, settings.GlobalArgs...)
	}

	// Add input parameters
	cmdParts = append(cmdParts,
		"-f", "lavfi",
		"-i", "testsrc2=duration=2:size=640x480:rate=30",
		"-t", "2",
		"-c:v", encoderName,
	)

	// Add video filters if specified
	if settings.VideoFilters != "" {
		cmdParts = append(cmdParts, "-vf", settings.VideoFilters)
	}

	// Add all output parameters from settings
	for key, value := range settings.OutputParams {
		cmdParts = append(cmdParts, fmt.Sprintf("-%s", key), value)
	}

	// Add output file and overwrite flag
	cmdParts = append(cmdParts, "-y", testFile)

	// Log the command for debugging
	fmt.Printf("Executing FFmpeg command: %s\n", strings.Join(cmdParts, " "))

	// Execute command with timeout
	cmd := exec.Command(cmdParts[0], cmdParts[1:]...)

	// Capture stderr for debugging
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	done := make(chan error, 1)
	go func() {
		done <- cmd.Run()
	}()

	select {
	case runErr := <-done:
		if runErr != nil {
			if stderr.Len() > 0 {
				fmt.Printf("FFmpeg stderr: %s\n", stderr.String())
			}
			return false, runErr
		}
	case <-time.After(10 * time.Second):
		if cmd.Process != nil {
			cmd.Process.Kill()
		}
		return false, fmt.Errorf("validation command timed out")
	}

	if fileInfo, statErr := os.Stat(testFile); statErr == nil && fileInfo.Size() > 1000 {
		return true, nil
	}
	return false, fmt.Errorf("output file missing or too small")
}
