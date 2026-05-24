package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/smazurov/videonode/internal/api/models"
	"github.com/smazurov/videonode/internal/events"
	"github.com/smazurov/videonode/internal/streams"
	"github.com/smazurov/videonode/internal/types"
)

// registerStreamRoutes registers all stream-related endpoints.
func (s *Server) registerStreamRoutes() {
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
		items, err := s.streamService.ListStreamsWithSpecs(ctx)
		if err != nil {
			return nil, s.mapStreamError(err)
		}

		apiStreams := make([]models.StreamData, len(items))
		for i, it := range items {
			spec := it.Spec
			apiStreams[i] = s.domainToAPIStreamWithSpec(it.Stream, &spec)
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
		params := convertCreateRequest(input.Body)

		stream, err := s.streamService.CreateStream(ctx, params)
		if err != nil {
			return nil, s.mapStreamError(err)
		}

		apiStream := s.domainToAPIStream(*stream)

		if s.eventBus != nil {
			s.eventBus.Publish(events.StreamCreatedEvent{
				Stream:    apiStream,
				Action:    "created",
				Timestamp: time.Now().Format(time.RFC3339),
			})
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

		stream, err := s.streamService.UpdatePartial(ctx, input.StreamID, func(spec *streams.StreamSpec) error {
			return applySlimUpdate(spec, body)
		})
		if err != nil {
			return nil, s.mapStreamError(err)
		}

		if body.Enabled != nil {
			if _, serr := s.streamService.SetEnabled(ctx, input.StreamID, *body.Enabled); serr != nil {
				return nil, s.mapStreamError(serr)
			}
			stream.Enabled = *body.Enabled
		}

		apiStream := s.domainToAPIStream(*stream)
		if s.eventBus != nil {
			s.eventBus.Publish(events.StreamUpdatedEvent{
				Stream:    apiStream,
				Action:    "updated",
				Timestamp: time.Now().Format(time.RFC3339),
			})
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
		if err := s.streamService.DeleteStream(ctx, input.StreamID); err != nil {
			return nil, s.mapStreamError(err)
		}

		if s.eventBus != nil {
			s.eventBus.Publish(events.StreamDeletedEvent{
				StreamID:  input.StreamID,
				Action:    "deleted",
				Timestamp: time.Now().Format(time.RFC3339),
			})
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
		stream, err := s.streamService.GetStream(ctx, input.StreamID)
		if err != nil {
			return nil, s.mapStreamError(err)
		}
		return &models.StreamResponse{Body: s.domainToAPIStream(*stream)}, nil
	})

	huma.Register(s.api, huma.Operation{
		OperationID: "get-stream-ffmpeg",
		Method:      http.MethodGet,
		Path:        "/api/streams/{stream_id}/ffmpeg",
		Summary:     "Get FFmpeg Command",
		Description: "Get the generated ffmpeg argv for a stream",
		Tags:        []string{"streams"},
		Errors:      []int{401, 404, 500},
		Security:    withAuth(),
	}, func(ctx context.Context, input *struct {
		StreamID        string `path:"stream_id" minLength:"1" maxLength:"50" pattern:"^[a-zA-Z0-9_-]+$" example:"stream-001" doc:"Stream identifier"`
		EncoderOverride string `query:"override" example:"h264_vaapi" doc:"Override the auto-selected encoder"`
	},
	) (*models.FFmpegCommandResponse, error) {
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

	huma.Register(s.api, huma.Operation{
		OperationID: "restart-stream",
		Method:      http.MethodPost,
		Path:        "/api/streams/{stream_id}/restart",
		Summary:     "Restart Stream",
		Description: "Restart the encoder process for a stream",
		Tags:        []string{"streams"},
		Errors:      []int{401, 404, 500, 503},
		Security:    withAuth(),
	}, func(ctx context.Context, input *struct {
		StreamID string `path:"stream_id" minLength:"1" maxLength:"50" pattern:"^[a-zA-Z0-9_-]+$" example:"stream-001" doc:"Stream identifier"`
	},
	) (*struct{}, error) {
		if err := s.streamService.RestartStream(ctx, input.StreamID); err != nil {
			return nil, s.mapStreamError(err)
		}

		if s.eventBus != nil {
			if stream, gerr := s.streamService.GetStream(ctx, input.StreamID); gerr == nil {
				s.eventBus.Publish(events.StreamUpdatedEvent{
					Stream:    s.domainToAPIStream(*stream),
					Action:    "restarted",
					Timestamp: time.Now().Format(time.RFC3339),
				})
			}
		}

		return &struct{}{}, nil
	})
}

// convertCreateRequest translates the slim API create payload into the
// legacy StreamCreateParams. The mapping is best-effort until B9 lands a
// slim StreamService: source-prefixed upstream populates DeviceID; codec
// and bitrate are pulled from the EncoderConfig; the audio device picks
// the first entry of Audio.Devices.
func convertCreateRequest(body models.StreamRequestData) streams.StreamCreateParams {
	params := streams.StreamCreateParams{
		StreamID: body.StreamID,
		Codec:    body.Encoder.Codec,
	}

	if dev, ok := parseUpstreamRef("source", body.Upstream); ok {
		params.DeviceID = dev
	}
	if len(body.Audio.Devices) > 0 {
		params.AudioDevice = body.Audio.Devices[0]
	}
	if mbps, ok := bitrateToMbps(body.Encoder.Bitrate); ok {
		params.Bitrate = &mbps
	}

	return params
}

// applySlimUpdate folds a slim partial update onto the legacy StreamSpec. It
// only touches fields the slim shape owns; everything else is preserved.
func applySlimUpdate(spec *streams.StreamSpec, body models.StreamUpdateRequestData) error {
	if body.Name != nil {
		spec.Name = *body.Name
	}
	if body.Upstream != nil {
		if dev, ok := parseUpstreamRef("source", *body.Upstream); ok {
			spec.Device = dev
		}
	}
	if body.Encoder.Sent && !body.Encoder.Null {
		spec.FFmpeg.Codec = body.Encoder.Value.Codec
		if mbps, ok := bitrateToMbps(body.Encoder.Value.Bitrate); ok {
			ensureQualityParams(spec)
			spec.FFmpeg.QualityParams.TargetBitrate = &mbps
		}
	}
	if body.Audio.Sent && !body.Audio.Null {
		if len(body.Audio.Value.Devices) > 0 {
			spec.FFmpeg.AudioDevice = body.Audio.Value.Devices[0]
		} else {
			spec.FFmpeg.AudioDevice = ""
		}
	}
	if body.CustomEncoderArgs != nil {
		spec.CustomFFmpegCommand = *body.CustomEncoderArgs
	}
	return nil
}

// parseUpstreamRef matches "<kind>:<id>" and returns the id when kind matches.
func parseUpstreamRef(kind, ref string) (string, bool) {
	prefix := kind + ":"
	if !strings.HasPrefix(ref, prefix) {
		return "", false
	}
	id := strings.TrimPrefix(ref, prefix)
	if id == "" {
		return "", false
	}
	return id, true
}

// bitrateToMbps converts pipeline-style bitrate strings ("4M", "1500k", "2.5")
// into the legacy Mbps float used by FFmpegConfig.QualityParams.
func bitrateToMbps(s string) (float64, bool) {
	if s == "" {
		return 0, false
	}
	idx := len(s)
	for i, r := range s {
		if (r >= '0' && r <= '9') || r == '.' {
			continue
		}
		idx = i
		break
	}
	numPart, unit := s[:idx], strings.ToLower(s[idx:])
	if numPart == "" {
		return 0, false
	}
	var n float64
	if _, err := fmt.Sscanf(numPart, "%f", &n); err != nil || n <= 0 {
		return 0, false
	}
	switch unit {
	case "", "m", "mb", "mbps":
		return n, true
	case "k", "kb", "kbps":
		return n / 1000.0, true
	}
	return 0, false
}

func ensureQualityParams(spec *streams.StreamSpec) {
	if spec.FFmpeg.QualityParams == nil {
		spec.FFmpeg.QualityParams = &types.QualityParams{}
	}
}

// domainToAPIStream fetches the spec and converts to the slim wire shape.
func (s *Server) domainToAPIStream(stream streams.Stream) models.StreamData {
	spec, err := s.streamService.GetStreamSpec(context.Background(), stream.ID)
	if err != nil {
		spec = nil
	}
	return s.domainToAPIStreamWithSpec(stream, spec)
}

// domainToAPIStreamWithSpec is the spec-supplied variant.
func (s *Server) domainToAPIStreamWithSpec(stream streams.Stream, spec *streams.StreamSpec) models.StreamData {
	apiData := models.StreamData{
		StreamID: stream.ID,
		Enabled:  stream.Enabled,
		RTSPURL:  fmt.Sprintf("%s/%s", s.rtspPortOrDefault(), stream.ID),
		SRTURL:   fmt.Sprintf("%s?streamid=%s", s.srtPortOrDefault(), stream.ID),
	}

	if spec == nil {
		// Minimal payload — runtime-only knowledge.
		apiData.Enabled = false
		return apiData
	}

	apiData.Name = spec.Name
	apiData.Upstream = deriveUpstream(spec)
	apiData.Encoder = encoderConfigFromSpec(spec)
	apiData.Audio = audioConfigFromSpec(spec)
	apiData.CustomEncoderArgs = spec.CustomFFmpegCommand
	apiData.CreatedAt = spec.CreatedAt
	apiData.UpdatedAt = spec.UpdatedAt

	return apiData
}

// deriveUpstream produces a slim upstream ref from the legacy spec shape.
// Canvas streams synthesize a composer ref keyed by the stream id; single-
// device streams point at "source:<device>".
func deriveUpstream(spec *streams.StreamSpec) string {
	if spec.Canvas != nil {
		return "composer:" + spec.ID
	}
	if spec.Device != "" {
		return "source:" + spec.Device
	}
	return ""
}

func encoderConfigFromSpec(spec *streams.StreamSpec) models.EncoderConfigData {
	out := models.EncoderConfigData{
		Codec: spec.FFmpeg.Codec,
	}
	if spec.FFmpeg.QualityParams != nil && spec.FFmpeg.QualityParams.TargetBitrate != nil {
		out.Bitrate = fmt.Sprintf("%.1fM", *spec.FFmpeg.QualityParams.TargetBitrate)
	}
	return out
}

func audioConfigFromSpec(spec *streams.StreamSpec) models.AudioConfigData {
	if spec.FFmpeg.AudioDevice == "" {
		return models.AudioConfigData{}
	}
	return models.AudioConfigData{
		Devices: []string{spec.FFmpeg.AudioDevice},
	}
}

// mapStreamError maps domain errors to HTTP errors.
func (s *Server) mapStreamError(err error) error {
	streamErr := &streams.StreamError{}
	if errors.As(err, &streamErr) {
		switch streamErr.Code {
		case streams.ErrCodeStreamNotFound:
			return huma.Error404NotFound(streamErr.Message, err)
		case streams.ErrCodeDeviceNotFound:
			return huma.Error404NotFound(streamErr.Message, err)
		case streams.ErrCodeStreamExists:
			return huma.Error409Conflict(streamErr.Message, err)
		case streams.ErrCodeInvalidParams:
			return huma.Error400BadRequest(streamErr.Message, err)
		case streams.ErrCodeConfigError:
			return huma.Error500InternalServerError(streamErr.Message, err)
		default:
			return huma.Error500InternalServerError("internal server error", err)
		}
	}
	return huma.Error500InternalServerError("internal server error", err)
}
