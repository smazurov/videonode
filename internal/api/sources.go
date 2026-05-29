package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/smazurov/videonode/internal/api/models"
)

// Source is the canonical source descriptor consumed by the API layer.
//
// Mirrors pipeline.Source defined by unit B1 of the
// sources/composers/streams split; the service-layer unit (B9) bridges
// this to the pipeline registry. Defined locally so this unit compiles
// independently of B1/B9 — the integrator deduplicates at merge.
//
// Consumers is the denormalized cross-entity rollup (which composers
// and streams currently reference this source). Populated by the
// service layer in Get/List; emitted to the UI on every
// `entity{type:source}` event so the UI never has to join client-side.
type Source struct {
	ID        string
	Device    string
	TestMode  bool
	Format    *SourceFormat
	Status    models.ProcessStatus
	Liveness  models.SourceLiveness
	CreatedAt time.Time
	UpdatedAt time.Time
	Consumers []models.SourceReference
}

// SourceFormat is the operator-selected V4L2 capture format mirrored
// from pipeline.SourceFormat. Defined locally so this package stays
// independent of internal/streams/pipeline.
type SourceFormat struct {
	FourCC string
	Width  uint32
	Height uint32
	FPS    uint32
}

// SourcePatch describes a partial source update. Nil fields are
// untouched.
type SourcePatch struct {
	Device   *string
	TestMode *bool
	Format   *SourceFormat
}

// SourceService is the contract the API layer requires of the service
// layer (B9). Methods are intentionally small and focused.
type SourceService interface {
	List(ctx context.Context) ([]Source, error)
	Get(ctx context.Context, id string) (*Source, error)
	Create(ctx context.Context, src Source) (*Source, error)
	Update(ctx context.Context, id string, patch SourcePatch) (*Source, error)
	// Delete refuses (returning an *SourceInUseError) when any composer
	// or stream still references the source.
	Delete(ctx context.Context, id string) error
}

// SourceInUseError reports references blocking a delete. Service-layer
// implementations return this from SourceService.Delete; the API maps it
// to a 409 with each reference as an ErrorDetail.
type SourceInUseError struct {
	SourceID   string
	References []models.SourceReference
}

func (e *SourceInUseError) Error() string {
	return fmt.Sprintf("source %q is referenced by %d entit(y/ies)", e.SourceID, len(e.References))
}

// SourceNotFoundError reports a missing source. The API maps it to 404.
type SourceNotFoundError struct {
	SourceID string
}

func (e *SourceNotFoundError) Error() string {
	return fmt.Sprintf("source %q not found", e.SourceID)
}

// SourceExistsError reports a duplicate source ID on create. Mapped to 409.
type SourceExistsError struct {
	SourceID string
}

func (e *SourceExistsError) Error() string {
	return fmt.Sprintf("source %q already exists", e.SourceID)
}

// SourceInvalidError reports validation failures (e.g. both device and
// test_mode set, or both empty). Mapped to 400.
type SourceInvalidError struct {
	Message string
}

func (e *SourceInvalidError) Error() string { return e.Message }

