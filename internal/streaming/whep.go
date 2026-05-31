package streaming

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
)

// WHEPOfferInput is the request for a WHEP resource creation.
type WHEPOfferInput struct {
	Stream  string `path:"stream" doc:"Stream ID to connect to"`
	RawBody []byte `contentType:"application/sdp" doc:"SDP offer from the WHEP client"`
}

// WHEPAnswerOutput is the 201 response carrying the SDP answer and the
// resource Location used for later teardown.
type WHEPAnswerOutput struct {
	Location    string `header:"Location"`
	ContentType string `header:"Content-Type"`
	Body        []byte
}

// WHEPDeleteInput addresses a WHEP resource for teardown.
type WHEPDeleteInput struct {
	Stream  string `path:"stream" doc:"Stream ID"`
	Session string `path:"session" doc:"Session ID returned in the Location header"`
}

// registerWHEP wires the WHEP egress endpoints. No auth is required, matching
// the existing /api/webrtc signaling endpoint.
func registerWHEP(api huma.API, webrtcManager *WebRTCManager) {
	huma.Register(api, huma.Operation{
		OperationID:   "whep-offer",
		Method:        http.MethodPost,
		Path:          "/whep/{stream}",
		Summary:       "WHEP egress",
		Description:   "WebRTC-HTTP Egress Protocol: POST an SDP offer, receive an SDP answer with a resource Location.",
		Tags:          []string{"streaming"},
		DefaultStatus: http.StatusCreated,
		Errors:        []int{404},
	}, func(_ context.Context, input *WHEPOfferInput) (*WHEPAnswerOutput, error) {
		sessionID, answer, err := webrtcManager.CreateConsumer(input.Stream, string(input.RawBody))
		if err != nil {
			return nil, huma.Error404NotFound("stream not found or connection failed", err)
		}
		return &WHEPAnswerOutput{
			Location:    "/whep/" + input.Stream + "/" + sessionID,
			ContentType: "application/sdp",
			Body:        []byte(answer),
		}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID:   "whep-delete",
		Method:        http.MethodDelete,
		Path:          "/whep/{stream}/{session}",
		Summary:       "WHEP teardown",
		Description:   "Tear down a WHEP session created via POST /whep/{stream}.",
		Tags:          []string{"streaming"},
		DefaultStatus: http.StatusNoContent,
		Errors:        []int{404},
	}, func(_ context.Context, input *WHEPDeleteInput) (*struct{}, error) {
		if !webrtcManager.DisconnectPeer(input.Session) {
			return nil, huma.Error404NotFound("session not found")
		}
		return &struct{}{}, nil
	})
}
