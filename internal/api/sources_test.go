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
)

// mockSourceService is a manual mock for SourceService used by the API
// tests. Behavior is controlled by the maps and the optional override
// hooks.
type mockSourceService struct {
	items       map[string]Source
	deleteErr   func(id string) error
	createErr   func(src Source) error
	updateErr   func(id string, patch SourcePatch) error
	createCalls []Source
}

func newMockSourceService(items ...Source) *mockSourceService {
	m := &mockSourceService{items: make(map[string]Source, len(items))}
	for _, it := range items {
		m.items[it.ID] = it
	}
	return m
}

func (m *mockSourceService) List(_ context.Context) ([]Source, error) {
	out := make([]Source, 0, len(m.items))
	for _, it := range m.items {
		out = append(out, it)
	}
	return out, nil
}

func (m *mockSourceService) Get(_ context.Context, id string) (*Source, error) {
	it, ok := m.items[id]
	if !ok {
		return nil, &SourceNotFoundError{SourceID: id}
	}
	return &it, nil
}

func (m *mockSourceService) Create(_ context.Context, src Source) (*Source, error) {
	m.createCalls = append(m.createCalls, src)
	if m.createErr != nil {
		if err := m.createErr(src); err != nil {
			return nil, err
		}
	}
	if _, dup := m.items[src.ID]; dup {
		return nil, &SourceExistsError{SourceID: src.ID}
	}
	src.CreatedAt = time.Unix(1700000000, 0).UTC()
	src.UpdatedAt = src.CreatedAt
	m.items[src.ID] = src
	return &src, nil
}

func (m *mockSourceService) Update(_ context.Context, id string, patch SourcePatch) (*Source, error) {
	if m.updateErr != nil {
		if err := m.updateErr(id, patch); err != nil {
			return nil, err
		}
	}
	it, ok := m.items[id]
	if !ok {
		return nil, &SourceNotFoundError{SourceID: id}
	}
	if patch.Device != nil {
		it.Device = *patch.Device
	}
	if patch.TestMode != nil {
		it.TestMode = *patch.TestMode
	}
	it.UpdatedAt = time.Unix(1700000100, 0).UTC()
	m.items[id] = it
	return &it, nil
}

func (m *mockSourceService) Delete(_ context.Context, id string) error {
	if m.deleteErr != nil {
		return m.deleteErr(id)
	}
	if _, ok := m.items[id]; !ok {
		return &SourceNotFoundError{SourceID: id}
	}
	delete(m.items, id)
	return nil
}

// newTestServer wires a humatest API around a Server with the given
// SourceService and registers the source routes.
func newTestServer(t *testing.T, svc SourceService) humatest.TestAPI {
	t.Helper()
	_, api := humatest.New(t, huma.DefaultConfig("test", "1.0.0"))
	s := &Server{api: api, sourceService: svc}
	s.registerSourceRoutes()
	return api
}

func TestListSources_EmptyAndPopulated(t *testing.T) {
	tests := []struct {
		name    string
		seed    []Source
		wantLen int
	}{
		{"empty", nil, 0},
		{"one device source", []Source{{ID: "hdmi-1", Device: "rk3588-hdmi-rx"}}, 1},
		{"mixed", []Source{
			{ID: "hdmi-1", Device: "rk3588-hdmi-rx"},
			{ID: "test", TestMode: true},
		}, 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := newMockSourceService(tt.seed...)
			api := newTestServer(t, svc)
			resp := api.Get("/api/sources")
			if resp.Code != http.StatusOK {
				t.Fatalf("status: got %d, want 200; body=%s", resp.Code, resp.Body.String())
			}
			var body models.SourceListData
			if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if body.Count != tt.wantLen || len(body.Sources) != tt.wantLen {
				t.Errorf("count: got %d, want %d", body.Count, tt.wantLen)
			}
		})
	}
}

func TestCreateSource_Roundtrip(t *testing.T) {
	svc := newMockSourceService()
	api := newTestServer(t, svc)

	resp := api.Post("/api/sources", strings.NewReader(`{"id":"hdmi-1","device":"rk3588-hdmi-rx"}`))
	if resp.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200; body=%s", resp.Code, resp.Body.String())
	}
	var got models.SourceData
	if err := json.Unmarshal(resp.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.SourceID != "hdmi-1" || got.Device != "rk3588-hdmi-rx" || got.TestMode {
		t.Errorf("source mismatch: %+v", got)
	}
	if got.CreatedAt.IsZero() {
		t.Errorf("created_at should be populated")
	}
	if len(svc.createCalls) != 1 {
		t.Errorf("Create called %d times, want 1", len(svc.createCalls))
	}
}

