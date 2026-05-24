//go:build planv2_tests

// CRUD tests for the post-B6 /api/composers endpoints, plus the
// sub-resources /layout and /inputs/{ref}/effect. Mocks the
// ComposerService interface defined by B9.
package api

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
)

// ComposerPlan / LayoutSlotPlan / InputPlan / EffectPlan mirror the
// API-side snake_case JSON shapes that B6 will define in
// internal/api/models/composers.go. Stubbed here so tests compile.
type ComposerPlan struct {
	ComposerID string       `json:"composer_id"`
	CanvasW    int          `json:"canvas_w"`
	CanvasH    int          `json:"canvas_h"`
	Inputs     []InputPlan  `json:"inputs"`
	Layout     []LayoutPlan `json:"layout"`
}

type InputPlan struct {
	Ref    string      `json:"ref"`
	Effect *EffectPlan `json:"effect,omitempty"`
}

type EffectPlan struct {
	Type    string    `json:"type"`
	Corners [4][2]int `json:"corners,omitempty"`
}

type LayoutPlan struct {
	Input string `json:"input"`
	X     int    `json:"x"`
	Y     int    `json:"y"`
	W     int    `json:"w"`
	H     int    `json:"h"`
}

type mockComposerAPISvc struct {
	store map[string]ComposerPlan
}

func (m *mockComposerAPISvc) Create(_ context.Context, c ComposerPlan) (ComposerPlan, error) {
	if c.ComposerID == "" {
		return c, errors.New("composer_id required")
	}
	if c.CanvasW <= 0 || c.CanvasH <= 0 {
		return c, errors.New("canvas dims must be positive")
	}
	if _, exists := m.store[c.ComposerID]; exists {
		return c, errors.New("composer exists")
	}
	m.store[c.ComposerID] = c
	return c, nil
}

func (m *mockComposerAPISvc) Get(_ context.Context, id string) (ComposerPlan, error) {
	c, ok := m.store[id]
	if !ok {
		return c, errors.New("not found")
	}
	return c, nil
}

func (m *mockComposerAPISvc) PatchLayout(_ context.Context, id string, layout []LayoutPlan) (ComposerPlan, error) {
	c, ok := m.store[id]
	if !ok {
		return c, errors.New("not found")
	}
	c.Layout = layout
	m.store[id] = c
	return c, nil
}

func (m *mockComposerAPISvc) PatchInputEffect(_ context.Context, id, ref string, effect *EffectPlan) (ComposerPlan, error) {
	c, ok := m.store[id]
	if !ok {
		return c, errors.New("not found")
	}
	found := false
	for i := range c.Inputs {
		if c.Inputs[i].Ref == ref {
			c.Inputs[i].Effect = effect
			found = true
			break
		}
	}
	if !found {
		return c, errors.New("input ref not found: " + ref)
	}
	m.store[id] = c
	return c, nil
}

func (m *mockComposerAPISvc) Delete(_ context.Context, id string) error {
	if _, ok := m.store[id]; !ok {
		return errors.New("not found")
	}
	delete(m.store, id)
	return nil
}

func (m *mockComposerAPISvc) List(_ context.Context) ([]ComposerPlan, error) {
	out := make([]ComposerPlan, 0, len(m.store))
	for _, c := range m.store {
		out = append(out, c)
	}
	return out, nil
}

type composerRouter struct {
	svc *mockComposerAPISvc
}

func newComposerRouter() *composerRouter {
	return &composerRouter{svc: &mockComposerAPISvc{store: map[string]ComposerPlan{}}}
}

