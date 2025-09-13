package api

import (
	"context"
	"net/http"
	"time"

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
			StreamID:    input.Body.StreamID,
			DeviceID:    input.Body.DeviceID,
			Codec:       string(input.Body.Codec),
			InputFormat: input.Body.InputFormat,
			Bitrate:     &input.Body.Bitrate,
			Width:       &input.Body.Width,
			Height:      &input.Body.Height,
			Framerate:   &input.Body.Framerate,
			AudioDevice: input.Body.AudioDevice,
			Options:     input.Body.Options,
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

		// Broadcast stream created event
		apiStream := s.domainToAPIStream(*stream)
		BroadcastStreamCreated(apiStream, time.Now().Format(time.RFC3339))

		return &models.StreamResponse{
			Body: apiStream,
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

		// Broadcast stream deleted event
		BroadcastStreamDeleted(input.StreamID, time.Now().Format(time.RFC3339))

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

	// Get FFmpeg command for a stream
	huma.Register(s.api, huma.Operation{
		OperationID: "get-stream-ffmpeg",
		Method:      http.MethodGet,
		Path:        "/api/streams/{stream_id}/ffmpeg",
		Summary:     "Get FFmpeg Command",
		Description: "Get the FFmpeg command for a specific stream (either auto-generated or custom)",
		Tags:        []string{"streams"},
		Errors:      []int{401, 404, 500},
		Security:    withAuth(),
	}, func(ctx context.Context, input *struct {
		StreamID        string `path:"stream_id" minLength:"1" maxLength:"50" pattern:"^[a-zA-Z0-9_-]+$" example:"stream-001" doc:"Stream identifier"`
		EncoderOverride string `query:"override" example:"h264_vaapi" doc:"Override the auto-selected encoder (e.g., h264_vaapi, h265_nvenc)"`
	}) (*models.FFmpegCommandResponse, error) {
		command, isCustom, err := s.streamService.GetFFmpegCommand(ctx, input.StreamID, input.EncoderOverride)
		if err != nil {
			return nil, s.mapStreamError(err)
		}

		return &models.FFmpegCommandResponse{
			Body: models.FFmpegCommandData{
				StreamID: input.StreamID,
				Command:  command,
				IsCustom: isCustom,
			},
		}, nil
	})

	// Set custom FFmpeg command for a stream
	huma.Register(s.api, huma.Operation{
		OperationID: "set-stream-ffmpeg",
		Method:      http.MethodPut,
		Path:        "/api/streams/{stream_id}/ffmpeg",
		Summary:     "Set Custom FFmpeg Command",
		Description: "Set a custom FFmpeg command for a specific stream, overriding auto-generation",
		Tags:        []string{"streams"},
		Errors:      []int{400, 401, 404, 500},
		Security:    withAuth(),
	}, func(ctx context.Context, input *struct {
		StreamID string `path:"stream_id" minLength:"1" maxLength:"50" pattern:"^[a-zA-Z0-9_-]+$" example:"stream-001" doc:"Stream identifier"`
		Body     struct {
			Command string `json:"command" minLength:"1" example:"ffmpeg -f v4l2 -i /dev/video0 ..." doc:"Custom FFmpeg command to use"`
		}
	}) (*models.FFmpegCommandResponse, error) {
		err := s.streamService.SetCustomFFmpegCommand(ctx, input.StreamID, input.Body.Command)
		if err != nil {
			return nil, s.mapStreamError(err)
		}

		// Return the updated command
		command, isCustom, err := s.streamService.GetFFmpegCommand(ctx, input.StreamID, "")
		if err != nil {
			return nil, s.mapStreamError(err)
		}

		return &models.FFmpegCommandResponse{
			Body: models.FFmpegCommandData{
				StreamID: input.StreamID,
				Command:  command,
				IsCustom: isCustom,
			},
		}, nil
	})

	// Clear custom FFmpeg command for a stream
	huma.Register(s.api, huma.Operation{
		OperationID: "clear-stream-ffmpeg",
		Method:      http.MethodDelete,
		Path:        "/api/streams/{stream_id}/ffmpeg",
		Summary:     "Clear Custom FFmpeg Command",
		Description: "Remove the custom FFmpeg command for a stream, reverting to auto-generation",
		Tags:        []string{"streams"},
		Errors:      []int{401, 404, 500},
		Security:    withAuth(),
	}, func(ctx context.Context, input *struct {
		StreamID string `path:"stream_id" minLength:"1" maxLength:"50" pattern:"^[a-zA-Z0-9_-]+$" example:"stream-001" doc:"Stream identifier"`
	}) (*struct{}, error) {
		err := s.streamService.ClearCustomFFmpegCommand(ctx, input.StreamID)
		if err != nil {
			return nil, s.mapStreamError(err)
		}

		return &struct{}{}, nil
	})

	// Reload streams from configuration
	huma.Register(s.api, huma.Operation{
		OperationID: "reload-streams",
		Method:      http.MethodPost,
		Path:        "/api/streams/reload",
		Summary:     "Reload Streams Configuration",
		Description: "Reload all streams from streams.toml and sync with MediaMTX. This will add/update/remove streams based on the current configuration file.",
		Tags:        []string{"streams"},
		Errors:      []int{401, 500},
		Security:    withAuth(),
	}, func(ctx context.Context, input *struct{}) (*models.ReloadResponse, error) {
		// Reload from config file
		err := s.streamService.LoadStreamsFromConfig()
		if err != nil {
			return nil, huma.Error500InternalServerError("failed to reload streams", err)
		}

		// Get current stream count
		streams, err := s.streamService.ListStreams(ctx)
		if err != nil {
			return nil, huma.Error500InternalServerError("failed to list streams", err)
		}

		return &models.ReloadResponse{
			Body: struct {
				Message string `json:"message" doc:"Operation result message"`
				Count   int    `json:"count" doc:"Number of streams synced"`
			}{
				Message: "Streams reloaded and synced with MediaMTX",
				Count:   len(streams),
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
		Bitrate:   stream.Bitrate,
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
