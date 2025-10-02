package api

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/smazurov/videonode/internal/api/models"
	"github.com/smazurov/videonode/internal/events"
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
		params := s.convertCreateRequest(input.Body)

		stream, err := s.streamService.CreateStream(ctx, params)
		if err != nil {
			return nil, s.mapStreamError(err)
		}

		apiStream := s.domainToAPIStream(*stream)

		// Broadcast stream created event
		if s.eventBus != nil {
			s.eventBus.Publish(events.StreamCreatedEvent{
				Stream:    apiStream,
				Action:    "created",
				Timestamp: time.Now().Format(time.RFC3339),
			})
		}

		return &models.StreamResponse{
			Body: apiStream,
		}, nil
	})

	// Update stream
	huma.Register(s.api, huma.Operation{
		OperationID: "update-stream",
		Method:      http.MethodPatch,
		Path:        "/api/streams/{stream_id}",
		Summary:     "Update Stream",
		Description: "Partially update an existing video stream with new parameters",
		Tags:        []string{"streams"},
		Errors:      []int{400, 401, 404, 500},
		Security:    withAuth(),
	}, func(ctx context.Context, input *struct {
		StreamID string `path:"stream_id" example:"stream-001" doc:"Stream identifier"`
		Body     models.StreamUpdateRequestData
	}) (*models.StreamResponse, error) {
		// Convert API request to domain parameters
		params := streams.StreamUpdateParams{
			Codec:               input.Body.Codec,
			InputFormat:         input.Body.InputFormat,
			Bitrate:             input.Body.Bitrate,
			Width:               input.Body.Width,
			Height:              input.Body.Height,
			Framerate:           input.Body.Framerate,
			AudioDevice:         input.Body.AudioDevice,
			Options:             input.Body.Options,
			CustomFFmpegCommand: input.Body.CustomFFmpegCommand,
			TestMode:            input.Body.TestMode,
			Enabled:             input.Body.Enabled,
		}

		stream, err := s.streamService.UpdateStream(ctx, input.StreamID, params)
		if err != nil {
			return nil, s.mapStreamError(err)
		}

		// Broadcast stream updated event
		apiStream := s.domainToAPIStream(*stream)
		if s.eventBus != nil {
			s.eventBus.Publish(events.StreamUpdatedEvent{
				Stream:    apiStream,
				Action:    "updated",
				Timestamp: time.Now().Format(time.RFC3339),
			})
		}

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
		if s.eventBus != nil {
			s.eventBus.Publish(events.StreamDeletedEvent{
				StreamID:  input.StreamID,
				Action:    "deleted",
				Timestamp: time.Now().Format(time.RFC3339),
			})
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

}

// convertCreateRequest converts API create request to domain params
func (s *Server) convertCreateRequest(body models.StreamRequestData) streams.StreamCreateParams {
	params := streams.StreamCreateParams{
		StreamID:    body.StreamID,
		DeviceID:    body.DeviceID,
		Codec:       string(body.Codec),
		InputFormat: body.InputFormat,
		AudioDevice: body.AudioDevice,
		Options:     body.Options,
	}

	// Handle optional numeric fields - convert zero values to nil
	if body.Bitrate != 0 {
		params.Bitrate = &body.Bitrate
	}
	if body.Width != 0 {
		params.Width = &body.Width
	}
	if body.Height != 0 {
		params.Height = &body.Height
	}
	if body.Framerate != 0 {
		params.Framerate = &body.Framerate
	}

	return params
}

// domainToAPIStream converts a domain stream to API stream data with configuration
func (s *Server) domainToAPIStream(stream streams.Stream) models.StreamData {
	// Get stream specification for configuration details
	config, err := s.streamService.GetStreamSpec(context.Background(), stream.ID)

	// Format display bitrate from quality params
	displayBitrate := "2M" // Default
	if err == nil && config.FFmpeg.QualityParams != nil && config.FFmpeg.QualityParams.TargetBitrate != nil {
		displayBitrate = fmt.Sprintf("%.1fM", *config.FFmpeg.QualityParams.TargetBitrate)
	}

	// Get device ID and codec from config (not runtime state)
	deviceID := ""
	codec := ""
	if err == nil {
		deviceID = config.Device
		codec = config.FFmpeg.Codec
	}

	apiData := models.StreamData{
		StreamID:  stream.ID,
		DeviceID:  deviceID,
		Codec:     codec,
		Bitrate:   displayBitrate,
		StartTime: stream.StartTime,
		WebRTCURL: fmt.Sprintf(":8889/%s", stream.ID),
		SRTURL:    fmt.Sprintf(":8890?streamid=read:%s", stream.ID),
	}

	// Include configuration details if available
	if err == nil {
		apiData.InputFormat = config.FFmpeg.InputFormat
		apiData.Resolution = config.FFmpeg.Resolution
		apiData.Framerate = config.FFmpeg.FPS
		apiData.AudioDevice = config.FFmpeg.AudioDevice
		apiData.CustomFFmpegCmd = config.CustomFFmpegCommand
		apiData.TestMode = config.TestMode
		apiData.Enabled = stream.Enabled // Use runtime enabled state from Stream
	}

	return apiData
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
