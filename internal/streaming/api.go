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

// RegisterWebRTCAPI registers WebRTC signaling endpoints with the Huma API.
func RegisterWebRTCAPI(api huma.API, webrtcManager *WebRTCManager) {
	// POST /api/webrtc?stream=<id> - WebRTC signaling
	huma.Register(api, huma.Operation{
		OperationID: "webrtc-offer",
		Method:      http.MethodPost,
		Path:        "/api/webrtc",
		Summary:     "WebRTC signaling",
		Description: "Exchange SDP offer/answer for WebRTC streaming",
		Tags:        []string{"streaming"},
	}, func(ctx context.Context, input *WebRTCOfferInput) (*WebRTCAnswerOutput, error) {
		answer, err := webrtcManager.CreateConsumer(input.StreamID, string(input.RawBody))
		if err != nil {
			return nil, huma.Error404NotFound("stream not found or connection failed", err)
		}
		return &WebRTCAnswerOutput{
			ContentType: "application/sdp",
			Body:        []byte(answer),
		}, nil
	})

	// GET /api/streams/live - List active streams
	huma.Register(api, huma.Operation{
		OperationID: "list-live-streams",
		Method:      http.MethodGet,
		Path:        "/api/streams/live",
		Summary:     "List live streams",
		Description: "Returns a list of stream IDs that currently have active producers",
		Tags:        []string{"streaming"},
	}, func(ctx context.Context, input *struct{}) (*StreamListOutput, error) {
		streams := webrtcManager.hub.ListStreams()
		return &StreamListOutput{
			Body: struct {
				Streams []string `json:"streams" doc:"List of active stream IDs"`
			}{
				Streams: streams,
			},
		}, nil
	})
}

