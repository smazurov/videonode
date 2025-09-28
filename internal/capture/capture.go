package capture

import (
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/smazurov/videonode/internal/devices"
	"github.com/smazurov/videonode/internal/encoders/validation"
	"github.com/smazurov/videonode/internal/ffmpeg"
)

// listDevices lists all available video devices
func ListDevices() {
	detector := devices.NewDetector()
	deviceList, err := detector.FindDevices()
	if err != nil {
		logger := slog.With("component", "capture")
		logger.Error("Error finding devices", "error", err)
		panic(err) // Maintain the same behavior as log.Fatalf
	}

	if len(deviceList) == 0 {
		fmt.Println("No V4L2 devices found.")
		return
	}

	fmt.Printf("Found %d V4L2 devices:\n", len(deviceList))
	for i, dev := range deviceList {
		fmt.Printf("%d. Device Path: %s\n", i+1, dev.DevicePath)
		fmt.Printf("   Device Name: %s\n", dev.DeviceName)
		fmt.Printf("   Device ID: %s\n", dev.DeviceId)
		fmt.Println()
	}
}

// getOptimalEncoder returns the best available encoder with its settings
func getOptimalEncoder() (string, *validation.EncoderSettings, error) {
	// For capture operations, we can use software encoder since it's just a single frame
	// This avoids needing StreamManager dependency in the capture package
	return "libx264", nil, nil
}

// CaptureToBytes captures a screenshot from the specified video device
// and returns the image data as bytes.
//
// If delaySeconds > 0, it will record video for that duration and extract
// the last frame, which allows devices like Elgato to naturally show
// their "no signal" message after a few seconds of video capture.
func CaptureToBytes(devicePath string, delaySeconds float64) ([]byte, error) {
	// Check if the device exists
	if _, err := os.Stat(devicePath); os.IsNotExist(err) {
		return nil, fmt.Errorf("device %s does not exist", devicePath)
	}

	// If no delay specified or too small, just capture a single frame
	if delaySeconds <= 0.1 {
		return captureDirectFrameToBytes(devicePath)
	}

	// Otherwise capture video for the specified duration and extract last frame
	return captureDelayedFrameToBytes(devicePath, delaySeconds)
}

// CaptureScreenshot captures a screenshot from the specified video device
// and saves it to the specified output path.
//
// If delaySeconds > 0, it will record video for that duration and extract
// the last frame, which allows devices like Elgato to naturally show
// their "no signal" message after a few seconds of video capture.
func CaptureScreenshot(devicePath, outputPath string, delaySeconds float64) error {
	// Ensure the output directory exists
	outputDir := filepath.Dir(outputPath)
	if outputDir != "." {
		err := os.MkdirAll(outputDir, 0755)
		if err != nil {
			return fmt.Errorf("failed to create output directory %s: %w", outputDir, err)
		}
	}

	// Check if the device exists
	if _, err := os.Stat(devicePath); os.IsNotExist(err) {
		return fmt.Errorf("device %s does not exist", devicePath)
	}

	// If no delay specified or too small, just capture a single frame
	if delaySeconds <= 0.1 {
		return captureDirectFrame(devicePath, outputPath)
	}

	// Otherwise capture video for the specified duration and extract last frame
	return captureDelayedFrame(devicePath, outputPath, delaySeconds)
}

// captureDirectFrame captures a single frame immediately
func captureDirectFrame(devicePath, outputPath string) error {
	fmt.Printf("Capturing immediate screenshot from %s...\n", devicePath)

	// Use our command builder to create the FFmpeg command
	builder := ffmpeg.NewCommandBuilder()
	config := ffmpeg.CaptureConfig{
		DevicePath:    devicePath,
		OutputPath:    outputPath,
		InputFormat:   "yuyv422",
		Resolution:    "1280x720",
		FPS:           "30",
		DelayMs:       0,
		FFmpegOptions: ffmpeg.GetDefaultOptions(),
	}

	cmdStr, err := builder.BuildCaptureCommand(config)
	if err != nil {
		return fmt.Errorf("error building capture command: %w", err)
	}

	// Execute the command with timeout
	cmd := exec.Command("sh", "-c", cmdStr)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// Create a channel to receive the result
	done := make(chan error, 1)

	go func() {
		done <- cmd.Run()
	}()

	// Wait for completion or timeout
	select {
	case err := <-done:
		if err != nil {
			return fmt.Errorf("error capturing screenshot: %w", err)
		}
	case <-time.After(10 * time.Second):
		if cmd.Process != nil {
			cmd.Process.Kill()
		}
		return fmt.Errorf("capture command timed out after 10 seconds")
	}

	fmt.Printf("Screenshot saved to %s\n", outputPath)
	return nil
}

