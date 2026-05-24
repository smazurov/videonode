//go:build smoke && planv2_tests

// Smoke API tests against the post-rewrite endpoints: /api/sources,
// /api/composers, /api/streams. Same harness as the v1 api_test.go;
// drives full CRUD against the live server.
package smoke

import (
	"bytes"
	"net/http"
	"strings"
	"testing"
)

func TestHealthV2(t *testing.T) {
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

func TestListSourcesV2(t *testing.T) {
	req := newAuthReq(t, http.MethodGet, "/api/sources", nil)
	body := doExpect(t, req, http.StatusOK)
	if !bytes.Contains(body, []byte(`"smoke-virtual-pipeline"`)) {
		t.Fatalf("/api/sources: bootstrap source missing\nbody: %s", body)
	}
}

func TestListStreamsV2(t *testing.T) {
	req := newAuthReq(t, http.MethodGet, "/api/streams", nil)
	body := doExpect(t, req, http.StatusOK)
	if !bytes.Contains(body, []byte(`"smoke-pipeline"`)) {
		t.Fatalf("/api/streams: bootstrap stream smoke-pipeline missing\nbody: %s", body)
	}
}

func TestSourceCRUDV2(t *testing.T) {
	const id = "smoke-source-crud"
	create := map[string]any{
		"source_id": id,
		"test_mode": true,
	}
	req := newAuthReq(t, http.MethodPost, "/api/sources", create)
	body := doExpect(t, req, http.StatusOK)
	if !strings.Contains(string(body), `"source_id":"`+id+`"`) {
		t.Fatalf("POST /api/sources: response missing source_id %q\nbody: %s", id, body)
	}

	req = newAuthReq(t, http.MethodGet, "/api/sources/"+id, nil)
	doExpect(t, req, http.StatusOK)

	req = newAuthReq(t, http.MethodDelete, "/api/sources/"+id, nil)
	resp, err := httpClient().Do(req)
	if err != nil {
		t.Fatalf("DELETE /api/sources/%s: %v", id, err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		t.Fatalf("DELETE /api/sources/%s: got %d, want 200|204", id, resp.StatusCode)
	}

	req = newAuthReq(t, http.MethodGet, "/api/sources/"+id, nil)
	resp, err = httpClient().Do(req)
	if err != nil {
		t.Fatalf("post-delete GET: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("post-delete GET /api/sources/%s: got %d, want 404", id, resp.StatusCode)
	}
}

func TestComposerCRUDV2(t *testing.T) {
	// Compose against the bootstrap sources, then tear down. Single
	// input is fine — composer engages because the entity exists, not
	// because of any input-count rule (post-rewrite topology is explicit).
	const id = "smoke-composer-crud"
	create := map[string]any{
		"composer_id": id,
		"canvas_w":    1920,
		"canvas_h":    1080,
		"inputs": []map[string]any{
			{"ref": "source:smoke-virtual-source"},
		},
		"layout": []map[string]any{
			{"input": "source:smoke-virtual-source", "x": 0, "y": 0, "w": 1920, "h": 1080},
		},
	}
	req := newAuthReq(t, http.MethodPost, "/api/composers", create)
	body := doExpect(t, req, http.StatusOK)
	if !strings.Contains(string(body), `"composer_id":"`+id+`"`) {
		t.Fatalf("POST /api/composers: response missing composer_id %q\nbody: %s", id, body)
	}

	patchLayout := map[string]any{
		"layout": []map[string]any{
			{"input": "source:smoke-virtual-source", "x": 10, "y": 10, "w": 1900, "h": 1060},
		},
	}
	req = newAuthReq(t, http.MethodPatch, "/api/composers/"+id+"/layout", patchLayout)
	doExpect(t, req, http.StatusOK)

	req = newAuthReq(t, http.MethodDelete, "/api/composers/"+id, nil)
	resp, err := httpClient().Do(req)
	if err != nil {
		t.Fatalf("DELETE composer: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		t.Fatalf("DELETE /api/composers/%s: got %d, want 200|204", id, resp.StatusCode)
	}
}

func TestStreamCRUDV2(t *testing.T) {
	const id = "smoke-stream-crud"
	create := map[string]any{
		"stream_id": id,
		"upstream":  "source:smoke-virtual-source",
		"encoder": map[string]any{
			"codec":   "h264",
			"bitrate": "2M",
		},
		"publish": []map[string]any{
			{"type": "rtsp", "url": "rtsp://127.0.0.1:8554/" + id},
		},
	}
	req := newAuthReq(t, http.MethodPost, "/api/streams", create)
	body := doExpect(t, req, http.StatusOK)
	if !strings.Contains(string(body), `"stream_id":"`+id+`"`) {
		t.Fatalf("POST /api/streams: response missing stream_id %q\nbody: %s", id, body)
	}

	req = newAuthReq(t, http.MethodGet, "/api/streams/"+id, nil)
	doExpect(t, req, http.StatusOK)

	patch := map[string]any{
		"encoder": map[string]any{"bitrate": "4M"},
	}
	req = newAuthReq(t, http.MethodPatch, "/api/streams/"+id, patch)
	doExpect(t, req, http.StatusOK)

	req = newAuthReq(t, http.MethodDelete, "/api/streams/"+id, nil)
	resp, err := httpClient().Do(req)
	if err != nil {
		t.Fatalf("DELETE stream: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		t.Fatalf("DELETE /api/streams/%s: got %d, want 200|204", id, resp.StatusCode)
	}
}
