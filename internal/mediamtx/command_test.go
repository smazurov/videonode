package mediamtx

import "testing"

func TestWrapCommand(t *testing.T) {
	originalConfig := globalConfig
	defer func() { globalConfig = originalConfig }()

	tests := []struct {
		name       string
		useSystemd bool
		command    string
		expected   string
	}{
		{
			name:       "systemd enabled wraps ffmpeg command",
			useSystemd: true,
			command:    "ffmpeg -i input.mp4 output.mp4",
			expected:   "systemd-run --user --quiet --collect --wait --pty --unit=ffmpeg_test-stream -p KillMode=control-group -p KillSignal=SIGINT -p TimeoutStopSec=5 -p SyslogIdentifier=ffmpeg-test-stream ffmpeg -i input.mp4 output.mp4",
		},
		{
			name:       "systemd enabled wraps complex args",
			useSystemd: true,
			command:    "ffmpeg -vf 'scale=1280:720' input.mp4",
			expected:   "systemd-run --user --quiet --collect --wait --pty --unit=ffmpeg_test-stream -p KillMode=control-group -p KillSignal=SIGINT -p TimeoutStopSec=5 -p SyslogIdentifier=ffmpeg-test-stream ffmpeg -vf 'scale=1280:720' input.mp4",
		},
		{
			name:       "systemd disabled returns original",
			useSystemd: false,
			command:    "ffmpeg -i input.mp4 output.mp4",
			expected:   "ffmpeg -i input.mp4 output.mp4",
		},
		{
			name:       "systemd disabled preserves complex command",
			useSystemd: false,
			command:    "ffmpeg -vf 'scale=1280:720' input.mp4",
			expected:   "ffmpeg -vf 'scale=1280:720' input.mp4",
		},
		{
			name:       "empty command returns empty",
			useSystemd: true,
			command:    "",
			expected:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			globalConfig = &Config{UseSystemd: tt.useSystemd}
			result := WrapCommand(tt.command, "test-stream")
			if result != tt.expected {
				t.Fatalf("WrapCommand() = %q, expected %q", result, tt.expected)
			}
		})
	}
}
