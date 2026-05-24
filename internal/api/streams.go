package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/smazurov/videonode/internal/api/models"
	"github.com/smazurov/videonode/internal/events"
	"github.com/smazurov/videonode/internal/streams/pipeline"
)

// registerStreamRoutes registers all stream-related endpoints.
func (s *Server) registerStreamRoutes() {
	if s.streamService == nil {
		return
	}

	huma.Register(s.api, huma.Operation{
		OperationID: "list-streams",
		Method:      http.MethodGet,
		Path:        "/api/streams",
		Summary:     "List Streams",
		Description: "List all configured video streams in slim shape",
		Tags:        []string{"streams"},
		Errors:      []int{401, 500},
		Security:    withAuth(),
	}, func(ctx context.Context, _ *struct{}) (*models.StreamListResponse, error) {
		items, err := s.streamService.List(ctx)
		if err != nil {
			return nil, s.mapStreamError(err)
		}

		apiStreams := make([]models.StreamData, len(items))
		for i, st := range items {
			apiStreams[i] = s.streamToAPI(st)
		}

		return &models.StreamListResponse{
			Body: models.StreamListData{
				Streams: apiStreams,
				Count:   len(apiStreams),
			},
		}, nil
	})

	huma.Register(s.api, huma.Operation{
		OperationID: "create-stream",
		Method:      http.MethodPost,
		Path:        "/api/streams",
		Summary:     "Create Stream",
		Description: "Create a new stream referencing a source or composer upstream",
		Tags:        []string{"streams"},
		Errors:      []int{400, 401, 404, 409, 500},
		Security:    withAuth(),
	}, func(ctx context.Context, input *models.StreamRequest) (*models.StreamResponse, error) {
		entity := streamFromCreateRequest(input.Body)

		created, err := s.streamService.Create(ctx, entity)
		if err != nil {
			return nil, s.mapStreamError(err)
		}

		apiStream := s.streamToAPI(*created)
		if s.eventBus != nil {
			s.eventBus.Publish(events.StreamCreatedEvent{
				Stream:    apiStream,
				Action:    "created",
				Timestamp: time.Now().Format(time.RFC3339),
			})
		}
		if s.streamEntity != nil {
			s.streamEntity.PublishCreated(apiStream)
		}
		return &models.StreamResponse{Body: apiStream}, nil
	})

	huma.Register(s.api, huma.Operation{
		OperationID: "update-stream",
		Method:      http.MethodPatch,
		Path:        "/api/streams/{stream_id}",
		Summary:     "Update Stream",
		Description: "Partially update a stream's slim configuration",
		Tags:        []string{"streams"},
		Errors:      []int{400, 401, 404, 500},
		Security:    withAuth(),
	}, func(ctx context.Context, input *struct {
		StreamID string `path:"stream_id" example:"stream-001" doc:"Stream identifier"`
		Body     models.StreamUpdateRequestData
	},
	) (*models.StreamResponse, error) {
		body := input.Body

		updated, err := s.streamService.Update(ctx, input.StreamID, func(st *pipeline.Stream) error {
			applyStreamPatch(st, body)
			return nil
		})
		if err != nil {
			return nil, s.mapStreamError(err)
		}

		apiStream := s.streamToAPI(*updated)
		if s.eventBus != nil {
			s.eventBus.Publish(events.StreamUpdatedEvent{
				Stream:    apiStream,
				Action:    "updated",
				Timestamp: time.Now().Format(time.RFC3339),
			})
		}
		if s.streamEntity != nil {
			s.streamEntity.PublishUpdated(apiStream)
		}
		return &models.StreamResponse{Body: apiStream}, nil
	})

	huma.Register(s.api, huma.Operation{
		OperationID: "delete-stream",
		Method:      http.MethodDelete,
		Path:        "/api/streams/{stream_id}",
		Summary:     "Delete Stream",
		Description: "Delete a stream",
		Tags:        []string{"streams"},
		Errors:      []int{401, 404, 500},
		Security:    withAuth(),
	}, func(ctx context.Context, input *struct {
		StreamID string `path:"stream_id" example:"stream-001" doc:"Stream identifier"`
	},
	) (*struct{}, error) {
		if err := s.streamService.Delete(ctx, input.StreamID); err != nil {
			return nil, s.mapStreamError(err)
		}

		if s.eventBus != nil {
			s.eventBus.Publish(events.StreamDeletedEvent{
				StreamID:  input.StreamID,
				Action:    "deleted",
				Timestamp: time.Now().Format(time.RFC3339),
			})
		}
		if s.streamEntity != nil {
			s.streamEntity.PublishDeleted(input.StreamID)
		}

		return &struct{}{}, nil
	})

	huma.Register(s.api, huma.Operation{
		OperationID: "get-stream",
		Method:      http.MethodGet,
		Path:        "/api/streams/{stream_id}",
		Summary:     "Get Stream",
		Description: "Get one stream's slim configuration",
		Tags:        []string{"streams"},
		Errors:      []int{401, 404, 500},
		Security:    withAuth(),
	}, func(ctx context.Context, input *struct {
		StreamID string `path:"stream_id" example:"stream-001" doc:"Stream identifier"`
	},
	) (*models.StreamResponse, error) {
		st, err := s.streamService.Get(ctx, input.StreamID)
		if err != nil {
			return nil, s.mapStreamError(err)
		}
		return &models.StreamResponse{Body: s.streamToAPI(*st)}, nil
	})

	huma.Register(s.api, huma.Operation{
		OperationID: "restart-stream",
		Method:      http.MethodPost,
		Path:        "/api/streams/{stream_id}/restart",
		Summary:     "Restart Stream",
		Description: "Re-apply the persisted spec to the pipeline (stop + start the encoder)",
		Tags:        []string{"streams"},
		Errors:      []int{401, 404, 500, 503},
		Security:    withAuth(),
	}, func(ctx context.Context, input *struct {
		StreamID string `path:"stream_id" minLength:"1" maxLength:"50" pattern:"^[a-zA-Z0-9_-]+$" example:"stream-001" doc:"Stream identifier"`
	},
	) (*struct{}, error) {
		if err := s.streamService.Restart(ctx, input.StreamID); err != nil {
			return nil, s.mapStreamError(err)
		}

		if st, gerr := s.streamService.Get(ctx, input.StreamID); gerr == nil {
			apiStream := s.streamToAPI(*st)
			if s.eventBus != nil {
				s.eventBus.Publish(events.StreamUpdatedEvent{
					Stream:    apiStream,
					Action:    "restarted",
					Timestamp: time.Now().Format(time.RFC3339),
				})
			}
			if s.streamEntity != nil {
				s.streamEntity.PublishUpdated(apiStream)
			}
		}

		return &struct{}{}, nil
	})
}

