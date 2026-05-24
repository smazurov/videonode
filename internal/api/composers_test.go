package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/humatest"

	"github.com/smazurov/videonode/internal/api/models"
)

// mockComposerService is a manual mock of ComposerService for tests.
type mockComposerService struct {
	composers map[string]*models.ComposerData

	listErr    error
	getErr     error
	createErr  error
	updateErr  error
	deleteErr  error
	layoutErr  error
	effectErr  error
	lastEffect *models.EffectData
	lastLayout []models.LayoutSlotData
	lastCreate models.ComposerCreateRequestData
	lastUpdate models.ComposerUpdateRequestData
}

func newMockComposerService() *mockComposerService {
	return &mockComposerService{composers: map[string]*models.ComposerData{}}
}

func (m *mockComposerService) ListComposers(_ context.Context) ([]models.ComposerData, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}
	out := make([]models.ComposerData, 0, len(m.composers))
	for _, c := range m.composers {
		out = append(out, *c)
	}
	return out, nil
}

func (m *mockComposerService) GetComposer(_ context.Context, id string) (*models.ComposerData, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}
	c, ok := m.composers[id]
	if !ok {
		return nil, &ComposerError{Code: ComposerErrNotFound, Message: "composer not found"}
	}
	return c, nil
}

func (m *mockComposerService) CreateComposer(_ context.Context, data models.ComposerCreateRequestData) (*models.ComposerData, error) {
	m.lastCreate = data
	if m.createErr != nil {
		return nil, m.createErr
	}
	if _, dup := m.composers[data.ID]; dup {
		return nil, &ComposerError{Code: ComposerErrExists, Message: "composer exists"}
	}
	c := &models.ComposerData{
		ID:     data.ID,
		Canvas: data.Canvas,
		Inputs: data.Inputs,
		Layout: data.Layout,
	}
	m.composers[data.ID] = c
	return c, nil
}

func (m *mockComposerService) UpdateComposer(_ context.Context, id string, patch models.ComposerUpdateRequestData) (*models.ComposerData, error) {
	m.lastUpdate = patch
	if m.updateErr != nil {
		return nil, m.updateErr
	}
	c, ok := m.composers[id]
	if !ok {
		return nil, &ComposerError{Code: ComposerErrNotFound, Message: "composer not found"}
	}
	if patch.Canvas != nil {
		c.Canvas = *patch.Canvas
	}
	if patch.Inputs != nil {
		c.Inputs = patch.Inputs
	}
	if patch.Layout != nil {
		c.Layout = patch.Layout
	}
	return c, nil
}

func (m *mockComposerService) DeleteComposer(_ context.Context, id string) error {
	if m.deleteErr != nil {
		return m.deleteErr
	}
	if _, ok := m.composers[id]; !ok {
		return &ComposerError{Code: ComposerErrNotFound, Message: "composer not found"}
	}
	delete(m.composers, id)
	return nil
}

func (m *mockComposerService) ReplaceLayout(_ context.Context, id string, layout []models.LayoutSlotData) (*models.ComposerData, error) {
	m.lastLayout = layout
	if m.layoutErr != nil {
		return nil, m.layoutErr
	}
	c, found := m.composers[id]
	if !found {
		return nil, &ComposerError{Code: ComposerErrNotFound, Message: "composer not found"}
	}
	// Validate layout slots reference known inputs.
	known := map[string]struct{}{}
	for _, in := range c.Inputs {
		known[in.Ref] = struct{}{}
	}
	for _, slot := range layout {
		if _, ok := known[slot.Input]; !ok {
			return nil, &ComposerError{Code: ComposerErrInvalid, Message: "layout slot references unknown input"}
		}
	}
	c.Layout = layout
	return c, nil
}

func (m *mockComposerService) SetInputEffect(_ context.Context, id, ref string, effect *models.EffectData) (*models.ComposerData, error) {
	m.lastEffect = effect
	if m.effectErr != nil {
		return nil, m.effectErr
	}
	c, ok := m.composers[id]
	if !ok {
		return nil, &ComposerError{Code: ComposerErrNotFound, Message: "composer not found"}
	}
	for i := range c.Inputs {
		if c.Inputs[i].Ref == ref {
			c.Inputs[i].Effect = effect
			return c, nil
		}
	}
	return nil, &ComposerError{Code: ComposerErrInputNotFound, Message: "input not found"}
}

// setupComposerTest builds a test server with the composer routes registered
// against the provided mock service.
func setupComposerTest(t *testing.T, svc ComposerService) humatest.TestAPI {
	t.Helper()
	_, api := humatest.New(t, huma.DefaultConfig("test", "1.0.0"))
	server := &Server{api: api, composerService: svc}
	server.registerComposerRoutes()
	return api
}

