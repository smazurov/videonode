package validation

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/smazurov/videonode/internal/types"
)

var (
	compiledEncodersOnce sync.Once
	compiledEncoders     []string
)

// compiledEncoderList returns the names of encoders compiled into the
// host's ffmpeg. Runs `ffmpeg -encoders` exactly once per process; the
// list doesn't change at runtime.
func compiledEncoderList() []string {
	compiledEncodersOnce.Do(func() {
		cmd := exec.Command("ffmpeg", "-hide_banner", "-nostats", "-encoders")
		out, err := cmd.Output()
		if err != nil {
			return
		}
		// `ffmpeg -encoders` prints "V..... h264_rkmpp Rockchip H.264 encoder ..."
		// per line after a banner. Extract the second column.
		for line := range strings.SplitSeq(string(out), "\n") {
			fields := strings.Fields(line)
			if len(fields) < 2 {
				continue
			}
			// Only lines whose first field looks like the V/A/S flag block
			// (six chars of letters or dots) are encoder rows.
			if len(fields[0]) != 6 {
				continue
			}
			compiledEncoders = append(compiledEncoders, fields[1])
		}
	})
	return compiledEncoders
}

// EncoderParams is a map of encoder-specific parameters.
type EncoderParams map[string]string

// EncoderSettings contains the specific FFmpeg settings needed for an encoder.
type EncoderSettings struct {
	GlobalArgs   []string          `json:"global_args"`
	OutputParams map[string]string `json:"output_params"`
	VideoFilters string            `json:"video_filters"`
}

// EncoderValidator defines the interface for validating specific encoder types.
type EncoderValidator interface {
	CanValidate(encoderName string) bool
	Validate(encoderName string) (bool, error)
	GetEncoderNames() []string
	GetDescription() string
	GetProductionSettings(encoderName string, inputFormat string) (*EncoderSettings, error)
	GetQualityParams(encoderName string, params *types.QualityParams) (EncoderParams, error)
	GetBackendName() string
	ValidateDecoders(logger Logger) (working, failed []string)
	ValidateFilters(logger Logger) (working, failed []string)
}

// Logger receives per-probe stderr output during validation runs.
type Logger interface {
	Printf(format string, v ...any)
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
	return IsEncoderCompiled(encoderName)
}

// IsEncoderCompiled reports whether `encoderName` appears in
// `ffmpeg -encoders`. Result is cached process-wide (first call runs
// ffmpeg; subsequent calls hit the cache) since the answer is fixed for
// the lifetime of the daemon.
func IsEncoderCompiled(encoderName string) bool {
	return slices.Contains(compiledEncoderList(), encoderName)
}

// AutodetectEncoder picks the best ffmpeg encoder for a logical codec
// ("h264"/"h265") by inspecting `ffmpeg -encoders`. Use this when no
// preloaded validation data is available — the result reflects only
// what's compiled in, not whether the encoder actually accepts a given
// input format.
//
// Order:
//  1. rkmpp — Rockchip ffmpeg builds typically exclude libx264, so on
//     an rkmpp box this is the only thing that works.
//  2. libx264/libx265 — safe everywhere it's compiled; no device
//     prerequisites.
//  3. vaapi — last resort. Requires `-vaapi_device` setup the daemon
//     doesn't emit today; only useful when the user explicitly wires
//     it. Better than nothing on a rkmpp-stripped host that also lacks
//     libx264.
func AutodetectEncoder(codec string) string {
	switch codec {
	case "h265", "hevc":
		for _, c := range []string{"hevc_rkmpp", "libx265", "hevc_vaapi"} {
			if IsEncoderCompiled(c) {
				return c
			}
		}
		return "libx265"
	default:
		for _, c := range []string{"h264_rkmpp", "libx264", "h264_vaapi"} {
			if IsEncoderCompiled(c) {
				return c
			}
		}
		return "libx264"
	}
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

	settings, err := validator.GetProductionSettings(encoderName, "")
	if err != nil {
		return false, fmt.Errorf("failed to get production settings: %w", err)
	}

	cmdParts := []string{"ffmpeg"}

	if len(settings.GlobalArgs) > 0 {
		cmdParts = append(cmdParts, settings.GlobalArgs...)
	}

	cmdParts = append(cmdParts,
		"-f", "lavfi",
		"-i", "testsrc2=duration=2:size=640x480:rate=30",
		"-t", "2",
		"-c:v", encoderName,
	)

	if settings.VideoFilters != "" {
		cmdParts = append(cmdParts, "-vf", settings.VideoFilters)
	}

	for key, value := range settings.OutputParams {
		cmdParts = append(cmdParts, fmt.Sprintf("-%s", key), value)
	}

	cmdParts = append(cmdParts, "-y", testFile)

	fmt.Printf("Executing FFmpeg command: %s\n", strings.Join(cmdParts, " "))

	cmd := exec.Command(cmdParts[0], cmdParts[1:]...)

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