// streamFromCreateRequest converts the slim API create payload into a
// pipeline.Stream entity ready for the service layer. Timestamps and Name
// fallbacks are filled in by the service itself.
func streamFromCreateRequest(body models.StreamRequestData) pipeline.Stream {
	st := pipeline.Stream{
		ID:                body.StreamID,
		Name:              body.Name,
		Upstream:          body.Upstream,
		Audio:             audioFromAPI(body.Audio),
		Encoder:           encoderFromAPI(body.Encoder),
		Publish:           publishFromAPI(body.Publish),
		CustomEncoderArgs: body.CustomEncoderArgs,
	}
	return st
}

// applyStreamPatch folds a slim partial-update payload onto an existing
// pipeline.Stream in-place. Touches only fields the caller marked as set.
func applyStreamPatch(st *pipeline.Stream, body models.StreamUpdateRequestData) {
	if body.Name != nil {
		st.Name = *body.Name
	}
	if body.Upstream != nil {
		st.Upstream = *body.Upstream
	}
	if body.Encoder.Sent {
		if body.Encoder.Null {
			st.Encoder = pipeline.EncoderConfig{}
		} else {
			st.Encoder = encoderFromAPI(body.Encoder.Value)
		}
	}
	if body.Audio.Sent {
		if body.Audio.Null {
			st.Audio = pipeline.AudioConfig{}
		} else {
			st.Audio = audioFromAPI(body.Audio.Value)
		}
	}
	if body.Publish.Sent {
		if body.Publish.Null {
			st.Publish = nil
		} else {
			st.Publish = publishFromAPI(body.Publish.Value)
		}
	}
	if body.CustomEncoderArgs != nil {
		st.CustomEncoderArgs = *body.CustomEncoderArgs
	}
}

