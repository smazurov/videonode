//go:build planv2_tests

// CRUD tests for the post-B7 /api/streams endpoints (slim shape: no
// device, canvas, inputs, layout, effects, vision, perspective). The
// new StreamData shape carries stream_id, upstream, encoder, audio,
// publish, custom_encoder_args, plus runtime fields.
package api

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
)

// StreamPlan mirrors the post-B7 StreamData snake_case shape.
type StreamPlan struct {
	StreamID          string        `json:"stream_id"`
	Upstream          string        `json:"upstream"`
	Encoder           EncoderPlan   `json:"encoder,omitzero"`
	Audio             AudioPlan     `json:"audio,omitzero"`
	Publish           []PublishPlan `json:"publish,omitempty"`
	CustomEncoderArgs string        `json:"custom_encoder_args,omitempty"`
	Enabled           bool          `json:"enabled,omitempty"`
}

type EncoderPlan struct {
	Codec   string `json:"codec,omitempty"`
	Bitrate string `json:"bitrate,omitempty"`
	GOP     int    `json:"gop,omitempty"`
}

type AudioPlan struct {
	Devices []string `json:"devices,omitempty"`
	Codec   string   `json:"codec,omitempty"`
	Bitrate string   `json:"bitrate,omitempty"`
}

type PublishPlan struct {
	Type string `json:"type"`
	URL  string `json:"url"`
}

type mockStreamAPISvc struct {
	store map[string]StreamPlan
}

func (m *mockStreamAPISvc) Create(_ context.Context, s StreamPlan) (StreamPlan, error) {
	if s.StreamID == "" {
		return s, errors.New("stream_id required")
	}
	if s.Upstream == "" {
		return s, errors.New("upstream required")
	}
	if _, exists := m.store[s.StreamID]; exists {
		return s, errors.New("stream exists")
	}
	m.store[s.StreamID] = s
	return s, nil
}

func (m *mockStreamAPISvc) Get(_ context.Context, id string) (StreamPlan, error) {
	s, ok := m.store[id]
	if !ok {
		return s, errors.New("not found")
	}
	return s, nil
}

func (m *mockStreamAPISvc) Patch(_ context.Context, id string, patch StreamPlan) (StreamPlan, error) {
	s, ok := m.store[id]
	if !ok {
		return s, errors.New("not found")
	}
	if patch.Encoder.Codec != "" {
		s.Encoder.Codec = patch.Encoder.Codec
	}
	if patch.Encoder.Bitrate != "" {
		s.Encoder.Bitrate = patch.Encoder.Bitrate
	}
	if patch.Upstream != "" {
		s.Upstream = patch.Upstream
	}
	m.store[id] = s
	return s, nil
}

func (m *mockStreamAPISvc) Delete(_ context.Context, id string) error {
	if _, ok := m.store[id]; !ok {
		return errors.New("not found")
	}
	delete(m.store, id)
	return nil
}

func (m *mockStreamAPISvc) List(_ context.Context) ([]StreamPlan, error) {
	out := make([]StreamPlan, 0, len(m.store))
	for _, s := range m.store {
		out = append(out, s)
	}
	return out, nil
}

type streamRouter struct {
	svc *mockStreamAPISvc
}

func newStreamRouter() *streamRouter {
	return &streamRouter{svc: &mockStreamAPISvc{store: map[string]StreamPlan{}}}
}

//nolint:dupl // CRUD handler shape intentionally mirrors sourceRouter / composerRouter for symmetry.
func (r *streamRouter) handle(method, path, body string) (int, string) {
	switch {
	case method == "GET" && path == "/api/streams":
		l, _ := r.svc.List(context.Background())
		return http.StatusOK, mustJSON(l)
	case method == "POST" && path == "/api/streams":
		var in StreamPlan
		mustUnmarshal(body, &in)
		out, err := r.svc.Create(context.Background(), in)
		if err != nil {
			return http.StatusBadRequest, err.Error()
		}
		return http.StatusOK, mustJSON(out)
	case method == "GET" && strings.HasPrefix(path, "/api/streams/"):
		id := strings.TrimPrefix(path, "/api/streams/")
		s, err := r.svc.Get(context.Background(), id)
		if err != nil {
			return http.StatusNotFound, err.Error()
		}
		return http.StatusOK, mustJSON(s)
	case method == "PATCH" && strings.HasPrefix(path, "/api/streams/"):
		id := strings.TrimPrefix(path, "/api/streams/")
		var in StreamPlan
		mustUnmarshal(body, &in)
		s, err := r.svc.Patch(context.Background(), id, in)
		if err != nil {
			return http.StatusNotFound, err.Error()
		}
		return http.StatusOK, mustJSON(s)
	case method == "DELETE" && strings.HasPrefix(path, "/api/streams/"):
		id := strings.TrimPrefix(path, "/api/streams/")
		if err := r.svc.Delete(context.Background(), id); err != nil {
			return http.StatusNotFound, err.Error()
		}
		return http.StatusNoContent, ""
	}
	return http.StatusNotFound, "no route"
}

