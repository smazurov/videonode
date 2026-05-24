package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/humatest"

	"github.com/smazurov/videonode/internal/api/models"
	"github.com/smazurov/videonode/internal/devices"
	"github.com/smazurov/videonode/internal/streams"
	"github.com/smazurov/videonode/internal/types"
)

// mockStreamService satisfies streams.StreamService for handler-level tests.
// Only the methods exercised by the slim CRUD handlers carry real behavior;
// the rest are no-op stubs.
type mockStreamService struct {
	streams            map[string]*streams.Stream
	streamSpecs        map[string]*streams.StreamSpec
	lastCreate         *streams.StreamCreateParams
	deleted            map[string]bool
	restarted          map[string]bool
	validationProvider types.ValidationProvider
}

func newMockStreamService() *mockStreamService {
	return &mockStreamService{
		streams:     map[string]*streams.Stream{},
		streamSpecs: map[string]*streams.StreamSpec{},
		deleted:     map[string]bool{},
		restarted:   map[string]bool{},
	}
}

func (m *mockStreamService) CreateStream(_ context.Context, params streams.StreamCreateParams) (*streams.Stream, error) {
	m.lastCreate = &params
	st := &streams.Stream{ID: params.StreamID, Enabled: true, StartTime: time.Now()}
	m.streams[params.StreamID] = st
	if _, ok := m.streamSpecs[params.StreamID]; !ok {
		m.streamSpecs[params.StreamID] = &streams.StreamSpec{
			ID:     params.StreamID,
			Device: params.DeviceID,
			FFmpeg: streams.FFmpegConfig{
				Codec:       params.Codec,
				AudioDevice: params.AudioDevice,
			},
		}
		if params.Bitrate != nil {
			m.streamSpecs[params.StreamID].FFmpeg.QualityParams = &types.QualityParams{TargetBitrate: params.Bitrate}
		}
	}
	return st, nil
}

func (m *mockStreamService) UpdateStream(_ context.Context, _ string, _ streams.StreamUpdateParams) (*streams.Stream, error) {
	return nil, nil
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

func (m *mockStreamService) DeleteStream(_ context.Context, streamID string) error {
	m.deleted[streamID] = true
	delete(m.streams, streamID)
	delete(m.streamSpecs, streamID)
	return nil
}

func (m *mockStreamService) RestartStream(_ context.Context, streamID string) error {
	m.restarted[streamID] = true
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
	out := make([]streams.Stream, 0, len(m.streams))
	for _, s := range m.streams {
		out = append(out, *s)
	}
	return out, nil
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

func (m *mockStreamService) GetFFmpegCommand(_ context.Context, streamID string, _ string) (string, bool, error) {
	if _, ok := m.streams[streamID]; !ok {
		return "", false, &streams.StreamError{Code: streams.ErrCodeStreamNotFound}
	}
	return "ffmpeg -i pipe:0 -c:v libx264 -f rtsp rtsp://localhost/" + streamID, false, nil
}

func (m *mockStreamService) BroadcastDeviceDiscovery(_ string, _ devices.DeviceInfo, _ string) {
}

func (m *mockStreamService) LoadStreamsFromConfig() error { return nil }

func (m *mockStreamService) GetProcessManager() streams.StreamProcessManager { return nil }

func (m *mockStreamService) ValidationProvider() types.ValidationProvider {
	return m.validationProvider
}

func (m *mockStreamService) StartPipeline(_ context.Context) (bool, error) { return false, nil }
func (m *mockStreamService) StopPipeline(_ context.Context) (bool, error)  { return false, nil }
func (m *mockStreamService) PipelineEnabled() bool                         { return true }

// setupTestServer wires a Server around the mock, register the slim routes, and
// returns both for assertions.
func setupTestServer(t *testing.T) (*mockStreamService, humatest.TestAPI, *Server) {
	t.Helper()
	mock := newMockStreamService()
	_, api := humatest.New(t, huma.DefaultConfig("test", "1.0.0"))
	server := &Server{streamService: mock, api: api}
	server.registerStreamRoutes()
	return mock, api, server
}

func TestDomainToAPIStream_PopulatesSlimShape(t *testing.T) {
	bitrate := 3.0
	spec := &streams.StreamSpec{
		ID:     "test-stream",
		Name:   "Test Stream",
		Device: "hdmi-rx",
		FFmpeg: streams.FFmpegConfig{
			Codec:       "h265",
			AudioDevice: "hw:CARD=USB,DEV=0",
			QualityParams: &types.QualityParams{
				TargetBitrate: &bitrate,
			},
		},
	}
	mock, _, server := setupTestServer(t)
	mock.streams["test-stream"] = &streams.Stream{ID: "test-stream", Enabled: true, StartTime: time.Now()}
	mock.streamSpecs["test-stream"] = spec

	apiData := server.domainToAPIStream(*mock.streams["test-stream"])

	tests := []struct {
		name string
		got  any
		want any
	}{
		{"stream_id", apiData.StreamID, "test-stream"},
		{"name", apiData.Name, "Test Stream"},
		{"upstream", apiData.Upstream, "source:hdmi-rx"},
		{"encoder.codec", apiData.Encoder.Codec, "h265"},
		{"encoder.bitrate", apiData.Encoder.Bitrate, "3.0M"},
		{"audio.devices[0]", apiData.Audio.Devices[0], "hw:CARD=USB,DEV=0"},
		{"enabled", apiData.Enabled, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Errorf("got %v, want %v", tt.got, tt.want)
			}
		})
	}
}