// streamToAPI converts a pipeline.Stream into the slim API view, filling
// in runtime-derived fields (RTSP/SRT URLs).
func (s *Server) streamToAPI(st pipeline.Stream) models.StreamData {
	return models.StreamData{
		StreamID:          st.ID,
		Name:              st.Name,
		Upstream:          st.Upstream,
		Audio:             audioToAPI(st.Audio),
		Encoder:           encoderToAPI(st.Encoder),
		Publish:           publishToAPI(st.Publish),
		CustomEncoderArgs: st.CustomEncoderArgs,
		// In v2 the encoder process is lazy: it spins up when a consumer
		// attaches and idles when the last consumer leaves. There is no
		// per-stream enabled toggle anymore; "true" here means "the stream
		// is configured and the daemon will start the encoder on demand".
		Enabled:   true,
		RTSPURL:   fmt.Sprintf("%s/%s", s.rtspPortOrDefault(), st.ID),
		SRTURL:    fmt.Sprintf("%s?streamid=%s", s.srtPortOrDefault(), st.ID),
		CreatedAt: st.CreatedAt,
		UpdatedAt: st.UpdatedAt,
	}
}

func audioFromAPI(a models.AudioConfigData) pipeline.AudioConfig {
	return pipeline.AudioConfig{
		Devices: append([]string(nil), a.Devices...),
		Codec:   a.Codec,
		Bitrate: a.Bitrate,
		Filters: a.Filters,
	}
}

func audioToAPI(a pipeline.AudioConfig) models.AudioConfigData {
	return models.AudioConfigData{
		Devices: append([]string(nil), a.Devices...),
		Codec:   a.Codec,
		Bitrate: a.Bitrate,
		Filters: a.Filters,
	}
}

func encoderFromAPI(e models.EncoderConfigData) pipeline.EncoderConfig {
	return pipeline.EncoderConfig{
		Codec:        e.Codec,
		EncoderName:  e.EncoderName,
		GlobalArgs:   append([]string(nil), e.GlobalArgs...),
		VideoFilters: e.VideoFilters,
		Bitrate:      e.Bitrate,
		GOP:          e.GOP,
		BFrames:      e.BFrames,
		RateControl:  e.RateControl,
		Preset:       e.Preset,
	}
}

func encoderToAPI(e pipeline.EncoderConfig) models.EncoderConfigData {
	return models.EncoderConfigData{
		Codec:        e.Codec,
		EncoderName:  e.EncoderName,
		GlobalArgs:   append([]string(nil), e.GlobalArgs...),
		VideoFilters: e.VideoFilters,
		Bitrate:      e.Bitrate,
		GOP:          e.GOP,
		BFrames:      e.BFrames,
		RateControl:  e.RateControl,
		Preset:       e.Preset,
	}
}

func publishFromAPI(targets []models.PublishTargetData) []pipeline.PublishTarget {
	if len(targets) == 0 {
		return nil
	}
	out := make([]pipeline.PublishTarget, len(targets))
	for i, t := range targets {
		out[i] = pipeline.PublishTarget{Type: t.Type, URL: t.URL}
	}
	return out
}

func publishToAPI(targets []pipeline.PublishTarget) []models.PublishTargetData {
	if len(targets) == 0 {
		return nil
	}
	out := make([]models.PublishTargetData, len(targets))
	for i, t := range targets {
		out[i] = models.PublishTargetData{Type: t.Type, URL: t.URL}
	}
	return out
}

// mapStreamError maps stream-service errors to HTTP errors.
func (s *Server) mapStreamError(err error) error {
	var notFound *StreamNotFoundError
	if errors.As(err, &notFound) {
		return huma.Error404NotFound(notFound.Error(), err)
	}
	var upstreamMissing *StreamUpstreamMissingError
	if errors.As(err, &upstreamMissing) {
		return huma.Error404NotFound(upstreamMissing.Error(), err)
	}
	var exists *StreamExistsError
	if errors.As(err, &exists) {
		return huma.Error409Conflict(exists.Error(), err)
	}
	var invalid *StreamInvalidError
	if errors.As(err, &invalid) {
		return huma.Error400BadRequest(invalid.Error(), err)
	}
	return huma.Error500InternalServerError("internal server error", err)
}
