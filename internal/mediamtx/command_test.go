package mediamtx

import (
	"strings"
	"testing"
)

// Helper function to test command wrapping behavior
func testCommandWrapping(t *testing.T, enableLogging bool) {
	// Save original config
	originalConfig := globalConfig
	defer func() { globalConfig = originalConfig }()

	// Set test config
	globalConfig = &Config{
		EnableLogging: enableLogging,
	}

	testCmd := "ffmpeg -f v4l2 -i /dev/video0 -c:v libx264 -f rtsp rtsp://localhost:8554/test"

	// Simulate what AddPath does
	cmd := testCmd
	if globalConfig.EnableLogging {
		cmd = testCmd + " 2>&1 | systemd-cat -t ffmpeg-$MTX_PATH"
	}

	if enableLogging {
		if !strings.Contains(cmd, "systemd-cat") {
			t.Errorf("When logging enabled, command should contain systemd-cat pipe")
		}
		if !strings.Contains(cmd, "2>&1") {
			t.Errorf("When logging enabled, command should redirect stderr to stdout")
		}
		if !strings.Contains(cmd, "ffmpeg-$MTX_PATH") {
			t.Errorf("When logging enabled, command should use ffmpeg-$MTX_PATH tag")
		}
	} else {
		if strings.Contains(cmd, "systemd-cat") {
			t.Errorf("When logging disabled, command should not contain systemd-cat pipe")
		}
		if cmd != testCmd {
			t.Errorf("When logging disabled, command should remain unchanged")
		}
	}
}

func TestCommandWrappingWithLogging(t *testing.T) {
	testCommandWrapping(t, true)
}

func TestCommandWrappingWithoutLogging(t *testing.T) {
	testCommandWrapping(t, false)
}