func TestDomainToAPIStream_MissingSpecReturnsMinimal(t *testing.T) {
	mock, _, server := setupTestServer(t)
	mock.streams["test-stream"] = &streams.Stream{ID: "test-stream", Enabled: true, StartTime: time.Now()}

	apiData := server.domainToAPIStream(*mock.streams["test-stream"])
	if apiData.StreamID != "test-stream" {
		t.Errorf("expected stream_id preserved, got %q", apiData.StreamID)
	}
	if apiData.Upstream != "" {
		t.Errorf("expected empty upstream, got %q", apiData.Upstream)
	}
	if apiData.Enabled {
		t.Errorf("expected enabled=false when spec missing, got true")
	}
}

func TestCreateStream_PopulatesParamsFromSlimBody(t *testing.T) {
	mock, api, _ := setupTestServer(t)
	body := `{
		"stream_id": "main",
		"name": "Main",
		"upstream": "source:hdmi0",
		"audio": {"devices": ["hw:CARD=USB,DEV=0"]},
		"encoder": {"codec": "h264", "bitrate": "4M"}
	}`
	resp := api.Post("/api/streams", strings.NewReader(body), "Content-Type: application/json")
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body.String())
	}
	if mock.lastCreate == nil {
		t.Fatal("expected CreateStream to be called")
	}
	if got := mock.lastCreate.StreamID; got != "main" {
		t.Errorf("StreamID: got %q, want %q", got, "main")
	}
	if got := mock.lastCreate.DeviceID; got != "hdmi0" {
		t.Errorf("DeviceID: got %q, want %q", got, "hdmi0")
	}
	if got := mock.lastCreate.Codec; got != "h264" {
		t.Errorf("Codec: got %q, want %q", got, "h264")
	}
	if got := mock.lastCreate.AudioDevice; got != "hw:CARD=USB,DEV=0" {
		t.Errorf("AudioDevice: got %q, want %q", got, "hw:CARD=USB,DEV=0")
	}
	if mock.lastCreate.Bitrate == nil || *mock.lastCreate.Bitrate != 4.0 {
		t.Errorf("Bitrate: got %v, want 4.0", mock.lastCreate.Bitrate)
	}
}

func TestCreateStream_RejectsEmptyUpstream(t *testing.T) {
	_, api, _ := setupTestServer(t)
	body := `{"stream_id": "x", "encoder": {"codec": "h264"}}`
	resp := api.Post("/api/streams", strings.NewReader(body), "Content-Type: application/json")
	if resp.Code == http.StatusOK {
		t.Errorf("expected validation failure, got 200: %s", resp.Body.String())
	}
}

func TestUpdateStream_AppliesSlimEncoderAndAudio(t *testing.T) {
	mock, api, _ := setupTestServer(t)
	mock.streams["s1"] = &streams.Stream{ID: "s1", Enabled: true, StartTime: time.Now()}
	mock.streamSpecs["s1"] = &streams.StreamSpec{
		ID:     "s1",
		Device: "hdmi0",
		FFmpeg: streams.FFmpegConfig{Codec: "h264"},
	}

	body := `{
		"encoder": {"codec": "h265", "bitrate": "6M"},
		"audio": {"devices": ["hw:CARD=USB,DEV=0"]},
		"custom_encoder_args": "-preset slow"
	}`
	resp := api.Patch("/api/streams/s1", strings.NewReader(body), "Content-Type: application/json")
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body.String())
	}

	spec := mock.streamSpecs["s1"]
	if spec.FFmpeg.Codec != "h265" {
		t.Errorf("codec: got %q, want h265", spec.FFmpeg.Codec)
	}
	if spec.FFmpeg.QualityParams == nil || spec.FFmpeg.QualityParams.TargetBitrate == nil ||
		*spec.FFmpeg.QualityParams.TargetBitrate != 6.0 {
		t.Errorf("bitrate: got %+v, want 6.0", spec.FFmpeg.QualityParams)
	}
	if spec.FFmpeg.AudioDevice != "hw:CARD=USB,DEV=0" {
		t.Errorf("audio device: got %q", spec.FFmpeg.AudioDevice)
	}
	if spec.CustomFFmpegCommand != "-preset slow" {
		t.Errorf("custom args: got %q", spec.CustomFFmpegCommand)
	}
}