func seedComposer(svc *mockComposerService, id string) *models.ComposerData {
	c := &models.ComposerData{
		ID:     id,
		Canvas: models.CanvasDimsData{W: 1920, H: 1080},
		Inputs: []models.ComposerInputData{
			{Ref: "source:hdmi"},
			{Ref: "source:cam"},
		},
		Layout: []models.LayoutSlotData{
			{Input: "source:hdmi", X: 0, Y: 0, W: 1920, H: 1080},
			{Input: "source:cam", X: 20, Y: 740, W: 320, H: 180},
		},
	}
	svc.composers[id] = c
	return c
}

func TestComposers_List(t *testing.T) {
	svc := newMockComposerService()
	seedComposer(svc, "main-scene")
	api := setupComposerTest(t, svc)

	resp := api.Get("/api/composers")
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body.String())
	}
	var body models.ComposerListData
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Count != 1 {
		t.Errorf("expected count=1, got %d", body.Count)
	}
	if len(body.Composers) != 1 || body.Composers[0].ID != "main-scene" {
		t.Errorf("unexpected composers: %+v", body.Composers)
	}
}

func TestComposers_Get(t *testing.T) {
	tests := []struct {
		name     string
		id       string
		wantCode int
	}{
		{"existing", "main-scene", http.StatusOK},
		{"missing", "ghost", http.StatusNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := newMockComposerService()
			seedComposer(svc, "main-scene")
			api := setupComposerTest(t, svc)

			resp := api.Get("/api/composers/" + tt.id)
			if resp.Code != tt.wantCode {
				t.Fatalf("expected %d, got %d: %s", tt.wantCode, resp.Code, resp.Body.String())
			}
		})
	}
}

func TestComposers_Create(t *testing.T) {
	svc := newMockComposerService()
	api := setupComposerTest(t, svc)

	payload := `{
        "id": "main-scene",
        "canvas": {"w": 1920, "h": 1080},
        "inputs": [{"ref": "source:hdmi"}],
        "layout": [{"input": "source:hdmi", "x": 0, "y": 0, "w": 1920, "h": 1080}]
    }`
	resp := api.Post("/api/composers", strings.NewReader(payload), "Content-Type: application/json")
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body.String())
	}
	if svc.lastCreate.ID != "main-scene" {
		t.Errorf("expected create id main-scene, got %s", svc.lastCreate.ID)
	}
	if _, ok := svc.composers["main-scene"]; !ok {
		t.Error("expected composer to be created")
	}
}

func TestComposers_Create_Duplicate(t *testing.T) {
	svc := newMockComposerService()
	seedComposer(svc, "main-scene")
	api := setupComposerTest(t, svc)

	payload := `{
        "id": "main-scene",
        "canvas": {"w": 1920, "h": 1080},
        "inputs": [{"ref": "source:hdmi"}]
    }`
	resp := api.Post("/api/composers", strings.NewReader(payload), "Content-Type: application/json")
	if resp.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", resp.Code, resp.Body.String())
	}
}

func TestComposers_Update_Canvas(t *testing.T) {
	svc := newMockComposerService()
	seedComposer(svc, "main-scene")
	api := setupComposerTest(t, svc)

	resp := api.Patch("/api/composers/main-scene",
		strings.NewReader(`{"canvas": {"w": 2560, "h": 1440}}`),
		"Content-Type: application/json")
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body.String())
	}
	got := svc.composers["main-scene"].Canvas
	if got.W != 2560 || got.H != 1440 {
		t.Errorf("expected canvas 2560x1440, got %dx%d", got.W, got.H)
	}
}

func TestComposers_Delete(t *testing.T) {
	tests := []struct {
		name     string
		setup    func(*mockComposerService)
		id       string
		wantCode int
	}{
		{
			name:     "ok",
			setup:    func(m *mockComposerService) { seedComposer(m, "main-scene") },
			id:       "main-scene",
			wantCode: http.StatusNoContent,
		},
		{
			name:     "missing",
			setup:    func(_ *mockComposerService) {},
			id:       "ghost",
			wantCode: http.StatusNotFound,
		},
		{
			name: "in-use",
			setup: func(m *mockComposerService) {
				seedComposer(m, "main-scene")
				m.deleteErr = &ComposerError{
					Code:               ComposerErrInUse,
					Message:            "composer is referenced by streams",
					ReferencingStreams: []string{"main-archive", "host-solo"},
				}
			},
			id:       "main-scene",
			wantCode: http.StatusConflict,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := newMockComposerService()
			tt.setup(svc)
			api := setupComposerTest(t, svc)

			resp := api.Delete("/api/composers/" + tt.id)
			if resp.Code != tt.wantCode {
				t.Fatalf("expected %d, got %d: %s", tt.wantCode, resp.Code, resp.Body.String())
			}
			if tt.name == "in-use" {
				var em huma.ErrorModel
				if err := json.Unmarshal(resp.Body.Bytes(), &em); err != nil {
					t.Fatalf("decode 409 body: %v", err)
				}
				if len(em.Errors) != 2 {
					t.Errorf("expected 2 referencing-stream details, got %d", len(em.Errors))
				}
			}
		})
	}
}