func (r *composerRouter) handle(method, path, body string) (int, string) {
	switch {
	case method == "GET" && path == "/api/composers":
		l, _ := r.svc.List(context.Background())
		return http.StatusOK, mustJSON(l)
	case method == "POST" && path == "/api/composers":
		var in ComposerPlan
		mustUnmarshal(body, &in)
		out, err := r.svc.Create(context.Background(), in)
		if err != nil {
			return http.StatusBadRequest, err.Error()
		}
		return http.StatusOK, mustJSON(out)
	case method == "GET" && strings.HasPrefix(path, "/api/composers/") && !strings.Contains(path, "/layout") && !strings.Contains(path, "/inputs/"):
		id := strings.TrimPrefix(path, "/api/composers/")
		c, err := r.svc.Get(context.Background(), id)
		if err != nil {
			return http.StatusNotFound, err.Error()
		}
		return http.StatusOK, mustJSON(c)
	case method == "PATCH" && strings.HasSuffix(path, "/layout"):
		id := strings.TrimSuffix(strings.TrimPrefix(path, "/api/composers/"), "/layout")
		var in struct {
			Layout []LayoutPlan `json:"layout"`
		}
		mustUnmarshal(body, &in)
		c, err := r.svc.PatchLayout(context.Background(), id, in.Layout)
		if err != nil {
			return http.StatusNotFound, err.Error()
		}
		return http.StatusOK, mustJSON(c)
	case method == "PATCH" && strings.Contains(path, "/inputs/") && strings.HasSuffix(path, "/effect"):
		// /api/composers/{id}/inputs/{ref}/effect
		rest := strings.TrimPrefix(path, "/api/composers/")
		parts := strings.Split(rest, "/")
		if len(parts) < 4 {
			return http.StatusBadRequest, "bad path"
		}
		id := parts[0]
		ref := strings.Join(parts[2:len(parts)-1], "/")
		var in struct {
			Effect *EffectPlan `json:"effect"`
		}
		mustUnmarshal(body, &in)
		c, err := r.svc.PatchInputEffect(context.Background(), id, ref, in.Effect)
		if err != nil {
			return http.StatusNotFound, err.Error()
		}
		return http.StatusOK, mustJSON(c)
	case method == "DELETE" && strings.HasPrefix(path, "/api/composers/"):
		id := strings.TrimPrefix(path, "/api/composers/")
		if err := r.svc.Delete(context.Background(), id); err != nil {
			return http.StatusNotFound, err.Error()
		}
		return http.StatusNoContent, ""
	}
	return http.StatusNotFound, "no route"
}

const sampleComposerJSON = `{
"composer_id":"main",
"canvas_w":1920,
"canvas_h":1080,
"inputs":[{"ref":"source:hdmi0"},{"ref":"source:cam-host"}],
"layout":[{"input":"source:hdmi0","x":0,"y":0,"w":1920,"h":1080}]
}`

func TestComposersAPI_PostThenGet(t *testing.T) {
	r := newComposerRouter()
	code, body := r.handle("POST", "/api/composers", sampleComposerJSON)
	if code != http.StatusOK {
		t.Fatalf("POST = %d: %s", code, body)
	}
	code, body = r.handle("GET", "/api/composers/main", "")
	if code != http.StatusOK {
		t.Fatalf("GET = %d: %s", code, body)
	}
	for _, want := range []string{`"composer_id":"main"`, `"canvas_w":1920`, `"ref":"source:hdmi0"`} {
		if !strings.Contains(body, want) {
			t.Errorf("GET response missing %s in: %s", want, body)
		}
	}
}

func TestComposersAPI_PostRejectsZeroCanvas(t *testing.T) {
	r := newComposerRouter()
	code, body := r.handle("POST", "/api/composers", `{"composer_id":"x","canvas_w":0,"canvas_h":0}`)
	if code != http.StatusBadRequest {
		t.Errorf("expected 400 for zero canvas, got %d: %s", code, body)
	}
}

func TestComposersAPI_PatchLayout(t *testing.T) {
	r := newComposerRouter()
	r.handle("POST", "/api/composers", sampleComposerJSON)
	patch := `{"layout":[{"input":"source:hdmi0","x":100,"y":50,"w":1720,"h":980}]}`
	code, body := r.handle("PATCH", "/api/composers/main/layout", patch)
	if code != http.StatusOK {
		t.Fatalf("PATCH layout = %d: %s", code, body)
	}
	if !strings.Contains(body, `"x":100`) {
		t.Errorf("PATCH layout did not persist new x: %s", body)
	}
}

func TestComposersAPI_PatchInputEffect(t *testing.T) {
	r := newComposerRouter()
	r.handle("POST", "/api/composers", sampleComposerJSON)
	patch := `{"effect":{"type":"perspective","corners":[[40,20],[1880,30],[1900,1060],[20,1050]]}}`
	code, body := r.handle("PATCH", "/api/composers/main/inputs/source:hdmi0/effect", patch)
	if code != http.StatusOK {
		t.Fatalf("PATCH input effect = %d: %s", code, body)
	}
	if !strings.Contains(body, `"type":"perspective"`) {
		t.Errorf("PATCH effect not applied: %s", body)
	}
}

func TestComposersAPI_DeleteThenGetIs404(t *testing.T) {
	r := newComposerRouter()
	r.handle("POST", "/api/composers", sampleComposerJSON)
	code, _ := r.handle("DELETE", "/api/composers/main", "")
	if code != http.StatusNoContent {
		t.Errorf("DELETE = %d, want 204", code)
	}
	code, _ = r.handle("GET", "/api/composers/main", "")
	if code != http.StatusNotFound {
		t.Errorf("post-DELETE GET = %d, want 404", code)
	}
}
