package api

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/smazurov/videonode/internal/api/models"
	"github.com/smazurov/videonode/internal/streams"
)

// registerStreamRoutes registers all stream-related endpoints
func (s *Server) registerStreamRoutes() {
	// List active streams
	huma.Register(s.api, huma.Operation{
		OperationID: "list-streams",
		Method:      http.MethodGet,
		Path:        "/api/streams",
		Summary:     "List Active Streams",
		Description: "Get a list of all currently active video streams",
		Tags:        []string{"streams"},
		Errors:      []int{401, 500},
		Security:    withAuth(),
	}, func(ctx context.Context, input *struct{}) (*models.StreamListResponse, error) {
		streams, err := s.streamService.ListStreams(ctx)
		if err != nil {
			return nil, s.mapStreamError(err)
		}

		// Convert domain streams to API response
		apiStreams := make([]models.StreamData, len(streams))
		for i, stream := range streams {
			apiStreams[i] = s.domainToAPIStream(stream)
		}

		return &models.StreamListResponse{
			Body: models.StreamListData{
				Streams: apiStreams,
				Count:   len(apiStreams),
			},
		}, nil
	})

	// Create new stream
	huma.Register(s.api, huma.Operation{
		OperationID: "create-stream",
		Method:      http.MethodPost,
		Path:        "/api/streams",
		Summary:     "Create Stream",
		Description: "Create a new video stream from a device using stable device ID",
		Tags:        []string{"streams"},
		Errors:      []int{400, 401, 404, 409, 500},
		Security:    withAuth(),
	}, func(ctx context.Context, input *models.StreamRequest) (*models.StreamResponse, error) {
		// Convert API request to domain parameters
		params := streams.StreamCreateParams{
			StreamID:  input.Body.StreamID,
			DeviceID:  input.Body.DeviceID,
			Codec:     string(input.Body.Codec),
			Bitrate:   &input.Body.Bitrate,
			Width:     &input.Body.Width,
			Height:    &input.Body.Height,
			Framerate: &input.Body.Framerate,
		}

		// Handle optional fields properly
		if input.Body.Bitrate == 0 {
			params.Bitrate = nil
		}
		if input.Body.Width == 0 {
			params.Width = nil
		}
		if input.Body.Height == 0 {
			params.Height = nil
		}
		if input.Body.Framerate == 0 {
			params.Framerate = nil
		}

		stream, err := s.streamService.CreateStream(ctx, params)
		if err != nil {
			return nil, s.mapStreamError(err)
		}

		return &models.StreamResponse{
			Body: s.domainToAPIStream(*stream),
		}, nil
	})

	// Delete stream
	huma.Register(s.api, huma.Operation{
		OperationID: "delete-stream",
		Method:      http.MethodDelete,
		Path:        "/api/streams/{stream_id}",
		Summary:     "Delete Stream",
		Description: "Delete an active video stream",
		Tags:        []string{"streams"},
		Errors:      []int{401, 404, 500},
		Security:    withAuth(),
	}, func(ctx context.Context, input *struct {
		StreamID string `path:"stream_id" example:"stream-001" doc:"Stream identifier"`
	}) (*struct{}, error) {
		err := s.streamService.DeleteStream(ctx, input.StreamID)
		if err != nil {
			return nil, s.mapStreamError(err)
		}

		return &struct{}{}, nil
	})

	// Get specific stream
	huma.Register(s.api, huma.Operation{
		OperationID: "get-stream",
		Method:      http.MethodGet,
		Path:        "/api/streams/{stream_id}",
		Summary:     "Get Stream",
		Description: "Get details of a specific stream",
		Tags:        []string{"streams"},
		Errors:      []int{401, 404, 500},
		Security:    withAuth(),
	}, func(ctx context.Context, input *struct {
		StreamID string `path:"stream_id" example:"stream-001" doc:"Stream identifier"`
	}) (*models.StreamResponse, error) {
		stream, err := s.streamService.GetStream(ctx, input.StreamID)
		if err != nil {
			return nil, s.mapStreamError(err)
		}

		return &models.StreamResponse{
			Body: s.domainToAPIStream(*stream),
		}, nil
	})

	// Get stream status
	huma.Register(s.api, huma.Operation{
		OperationID: "get-stream-status",
		Method:      http.MethodGet,
		Path:        "/api/streams/{stream_id}/status",
		Summary:     "Get Stream Status",
		Description: "Get runtime status of a specific stream",
		Tags:        []string{"streams"},
		Errors:      []int{401, 404, 500},
		Security:    withAuth(),
	}, func(ctx context.Context, input *struct {
		StreamID string `path:"stream_id" example:"stream-001" doc:"Stream identifier"`
	}) (*models.StreamStatusResponse, error) {
		status, err := s.streamService.GetStreamStatus(ctx, input.StreamID)
		if err != nil {
			return nil, s.mapStreamError(err)
		}

		return &models.StreamStatusResponse{
			Body: models.StreamStatusData{
				StreamID:  status.StreamID,
				Uptime:    status.Uptime,
				StartTime: status.StartTime,
			},
		}, nil
	})
}

// domainToAPIStream converts a domain stream to API stream data
func (s *Server) domainToAPIStream(stream streams.Stream) models.StreamData {
	return models.StreamData{
		StreamID:  stream.ID,
		DeviceID:  stream.DeviceID,
		Codec:     stream.Codec,
		Uptime:    0, // Will be calculated when needed
		StartTime: stream.StartTime,
		WebRTCURL: stream.WebRTCURL,
		RTSPURL:   stream.RTSPURL,
	}
}

// mapStreamError maps domain errors to HTTP errors
func (s *Server) mapStreamError(err error) error {
	if streamErr, ok := err.(*streams.StreamError); ok {
		switch streamErr.Code {
		case streams.ErrCodeStreamNotFound:
			return huma.Error404NotFound(streamErr.Message, err)
		case streams.ErrCodeDeviceNotFound:
			return huma.Error404NotFound(streamErr.Message, err)
		case streams.ErrCodeStreamExists:
			return huma.Error409Conflict(streamErr.Message, err)
		case streams.ErrCodeInvalidParams:
			return huma.Error400BadRequest(streamErr.Message, err)
		case streams.ErrCodeConfigError, streams.ErrCodeMediaMTXError:
			return huma.Error500InternalServerError(streamErr.Message, err)
		default:
			return huma.Error500InternalServerError("internal server error", err)
		}
	}
	return huma.Error500InternalServerError("internal server error", err)
}