// captureDelayedFrame records video for the specified duration and extracts the last frame
func captureDelayedFrame(devicePath, outputPath string, delaySeconds float64) error {
	fmt.Printf("Capturing frames for %.1f seconds from %s to allow 'no signal' message...\n",
		delaySeconds, devicePath)

	// Generate a temp dir for the capture process
	tempDir, err := os.MkdirTemp("", "capture")
	if err != nil {
		return fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer os.RemoveAll(tempDir)

	// Create a temporary video file
	tempVideo := filepath.Join(tempDir, "capture.mp4")

	// Convert delaySeconds to time.Duration for the timeout
	captureTimeout := time.Duration(delaySeconds*float64(time.Second)) + (5 * time.Second)

	// Get the optimal encoder with its settings
	encoderName, settings, err := getOptimalEncoder()
	if err != nil {
		return fmt.Errorf("failed to get optimal encoder: %w", err)
	}

	// Step 1: Capture video for the specified duration
	// Build the command manually since we need more complex parameters
	videoCmd := fmt.Sprintf("ffmpeg -f v4l2 -framerate 30 -video_size 1280x720 -i %s -t %.1f -c:v %s",
		devicePath, delaySeconds, encoderName)

	// Add encoder-specific settings
	if settings != nil {
		// Add global arguments
		for _, globalArg := range settings.GlobalArgs {
			videoCmd = fmt.Sprintf("%s %s", globalArg, videoCmd)
		}

		// Add video filters if specified
		if settings.VideoFilters != "" {
			videoCmd = fmt.Sprintf("%s -vf %s", videoCmd, settings.VideoFilters)
		}

		// Add output parameters
		for key, value := range settings.OutputParams {
			videoCmd = fmt.Sprintf("%s -%s %s", videoCmd, key, value)
		}
	}

	videoCmd = fmt.Sprintf("%s -y %s", videoCmd, tempVideo)

	// Execute video capture command
	cmd := exec.Command("sh", "-c", videoCmd)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	done := make(chan error, 1)
	go func() {
		done <- cmd.Run()
	}()

	select {
	case err := <-done:
		if err != nil {
			return fmt.Errorf("error capturing video: %w", err)
		}
	case <-time.After(captureTimeout):
		if cmd.Process != nil {
			cmd.Process.Kill()
		}
		return fmt.Errorf("video capture timed out")
	}

	// Step 2: Extract a frame near the end of the video
	seekTime := fmt.Sprintf("%.1f", delaySeconds*0.95) // Seek to 95% of the duration
	extractCmd := fmt.Sprintf("ffmpeg -ss %s -i %s -frames:v 1 -q:v 1 -y %s", seekTime, tempVideo, outputPath)

	cmd = exec.Command("sh", "-c", extractCmd)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	err = cmd.Run()
	if err != nil {
		return fmt.Errorf("error extracting final frame: %w", err)
	}

	fmt.Printf("Screenshot with 'no signal' message saved to %s\n", outputPath)
	return nil
}

// captureDirectFrameToBytes captures a single frame immediately and returns as bytes
func captureDirectFrameToBytes(devicePath string) ([]byte, error) {
	fmt.Printf("Capturing immediate screenshot from %s...\n", devicePath)

	// Create a temporary file for the capture
	tempFile, err := os.CreateTemp("", "capture_*.jpg")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp file: %w", err)
	}
	defer os.Remove(tempFile.Name())
	tempFile.Close()

	// Use our capture function to save to the temp file
	err = captureDirectFrame(devicePath, tempFile.Name())
	if err != nil {
		return nil, err
	}

	// Read the file back as bytes
	data, err := os.ReadFile(tempFile.Name())
	if err != nil {
		return nil, fmt.Errorf("failed to read captured image: %w", err)
	}

	fmt.Printf("Screenshot captured (%d bytes)\n", len(data))
	return data, nil
}

// captureDelayedFrameToBytes records video for the specified duration and extracts the last frame as bytes
func captureDelayedFrameToBytes(devicePath string, delaySeconds float64) ([]byte, error) {
	fmt.Printf("Capturing frames for %.1f seconds from %s to allow 'no signal' message...\n",
		delaySeconds, devicePath)

	// Create a temporary file for the capture
	tempFile, err := os.CreateTemp("", "capture_delayed_*.jpg")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp file: %w", err)
	}
	defer os.Remove(tempFile.Name())
	tempFile.Close()

	// Use our delayed capture function to save to the temp file
	err = captureDelayedFrame(devicePath, tempFile.Name(), delaySeconds)
	if err != nil {
		return nil, err
	}

	// Read the file back as bytes
	data, err := os.ReadFile(tempFile.Name())
	if err != nil {
		return nil, fmt.Errorf("failed to read captured image: %w", err)
	}

	fmt.Printf("Screenshot with 'no signal' message captured (%d bytes)\n", len(data))
	return data, nil
}
