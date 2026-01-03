package streams

import (
	"strings"
	"testing"

	"github.com/smazurov/videonode/internal/ffmpeg"
	"github.com/smazurov/videonode/internal/types"
)

// mockStore is a test implementation of Store.
type mockStore struct {
	streams map[string]StreamSpec
}

func (m *mockStore) Load() error                       { return nil }
func (m *mockStore) Save() error                       { return nil }
func (m *mockStore) AddStream(stream StreamSpec) error { m.streams[stream.ID] = stream; return nil }
func (m *mockStore) UpdateStream(id string, stream StreamSpec) error {
	m.streams[id] = stream
	return nil
}
func (m *mockStore) RemoveStream(id string) error                    { delete(m.streams, id); return nil }
func (m *mockStore) GetStream(id string) (StreamSpec, bool)          { s, ok := m.streams[id]; return s, ok }
func (m *mockStore) GetAllStreams() map[string]StreamSpec            { return m.streams }
func (m *mockStore) GetValidation() *types.ValidationResults         { return nil }
func (m *mockStore) UpdateValidation(*types.ValidationResults) error { return nil }

func TestProcessorRemovesBitrateFromEncoderParams(t *testing.T) {
	// Create mock repository
	repo := &mockStore{streams: make(map[string]StreamSpec)}
	stream := StreamSpec{
		ID:     "test",
		Device: "usb-test",
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
	if err := repo.AddStream(stream); err != nil {
		t.Fatalf("AddStream failed: %v", err)
	}

	processor := newProcessor(repo)

	// Mock encoder selector that returns FFmpegParams
	processor.setEncoderSelector(func(codec, inputFormat string, qualityParams *types.QualityParams, encoderOverride string) *ffmpeg.Params {
		// Verify expected parameters from stream config
		if codec != "h264" {
			t.Errorf("expected codec h264, got %s", codec)
		}
		if inputFormat != "yuyv422" {
			t.Errorf("expected inputFormat yuyv422, got %s", inputFormat)
		}
		if qualityParams == nil || qualityParams.Mode != types.RateControlCBR {
			t.Errorf("expected CBR mode, got %+v", qualityParams)
		}
		if qualityParams == nil || qualityParams.TargetBitrate == nil || *qualityParams.TargetBitrate != 5.0 {
			t.Errorf("expected targetBitrate 5.0, got %+v", qualityParams)
		}
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
	processor.setDeviceResolver(func(_ string) string {
		return "/dev/video0"
	})

	// Mock stream state getter (enabled = true)
	processor.setStreamStateGetter(func(streamID string) (*Stream, bool) {
		return &Stream{ID: streamID, Enabled: true}, true
	})

	processed, err := processor.processStream("test")
	if err != nil {
		t.Fatalf("ProcessStream failed: %v", err)
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

func TestPrecedenceNoSignalOverCustomCommand(t *testing.T) {
	repo := &mockStore{streams: make(map[string]StreamSpec)}
	stream := StreamSpec{
		ID:                  "test",
		Device:              "usb-test",
		CustomFFmpegCommand: "ffmpeg -f v4l2 -i /dev/video0 -c:v copy -f rtsp rtsp://localhost:8554/test",
		FFmpeg: FFmpegConfig{
			Codec:       "h264",
			InputFormat: "yuyv422",
		},
	}
	if err := repo.AddStream(stream); err != nil {
		t.Fatalf("AddStream failed: %v", err)
	}

	processor := newProcessor(repo)
	processor.setDeviceResolver(func(_ string) string {
		return "/dev/video0"
	})

	// Mock stream state getter (enabled = false, device offline)
	processor.setStreamStateGetter(func(streamID string) (*Stream, bool) {
		return &Stream{ID: streamID, Enabled: false}, true
	})

	processed, err := processor.processStream("test")
	if err != nil {
		t.Fatalf("ProcessStream failed: %v", err)
	}

	// Should generate NO SIGNAL test pattern, NOT use custom command
	if processed.FFmpegCommand == stream.CustomFFmpegCommand {
		t.Errorf("Expected NO SIGNAL test pattern, got custom command: %s", processed.FFmpegCommand)
	}
	if !strings.Contains(processed.FFmpegCommand, "testsrc") {
		t.Errorf("Expected test source for NO SIGNAL, got: %s", processed.FFmpegCommand)
	}
	if !strings.Contains(processed.FFmpegCommand, "NO SIGNAL") {
		t.Errorf("Expected 'NO SIGNAL' overlay, got: %s", processed.FFmpegCommand)
	}
}

func TestPrecedenceCustomCommandWhenOnline(t *testing.T) {
	repo := &mockStore{streams: make(map[string]StreamSpec)}
	customCmd := "ffmpeg -f v4l2 -i /dev/video0 -c:v copy -f rtsp rtsp://localhost:8554/test"
	stream := StreamSpec{
		ID:                  "test",
		Device:              "usb-test",
		CustomFFmpegCommand: customCmd,
		FFmpeg: FFmpegConfig{
			Codec:       "h264",
			InputFormat: "yuyv422",
		},
	}
	if err := repo.AddStream(stream); err != nil {
		t.Fatalf("AddStream failed: %v", err)
	}

	processor := newProcessor(repo)

	// Mock stream state getter (enabled = true, device online)
	processor.setStreamStateGetter(func(streamID string) (*Stream, bool) {
		return &Stream{ID: streamID, Enabled: true}, true
	})

	processed, err := processor.processStream("test")
	if err != nil {
		t.Fatalf("ProcessStream failed: %v", err)
	}

	// Should use custom command when device is online
	if processed.FFmpegCommand != customCmd {
		t.Errorf("Expected custom command when device online, got: %s", processed.FFmpegCommand)
	}
}

func TestPrecedenceTestModeWhenOnlineNoCustomCommand(t *testing.T) {
	repo := &mockStore{streams: make(map[string]StreamSpec)}
	stream := StreamSpec{
		ID:       "test",
		Device:   "usb-test",
		TestMode: true, // Test mode enabled
		FFmpeg: FFmpegConfig{
			Codec:       "h264",
			InputFormat: "yuyv422",
		},
	}
	if err := repo.AddStream(stream); err != nil {
		t.Fatalf("AddStream failed: %v", err)
	}

	processor := newProcessor(repo)
	processor.setDeviceResolver(func(_ string) string {
		return "/dev/video0"
	})

	// Mock stream state getter (enabled = true, device online)
	processor.setStreamStateGetter(func(streamID string) (*Stream, bool) {
		return &Stream{ID: streamID, Enabled: true}, true
	})

	processed, err := processor.processStream("test")
	if err != nil {
		t.Fatalf("ProcessStream failed: %v", err)
	}

	// Should generate TEST MODE test pattern
	if !strings.Contains(processed.FFmpegCommand, "testsrc") {
		t.Errorf("Expected test source for TEST MODE, got: %s", processed.FFmpegCommand)
	}
	if !strings.Contains(processed.FFmpegCommand, "TEST MODE") {
		t.Errorf("Expected 'TEST MODE' overlay, got: %s", processed.FFmpegCommand)
	}
}

func TestPrecedenceTestModeIgnoredWhenCustomCommand(t *testing.T) {
	repo := &mockStore{streams: make(map[string]StreamSpec)}
	customCmd := "ffmpeg -f v4l2 -i /dev/video0 -c:v copy -f rtsp rtsp://localhost:8554/test"
	stream := StreamSpec{
		ID:                  "test",
		Device:              "usb-test",
		TestMode:            true, // Test mode enabled but should be ignored
		CustomFFmpegCommand: customCmd,
		FFmpeg: FFmpegConfig{
			Codec:       "h264",
			InputFormat: "yuyv422",
		},
	}
	if err := repo.AddStream(stream); err != nil {
		t.Fatalf("AddStream failed: %v", err)
	}

	processor := newProcessor(repo)

	// Mock stream state getter (enabled = true, device online)
	processor.setStreamStateGetter(func(streamID string) (*Stream, bool) {
		return &Stream{ID: streamID, Enabled: true}, true
	})

	processed, err := processor.processStream("test")
	if err != nil {
		t.Fatalf("ProcessStream failed: %v", err)
	}

	// Custom command takes precedence over test mode
	if processed.FFmpegCommand != customCmd {
		t.Errorf("Expected custom command to override test mode, got: %s", processed.FFmpegCommand)
	}
}
