// Package recording provides stream snapshot capture and serving.
package recording

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/smazurov/videonode/internal/streaming"
)

const snapshotsURLPrefix = "/api/snapshots/"

// RawSnapshotProvider captures JPEG snapshots from the raw vision pipe.
type RawSnapshotProvider interface {
	CaptureRawSnapshot(streamID string) ([]byte, error)
}

// ErrSnapshotNotSupported is returned by RawSnapshotProvider.CaptureRawSnapshot
// when the target stream isn't snapshottable (e.g. a canvas). The API maps this
// to 400 without falling through to the RTSP keyframe path.
var ErrSnapshotNotSupported = errors.New("snapshot not supported for this stream")

// SnapshotInput is the request for capturing a stream snapshot.
type SnapshotInput struct {
	StreamID string `path:"stream_id" required:"true" doc:"Stream ID to capture snapshot from"`
}

// SnapshotOutput is the response containing the snapshot URL.
type SnapshotOutput struct {
	Body struct {
		URL string `json:"url" example:"/api/snapshots/test.jpg" doc:"URL path to the snapshot image"`
	}
}

// RegisterAPI registers recording endpoints with the Huma API and mounts
// the static file server for serving snapshot images.
func RegisterAPI(api huma.API, mux *http.ServeMux, streams streaming.StreamProvider, rawProvider RawSnapshotProvider, recordingDir string) {
	// Serve snapshot files statically (GET-only to avoid conflict with OPTIONS / CORS handler)
	// Files are organized as <recordingDir>/<streamID>/<timestamp>.jpg
	fs := http.StripPrefix(snapshotsURLPrefix, http.FileServer(http.Dir(recordingDir)))
	mux.Handle("GET "+snapshotsURLPrefix, fs)

	// POST /api/streams/{stream_id}/snapshot - Capture a new snapshot
	huma.Register(api, huma.Operation{
		OperationID: "capture-snapshot",
		Method:      http.MethodPost,
		Path:        "/api/streams/{stream_id}/snapshot",
		Summary:     "Capture stream snapshot",
		Description: "Captures a JPEG snapshot from a running video stream and returns the URL",
		Tags:        []string{"streaming"},
		Security:    withAuth(),
		Errors:      []int{400, 404, 500, 504},
	}, func(_ context.Context, input *SnapshotInput) (*SnapshotOutput, error) {
		// Try raw snapshot from native producer or vision pipe first.
		if rawProvider != nil {
			jpeg, err := rawProvider.CaptureRawSnapshot(input.StreamID)
			if err == nil {
				relPath, writeErr := writeSnapshotJPEG(jpeg, input.StreamID, recordingDir)
				if writeErr != nil {
					return nil, huma.Error500InternalServerError("failed to write snapshot: " + writeErr.Error())
				}
				resp := &SnapshotOutput{}
				resp.Body.URL = snapshotsURLPrefix + relPath
				return resp, nil
			}
			if errors.Is(err, ErrSnapshotNotSupported) {
				return nil, huma.Error400BadRequest(err.Error())
			}
		}

		// Fall back to RTSP-based snapshot (for streams without vision)
		stream := streams.GetStream(input.StreamID)
		if stream == nil {
			return nil, huma.Error404NotFound("stream not found or not active")
		}

		relPath, err := Snapshot(stream, recordingDir, snapshotTimeout)
		if err != nil {
			if errors.Is(err, ErrTimeout) {
				return nil, huma.Error504GatewayTimeout("timeout waiting for keyframe")
			}
			if errors.Is(err, ErrNoVideoTrack) {
				return nil, huma.Error400BadRequest("stream has no video track")
			}
			return nil, huma.Error500InternalServerError("snapshot capture failed: " + err.Error())
		}

		resp := &SnapshotOutput{}
		resp.Body.URL = snapshotsURLPrefix + relPath
		return resp, nil
	})
}

// writeSnapshotJPEG writes JPEG data to disk and returns the relative path.
func writeSnapshotJPEG(jpeg []byte, streamID, baseDir string) (string, error) {
	streamDir := filepath.Join(baseDir, streamID)
	if err := os.MkdirAll(streamDir, 0o755); err != nil {
		return "", fmt.Errorf("create snapshot dir: %w", err)
	}
	filename := time.Now().Format("20060102_150405") + ".jpg"
	absPath := filepath.Join(streamDir, filename)
	if err := os.WriteFile(absPath, jpeg, 0o644); err != nil {
		return "", fmt.Errorf("write snapshot: %w", err)
	}
	return filepath.Join(streamID, filename), nil
}

func withAuth() []map[string][]string {
	return []map[string][]string{
		{"basicAuth": {}},
	}
}
