package recording

import (
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/danielgtaylor/huma/v2/humatest"
	"github.com/smazurov/videonode/internal/streaming"
)

type mockSourceProvider struct {
	jpeg []byte
	err  error
}

func (m *mockSourceProvider) CaptureSourceSnapshot(_ string) ([]byte, error) {
	return m.jpeg, m.err
}

// nilStreamProvider returns no streams — sufficient for the "stream not
// found" path test without spinning up an RTSP session.
type nilStreamProvider struct{}

func (nilStreamProvider) GetStream(string) *streaming.Stream { return nil }
func (nilStreamProvider) HasStream(string) bool              { return false }
func (nilStreamProvider) ListStreams() []string              { return nil }
func (nilStreamProvider) GetStreamReaderCount(string) int    { return 0 }

func TestSourceSnapshotEndpoint(t *testing.T) {
	setupTestLogging(t)

	tests := []struct {
		name       string
		provider   SourceSnapshotProvider
		sourceID   string
		wantStatus int
		wantURL    string // substring expected in 200 body
	}{
		{
			name:       "captures and returns url",
			provider:   &mockSourceProvider{jpeg: []byte("jpegdata")},
			sourceID:   "hdmi-slides",
			wantStatus: http.StatusOK,
			wantURL:    "/api/snapshots/sources/hdmi-slides/",
		},
		{
			name:       "missing source maps to 404",
			provider:   &mockSourceProvider{err: ErrSourceNotFound},
			sourceID:   "ghost",
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "provider error maps to 500",
			provider:   &mockSourceProvider{err: errors.New("rpc broke")},
			sourceID:   "hdmi-slides",
			wantStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, api := humatest.New(t)
			mux := http.NewServeMux()
			RegisterAPI(api, mux, nilStreamProvider{}, tt.provider, t.TempDir())

			resp := api.Post("/api/sources/" + tt.sourceID + "/snapshot")
			if resp.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d (body=%s)", resp.Code, tt.wantStatus, resp.Body.String())
			}
			if tt.wantStatus == http.StatusOK && !strings.Contains(resp.Body.String(), tt.wantURL) {
				t.Errorf("body %q missing expected URL substring %q", resp.Body.String(), tt.wantURL)
			}
		})
	}
}

func TestStreamSnapshotEndpoint_NotFound(t *testing.T) {
	setupTestLogging(t)

	_, api := humatest.New(t)
	mux := http.NewServeMux()
	RegisterAPI(api, mux, nilStreamProvider{}, &mockSourceProvider{}, t.TempDir())

	resp := api.Post("/api/streams/nonexistent/snapshot")
	if resp.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 (body=%s)", resp.Code, resp.Body.String())
	}
}

func TestRegisterAPI_NilProvidersSkipRoutes(t *testing.T) {
	setupTestLogging(t)

	_, api := humatest.New(t)
	mux := http.NewServeMux()
	// Both providers nil → neither POST route registers, both should 404.
	RegisterAPI(api, mux, nil, nil, t.TempDir())

	if resp := api.Post("/api/sources/x/snapshot"); resp.Code != http.StatusNotFound {
		t.Errorf("expected 404 for unregistered source route, got %d", resp.Code)
	}
	if resp := api.Post("/api/streams/x/snapshot"); resp.Code != http.StatusNotFound {
		t.Errorf("expected 404 for unregistered stream route, got %d", resp.Code)
	}
}
