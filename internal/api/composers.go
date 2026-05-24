package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/smazurov/videonode/internal/api/models"
)

// ComposerService is the API-layer contract for composer CRUD + sub-resource
// edits. The real implementation lands in B9; tests inject a manual mock.
type ComposerService interface {
	ListComposers(ctx context.Context) ([]models.ComposerData, error)
	GetComposer(ctx context.Context, id string) (*models.ComposerData, error)
	CreateComposer(ctx context.Context, data models.ComposerCreateRequestData) (*models.ComposerData, error)
	UpdateComposer(ctx context.Context, id string, patch models.ComposerUpdateRequestData) (*models.ComposerData, error)
	DeleteComposer(ctx context.Context, id string) error
	ReplaceLayout(ctx context.Context, id string, layout []models.LayoutSlotData) (*models.ComposerData, error)
	SetInputEffect(ctx context.Context, id, ref string, effect *models.EffectData) (*models.ComposerData, error)
}

// ComposerErrorCode tags ComposerError values so the API layer can map them
// to HTTP status codes without sniffing strings.
type ComposerErrorCode string

// Composer error codes returned by ComposerService implementations.
const (
	ComposerErrNotFound      ComposerErrorCode = "not_found"
	ComposerErrInputNotFound ComposerErrorCode = "input_not_found"
	ComposerErrExists        ComposerErrorCode = "exists"
	ComposerErrInvalid       ComposerErrorCode = "invalid"
	ComposerErrInUse         ComposerErrorCode = "in_use"
	ComposerErrInternal      ComposerErrorCode = "internal"
)

// ComposerError is the typed error returned by ComposerService methods.
// ReferencingStreams is populated for ComposerErrInUse on DeleteComposer.
type ComposerError struct {
	Code               ComposerErrorCode
	Message            string
	ReferencingStreams []string
}

// Error implements the error interface.
func (e *ComposerError) Error() string {
	if e == nil {
		return ""
	}
	if e.Message != "" {
		return string(e.Code) + ": " + e.Message
	}
	return string(e.Code)
}

