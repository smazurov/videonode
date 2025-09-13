package streams

import (
	"strings"
	"testing"

	"github.com/smazurov/videonode/internal/ffmpeg"
	"github.com/smazurov/videonode/internal/types"
)

func TestProcessorRemovesBitrateFromEncoderParams(t *testing.T) {
	// Create mock repository
	repo := NewTOMLRepository("test.toml")
	stream := StreamConfig{
		ID:      "test",
		Device:  "usb-test",
		Enabled: true,
		FFmpeg: FFmpegConfig{
			Codec:       "h264",
			InputFormat: "yuyv422",
			Resolution:  "1920x1080",
			FPS:         "30",
			QualityParams: &types.QualityParams{
				Mode:          types.RateControlCBR,
				TargetBitrate: func() *float64 { v := 5.0; return &v }(),
			},
		},
	}
	repo.config.Streams["test"] = stream

	processor := NewProcessor(repo)

	// Mock encoder selector that returns FFmpegParams
	processor.SetEncoderSelector(func(codec, inputFormat string, qualityParams *types.QualityParams, encoderOverride string) *ffmpeg.Params {
		params := &ffmpeg.Params{
			Encoder:      "h264_vaapi",
			Bitrate:      "5M",
			RCMode:       "CBR",
			GOP:          30,
			GlobalArgs:   []string{"-vaapi_device", "/dev/dri/renderD128"},
			VideoFilters: "format=nv12,hwupload",
		}
		if encoderOverride != "" {
			params.Encoder = encoderOverride
		}
		return params
	})

	// Mock device resolver
	processor.SetDeviceResolver(func(device string) string {
		return "/dev/video0"
	})

	// Mock socket creator
	processor.SetSocketCreator(func(streamID string) string {
		return "/tmp/test.sock"
	})

	processed, err := processor.ProcessStream("test")
	if err != nil {
		t.Fatalf("ProcessStream failed: %v", err)
	}

	// Check that bitrate was extracted correctly
	if processed.Bitrate != "5M" {
		t.Errorf("Expected Bitrate to be '5M', got '%s'", processed.Bitrate)
	}

	// Verify the command has exactly one -b:v flag
	count := strings.Count(processed.FFmpegCommand, "-b:v")
	if count != 1 {
		t.Errorf("Expected exactly 1 -b:v flag, got %d in command: %s", count, processed.FFmpegCommand)
	}

	// Verify the command has the correct bitrate value
	if !strings.Contains(processed.FFmpegCommand, "-b:v 5M") {
		t.Errorf("FFmpeg command should contain '-b:v 5M', got: %s", processed.FFmpegCommand)
	}

	// Verify the command has rc_mode for hardware encoder
	if !strings.Contains(processed.FFmpegCommand, "-rc_mode CBR") {
		t.Errorf("FFmpeg command should contain '-rc_mode CBR' for hardware encoder, got: %s", processed.FFmpegCommand)
	}
}