func TestUpdateStream_EnabledTogglesRuntime(t *testing.T) {
	mock, api, _ := setupTestServer(t)
	mock.streams["s1"] = &streams.Stream{ID: "s1", Enabled: true, StartTime: time.Now()}
	mock.streamSpecs["s1"] = &streams.StreamSpec{ID: "s1", Device: "hdmi0"}

	resp := api.Patch("/api/streams/s1", strings.NewReader(`{"enabled": false}`), "Content-Type: application/json")
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body.String())
	}
	if mock.streams["s1"].Enabled {
		t.Errorf("expected enabled=false after PATCH")
	}
}

func TestDeleteStream_RemovesFromService(t *testing.T) {
	mock, api, _ := setupTestServer(t)
	mock.streams["s1"] = &streams.Stream{ID: "s1", Enabled: true, StartTime: time.Now()}
	mock.streamSpecs["s1"] = &streams.StreamSpec{ID: "s1"}

	resp := api.Delete("/api/streams/s1")
	if resp.Code != http.StatusNoContent && resp.Code != http.StatusOK {
		t.Fatalf("expected 204/200, got %d: %s", resp.Code, resp.Body.String())
	}
	if !mock.deleted["s1"] {
		t.Errorf("expected DeleteStream to be called")
	}
}

func TestRestartStream_CallsService(t *testing.T) {
	mock, api, _ := setupTestServer(t)
	mock.streams["s1"] = &streams.Stream{ID: "s1", Enabled: true, StartTime: time.Now()}
	mock.streamSpecs["s1"] = &streams.StreamSpec{ID: "s1"}

	resp := api.Post("/api/streams/s1/restart", strings.NewReader(""), "Content-Type: application/json")
	if resp.Code != http.StatusNoContent && resp.Code != http.StatusOK {
		t.Fatalf("expected 204/200, got %d: %s", resp.Code, resp.Body.String())
	}
	if !mock.restarted["s1"] {
		t.Errorf("expected RestartStream to be called")
	}
}

func TestGetStream_ReturnsSlimShape(t *testing.T) {
	mock, api, _ := setupTestServer(t)
	mock.streams["s1"] = &streams.Stream{ID: "s1", Enabled: true, StartTime: time.Now()}
	mock.streamSpecs["s1"] = &streams.StreamSpec{
		ID:     "s1",
		Name:   "Solo",
		Device: "cam1",
		FFmpeg: streams.FFmpegConfig{Codec: "h264"},
	}

	resp := api.Get("/api/streams/s1")
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body.String())
	}
	var got models.StreamData
	if err := json.Unmarshal(resp.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.StreamID != "s1" || got.Upstream != "source:cam1" || got.Encoder.Codec != "h264" {
		t.Errorf("unexpected slim payload: %+v", got)
	}
}

func TestGetStreamFFmpeg_ReturnsCommand(t *testing.T) {
	mock, api, _ := setupTestServer(t)
	mock.streams["s1"] = &streams.Stream{ID: "s1", Enabled: true, StartTime: time.Now()}
	mock.streamSpecs["s1"] = &streams.StreamSpec{ID: "s1"}

	resp := api.Get("/api/streams/s1/ffmpeg")
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body.String())
	}
	var got models.FFmpegCommandData
	if err := json.Unmarshal(resp.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.StreamID != "s1" || got.Command == "" {
		t.Errorf("unexpected ffmpeg payload: %+v", got)
	}
}

func TestParseUpstreamRef_TableDriven(t *testing.T) {
	tests := []struct {
		name   string
		kind   string
		ref    string
		wantID string
		wantOk bool
	}{
		{"source ref", "source", "source:hdmi0", "hdmi0", true},
		{"composer ref via source kind", "source", "composer:main", "", false},
		{"empty id", "source", "source:", "", false},
		{"no prefix", "source", "hdmi0", "", false},
		{"empty input", "source", "", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := parseUpstreamRef(tt.kind, tt.ref)
			if got != tt.wantID || ok != tt.wantOk {
				t.Errorf("parseUpstreamRef(%q, %q) = (%q,%v), want (%q,%v)", tt.kind, tt.ref, got, ok, tt.wantID, tt.wantOk)
			}
		})
	}
}

func TestBitrateToMbps_TableDriven(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want float64
		ok   bool
	}{
		{"mbps", "4M", 4.0, true},
		{"mbps lowercase", "12m", 12.0, true},
		{"kbps", "1500k", 1.5, true},
		{"plain number", "2.5", 2.5, true},
		{"empty", "", 0, false},
		{"junk", "abc", 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := bitrateToMbps(tt.in)
			if ok != tt.ok {
				t.Fatalf("ok: got %v, want %v", ok, tt.ok)
			}
			if ok && got != tt.want {
				t.Errorf("value: got %v, want %v", got, tt.want)
			}
		})
	}
}