const sampleStreamJSON = `{
"stream_id":"archive",
"upstream":"composer:main",
"encoder":{"codec":"h265","bitrate":"12M","gop":120},
"audio":{"devices":["hw:CARD=USB,DEV=0"],"codec":"aac","bitrate":"192k"},
"publish":[{"type":"rtsp","url":"rtsp://nas.lan:8554/archive/main"}]
}`

func TestStreamsAPI_PostThenGet(t *testing.T) {
	r := newStreamRouter()
	code, body := r.handle("POST", "/api/streams", sampleStreamJSON)
	if code != http.StatusOK {
		t.Fatalf("POST = %d: %s", code, body)
	}
	code, body = r.handle("GET", "/api/streams/archive", "")
	if code != http.StatusOK {
		t.Fatalf("GET = %d: %s", code, body)
	}
	for _, want := range []string{`"stream_id":"archive"`, `"upstream":"composer:main"`, `"codec":"h265"`} {
		if !strings.Contains(body, want) {
			t.Errorf("GET missing %s in: %s", want, body)
		}
	}
}

func TestStreamsAPI_PostRequiresUpstream(t *testing.T) {
	r := newStreamRouter()
	code, body := r.handle("POST", "/api/streams", `{"stream_id":"x"}`)
	if code != http.StatusBadRequest {
		t.Errorf("expected 400 missing upstream, got %d: %s", code, body)
	}
}

func TestStreamsAPI_PatchUpdatesUpstream(t *testing.T) {
	// Repointing a stream from one composer to another is a supported
	// operation (drives the multi-encode flexibility headline).
	r := newStreamRouter()
	r.handle("POST", "/api/streams", sampleStreamJSON)
	code, body := r.handle("PATCH", "/api/streams/archive", `{"upstream":"source:cam-host"}`)
	if code != http.StatusOK {
		t.Fatalf("PATCH = %d: %s", code, body)
	}
	if !strings.Contains(body, `"upstream":"source:cam-host"`) {
		t.Errorf("PATCH did not update upstream: %s", body)
	}
}

func TestStreamsAPI_PatchUpdatesEncoderCodec(t *testing.T) {
	r := newStreamRouter()
	r.handle("POST", "/api/streams", sampleStreamJSON)
	code, body := r.handle("PATCH", "/api/streams/archive", `{"encoder":{"codec":"h264"}}`)
	if code != http.StatusOK {
		t.Fatalf("PATCH = %d: %s", code, body)
	}
	if !strings.Contains(body, `"codec":"h264"`) {
		t.Errorf("PATCH did not update codec: %s", body)
	}
}

func TestStreamsAPI_DeleteThenGetIs404(t *testing.T) {
	r := newStreamRouter()
	r.handle("POST", "/api/streams", sampleStreamJSON)
	code, _ := r.handle("DELETE", "/api/streams/archive", "")
	if code != http.StatusNoContent {
		t.Errorf("DELETE = %d, want 204", code)
	}
	code, _ = r.handle("GET", "/api/streams/archive", "")
	if code != http.StatusNotFound {
		t.Errorf("post-DELETE GET = %d, want 404", code)
	}
}

func TestStreamsAPI_ListReturnsAll(t *testing.T) {
	r := newStreamRouter()
	r.handle("POST", "/api/streams", sampleStreamJSON)
	second := strings.Replace(sampleStreamJSON, `"archive"`, `"low-latency"`, 1)
	r.handle("POST", "/api/streams", second)
	code, body := r.handle("GET", "/api/streams", "")
	if code != http.StatusOK {
		t.Fatalf("LIST = %d: %s", code, body)
	}
	for _, want := range []string{`"archive"`, `"low-latency"`} {
		if !strings.Contains(body, want) {
			t.Errorf("LIST missing %s: %s", want, body)
		}
	}
}
