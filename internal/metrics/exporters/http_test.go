package exporters

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/smazurov/videonode/internal/metrics"
)

func TestHTTPHandler(t *testing.T) {
	handler := HTTPHandler()
	if handler == nil {
		t.Fatal("expected non-nil handler")
	}

	// Set a metric so there's something to export
	metrics.SetFFmpegFPS("http-test-stream", 25.0)

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	body := w.Body.String()
	if !strings.Contains(body, "videonode_ffmpeg_fps") {
		t.Error("expected prometheus metrics in response")
	}

	metrics.DeleteFFmpegMetrics("http-test-stream")
}
