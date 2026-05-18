package api

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/humatest"
	"github.com/smazurov/videonode/internal/devices"
	"github.com/smazurov/videonode/internal/ffmpeg"
	"github.com/smazurov/videonode/internal/streams"
	"github.com/smazurov/videonode/internal/types"
)

// mockStreamService is a test implementation of streams.StreamService.
type mockStreamService struct {
	streams            map[string]*streams.Stream
	streamSpecs        map[string]*streams.StreamSpec
	lastUpdate         *streams.StreamUpdateParams
	validationProvider types.ValidationProvider
}

func (m *mockStreamService) CreateStream(_ context.Context, _ streams.StreamCreateParams) (*streams.Stream, error) {
	return nil, nil
}

func (m *mockStreamService) UpdateStream(_ context.Context, streamID string, params streams.StreamUpdateParams) (*streams.Stream, error) {
	m.lastUpdate = &params
	// Apply params to spec (mirrors real service)
	if spec, ok := m.streamSpecs[streamID]; ok {
		spec.FFmpeg.Codec = params.Codec
		spec.FFmpeg.InputFormat = params.InputFormat
		spec.FFmpeg.Resolution = params.Resolution
		spec.FFmpeg.FPS = params.FPS
		spec.FFmpeg.AudioDevice = params.AudioDevice
		spec.FFmpeg.Options = params.Options
		spec.FFmpeg.QualityParams = params.QualityParams
		spec.CustomFFmpegCommand = params.CustomFFmpegCommand
		spec.TestMode = params.TestMode
		spec.Canvas = params.Canvas
		spec.Perspective = params.Perspective
		spec.Vision = params.Vision
	}
	return m.streams[streamID], nil
}

func (m *mockStreamService) SetEnabled(_ context.Context, streamID string, enabled bool) (bool, error) {
	if s, ok := m.streams[streamID]; ok {
		s.Enabled = enabled
	}
	return enabled, nil
}

func (m *mockStreamService) UpdatePartial(_ context.Context, streamID string, patch func(*streams.StreamSpec) error) (*streams.Stream, error) {
	spec, ok := m.streamSpecs[streamID]
	if !ok {
		return nil, &streams.StreamError{Code: streams.ErrCodeStreamNotFound}
	}
	if err := patch(spec); err != nil {
		return nil, err
	}
	return m.streams[streamID], nil
}

func (m *mockStreamService) DeleteStream(_ context.Context, _ string) error {
	return nil
}

func (m *mockStreamService) RestartStream(_ context.Context, _ string) error {
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

func (m *mockStreamService) ListStreamsWithSpecs(_ context.Context) ([]streams.StreamWithSpec, error) {
	out := make([]streams.StreamWithSpec, 0, len(m.streams))
	for id, s := range m.streams {
		var spec streams.StreamSpec
		if sp, ok := m.streamSpecs[id]; ok && sp != nil {
			spec = *sp
		}
		out = append(out, streams.StreamWithSpec{Stream: *s, Spec: spec})
	}
	return out, nil
}

func (m *mockStreamService) GetFFmpegCommand(_ context.Context, _ string, _ string) (string, bool, error) {
	return "", false, nil
}

func (m *mockStreamService) BroadcastDeviceDiscovery(_ string, _ devices.DeviceInfo, _ string) {
}

func (m *mockStreamService) LoadStreamsFromConfig() error {
	return nil
}

func (m *mockStreamService) GetProcessManager() streams.StreamProcessManager {
	return nil
}

func (m *mockStreamService) ValidationProvider() types.ValidationProvider {
	return m.validationProvider
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

// setupUpdateTest creates a test server with a mock service and registers the update route.
func setupUpdateTest(t *testing.T, spec *streams.StreamSpec) (*mockStreamService, humatest.TestAPI) {
	t.Helper()
	mockSvc := &mockStreamService{
		streams: map[string]*streams.Stream{
			spec.ID: {ID: spec.ID, Enabled: true, StartTime: time.Now()},
		},
		streamSpecs: map[string]*streams.StreamSpec{
			spec.ID: spec,
		},
	}

	_, api := humatest.New(t, huma.DefaultConfig("test", "1.0.0"))
	server := &Server{
		streamService: mockSvc,
		api:           api,
	}
	server.registerStreamRoutes()
	return mockSvc, api
}

func TestUpdateStream_OnlyCodec(t *testing.T) {
	spec := &streams.StreamSpec{
		ID:     "test",
		Device: "usb-test",
		FFmpeg: streams.FFmpegConfig{
			Codec:       "h264",
			InputFormat: "mjpeg",
			Resolution:  "1920x1080",
			FPS:         "30",
			AudioDevice: "hw:4,0",
		},
		Perspective: &ffmpeg.PerspectiveConfig{Corners: [4][2]int{{10, 10}, {100, 10}, {100, 100}, {10, 100}}},
		Vision:      &ffmpeg.VisionConfig{Enabled: true, Width: 640, Height: 480},
	}
	_, api := setupUpdateTest(t, spec)

	resp := api.Patch("/api/streams/test", strings.NewReader(`{"codec": "h265"}`), "Content-Type: application/json")
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body.String())
	}

	if spec.FFmpeg.Codec != "h265" {
		t.Errorf("expected codec h265, got %s", spec.FFmpeg.Codec)
	}
	if spec.FFmpeg.InputFormat != "mjpeg" {
		t.Errorf("expected input_format mjpeg, got %s", spec.FFmpeg.InputFormat)
	}
	if spec.FFmpeg.Resolution != "1920x1080" {
		t.Errorf("expected resolution 1920x1080, got %s", spec.FFmpeg.Resolution)
	}
	if spec.FFmpeg.AudioDevice != "hw:4,0" {
		t.Errorf("expected audio_device hw:4,0, got %s", spec.FFmpeg.AudioDevice)
	}
	if spec.Perspective == nil {
		t.Error("expected perspective to be preserved, got nil")
	}
	if spec.Vision == nil || !spec.Vision.Enabled {
		t.Error("expected vision to be preserved")
	}
}

