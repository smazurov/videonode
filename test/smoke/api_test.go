//go:build smoke

package smoke

import (
	"bytes"
	"net/http"
	"strings"
	"testing"
)

func TestHealth(t *testing.T) {
	req := newReq(t, http.MethodGet, "/api/health", nil)
	body := doExpect(t, req, http.StatusOK)
	var hd struct {
		Status string `json:"status"`
	}
	decodeJSON(t, body, &hd)
	if hd.Status != "ok" {
		t.Fatalf("/api/health: want status=ok, got %q\nbody: %s", hd.Status, body)
	}
}

func TestVersion(t *testing.T) {
	req := newReq(t, http.MethodGet, "/api/update/version", nil)
	body := doExpect(t, req, http.StatusOK)
	var vd struct {
		Version string `json:"version"`
	}
	decodeJSON(t, body, &vd)
	if vd.Version == "" {
		t.Fatalf("/api/update/version: empty version field\nbody: %s", body)
	}
	t.Logf("videonode version: %s", vd.Version)
}

func TestMetricsRequiresAuth(t *testing.T) {
	req := newReq(t, http.MethodGet, "/api/metrics", nil)
	resp, err := httpClient().Do(req)
	if err != nil {
		t.Fatalf("/api/metrics no-auth: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("/api/metrics: expected 401 without auth, got %d", resp.StatusCode)
	}
}

func TestMetrics(t *testing.T) {
	// /api/metrics returns a JSON array of metric records (the Huma-exposed
	// view). The raw Prometheus text format lives under /metrics on the
	// underlying mux, not on the Huma API.
	req := newAuthReq(t, http.MethodGet, "/api/metrics", nil)
	body := doExpect(t, req, http.StatusOK)
	if !bytes.Contains(body, []byte(`"name"`)) {
		t.Fatalf("/api/metrics: body does not look like a JSON metrics array\nbody (first 200 bytes): %s",
			body[:min(200, len(body))])
	}
}

func TestPrometheusMetrics(t *testing.T) {
	// /metrics is the raw Prometheus text endpoint registered directly on
	// the HTTP mux (no auth, not under Huma).
	req := newReq(t, http.MethodGet, "/metrics", nil)
	body := doExpect(t, req, http.StatusOK)
	if !bytes.Contains(body, []byte("# HELP")) {
		t.Fatalf("/metrics: body does not look like Prometheus text format\nbody (first 200 bytes): %s",
			body[:min(200, len(body))])
	}
}

func TestListStreams(t *testing.T) {
	req := newAuthReq(t, http.MethodGet, "/api/streams", nil)
	body := doExpect(t, req, http.StatusOK)
	if !bytes.Contains(body, []byte(`"smoke-pipeline"`)) {
		t.Fatalf("/api/streams: bootstrap stream smoke-pipeline missing\nbody: %s", body)
	}
}

func TestListEncoders(t *testing.T) {
	req := newAuthReq(t, http.MethodGet, "/api/encoders", nil)
	body := doExpect(t, req, http.StatusOK)
	if !bytes.Contains(body, []byte(`"video_encoders"`)) {
		t.Fatalf("/api/encoders: missing video_encoders field\nbody: %s", body)
	}
}

func TestListDevices(t *testing.T) {
	req := newAuthReq(t, http.MethodGet, "/api/devices", nil)
	doExpect(t, req, http.StatusOK)
}

func TestStreamCRUD(t *testing.T) {
	const id = "smoke-crud"

	// Create a canvas stream — canvas streams don't need a real V4L2
	// device, so we can exercise the full CRUD plumbing without depending
	// on host hardware. smoke-source is the sacrificial bootstrap stream
	// that exists solely so the canvas has a valid source reference.
	createBody := map[string]any{
		"stream_id": id,
		"codec":     "h264",
		"bitrate":   2.0,
		"canvas": map[string]any{
			"width":          1920,
			"height":         1080,
			"fps":            "30",
			"source_streams": []string{"smoke-source"},
		},
	}
	req := newAuthReq(t, http.MethodPost, "/api/streams", createBody)
	body := doExpect(t, req, http.StatusOK)
	if !strings.Contains(string(body), `"stream_id":"`+id+`"`) {
		t.Fatalf("POST /api/streams: response missing stream_id %q\nbody: %s", id, body)
	}

	req = newAuthReq(t, http.MethodGet, "/api/streams/"+id, nil)
	doExpect(t, req, http.StatusOK)

	patchBody := map[string]any{"framerate": 60}
	req = newAuthReq(t, http.MethodPatch, "/api/streams/"+id, patchBody)
	doExpect(t, req, http.StatusOK)

	req = newAuthReq(t, http.MethodDelete, "/api/streams/"+id, nil)
	resp, err := httpClient().Do(req)
	if err != nil {
		t.Fatalf("DELETE /api/streams/%s: %v", id, err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		t.Fatalf("DELETE /api/streams/%s: got %d, want 200 or 204", id, resp.StatusCode)
	}

	req = newAuthReq(t, http.MethodGet, "/api/streams/"+id, nil)
	resp, err = httpClient().Do(req)
	if err != nil {
		t.Fatalf("post-delete GET: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("post-delete GET /api/streams/%s: got %d, want 404", id, resp.StatusCode)
	}
}
