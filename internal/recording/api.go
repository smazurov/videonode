// Package recording provides stream snapshot capture and serving.
package recording

import (
	"context"
	"errors"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/smazurov/videonode/internal/streaming"
)

const snapshotsURLPrefix = "/api/snapshots/"

// SourceSnapshotProvider captures raw NV12-derived JPEG snapshots straight
// from a source producer via the pipelinectl Snapshot RPC. Stream-encoded
// snapshots go through the RTSP keyframe path instead.
type SourceSnapshotProvider interface {
	CaptureSourceSnapshot(sourceID string) ([]byte, error)
}

// ErrSourceNotFound is returned by SourceSnapshotProvider implementations
// when the requested source_id does not exist. The API maps this to 404.
var ErrSourceNotFound = errors.New("source not found")

// SourceSnapshotInput is the request for capturing a raw source snapshot.
type SourceSnapshotInput struct {
	SourceID string `path:"source_id" required:"true" doc:"Source ID to capture snapshot from"`
}

// StreamSnapshotInput is the request for capturing an encoded stream snapshot.
type StreamSnapshotInput struct {
	StreamID string `path:"stream_id" required:"true" doc:"Stream ID to capture snapshot from"`
}

// SnapshotOutput is the response containing the snapshot URL.
type SnapshotOutput struct {
	Body struct {
		URL string `json:"url" example:"/api/snapshots/streams/test/20260404_005015.jpg" doc:"URL path to the snapshot image"`
	}
}

// RegisterAPI registers recording endpoints with the Huma API and mounts
// the static file server for serving snapshot images. Snapshots live at
// <recordingDir>/<kind>/<id>/<timestamp>.jpg where kind is "sources" or
// "streams".
func RegisterAPI(api huma.API, mux *http.ServeMux, streams streaming.StreamProvider, sourceProvider SourceSnapshotProvider, recordingDir string) {
	// Serve snapshot files statically (GET-only to avoid conflict with OPTIONS / CORS handler).
	fs := http.StripPrefix(snapshotsURLPrefix, http.FileServer(http.Dir(recordingDir)))
	mux.Handle("GET "+snapshotsURLPrefix, fs)

	registerSourceSnapshotRoute(api, sourceProvider, recordingDir)
	registerStreamSnapshotRoute(api, streams, recordingDir)
}

func registerSourceSnapshotRoute(api huma.API, sourceProvider SourceSnapshotProvider, recordingDir string) {
	if sourceProvider == nil {
		return
	}
	huma.Register(api, huma.Operation{
		OperationID: "capture-source-snapshot",
		Method:      http.MethodPost,
		Path:        "/api/sources/{source_id}/snapshot",
		Summary:     "Capture source snapshot",
		Description: "Captures a raw NV12-derived JPEG snapshot from a running source producer.",
		Tags:        []string{"sources"},
		Security:    withAuth(),
		Errors:      []int{400, 404, 500, 504},
	}, func(_ context.Context, input *SourceSnapshotInput) (*SnapshotOutput, error) {
		jpeg, err := sourceProvider.CaptureSourceSnapshot(input.SourceID)
		if err != nil {
			if errors.Is(err, ErrSourceNotFound) {
				return nil, huma.Error404NotFound(err.Error())
			}
			return nil, huma.Error500InternalServerError("source snapshot failed: " + err.Error())
		}
		relPath, err := writeSnapshotFile(jpeg, SnapshotKindSource, input.SourceID, recordingDir)
		if err != nil {
			return nil, huma.Error500InternalServerError("failed to write snapshot: " + err.Error())
		}
		resp := &SnapshotOutput{}
		resp.Body.URL = snapshotsURLPrefix + relPath
		return resp, nil
	})
}

func registerStreamSnapshotRoute(api huma.API, streams streaming.StreamProvider, recordingDir string) {
	if streams == nil {
		return
	}
	huma.Register(api, huma.Operation{
		OperationID: "capture-stream-snapshot",
		Method:      http.MethodPost,
		Path:        "/api/streams/{stream_id}/snapshot",
		Summary:     "Capture stream snapshot",
		Description: "Captures a JPEG snapshot from a running encoded stream's RTSP keyframe.",
		Tags:        []string{"streaming"},
		Security:    withAuth(),
		Errors:      []int{400, 404, 500, 504},
	}, func(_ context.Context, input *StreamSnapshotInput) (*SnapshotOutput, error) {
		stream := streams.GetStream(input.StreamID)
		if stream == nil {
			return nil, huma.Error404NotFound("stream not found or not active")
		}
		relPath, err := SnapshotStream(stream, recordingDir, snapshotTimeout)
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

func withAuth() []map[string][]string {
	return []map[string][]string{
		{"basicAuth": {}},
	}
}
