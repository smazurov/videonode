package streams

import (
	"strings"
	"testing"

	"github.com/smazurov/videonode/internal/ffmpeg"
	"github.com/smazurov/videonode/internal/types"
)

// mockStore is a test implementation of Store
type mockStore struct {
	streams map[string]StreamSpec
}

func (m *mockStore) Load() error                                         { return nil }
func (m *mockStore) Save() error                                         { return nil }
func (m *mockStore) AddStream(stream StreamSpec) error                   { m.streams[stream.ID] = stream; return nil }
func (m *mockStore) UpdateStream(id string, stream StreamSpec) error     { m.streams[id] = stream; return nil }
func (m *mockStore) RemoveStream(id string) error                        { delete(m.streams, id); return nil }
func (m *mockStore) GetStream(id string) (StreamSpec, bool)              { s, ok := m.streams[id]; return s, ok }
func (m *mockStore) GetAllStreams() map[string]StreamSpec                { return m.streams }
func (m *mockStore) GetValidation() *types.ValidationResults             { return nil }
func (m *mockStore) UpdateValidation(*types.ValidationResults) error     { return nil }

func TestProcessorRemovesBitrateFromEncoderParams(t *testing.T) {
	// Create mock repository
	repo := &mockStore{streams: make(map[string]StreamSpec)}
	stream := StreamSpec{
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
	repo.AddStream(stream)

	processor := newProcessor(repo)

	// Mock encoder selector that returns FFmpegParams
	processor.setEncoderSelector(func(codec, inputFormat string, qualityParams *types.QualityParams, encoderOverride string) *ffmpeg.Params {
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
	processor.setDeviceResolver(func(device string) string {
		return "/dev/video0"
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
