// Package streaming provides WebRTC and RTSP streaming functionality.
package streaming

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
)

// WebRTCOfferInput is the request body for WebRTC signaling.
type WebRTCOfferInput struct {
	StreamID string `query:"stream" required:"true" doc:"Stream ID to connect to"`
	RawBody  []byte `contentType:"application/sdp" doc:"SDP offer from browser"`
}

// WebRTCAnswerOutput is the response body for WebRTC signaling.
type WebRTCAnswerOutput struct {
	ContentType string `header:"Content-Type"`
	Body        []byte
}

// StreamListOutput is the response for listing active streams.
type StreamListOutput struct {
	Body struct {
		Streams []string `json:"streams" doc:"List of active stream IDs"`
	}
}

// RegisterStreamingAPI registers WebRTC signaling and consumer management endpoints.
func RegisterStreamingAPI(api huma.API, webrtcManager *WebRTCManager, srtServer *SRTServer) {
	huma.Register(api, huma.Operation{
		OperationID: "webrtc-offer",
		Method:      http.MethodPost,
		Path:        "/api/webrtc",
		Summary:     "WebRTC signaling",
		Description: "Exchange SDP offer/answer for WebRTC streaming",
		Tags:        []string{"streaming"},
	}, func(_ context.Context, input *WebRTCOfferInput) (*WebRTCAnswerOutput, error) {
		answer, err := webrtcManager.CreateConsumer(input.StreamID, string(input.RawBody))
		if err != nil {
			return nil, huma.Error404NotFound("stream not found or connection failed", err)
		}
		return &WebRTCAnswerOutput{
			ContentType: "application/sdp",
			Body:        []byte(answer),
		}, nil
	})

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