func TestUpdateStream_SetPerspective(t *testing.T) {
	spec := &streams.StreamSpec{
		ID:     "test",
		Device: "usb-test",
		FFmpeg: streams.FFmpegConfig{Codec: "h264"},
	}
	_, api := setupUpdateTest(t, spec)

	resp := api.Patch("/api/streams/test", strings.NewReader(`{"perspective": {"corners": [[120,45],[1800,60],[1850,1035],[100,1020]]}}`), "Content-Type: application/json")
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body.String())
	}
	if spec.Perspective == nil {
		t.Fatal("expected perspective to be set")
	}
	if spec.Perspective.Corners[0] != [2]int{120, 45} {
		t.Errorf("expected corner[0]=[120,45], got %v", spec.Perspective.Corners[0])
	}
}

func TestUpdateStream_ClearPerspective(t *testing.T) {
	spec := &streams.StreamSpec{
		ID:          "test",
		Device:      "usb-test",
		FFmpeg:      streams.FFmpegConfig{Codec: "h264"},
		Perspective: &ffmpeg.PerspectiveConfig{Corners: [4][2]int{{10, 10}, {100, 10}, {100, 100}, {10, 100}}},
	}
	_, api := setupUpdateTest(t, spec)

	resp := api.Patch("/api/streams/test", strings.NewReader(`{"perspective": null}`), "Content-Type: application/json")
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body.String())
	}
	if spec.Perspective != nil {
		t.Errorf("expected perspective to be cleared, got %v", spec.Perspective)
	}
}

func TestUpdateStream_OmitPerspective(t *testing.T) {
	original := &ffmpeg.PerspectiveConfig{Corners: [4][2]int{{10, 10}, {100, 10}, {100, 100}, {10, 100}}}
	spec := &streams.StreamSpec{
		ID:          "test",
		Device:      "usb-test",
		FFmpeg:      streams.FFmpegConfig{Codec: "h264"},
		Perspective: original,
	}
	_, api := setupUpdateTest(t, spec)

	resp := api.Patch("/api/streams/test", strings.NewReader(`{"codec": "h265"}`), "Content-Type: application/json")
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body.String())
	}
	if spec.Perspective == nil {
		t.Error("expected perspective to be preserved, got nil")
	}
	if spec.Perspective.Corners != original.Corners {
		t.Errorf("expected perspective unchanged, got %v", spec.Perspective.Corners)
	}
}

func TestUpdateStream_SetVision(t *testing.T) {
	spec := &streams.StreamSpec{
		ID:     "test",
		Device: "usb-test",
		FFmpeg: streams.FFmpegConfig{Codec: "h264"},
	}
	_, api := setupUpdateTest(t, spec)

	resp := api.Patch("/api/streams/test", strings.NewReader(`{"vision": {"enabled": true, "width": 320, "height": 240}}`), "Content-Type: application/json")
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body.String())
	}
	if spec.Vision == nil {
		t.Fatal("expected vision to be set")
	}
	if !spec.Vision.Enabled || spec.Vision.Width != 320 || spec.Vision.Height != 240 {
		t.Errorf("unexpected vision: %+v", spec.Vision)
	}
}

func TestUpdateStream_ClearVision(t *testing.T) {
	spec := &streams.StreamSpec{
		ID:     "test",
		Device: "usb-test",
		FFmpeg: streams.FFmpegConfig{Codec: "h264"},
		Vision: &ffmpeg.VisionConfig{Enabled: true, Width: 640, Height: 480},
	}
	_, api := setupUpdateTest(t, spec)

	resp := api.Patch("/api/streams/test", strings.NewReader(`{"vision": null}`), "Content-Type: application/json")
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body.String())
	}
	if spec.Vision != nil {
		t.Errorf("expected vision to be cleared, got %+v", spec.Vision)
	}
}