// registerComposerRoutes wires all /api/composers endpoints. Skipped when
// the server was constructed without a ComposerService (legacy daemons).
func (s *Server) registerComposerRoutes() {
	svc := s.composerService
	if svc == nil {
		return
	}

	huma.Register(s.api, huma.Operation{
		OperationID: "list-composers",
		Method:      http.MethodGet,
		Path:        "/api/composers",
		Summary:     "List Composers",
		Description: "List all configured composers.",
		Tags:        []string{"composers"},
		Errors:      []int{401, 500},
		Security:    withAuth(),
	}, func(ctx context.Context, _ *struct{}) (*models.ComposerListResponse, error) {
		list, err := svc.ListComposers(ctx)
		if err != nil {
			return nil, mapComposerError(err)
		}
		if list == nil {
			list = []models.ComposerData{}
		}
		return &models.ComposerListResponse{
			Body: models.ComposerListData{Composers: list, Count: len(list)},
		}, nil
	})

	huma.Register(s.api, huma.Operation{
		OperationID: "create-composer",
		Method:      http.MethodPost,
		Path:        "/api/composers",
		Summary:     "Create Composer",
		Description: "Create a new composer with inputs and optional layout.",
		Tags:        []string{"composers"},
		Errors:      []int{400, 401, 409, 500},
		Security:    withAuth(),
	}, func(ctx context.Context, input *models.ComposerCreateRequest) (*models.ComposerResponse, error) {
		c, err := svc.CreateComposer(ctx, input.Body)
		if err != nil {
			return nil, mapComposerError(err)
		}
		if s.composerEntity != nil {
			s.composerEntity.PublishCreated(*c)
		}
		return &models.ComposerResponse{Body: *c}, nil
	})

	huma.Register(s.api, huma.Operation{
		OperationID: "get-composer",
		Method:      http.MethodGet,
		Path:        "/api/composers/{id}",
		Summary:     "Get Composer",
		Description: "Fetch a composer by id.",
		Tags:        []string{"composers"},
		Errors:      []int{401, 404, 500},
		Security:    withAuth(),
	}, func(ctx context.Context, input *struct {
		ID string `path:"id" example:"main-scene" doc:"Composer identifier"`
	},
	) (*models.ComposerResponse, error) {
		c, err := svc.GetComposer(ctx, input.ID)
		if err != nil {
			return nil, mapComposerError(err)
		}
		return &models.ComposerResponse{Body: *c}, nil
	})

	huma.Register(s.api, huma.Operation{
		OperationID: "update-composer",
		Method:      http.MethodPatch,
		Path:        "/api/composers/{id}",
		Summary:     "Update Composer",
		Description: "Patch composer fields. Omitted fields are left untouched.",
		Tags:        []string{"composers"},
		Errors:      []int{400, 401, 404, 500},
		Security:    withAuth(),
	}, func(ctx context.Context, input *struct {
		ID   string `path:"id" example:"main-scene" doc:"Composer identifier"`
		Body models.ComposerUpdateRequestData
	},
	) (*models.ComposerResponse, error) {
		// Snapshot before the patch so a change to inputs[].ref fans
		// out to BOTH old and new source refs. Layout/effect-only
		// updates would also publish a redundant fan-out — harmless,
		// since inputSourceRefs returns the same set in that case and
		// the dispatch dedupes within scope.
		var prev *models.ComposerData
		if s.composerEntity != nil {
			if got, gerr := svc.GetComposer(ctx, input.ID); gerr == nil && got != nil {
				prev = got
			}
		}

		c, err := svc.UpdateComposer(ctx, input.ID, input.Body)
		if err != nil {
			return nil, mapComposerError(err)
		}
		if s.composerEntity != nil {
			if prev != nil {
				s.composerEntity.PublishUpdatedWith(*prev, *c)
			} else {
				s.composerEntity.PublishUpdated(*c)
			}
		}
		return &models.ComposerResponse{Body: *c}, nil
	})

	huma.Register(s.api, huma.Operation{
		OperationID: "delete-composer",
		Method:      http.MethodDelete,
		Path:        "/api/composers/{id}",
		Summary:     "Delete Composer",
		Description: "Delete a composer. Refuses with 409 if any stream still references it.",
		Tags:        []string{"composers"},
		Errors:      []int{401, 404, 409, 500},
		Security:    withAuth(),
	}, func(ctx context.Context, input *struct {
		ID string `path:"id" example:"main-scene" doc:"Composer identifier"`
	},
	) (*struct{}, error) {
		// Snapshot before delete so the composer→source dep hook can
		// fan out to each source the composer was referencing (their
		// Consumers list named this composer; needs to drop it).
		var prev *models.ComposerData
		if s.composerEntity != nil {
			if got, gerr := svc.GetComposer(ctx, input.ID); gerr == nil && got != nil {
				prev = got
			}
		}

		if err := svc.DeleteComposer(ctx, input.ID); err != nil {
			return nil, mapComposerError(err)
		}
		if s.composerEntity != nil {
			if prev != nil {
				s.composerEntity.PublishDeletedWith(*prev)
			} else {
				s.composerEntity.PublishDeleted(input.ID)
			}
		}
		return &struct{}{}, nil
	})

	huma.Register(s.api, huma.Operation{
		OperationID: "replace-composer-layout",
		Method:      http.MethodPatch,
		Path:        "/api/composers/{id}/layout",
		Summary:     "Replace Composer Layout",
		Description: "Replace the composer's full layout array. Validated against inputs[].",
		Tags:        []string{"composers"},
		Errors:      []int{400, 401, 404, 500},
		Security:    withAuth(),
	}, func(ctx context.Context, input *struct {
		ID   string `path:"id" example:"main-scene" doc:"Composer identifier"`
		Body models.ComposerLayoutRequestData
	},
	) (*models.ComposerResponse, error) {
		c, err := svc.ReplaceLayout(ctx, input.ID, input.Body.Layout)
		if err != nil {
			return nil, mapComposerError(err)
		}
		if s.composerEntity != nil {
			s.composerEntity.PublishUpdated(*c)
		}
		return &models.ComposerResponse{Body: *c}, nil
	})

	huma.Register(s.api, huma.Operation{
		OperationID: "set-composer-input-effect",
		Method:      http.MethodPatch,
		Path:        "/api/composers/{id}/inputs/{ref}/effect",
		Summary:     "Set Composer Input Effect",
		Description: "Set or clear the effect on a specific composer input (matched by ref).",
		Tags:        []string{"composers"},
		Errors:      []int{400, 401, 404, 500},
		Security:    withAuth(),
	}, func(ctx context.Context, input *struct {
		ID   string `path:"id" example:"main-scene" doc:"Composer identifier"`
		Ref  string `path:"ref" example:"source:cam-host" doc:"Input ref (matches inputs[].ref)"`
		Body models.ComposerEffectRequestData
	},
	) (*models.ComposerResponse, error) {
		if !input.Body.Effect.Sent {
			return nil, huma.Error400BadRequest("effect field is required")
		}
		var effect *models.EffectData
		if !input.Body.Effect.Null {
			v := input.Body.Effect.Value
			effect = &v
		}
		c, err := svc.SetInputEffect(ctx, input.ID, input.Ref, effect)
		if err != nil {
			return nil, mapComposerError(err)
		}
		if s.composerEntity != nil {
			s.composerEntity.PublishUpdated(*c)
		}
		return &models.ComposerResponse{Body: *c}, nil
	})
}

// mapComposerError converts a *ComposerError into the appropriate huma HTTP
// error. Non-typed errors fall through as 500.
func mapComposerError(err error) error {
	ce := &ComposerError{}
	if !errors.As(err, &ce) {
		return huma.Error500InternalServerError("internal server error", err)
	}
	switch ce.Code {
	case ComposerErrNotFound, ComposerErrInputNotFound:
		return huma.Error404NotFound(ce.Message, err)
	case ComposerErrExists:
		return huma.Error409Conflict(ce.Message, err)
	case ComposerErrInvalid:
		return huma.Error400BadRequest(ce.Message, err)
	case ComposerErrInUse:
		return &huma.ErrorModel{
			Status: http.StatusConflict,
			Title:  http.StatusText(http.StatusConflict),
			Detail: ce.Message,
			Errors: composerInUseDetails(ce),
		}
	default:
		return huma.Error500InternalServerError(ce.Message, err)
	}
}

// composerInUseDetails turns each referencing stream id into a huma error
// detail so the 409 body carries the full conflict context.
func composerInUseDetails(ce *ComposerError) []*huma.ErrorDetail {
	if len(ce.ReferencingStreams) == 0 {
		return nil
	}
	out := make([]*huma.ErrorDetail, len(ce.ReferencingStreams))
	for i, sid := range ce.ReferencingStreams {
		out[i] = &huma.ErrorDetail{
			Message:  fmt.Sprintf("stream %q references this composer", sid),
			Location: "stream",
			Value:    sid,
		}
	}
	return out
}
