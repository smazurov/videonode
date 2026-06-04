// Package streaming provides WebRTC and RTSP streaming functionality.
package streaming

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
)

// StreamListOutput is the response for listing active streams.
type StreamListOutput struct {
	Body struct {
		Streams []string `json:"streams" doc:"List of active stream IDs"`
	}
}

// RegisterStreamingAPI registers WebRTC signaling and consumer management endpoints.
func RegisterStreamingAPI(api huma.API, webrtcManager *WebRTCManager, srtServer *SRTServer) {
	registerWHEP(api, webrtcManager)

	huma.Register(api, huma.Operation{
		OperationID: "list-live-streams",
		Method:      http.MethodGet,
		Path:        "/api/streams/live",
		Summary:     "List live streams",
		Description: "Returns a list of stream IDs that currently have active producers",
		Tags:        []string{"streaming"},
	}, func(_ context.Context, _ *struct{}) (*StreamListOutput, error) {
		streams := webrtcManager.ListStreams()
		return &StreamListOutput{
			Body: struct {
				Streams []string `json:"streams" doc:"List of active stream IDs"`
			}{
				Streams: streams,
			},
		}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "disconnect-consumer",
		Method:      http.MethodDelete,
		Path:        "/api/streams/{stream_id}/{protocol}/consumers/{client_id}",
		Summary:     "Disconnect consumer",
		Description: "Disconnect a WebRTC peer or SRT consumer by protocol and client ID",
		Tags:        []string{"streaming"},
		Errors:      []int{400, 404},
	}, func(_ context.Context, input *struct {
		StreamID string `path:"stream_id" doc:"Stream identifier"`
		Protocol string `path:"protocol" enum:"webrtc,srt" doc:"Consumer protocol"`
		ClientID string `path:"client_id" doc:"Client identifier (peer name or consumer ID)"`
	},
	) (*struct{}, error) {
		var found bool
		switch input.Protocol {
		case "webrtc":
			found = webrtcManager.DisconnectPeer(input.ClientID)
		case "srt":
			if srtServer != nil {
				found = srtServer.DisconnectConsumer(input.ClientID)
			}
		default:
			return nil, huma.Error400BadRequest("unsupported protocol: " + input.Protocol)
		}
		if !found {
			return nil, huma.Error404NotFound("consumer not found")
		}
		return &struct{}{}, nil
	})
}
