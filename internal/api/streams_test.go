package api

import (
	"context"
	"testing"
	"time"

	"github.com/smazurov/videonode/internal/devices"
	"github.com/smazurov/videonode/internal/streams"
	"github.com/smazurov/videonode/internal/types"
)

// mockStreamService is a test implementation of streams.StreamService.
type mockStreamService struct {
	streams     map[string]*streams.Stream
	streamSpecs map[string]*streams.StreamSpec
}

func (m *mockStreamService) CreateStream(_ context.Context, _ streams.StreamCreateParams) (*streams.Stream, error) {
	return nil, nil
}

func (m *mockStreamService) UpdateStream(_ context.Context, _ string, _ streams.StreamUpdateParams) (*streams.Stream, error) {
	return nil, nil
}

func (m *mockStreamService) DeleteStream(_ context.Context, _ string) error {
	return nil
}

func (m *mockStreamService) GetStream(_ context.Context, streamID string) (*streams.Stream, error) {
	s, ok := m.streams[streamID]
	if !ok {
		return nil, &streams.StreamError{Code: streams.ErrCodeStreamNotFound}
	}
	return s, nil
}

func (m *mockStreamService) GetStreamSpec(_ context.Context, streamID string) (*streams.StreamSpec, error) {
	spec, ok := m.streamSpecs[streamID]
	if !ok {
		return nil, &streams.StreamError{Code: streams.ErrCodeStreamNotFound}
	}
	return spec, nil
}

func (m *mockStreamService) ListStreams(_ context.Context) ([]streams.Stream, error) {
	result := make([]streams.Stream, 0, len(m.streams))
	for _, s := range m.streams {
		result = append(result, *s)
	}
	return result, nil
}

func (m *mockStreamService) GetFFmpegCommand(_ context.Context, _ string, _ string) (string, bool, error) {
	return "", false, nil
}

func (m *mockStreamService) BroadcastDeviceDiscovery(_ string, _ devices.DeviceInfo, _ string) {
}

func (m *mockStreamService) LoadStreamsFromConfig() error {
	return nil
}

func TestDomainToAPIStream_ReadsCodecFromConfig(t *testing.T) {
	// Setup mock service
	mockSvc := &mockStreamService{
		streams:     make(map[string]*streams.Stream),
		streamSpecs: make(map[string]*streams.StreamSpec),
	}

	// Create runtime stream state
	stream := &streams.Stream{
		ID:        "test-stream",
		Enabled:   true,
		StartTime: time.Now(),
	}
	mockSvc.streams["test-stream"] = stream

	// Create config with h265 codec
	spec := &streams.StreamSpec{
		ID:     "test-stream",
		Device: "platform-test-device",
		FFmpeg: streams.FFmpegConfig{
			Codec:       "h265",
			InputFormat: "nv16",
			Resolution:  "1920x1080",
			FPS:         "30",
			QualityParams: &types.QualityParams{
				TargetBitrate: func() *float64 { v := 3.0; return &v }(),
			},
		},
	}
	mockSvc.streamSpecs["test-stream"] = spec

	// Create server with mock service
	server := &Server{
		streamService: mockSvc,
	}

	// Convert to API model
	apiData := server.domainToAPIStream(*stream)

	// Verify codec comes from config, not runtime state
	if apiData.Codec != "h265" {
		t.Errorf("Expected codec 'h265' from config, got '%s'", apiData.Codec)
	}

	// Verify device comes from config
	if apiData.DeviceID != "platform-test-device" {
		t.Errorf("Expected device 'platform-test-device' from config, got '%s'", apiData.DeviceID)
	}

	// Verify enabled comes from runtime state
	if apiData.Enabled != true {
		t.Errorf("Expected enabled 'true' from runtime state, got '%v'", apiData.Enabled)
	}

	// Verify other config fields
	if apiData.InputFormat != "nv16" {
		t.Errorf("Expected input format 'nv16' from config, got '%s'", apiData.InputFormat)
	}

	if apiData.Resolution != "1920x1080" {
		t.Errorf("Expected resolution '1920x1080' from config, got '%s'", apiData.Resolution)
	}

	if apiData.Bitrate != "3.0M" {
		t.Errorf("Expected bitrate '3.0M' from config, got '%s'", apiData.Bitrate)
	}
}

