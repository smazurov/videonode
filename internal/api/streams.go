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
	"github.com/smazurov/videonode/internal/ffmpeg"
	"github.com/smazurov/videonode/internal/streams"
	"github.com/smazurov/videonode/internal/types"
)

// registerStreamRoutes registers all stream-related endpoints.
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

			if apiStream.Canvas != nil {
				s.emitSourceStreamUpdates(ctx, apiStream.Canvas.SourceStreams)
			}
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
	},
	) (*models.StreamResponse, error) {
		body := input.Body

		var prevSources []string
		if existing, gerr := s.streamService.GetStreamSpec(ctx, input.StreamID); gerr == nil &&
			existing.Canvas != nil {
			prevSources = append(prevSources, existing.Canvas.SourceStreams...)
		}

		stream, err := s.streamService.UpdatePartial(ctx, input.StreamID, func(spec *streams.StreamSpec) error {
			if body.Codec != nil {
				spec.FFmpeg.Codec = *body.Codec
			}
			if body.InputFormat != nil {
				spec.FFmpeg.InputFormat = *body.InputFormat
			}
			if body.Width != nil && body.Height != nil {
				spec.FFmpeg.Resolution = fmt.Sprintf("%dx%d", *body.Width, *body.Height)
			}
			if body.Framerate != nil {
				spec.FFmpeg.FPS = fmt.Sprintf("%d", *body.Framerate)
			}
			if body.AudioDevice != nil {
				spec.FFmpeg.AudioDevice = *body.AudioDevice
			}
			if body.Options != nil {
				opts := make([]ffmpeg.OptionType, len(body.Options))
				for i, o := range body.Options {
					opts[i] = ffmpeg.OptionType(o)
				}
				spec.FFmpeg.Options = opts
			}
			if body.Bitrate != nil {
				if spec.FFmpeg.QualityParams == nil {
					spec.FFmpeg.QualityParams = &types.QualityParams{}
				}
				spec.FFmpeg.QualityParams.TargetBitrate = body.Bitrate
			}
			if body.Rotation != nil {
				spec.FFmpeg.Rotation = *body.Rotation
			}
			if body.CustomFFmpegCommand != nil {
				spec.CustomFFmpegCommand = *body.CustomFFmpegCommand
			}
			if body.TestMode != nil {
				spec.TestMode = *body.TestMode
			}
			if body.Canvas != nil {
				spec.Canvas = apiCanvasToDomain(body.Canvas)
			}
			if body.Perspective.Sent {
				if body.Perspective.Null {
					spec.Perspective = nil
				} else {
					spec.Perspective = &ffmpeg.PerspectiveConfig{Corners: body.Perspective.Value.Corners}
				}
			}
			if body.Vision.Sent {
				if body.Vision.Null {
					spec.Vision = nil
				} else {
					spec.Vision = &ffmpeg.VisionConfig{
						Enabled: body.Vision.Value.Enabled,
						Width:   body.Vision.Value.Width,
						Height:  body.Vision.Value.Height,
						FPS:     body.Vision.Value.FPS,
					}
				}
			}
			return nil
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

			// Canvas members trigger an owning-canvas restart event so its card refreshes.
			if pm := s.streamService.GetProcessManager(); pm != nil {
				if ownerID := pm.OwnedBy(input.StreamID); ownerID != "" {
					if canvasStream, cerr := s.streamService.GetStream(ctx, ownerID); cerr == nil {
						s.eventBus.Publish(events.CanvasRestartedEvent{
							CanvasID:  ownerID,
							TriggerID: input.StreamID,
							Canvas:    s.domainToAPIStream(*canvasStream),
							Timestamp: time.Now().Format(time.RFC3339),
						})
					}
				}
			}

			// Republish union of pre/post sources to flip owned_by on both released and claimed.
			if apiStream.Canvas != nil || len(prevSources) > 0 {
				affected := append([]string{}, prevSources...)
				if apiStream.Canvas != nil {
					affected = append(affected, apiStream.Canvas.SourceStreams...)
				}
				s.emitSourceStreamUpdates(ctx, affected)
			}
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
	},
	) (*struct{}, error) {
		var prevSources []string
		if existing, gerr := s.streamService.GetStreamSpec(ctx, input.StreamID); gerr == nil &&
			existing.Canvas != nil {
			prevSources = append(prevSources, existing.Canvas.SourceStreams...)
		}

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

			s.emitSourceStreamUpdates(ctx, prevSources)
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
	},
	) (*models.StreamResponse, error) {
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

	// Restart stream process
	huma.Register(s.api, huma.Operation{
		OperationID: "restart-stream",
		Method:      http.MethodPost,
		Path:        "/api/streams/{stream_id}/restart",
		Summary:     "Restart Stream",
		Description: "Restart a stream process. Stops and restarts the FFmpeg process with current configuration.",
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

		// Get updated stream to broadcast
		stream, err := s.streamService.GetStream(ctx, input.StreamID)
		if err == nil && s.eventBus != nil {
			apiStream := s.domainToAPIStream(*stream)
			s.eventBus.Publish(events.StreamUpdatedEvent{
				Stream:    apiStream,
				Action:    "restarted",
				Timestamp: time.Now().Format(time.RFC3339),
			})
		}

		return &struct{}{}, nil
	})

	// Canvas layout preview (stateless)
	huma.Register(s.api, huma.Operation{
		OperationID: "canvas-layout-preview",
		Method:      http.MethodPost,
		Path:        "/api/streams/canvas/layout",
		Summary:     "Preview Canvas Layout",
		Description: "Compute the resolved slot + content geometry for a canvas configuration without creating it. The returned layout is the single source of truth for both the ffmpeg composite pipeline and the UI preview.",
		Tags:        []string{"streams"},
		Errors:      []int{400, 401, 404, 500},
		Security:    withAuth(),
	}, func(ctx context.Context, input *models.CanvasLayoutRequest) (*models.CanvasLayoutResponse, error) {
		body := input.Body
		canvas := apiCanvasToDomain(&body)
		sources := make(map[string]*streams.StreamSpec, len(canvas.SourceStreams))
		for _, id := range canvas.SourceStreams {
			spec, err := s.streamService.GetStreamSpec(ctx, id)
			if err != nil || spec == nil {
				return nil, huma.Error404NotFound(fmt.Sprintf("source stream %q not found", id))
			}
			sources[id] = spec
		}
		layout := streams.ComputeCanvasLayout(canvas, sources)
		if len(layout.Slots) == 0 {
			return nil, huma.Error500InternalServerError("layout solver returned no slots")
		}
		return &models.CanvasLayoutResponse{Body: *domainLayoutToAPI(layout)}, nil
	})
}