func TestCreateSource_DuplicateConflict(t *testing.T) {
	svc := newMockSourceService(Source{ID: "hdmi-1", Device: "rk3588-hdmi-rx"})
	api := newTestServer(t, svc)

	resp := api.Post("/api/sources", strings.NewReader(`{"id":"hdmi-1","device":"rk3588-hdmi-rx"}`))
	if resp.Code != http.StatusConflict {
		t.Fatalf("status: got %d, want 409; body=%s", resp.Code, resp.Body.String())
	}
}

func TestCreateSource_InvalidIDRejectedBySchema(t *testing.T) {
	svc := newMockSourceService()
	api := newTestServer(t, svc)

	resp := api.Post("/api/sources", strings.NewReader(`{"id":"Bad ID","device":"d"}`))
	if resp.Code != http.StatusUnprocessableEntity && resp.Code != http.StatusBadRequest {
		t.Fatalf("status: got %d, want 4xx schema reject; body=%s", resp.Code, resp.Body.String())
	}
}

func TestGetSource_NotFound(t *testing.T) {
	api := newTestServer(t, newMockSourceService())
	resp := api.Get("/api/sources/missing")
	if resp.Code != http.StatusNotFound {
		t.Fatalf("status: got %d, want 404; body=%s", resp.Code, resp.Body.String())
	}
}

func TestUpdateSource_TogglesTestMode(t *testing.T) {
	svc := newMockSourceService(Source{ID: "src-1", Device: "/dev/video0"})
	api := newTestServer(t, svc)

	resp := api.Patch("/api/sources/src-1", strings.NewReader(`{"test_mode":true,"device":""}`))
	if resp.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200; body=%s", resp.Code, resp.Body.String())
	}
	var got models.SourceData
	if err := json.Unmarshal(resp.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !got.TestMode || got.Device != "" {
		t.Errorf("expected test_mode=true and empty device; got %+v", got)
	}
}

func TestDeleteSource_RefusesWhenReferenced(t *testing.T) {
	svc := newMockSourceService(Source{ID: "src-1", Device: "/dev/video0"})
	svc.deleteErr = func(id string) error {
		return &SourceInUseError{
			SourceID: id,
			References: []models.SourceReference{
				{Kind: models.SourceReferenceKindComposer, ID: "main-scene"},
				{Kind: models.SourceReferenceKindStream, ID: "main-archive"},
			},
		}
	}
	api := newTestServer(t, svc)

	resp := api.Delete("/api/sources/src-1")
	if resp.Code != http.StatusConflict {
		t.Fatalf("status: got %d, want 409; body=%s", resp.Code, resp.Body.String())
	}
	var errBody huma.ErrorModel
	if err := json.Unmarshal(resp.Body.Bytes(), &errBody); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(errBody.Errors) != 2 {
		t.Fatalf("errors len: got %d, want 2; body=%s", len(errBody.Errors), resp.Body.String())
	}
	wantLocs := map[string]bool{"composer:main-scene": false, "stream:main-archive": false}
	for _, d := range errBody.Errors {
		if _, ok := wantLocs[d.Location]; ok {
			wantLocs[d.Location] = true
		}
	}
	for loc, seen := range wantLocs {
		if !seen {
			t.Errorf("missing reference detail for %q", loc)
		}
	}
}

func TestDeleteSource_SuccessAndNotFound(t *testing.T) {
	tests := []struct {
		name       string
		seed       []Source
		id         string
		wantStatus int
	}{
		{"success", []Source{{ID: "src-1"}}, "src-1", http.StatusNoContent},
		{"not found", nil, "missing", http.StatusNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := newTestServer(t, newMockSourceService(tt.seed...))
			resp := api.Delete("/api/sources/" + tt.id)
			if resp.Code != tt.wantStatus {
				t.Errorf("status: got %d, want %d; body=%s", resp.Code, tt.wantStatus, resp.Body.String())
			}
		})
	}
}