func TestComposers_ReplaceLayout(t *testing.T) {
	tests := []struct {
		name     string
		body     string
		wantCode int
	}{
		{
			name:     "ok",
			body:     `{"layout": [{"input": "source:hdmi", "x": 100, "y": 100, "w": 800, "h": 600}]}`,
			wantCode: http.StatusOK,
		},
		{
			name:     "unknown input",
			body:     `{"layout": [{"input": "source:nope", "x": 0, "y": 0, "w": 100, "h": 100}]}`,
			wantCode: http.StatusBadRequest,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := newMockComposerService()
			seedComposer(svc, "main-scene")
			api := setupComposerTest(t, svc)

			resp := api.Patch("/api/composers/main-scene/layout",
				strings.NewReader(tt.body),
				"Content-Type: application/json")
			if resp.Code != tt.wantCode {
				t.Fatalf("expected %d, got %d: %s", tt.wantCode, resp.Code, resp.Body.String())
			}
			if tt.wantCode == http.StatusOK {
				if len(svc.lastLayout) != 1 || svc.lastLayout[0].Input != "source:hdmi" {
					t.Errorf("unexpected layout snapshot: %+v", svc.lastLayout)
				}
			}
		})
	}
}

func TestComposers_SetInputEffect(t *testing.T) {
	tests := []struct {
		name     string
		ref      string
		body     string
		wantCode int
		wantNil  bool
		wantType string
	}{
		{
			name:     "set",
			ref:      "source:cam",
			body:     `{"effect": {"type": "perspective", "corners": [[40,20],[1880,30],[1900,1060],[20,1050]]}}`,
			wantCode: http.StatusOK,
			wantType: "perspective",
		},
		{
			name:     "clear",
			ref:      "source:cam",
			body:     `{"effect": null}`,
			wantCode: http.StatusOK,
			wantNil:  true,
		},
		{
			name:     "unknown ref",
			ref:      "source:ghost",
			body:     `{"effect": null}`,
			wantCode: http.StatusNotFound,
			wantNil:  true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := newMockComposerService()
			seedComposer(svc, "main-scene")
			api := setupComposerTest(t, svc)

			resp := api.Patch("/api/composers/main-scene/inputs/"+tt.ref+"/effect",
				strings.NewReader(tt.body),
				"Content-Type: application/json")
			if resp.Code != tt.wantCode {
				t.Fatalf("expected %d, got %d: %s", tt.wantCode, resp.Code, resp.Body.String())
			}
			if tt.wantCode != http.StatusOK {
				return
			}
			if tt.wantNil {
				if svc.lastEffect != nil {
					t.Errorf("expected effect cleared, got %+v", svc.lastEffect)
				}
			} else {
				if svc.lastEffect == nil || svc.lastEffect.Type != tt.wantType {
					t.Errorf("expected effect type %q, got %+v", tt.wantType, svc.lastEffect)
				}
			}
		})
	}
}

func TestMapComposerError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		wantCode int
	}{
		{"nil-ish typed", &ComposerError{Code: ComposerErrInternal, Message: "boom"}, http.StatusInternalServerError},
		{"not found", &ComposerError{Code: ComposerErrNotFound, Message: "x"}, http.StatusNotFound},
		{"input not found", &ComposerError{Code: ComposerErrInputNotFound, Message: "x"}, http.StatusNotFound},
		{"exists", &ComposerError{Code: ComposerErrExists, Message: "x"}, http.StatusConflict},
		{"invalid", &ComposerError{Code: ComposerErrInvalid, Message: "x"}, http.StatusBadRequest},
		{"in use", &ComposerError{Code: ComposerErrInUse, Message: "x", ReferencingStreams: []string{"a"}}, http.StatusConflict},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mapComposerError(tt.err)
			var status int
			var em *huma.ErrorModel
			var se huma.StatusError
			switch {
			case errors.As(got, &em):
				status = em.Status
			case errors.As(got, &se):
				status = se.GetStatus()
			default:
				t.Fatalf("unexpected error type %T", got)
			}
			if status != tt.wantCode {
				t.Errorf("expected status %d, got %d", tt.wantCode, status)
			}
		})
	}
}