// registerSourceRoutes wires the /api/sources CRUD surface.
func (s *Server) registerSourceRoutes() {
	if s.sourceService == nil {
		return
	}

	huma.Register(s.api, huma.Operation{
		OperationID: "list-sources",
		Method:      http.MethodGet,
		Path:        "/api/sources",
		Summary:     "List Sources",
		Description: "List all configured sources (V4L2 producers and test-pattern producers).",
		Tags:        []string{"sources"},
		Errors:      []int{401, 500},
		Security:    withAuth(),
	}, func(ctx context.Context, _ *struct{}) (*models.SourceListResponse, error) {
		items, err := s.sourceService.List(ctx)
		if err != nil {
			return nil, mapSourceError(err)
		}
		apiSources := make([]models.SourceData, len(items))
		for i, src := range items {
			apiSources[i] = sourceToAPI(src)
		}
		return &models.SourceListResponse{
			Body: models.SourceListData{
				Sources: apiSources,
				Count:   len(apiSources),
			},
		}, nil
	})

	huma.Register(s.api, huma.Operation{
		OperationID: "create-source",
		Method:      http.MethodPost,
		Path:        "/api/sources",
		Summary:     "Create Source",
		Description: "Register a new source. Provide either device for a V4L2 producer or test_mode=true for the test-pattern producer (mutually exclusive).",
		Tags:        []string{"sources"},
		Errors:      []int{400, 401, 409, 500},
		Security:    withAuth(),
	}, func(ctx context.Context, input *models.SourceCreateRequest) (*models.SourceResponse, error) {
		format, err := sourceFormatFromBody(input.Body.Format)
		if err != nil {
			return nil, huma.Error400BadRequest(err.Error(), err)
		}
		src := Source{
			ID:       input.Body.SourceID,
			Device:   input.Body.Device,
			TestMode: input.Body.TestMode,
			Format:   format,
		}
		created, err := s.sourceService.Create(ctx, src)
		if err != nil {
			return nil, mapSourceError(err)
		}
		apiSource := sourceToAPI(*created)
		if s.sourceEntity != nil {
			s.sourceEntity.PublishCreated(apiSource)
		}
		return &models.SourceResponse{Body: apiSource}, nil
	})

	huma.Register(s.api, huma.Operation{
		OperationID: "get-source",
		Method:      http.MethodGet,
		Path:        "/api/sources/{source_id}",
		Summary:     "Get Source",
		Description: "Fetch a single source by ID.",
		Tags:        []string{"sources"},
		Errors:      []int{401, 404, 500},
		Security:    withAuth(),
	}, func(ctx context.Context, input *struct {
		SourceID string `path:"source_id" example:"hdmi-slides" doc:"Source identifier"`
	},
	) (*models.SourceResponse, error) {
		src, err := s.sourceService.Get(ctx, input.SourceID)
		if err != nil {
			return nil, mapSourceError(err)
		}
		return &models.SourceResponse{Body: sourceToAPI(*src)}, nil
	})

	huma.Register(s.api, huma.Operation{
		OperationID: "update-source",
		Method:      http.MethodPatch,
		Path:        "/api/sources/{source_id}",
		Summary:     "Update Source",
		Description: "Patch a source. Only the supplied fields are modified.",
		Tags:        []string{"sources"},
		Errors:      []int{400, 401, 404, 500},
		Security:    withAuth(),
	}, func(ctx context.Context, input *models.SourceUpdateRequest) (*models.SourceResponse, error) {
		format, err := sourceFormatFromBody(input.Body.Format)
		if err != nil {
			return nil, huma.Error400BadRequest(err.Error(), err)
		}
		patch := SourcePatch{
			Device:   input.Body.Device,
			TestMode: input.Body.TestMode,
			Format:   format,
		}
		updated, err := s.sourceService.Update(ctx, input.SourceID, patch)
		if err != nil {
			return nil, mapSourceError(err)
		}
		apiSource := sourceToAPI(*updated)
		if s.sourceEntity != nil {
			s.sourceEntity.PublishUpdated(apiSource)
		}
		return &models.SourceResponse{Body: apiSource}, nil
	})

	huma.Register(s.api, huma.Operation{
		OperationID: "delete-source",
		Method:      http.MethodDelete,
		Path:        "/api/sources/{source_id}",
		Summary:     "Delete Source",
		Description: "Delete a source. Refused with 409 if any composer or stream still references it.",
		Tags:        []string{"sources"},
		Errors:      []int{401, 404, 409, 500},
		Security:    withAuth(),
	}, func(ctx context.Context, input *struct {
		SourceID string `path:"source_id" example:"hdmi-slides" doc:"Source identifier"`
	},
	) (*struct{}, error) {
		if err := s.sourceService.Delete(ctx, input.SourceID); err != nil {
			return nil, mapSourceError(err)
		}
		if s.sourceEntity != nil {
			s.sourceEntity.PublishDeleted(input.SourceID)
		}
		return &struct{}{}, nil
	})
}

// sourceToAPI converts the internal source descriptor to the wire model.
func sourceToAPI(src Source) models.SourceData {
	out := models.SourceData{
		SourceID:  src.ID,
		Device:    src.Device,
		TestMode:  src.TestMode,
		Status:    src.Status,
		Liveness:  src.Liveness,
		CreatedAt: src.CreatedAt,
		UpdatedAt: src.UpdatedAt,
		Consumers: src.Consumers,
	}
	if src.Format != nil {
		name, ok := models.PixelFormatToVideoFormatByFourCC(src.Format.FourCC)
		if ok {
			out.Format = &models.SourceFormatBody{
				FormatName: name,
				Width:      src.Format.Width,
				Height:     src.Format.Height,
				FPS:        src.Format.FPS,
			}
		}
	}
	return out
}

// sourceFormatFromBody validates and converts an API SourceFormatBody to
// the internal Source.Format shape. Returns (nil, nil) when the body is
// nil so optional fields stay optional.
func sourceFormatFromBody(b *models.SourceFormatBody) (*SourceFormat, error) {
	if b == nil {
		return nil, nil
	}
	if b.Width == 0 || b.Height == 0 {
		return nil, errors.New("source format requires width and height > 0")
	}
	fourcc, err := b.FormatName.ToFourCC()
	if err != nil {
		return nil, fmt.Errorf("source format: %w", err)
	}
	return &SourceFormat{
		FourCC: fourcc,
		Width:  b.Width,
		Height: b.Height,
		FPS:    b.FPS,
	}, nil
}

// mapSourceError translates service-layer errors into huma StatusErrors.
func mapSourceError(err error) error {
	var notFound *SourceNotFoundError
	if errors.As(err, &notFound) {
		return huma.Error404NotFound(notFound.Error(), err)
	}
	var exists *SourceExistsError
	if errors.As(err, &exists) {
		return huma.Error409Conflict(exists.Error(), err)
	}
	var invalid *SourceInvalidError
	if errors.As(err, &invalid) {
		return huma.Error400BadRequest(invalid.Error(), err)
	}
	var inUse *SourceInUseError
	if errors.As(err, &inUse) {
		details := make([]error, len(inUse.References))
		for i, ref := range inUse.References {
			details[i] = &huma.ErrorDetail{
				Message:  fmt.Sprintf("%s %q still references this source", ref.Kind, ref.ID),
				Location: fmt.Sprintf("%s:%s", ref.Kind, ref.ID),
				Value:    ref.ID,
			}
		}
		return huma.Error409Conflict(inUse.Error(), details...)
	}
	return huma.Error500InternalServerError("internal server error", err)
}