// convertCreateRequest converts API create request to domain params.
func (s *Server) convertCreateRequest(body models.StreamRequestData) streams.StreamCreateParams {
	params := streams.StreamCreateParams{
		StreamID:    body.StreamID,
		DeviceID:    body.DeviceID,
		Codec:       string(body.Codec),
		InputFormat: body.InputFormat,
		AudioDevice: body.AudioDevice,
		Options:     body.Options,
		Rotation:    body.Rotation,
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

	// Convert canvas if present
	if body.Canvas != nil {
		params.Canvas = apiCanvasToDomain(body.Canvas)
	}

	return params
}

// rotationOverrideCopier abstracts the read-rotation / set-rotation operations
// so canvas-overrides cloning logic can be expressed once for either direction.
type rotationOverrideCopier[Src, Dst any] struct {
	read func(Src) *int
	set  func(*Dst, *int)
}

func cloneSourceOverrides[Src, Dst any](src []Src, c rotationOverrideCopier[Src, Dst]) []Dst {
	if len(src) == 0 {
		return nil
	}
	out := make([]Dst, len(src))
	for i, ov := range src {
		if r := c.read(ov); r != nil {
			rot := *r
			c.set(&out[i], &rot)
		}
	}
	return out
}

func apiCanvasToDomain(c *models.CanvasData) *streams.CanvasConfig {
	return &streams.CanvasConfig{
		Width:         c.Width,
		Height:        c.Height,
		FPS:           c.FPS,
		KeyColor:      c.KeyColor,
		SourceStreams: append([]string(nil), c.SourceStreams...),
		AudioDevices:  append([]string(nil), c.AudioDevices...),
		SourceOverrides: cloneSourceOverrides(c.SourceOverrides, rotationOverrideCopier[models.CanvasSourceOverrideData, streams.CanvasSourceOverride]{
			read: func(o models.CanvasSourceOverrideData) *int { return o.Rotation },
			set:  func(d *streams.CanvasSourceOverride, r *int) { d.Rotation = r },
		}),
		LayoutName: c.LayoutName,
	}
}

func domainCanvasToAPI(c *streams.CanvasConfig) *models.CanvasData {
	return &models.CanvasData{
		Width:         c.Width,
		Height:        c.Height,
		FPS:           c.FPS,
		KeyColor:      c.KeyColor,
		SourceStreams: append([]string(nil), c.SourceStreams...),
		AudioDevices:  append([]string(nil), c.AudioDevices...),
		SourceOverrides: cloneSourceOverrides(c.SourceOverrides, rotationOverrideCopier[streams.CanvasSourceOverride, models.CanvasSourceOverrideData]{
			read: func(o streams.CanvasSourceOverride) *int { return o.Rotation },
			set:  func(d *models.CanvasSourceOverrideData, r *int) { d.Rotation = r },
		}),
		LayoutName: c.LayoutName,
	}
}

// domainLayoutToAPI converts a resolved canvas layout to the API wire type.
func domainLayoutToAPI(l streams.CanvasLayout) *models.CanvasLayoutData {
	out := &models.CanvasLayoutData{
		Slots:            make([]models.CanvasLayoutSlotData, len(l.Slots)),
		ChosenLayout:     l.ChosenLayout,
		AvailableLayouts: append([]string(nil), l.AvailableLayouts...),
	}
	for i, s := range l.Slots {
		out.Slots[i] = models.CanvasLayoutSlotData{
			SourceStreamID:       s.SourceStreamID,
			SlotX:                s.SlotX,
			SlotY:                s.SlotY,
			SlotW:                s.SlotW,
			SlotH:                s.SlotH,
			ContentX:             s.ContentX,
			ContentY:             s.ContentY,
			ContentW:             s.ContentW,
			ContentH:             s.ContentH,
			EffectiveAspectRatio: s.EffectiveAspectRatio,
			RotationApplied:      s.RotationApplied,
		}
	}
	return out
}

// domainToAPIStream fetches the spec and converts; prefer the WithSpec variant when spec is in hand.
func (s *Server) domainToAPIStream(stream streams.Stream) models.StreamData {
	config, err := s.streamService.GetStreamSpec(context.Background(), stream.ID)
	if err != nil {
		config = nil
	}
	return s.domainToAPIStreamWithSpec(stream, config)
}

// domainToAPIStreamWithSpec is the spec-supplied variant.
func (s *Server) domainToAPIStreamWithSpec(stream streams.Stream, config *streams.StreamSpec) models.StreamData {
	displayBitrate := "2M"
	if config != nil && config.FFmpeg.QualityParams != nil && config.FFmpeg.QualityParams.TargetBitrate != nil {
		displayBitrate = fmt.Sprintf("%.1fM", *config.FFmpeg.QualityParams.TargetBitrate)
	}

	deviceID := ""
	codec := ""
	if config != nil {
		deviceID = config.Device
		codec = config.FFmpeg.Codec
	}

	apiData := models.StreamData{
		StreamID:  stream.ID,
		DeviceID:  deviceID,
		Codec:     codec,
		Bitrate:   displayBitrate,
		StartTime: stream.StartTime,
		RTSPURL:   fmt.Sprintf(":8554/%s", stream.ID),
		SRTURL:    fmt.Sprintf(":6001?streamid=%s", stream.ID),
	}

	if config != nil {
		if config.Canvas != nil {
			apiData.Resolution = fmt.Sprintf("%dx%d", config.Canvas.Width, config.Canvas.Height)
			apiData.Framerate = config.Canvas.FPS
			apiData.Canvas = domainCanvasToAPI(config.Canvas)
			apiData.InputsEnabled = stream.InputsEnabled
			apiData.Enabled = true
		} else {
			apiData.InputFormat = config.FFmpeg.InputFormat
			apiData.Resolution = config.FFmpeg.Resolution
			apiData.Framerate = config.FFmpeg.FPS
			apiData.AudioDevice = config.FFmpeg.AudioDevice
			apiData.Rotation = config.FFmpeg.Rotation
			apiData.Enabled = stream.Enabled
			apiData.OwnedBy = stream.OwnedBy
		}

		apiData.CustomFFmpegCmd = config.CustomFFmpegCommand
		apiData.TestMode = config.TestMode

		if config.Perspective != nil {
			apiData.Perspective = &models.PerspectiveData{
				Corners: config.Perspective.Corners,
			}
		}
		if config.Vision != nil {
			apiData.Vision = &models.VisionData{
				Enabled: config.Vision.Enabled,
				Width:   config.Vision.Width,
				Height:  config.Vision.Height,
				FPS:     config.Vision.FPS,
			}
		}

		if len(config.FFmpeg.Options) > 0 {
			options := make([]string, len(config.FFmpeg.Options))
			for i, opt := range config.FFmpeg.Options {
				options[i] = string(opt)
			}
			apiData.Options = options
		}
	}

	return apiData
}

// emitSourceStreamUpdates publishes StreamUpdatedEvent for each unique source ID. Caller ensures eventBus is non-nil.
func (s *Server) emitSourceStreamUpdates(ctx context.Context, sourceIDs []string) {
	if len(sourceIDs) == 0 {
		return
	}
	seen := make(map[string]struct{}, len(sourceIDs))
	for _, srcID := range sourceIDs {
		if srcID == "" {
			continue
		}
		if _, dup := seen[srcID]; dup {
			continue
		}
		seen[srcID] = struct{}{}
		srcStream, err := s.streamService.GetStream(ctx, srcID)
		if err != nil {
			continue
		}
		s.eventBus.Publish(events.StreamUpdatedEvent{
			Stream:    s.domainToAPIStream(*srcStream),
			Action:    "updated",
			Timestamp: time.Now().Format(time.RFC3339),
		})
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
