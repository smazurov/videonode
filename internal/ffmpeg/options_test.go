package ffmpeg

import (
	"strings"
	"testing"
)

func TestNewCommandBuilder(t *testing.T) {
	builder := NewCommandBuilder()
	if builder == nil {
		t.Fatal("NewCommandBuilder() returned nil")
	}

	// Test that it implements the CommandBuilder interface
	var _ CommandBuilder = builder
}

func TestBuildEncodersListCommand(t *testing.T) {
	builder := NewCommandBuilder()
	cmd, err := builder.BuildEncodersListCommand()

	if err != nil {
		t.Fatalf("BuildEncodersListCommand() failed: %v", err)
	}

	expected := "ffmpeg -hide_banner -encoders"
	if cmd != expected {
		t.Errorf("BuildEncodersListCommand() = %q, want %q", cmd, expected)
	}
}

func TestBuildProbeCommand(t *testing.T) {
	builder := NewCommandBuilder()

	tests := []struct {
		name       string
		devicePath string
		want       string
		wantErr    bool
	}{
		{
			name:       "valid device path",
			devicePath: "/dev/video0",
			want:       "ffprobe -hide_banner -f v4l2 -list_formats all -i /dev/video0",
			wantErr:    false,
		},
		{
			name:       "empty device path",
			devicePath: "",
			want:       "",
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd, err := builder.BuildProbeCommand(tt.devicePath)

			if tt.wantErr {
				if err == nil {
					t.Errorf("BuildProbeCommand() expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("BuildProbeCommand() unexpected error: %v", err)
				return
			}

			if cmd != tt.want {
				t.Errorf("BuildProbeCommand() = %q, want %q", cmd, tt.want)
			}
		})
	}
}

func TestBuildCaptureCommand(t *testing.T) {
	builder := NewCommandBuilder()

	tests := []struct {
		name    string
		config  CaptureConfig
		wantErr bool
		checks  []string // Strings that should be present in the command
	}{
		{
			name: "basic capture config",
			config: CaptureConfig{
				DevicePath:  "/dev/video0",
				OutputPath:  "test.jpg",
				InputFormat: "yuyv422",
				Resolution:  "1280x720",
				FPS:         "30",
				DelayMs:     0,
			},
			wantErr: false,
			checks: []string{
				"ffmpeg",
				"-f v4l2",
				"-input_format yuyv422",
				"-video_size 1280x720",
				"-framerate 30",
				"-i /dev/video0",
				"-frames:v 1",
				"-y test.jpg",
			},
		},
		{
			name: "capture with delay",
			config: CaptureConfig{
				DevicePath: "/dev/video0",
				OutputPath: "test.jpg",
				DelayMs:    2500,
			},
			wantErr: false,
			checks: []string{
				"ffmpeg",
				"-ss 2.500",
				"-f v4l2",
			},
		},
		{
			name: "empty device path",
			config: CaptureConfig{
				DevicePath: "",
				OutputPath: "test.jpg",
			},
			wantErr: true,
		},
		{
			name: "empty output path",
			config: CaptureConfig{
				DevicePath: "/dev/video0",
				OutputPath: "",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd, err := builder.BuildCaptureCommand(tt.config)

			if tt.wantErr {
				if err == nil {
					t.Errorf("BuildCaptureCommand() expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("BuildCaptureCommand() unexpected error: %v", err)
				return
			}

			// Check that all expected strings are present
			for _, check := range tt.checks {
				if !strings.Contains(cmd, check) {
					t.Errorf("BuildCaptureCommand() missing expected string %q in command: %s", check, cmd)
				}
			}
		})
	}
}

func TestBuildStreamCommand(t *testing.T) {
	builder := NewCommandBuilder()

	tests := []struct {
		name    string
		config  StreamConfig
		wantErr bool
		checks  []string // Strings that should be present in the command
	}{
		{
			name: "basic stream config",
			config: StreamConfig{
				DevicePath:  "/dev/video0",
				InputFormat: "yuyv422",
				Resolution:  "1920x1080",
				FPS:         "30",
				Codec:       "libx264",
				Preset:      "fast",
				Bitrate:     "2M",
			},
			wantErr: false,
			checks: []string{
				"ffmpeg",
				"-f v4l2",
				"-input_format yuyv422",
				"-video_size 1920x1080",
				"-framerate 30",
				"-i /dev/video0",
				"-c:v libx264",
				"-preset fast",
				"-b:v 2M",
				"-tune zerolatency",
				"-g 30",
				"-f rtsp",
				"rtsp://localhost:8554/$MTX_PATH",
			},
		},
		{
			name: "stream with progress socket",
			config: StreamConfig{
				DevicePath:     "/dev/video0",
				ProgressSocket: "/tmp/progress.sock",
			},
			wantErr: false,
			checks: []string{
				"-progress unix:///tmp/progress.sock",
			},
		},
		{
			name: "stream with default codec",
			config: StreamConfig{
				DevicePath: "/dev/video0",
				// No codec specified, should default to libx264
			},
			wantErr: false,
			checks: []string{
				"-c:v libx264",
			},
		},
		{
			name: "hardware encoder (no zerolatency)",
			config: StreamConfig{
				DevicePath: "/dev/video0",
				Codec:      "h264_vaapi",
			},
			wantErr: false,
			checks: []string{
				"-c:v h264_vaapi",
				// Hardware encoders should not have hardcoded GOP settings
				// These should come from EncoderParams if needed
			},
		},
		{
			name: "VAAPI encoder with full settings",
			config: StreamConfig{
				DevicePath:    "/dev/video0",
				Codec:         "h264_vaapi",
				GlobalArgs:    []string{"-vaapi_device", "/dev/dri/renderD128"},
				VideoFilters:  "format=nv12,hwupload",
				EncoderParams: map[string]string{"qp": "20"},
			},
			wantErr: false,
			checks: []string{
				"ffmpeg",
				"-vaapi_device /dev/dri/renderD128",
				"-f v4l2",
				"-i /dev/video0",
				"-vf format=nv12,hwupload",
				"-c:v h264_vaapi",
				"-qp 20",
			},
		},
		{
			name: "with audio device",
			config: StreamConfig{
				DevicePath:  "/dev/video0",
				AudioDevice: "hw:4,0",
				Codec:       "libx264",
			},
			checks: []string{
				"-f v4l2",
				"-i /dev/video0",
				"-thread_queue_size 512",
				"-f alsa",
				"-ac 2",
				"-i hw:4,0",
				"-map 0:v -map 1:a",
				"-c:v libx264",
				"-c:a copy",
			},
		},
		{
			name: "empty device path",
			config: StreamConfig{
				DevicePath: "",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd, err := builder.BuildStreamCommand(tt.config)

			if tt.wantErr {
				if err == nil {
					t.Errorf("BuildStreamCommand() expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("BuildStreamCommand() unexpected error: %v", err)
				return
			}

			// Print the command for VAAPI test
			if tt.name == "VAAPI encoder with full settings" {
				t.Logf("Generated command: %s", cmd)
			}

			// Check that all expected strings are present
			for _, check := range tt.checks {
				if !strings.Contains(cmd, check) {
					t.Errorf("BuildStreamCommand() missing expected string %q in command: %s", check, cmd)
				}
			}

			// For hardware encoders, make sure software-only options are NOT present
			if tt.name == "hardware encoder (no zerolatency)" {
				if strings.Contains(cmd, "-tune zerolatency") {
					t.Errorf("BuildStreamCommand() should not include zerolatency for hardware encoder: %s", cmd)
				}
				if strings.Contains(cmd, "-sc_threshold") {
					t.Errorf("BuildStreamCommand() should not include sc_threshold for hardware encoder: %s", cmd)
				}
				if strings.Contains(cmd, "-keyint_min") {
					t.Errorf("BuildStreamCommand() should not include keyint_min for hardware encoder: %s", cmd)
				}
				if strings.Contains(cmd, "-g 30") {
					t.Errorf("BuildStreamCommand() should not include hardcoded GOP for hardware encoder: %s", cmd)
				}
			}
		})
	}
}

func TestApplyOptionsToCommand(t *testing.T) {
	tests := []struct {
		name    string
		options []OptionType
		want    []string // Strings that should be present in the builder
	}{
		{
			name:    "no options",
			options: []OptionType{},
			want:    []string{},
		},
		{
			name:    "copyts with genpts option",
			options: []OptionType{OptionCopytsWithGenpts},
			want:    []string{"-fflags +genpts"},
		},
		{
			name:    "thread queue option",
			options: []OptionType{OptionThreadQueue1024},
			want:    []string{"-thread_queue_size 1024"},
		},
		{
			name:    "low latency option",
			options: []OptionType{OptionLowLatency},
			want:    []string{"-fflags +flush_packets", "-flags +low_delay"},
		},
		{
			name:    "multiple options",
			options: []OptionType{OptionCopytsWithGenpts, OptionLowLatency},
			want:    []string{"-fflags +genpts", "-fflags +flush_packets", "-flags +low_delay"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var builder strings.Builder
			applied := ApplyOptionsToCommand(tt.options, &builder)

			result := builder.String()

			// Check applied options count
			if len(applied) != len(tt.options) {
				t.Errorf("ApplyOptionsToCommand() applied %d options, want %d", len(applied), len(tt.options))
			}

			// Check that expected strings are present
			for _, want := range tt.want {
				if !strings.Contains(result, want) {
					t.Errorf("ApplyOptionsToCommand() missing expected string %q in result: %s", want, result)
				}
			}
		})
	}
}

func TestGetDefaultOptions(t *testing.T) {
	defaults := GetDefaultOptions()

	// Check that we get some default options
	if len(defaults) == 0 {
		t.Error("GetDefaultOptions() returned no default options")
	}

	// Check that OptionThreadQueue1024 is included (it's marked as AppDefault: true)
	foundThreadQueue := false
	foundCopyTS := false
	for _, opt := range defaults {
		if opt == OptionThreadQueue1024 {
			foundThreadQueue = true
		}
		if opt == OptionCopytsWithGenpts {
			foundCopyTS = true
		}
	}
	if !foundThreadQueue {
		t.Error("GetDefaultOptions() should include OptionThreadQueue1024")
	}
	if !foundCopyTS {
		t.Error("GetDefaultOptions() should include OptionCopytsWithGenpts")
	}
}

func TestValidateOptions(t *testing.T) {
	tests := []struct {
		name    string
		options []OptionType
		wantErr bool
		errMsg  string
	}{
		{
			name:    "valid single option",
			options: []OptionType{OptionCopytsWithGenpts},
			wantErr: false,
		},
		{
			name:    "valid non-conflicting options",
			options: []OptionType{OptionCopytsWithGenpts, OptionThreadQueue1024},
			wantErr: false,
		},
		{
			name:    "conflicting timestamp options",
			options: []OptionType{OptionCopytsWithGenpts, OptionWallclockWithGenpts},
			wantErr: true,
			errMsg:  "exclusive group",
		},
		{
			name:    "exclusive thread queue options",
			options: []OptionType{OptionThreadQueue1024, OptionThreadQueue4096},
			wantErr: true,
			errMsg:  "exclusive group",
		},
		{
			name:    "empty options",
			options: []OptionType{},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateOptions(tt.options)

			if tt.wantErr {
				if err == nil {
					t.Errorf("ValidateOptions() expected error but got none")
					return
				}
				if tt.errMsg != "" && !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("ValidateOptions() error message should contain %q, got: %v", tt.errMsg, err)
				}
			} else {
				if err != nil {
					t.Errorf("ValidateOptions() unexpected error: %v", err)
				}
			}
		})
	}
}

func TestGenerateCommandBackwardCompatibility(t *testing.T) {
	// Test that the old GenerateCommand function still works
	config := StreamConfig{
		DevicePath: "/dev/video0",
		Codec:      "libx264",
	}

	cmd, err := GenerateCommand(config)
	if err != nil {
		t.Fatalf("GenerateCommand() failed: %v", err)
	}

	if !strings.Contains(cmd, "ffmpeg") {
		t.Errorf("GenerateCommand() should generate ffmpeg command, got: %s", cmd)
	}
}