func TestDomainToAPIStream_AfterCodecUpdate(t *testing.T) {
	// Setup mock service
	mockSvc := &mockStreamService{
		streams:     make(map[string]*streams.Stream),
		streamSpecs: make(map[string]*streams.StreamSpec),
	}

	// Create runtime stream state (doesn't store codec)
	stream := &streams.Stream{
		ID:        "test-stream",
		Enabled:   false,
		StartTime: time.Now(),
	}
	mockSvc.streams["test-stream"] = stream

	// Create config with h264 initially
	spec := &streams.StreamSpec{
		ID:     "test-stream",
		Device: "platform-test-device",
		FFmpeg: streams.FFmpegConfig{
			Codec:       "h264",
			InputFormat: "nv16",
		},
	}
	mockSvc.streamSpecs["test-stream"] = spec

	server := &Server{
		streamService: mockSvc,
	}

	// First conversion - should show h264
	apiData := server.domainToAPIStream(*stream)
	if apiData.Codec != "h264" {
		t.Errorf("Expected initial codec 'h264', got '%s'", apiData.Codec)
	}

	// Simulate UpdateStream changing codec to h265 in config
	spec.FFmpeg.Codec = "h265"

	// Second conversion - should show h265 (not stale h264)
	apiData = server.domainToAPIStream(*stream)
	if apiData.Codec != "h265" {
		t.Errorf("Expected updated codec 'h265', got '%s'", apiData.Codec)
	}
}

func TestDomainToAPIStream_EnabledFromRuntimeState(t *testing.T) {
	// Setup mock service
	mockSvc := &mockStreamService{
		streams:     make(map[string]*streams.Stream),
		streamSpecs: make(map[string]*streams.StreamSpec),
	}

	// Create runtime stream state with enabled = false
	stream := &streams.Stream{
		ID:        "test-stream",
		Enabled:   false, // Device offline
		StartTime: time.Now(),
	}
	mockSvc.streams["test-stream"] = stream

	// Create config
	spec := &streams.StreamSpec{
		ID:     "test-stream",
		Device: "platform-test-device",
		FFmpeg: streams.FFmpegConfig{
			Codec: "h264",
		},
	}
	mockSvc.streamSpecs["test-stream"] = spec

	server := &Server{
		streamService: mockSvc,
	}

	// Convert - should show enabled = false
	apiData := server.domainToAPIStream(*stream)
	if apiData.Enabled != false {
		t.Errorf("Expected enabled 'false' from runtime state, got '%v'", apiData.Enabled)
	}

	// Simulate device coming online (runtime state change)
	stream.Enabled = true

	// Convert again - should show enabled = true
	apiData = server.domainToAPIStream(*stream)
	if apiData.Enabled != true {
		t.Errorf("Expected enabled 'true' after runtime state change, got '%v'", apiData.Enabled)
	}
}

func TestDomainToAPIStream_HandlesConfigError(t *testing.T) {
	// Setup mock service that fails to get config
	mockSvc := &mockStreamService{
		streams:     make(map[string]*streams.Stream),
		streamSpecs: make(map[string]*streams.StreamSpec),
	}

	// Create runtime stream state but NO config
	stream := &streams.Stream{
		ID:        "test-stream",
		Enabled:   true,
		StartTime: time.Now(),
	}
	mockSvc.streams["test-stream"] = stream

	server := &Server{
		streamService: mockSvc,
	}

	// Convert - when config is unavailable, should return minimal data (no config fields, no runtime state)
	apiData := server.domainToAPIStream(*stream)
	if apiData.Codec != "" {
		t.Errorf("Expected empty codec when config unavailable, got '%s'", apiData.Codec)
	}
	if apiData.DeviceID != "" {
		t.Errorf("Expected empty device when config unavailable, got '%s'", apiData.DeviceID)
	}
	// Runtime state also should not be populated when config is missing (incomplete data)
	if apiData.Enabled != false {
		t.Errorf("Expected enabled 'false' (zero value) when config unavailable, got '%v'", apiData.Enabled)
	}
	// Only basic fields should be set
	if apiData.StreamID != "test-stream" {
		t.Errorf("Expected stream ID 'test-stream', got '%s'", apiData.StreamID)
	}
}

func TestDomainToAPIStream_BitrateFormatting(t *testing.T) {
	mockSvc := &mockStreamService{
		streams:     make(map[string]*streams.Stream),
		streamSpecs: make(map[string]*streams.StreamSpec),
	}

	stream := &streams.Stream{
		ID:        "test-stream",
		Enabled:   true,
		StartTime: time.Now(),
	}
	mockSvc.streams["test-stream"] = stream

	// Test with bitrate value
	spec := &streams.StreamSpec{
		ID:     "test-stream",
		Device: "test-device",
		FFmpeg: streams.FFmpegConfig{
			Codec: "h264",
			QualityParams: &types.QualityParams{
				TargetBitrate: func() *float64 { v := 5.5; return &v }(),
			},
		},
	}
	mockSvc.streamSpecs["test-stream"] = spec

	server := &Server{
		streamService: mockSvc,
	}

	apiData := server.domainToAPIStream(*stream)
	if apiData.Bitrate != "5.5M" {
		t.Errorf("Expected bitrate '5.5M', got '%s'", apiData.Bitrate)
	}

	// Test with nil quality params - should use default
	spec.FFmpeg.QualityParams = nil
	apiData = server.domainToAPIStream(*stream)
	if apiData.Bitrate != "2M" {
		t.Errorf("Expected default bitrate '2M', got '%s'", apiData.Bitrate)
	}
}
