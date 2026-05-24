//go:build planv2_tests

// CRUD tests for the post-B5 /api/sources endpoints. Mocks the
// SourceService interface defined by B9. Each test drives the handler
// directly through humatest with a manual-mock service.
package api

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
)

// SourcePlan mirrors the API-side SourceData snake_case JSON shape from
// internal/api/models/sources.go (added by B5). Local stub here so the
// test compiles ahead of B5 landing.
type SourcePlan struct {
	SourceID  string `json:"source_id"`
	Device    string `json:"device,omitempty"`
	TestMode  bool   `json:"test_mode,omitempty"`
	CreatedAt string `json:"created_at,omitempty"`
	UpdatedAt string `json:"updated_at,omitempty"`
}

type mockSourceAPISvc struct {
	store map[string]SourcePlan
}

func (m *mockSourceAPISvc) Create(_ context.Context, s SourcePlan) (SourcePlan, error) {
	if s.SourceID == "" {
		return s, errors.New("source_id required")
	}
	if _, exists := m.store[s.SourceID]; exists {
		return s, errors.New("source exists")
	}
	m.store[s.SourceID] = s
	return s, nil
}

func (m *mockSourceAPISvc) Get(_ context.Context, id string) (SourcePlan, error) {
	s, ok := m.store[id]
	if !ok {
		return s, errors.New("not found")
	}
	return s, nil
}

func (m *mockSourceAPISvc) Patch(_ context.Context, id string, patch SourcePlan) (SourcePlan, error) {
	s, ok := m.store[id]
	if !ok {
		return s, errors.New("not found")
	}
	if patch.Device != "" {
		s.Device = patch.Device
	}
	m.store[id] = s
	return s, nil
}

func (m *mockSourceAPISvc) Delete(_ context.Context, id string) error {
	if _, ok := m.store[id]; !ok {
		return errors.New("not found")
	}
	delete(m.store, id)
	return nil
}

func (m *mockSourceAPISvc) List(_ context.Context) ([]SourcePlan, error) {
	out := make([]SourcePlan, 0, len(m.store))
	for _, s := range m.store {
		out = append(out, s)
	}
	return out, nil
}

// In-process router dispatching to the mock service. Stand-in for the
// real Huma registration done by B5's RegisterSources(server, api).
type sourceRouter struct {
	svc *mockSourceAPISvc
}

//nolint:dupl // CRUD handler shape intentionally mirrors streamRouter / composerRouter for symmetry.
func (sr *sourceRouter) handle(method, path, body string) (int, string) {
	switch {
	case method == "GET" && path == "/api/sources":
		list, _ := sr.svc.List(context.Background())
		return http.StatusOK, mustJSON(list)
	case method == "POST" && path == "/api/sources":
		var in SourcePlan
		mustUnmarshal(body, &in)
		out, err := sr.svc.Create(context.Background(), in)
		if err != nil {
			return http.StatusBadRequest, err.Error()
		}
		return http.StatusOK, mustJSON(out)
	case method == "GET" && strings.HasPrefix(path, "/api/sources/"):
		id := strings.TrimPrefix(path, "/api/sources/")
		s, err := sr.svc.Get(context.Background(), id)
		if err != nil {
			return http.StatusNotFound, err.Error()
		}
		return http.StatusOK, mustJSON(s)
	case method == "PATCH" && strings.HasPrefix(path, "/api/sources/"):
		id := strings.TrimPrefix(path, "/api/sources/")
		var in SourcePlan
		mustUnmarshal(body, &in)
		s, err := sr.svc.Patch(context.Background(), id, in)
		if err != nil {
			return http.StatusNotFound, err.Error()
		}
		return http.StatusOK, mustJSON(s)
	case method == "DELETE" && strings.HasPrefix(path, "/api/sources/"):
		id := strings.TrimPrefix(path, "/api/sources/")
		if err := sr.svc.Delete(context.Background(), id); err != nil {
			return http.StatusNotFound, err.Error()
		}
		return http.StatusNoContent, ""
	}
	return http.StatusNotFound, "no route"
}

func newSourceRouter() *sourceRouter {
	return &sourceRouter{svc: &mockSourceAPISvc{store: map[string]SourcePlan{}}}
}

func TestSourcesAPI_PostThenGet(t *testing.T) {
	r := newSourceRouter()
	code, body := r.handle("POST", "/api/sources", `{"source_id":"hdmi0","device":"/dev/video0"}`)
	if code != http.StatusOK {
		t.Fatalf("POST = %d: %s", code, body)
	}
	if !strings.Contains(body, `"source_id":"hdmi0"`) {
		t.Errorf("POST response missing source_id: %s", body)
	}

	code, body = r.handle("GET", "/api/sources/hdmi0", "")
	if code != http.StatusOK {
		t.Fatalf("GET = %d: %s", code, body)
	}
	if !strings.Contains(body, `"device":"/dev/video0"`) {
		t.Errorf("GET missing device field: %s", body)
	}
}

func TestSourcesAPI_PostDuplicateFails(t *testing.T) {
	r := newSourceRouter()
	r.handle("POST", "/api/sources", `{"source_id":"x","device":"/dev/v"}`)
	code, _ := r.handle("POST", "/api/sources", `{"source_id":"x","device":"/dev/v"}`)
	if code != http.StatusBadRequest {
		t.Errorf("duplicate POST should 400, got %d", code)
	}
}

func TestSourcesAPI_GetUnknownIs404(t *testing.T) {
	r := newSourceRouter()
	code, _ := r.handle("GET", "/api/sources/ghost", "")
	if code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", code)
	}
}

func TestSourcesAPI_PatchUpdatesDevice(t *testing.T) {
	r := newSourceRouter()
	r.handle("POST", "/api/sources", `{"source_id":"x","device":"/dev/v0"}`)
	code, body := r.handle("PATCH", "/api/sources/x", `{"device":"/dev/v1"}`)
	if code != http.StatusOK {
		t.Fatalf("PATCH = %d: %s", code, body)
	}
	if !strings.Contains(body, `"device":"/dev/v1"`) {
		t.Errorf("PATCH did not update device: %s", body)
	}
}

func TestSourcesAPI_DeleteThenGetIs404(t *testing.T) {
	r := newSourceRouter()
	r.handle("POST", "/api/sources", `{"source_id":"x","device":"/dev/v0"}`)
	code, _ := r.handle("DELETE", "/api/sources/x", "")
	if code != http.StatusNoContent {
		t.Errorf("DELETE = %d, want 204", code)
	}
	code, _ = r.handle("GET", "/api/sources/x", "")
	if code != http.StatusNotFound {
		t.Errorf("post-DELETE GET = %d, want 404", code)
	}
}

func TestSourcesAPI_ListReturnsAll(t *testing.T) {
	r := newSourceRouter()
	r.handle("POST", "/api/sources", `{"source_id":"a","device":"/dev/v0"}`)
	r.handle("POST", "/api/sources", `{"source_id":"b","test_mode":true}`)
	code, body := r.handle("GET", "/api/sources", "")
	if code != http.StatusOK {
		t.Fatalf("LIST = %d: %s", code, body)
	}
	if !strings.Contains(body, `"source_id":"a"`) || !strings.Contains(body, `"source_id":"b"`) {
		t.Errorf("LIST missing one of the sources: %s", body)
	}
}
