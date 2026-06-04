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

// Sensor is the canonical sensor descriptor consumed by the API layer.
// Mirrors pipeline.Sensor; the service layer bridges this to the entity store
// and the perception subsystem. Bindings is the denormalized cross-entity
// rollup (which composer inputs select this sensor), populated by the service
// layer in Get/List.
type Sensor struct {
	ID            string
	Source        string
	Detector      string
	ModelID       string
	Mode          string
	Margin        float64
	MinConfidence float64
	TickMs        int
	Status        models.ProcessStatus
	CreatedAt     time.Time
	UpdatedAt     time.Time
	Bindings      []models.SensorReference
}

// SensorPatch describes a partial sensor update. Nil fields are untouched.
type SensorPatch struct {
	Source        *string
	Detector      *string
	ModelID       *string
	Mode          *string
	Margin        *float64
	MinConfidence *float64
	TickMs        *int
}

// SensorService is the contract the API layer requires of the service layer.
type SensorService interface {
	List(ctx context.Context) ([]Sensor, error)
	Get(ctx context.Context, id string) (*Sensor, error)
	Create(ctx context.Context, sn Sensor) (*Sensor, error)
	Update(ctx context.Context, id string, patch SensorPatch) (*Sensor, error)
	// Delete refuses (returning a *SensorInUseError) when any composer input
	// still selects the sensor via an auto_crop effect.
	Delete(ctx context.Context, id string) error
}

// SensorInUseError reports composer-input bindings blocking a delete.
type SensorInUseError struct {
	SensorID   string
	References []models.SensorReference
}

func (e *SensorInUseError) Error() string {
	return fmt.Sprintf("sensor %q is referenced by %d composer input(s)", e.SensorID, len(e.References))
}

// SensorNotFoundError reports a missing sensor. The API maps it to 404.
type SensorNotFoundError struct {
	SensorID string
}

func (e *SensorNotFoundError) Error() string {
	return fmt.Sprintf("sensor %q not found", e.SensorID)
}

// SensorExistsError reports a duplicate sensor ID on create. Mapped to 409.
type SensorExistsError struct {
	SensorID string
}

func (e *SensorExistsError) Error() string {
	return fmt.Sprintf("sensor %q already exists", e.SensorID)
}

// SensorInvalidError reports validation failures. Mapped to 400.
type SensorInvalidError struct {
	Message string
}

func (e *SensorInvalidError) Error() string { return e.Message }

// registerSensorRoutes wires the /api/sensors CRUD surface.
func (s *Server) registerSensorRoutes() {
	if s.sensorService == nil {
		return
	}

	huma.Register(s.api, huma.Operation{
		OperationID: "list-sensors",
		Method:      http.MethodGet,
		Path:        "/api/sensors",
		Summary:     "List Sensors",
		Description: "List all configured perception sensors.",
		Tags:        []string{"sensors"},
		Errors:      []int{401, 500},
		Security:    withAuth(),
	}, func(ctx context.Context, _ *struct{}) (*models.SensorListResponse, error) {
		items, err := s.sensorService.List(ctx)
		if err != nil {
			return nil, mapSensorError(err)
		}
		out := make([]models.SensorData, len(items))
		for i, sn := range items {
			out[i] = sensorToAPI(sn)
		}
		return &models.SensorListResponse{
			Body: models.SensorListData{Sensors: out, Count: len(out)},
		}, nil
	})

	huma.Register(s.api, huma.Operation{
		OperationID: "create-sensor",
		Method:      http.MethodPost,
		Path:        "/api/sensors",
		Summary:     "Create Sensor",
		Description: "Register a new perception sensor observing a source or composer ref.",
		Tags:        []string{"sensors"},
		Errors:      []int{400, 401, 409, 500},
		Security:    withAuth(),
	}, func(ctx context.Context, input *models.SensorCreateRequest) (*models.SensorResponse, error) {
		sn := Sensor{
			ID:            input.Body.SensorID,
			Source:        input.Body.Source,
			Detector:      input.Body.Detector,
			ModelID:       input.Body.ModelID,
			Mode:          input.Body.Mode,
			Margin:        input.Body.Margin,
			MinConfidence: input.Body.MinConfidence,
			TickMs:        input.Body.TickMs,
		}
		created, err := s.sensorService.Create(ctx, sn)
		if err != nil {
			return nil, mapSensorError(err)
		}
		apiSensor := sensorToAPI(*created)
		if s.sensorEntity != nil {
			s.sensorEntity.PublishCreated(apiSensor)
		}
		return &models.SensorResponse{Body: apiSensor}, nil
	})

	huma.Register(s.api, huma.Operation{
		OperationID: "get-sensor",
		Method:      http.MethodGet,
		Path:        "/api/sensors/{sensor_id}",
		Summary:     "Get Sensor",
		Description: "Fetch a single sensor by ID.",
		Tags:        []string{"sensors"},
		Errors:      []int{401, 404, 500},
		Security:    withAuth(),
	}, func(ctx context.Context, input *struct {
		SensorID string `path:"sensor_id" example:"playfield" doc:"Sensor identifier"`
	},
	) (*models.SensorResponse, error) {
		sn, err := s.sensorService.Get(ctx, input.SensorID)
		if err != nil {
			return nil, mapSensorError(err)
		}
		return &models.SensorResponse{Body: sensorToAPI(*sn)}, nil
	})

	huma.Register(s.api, huma.Operation{
		OperationID: "update-sensor",
		Method:      http.MethodPatch,
		Path:        "/api/sensors/{sensor_id}",
		Summary:     "Update Sensor",
		Description: "Patch a sensor. Only the supplied fields are modified.",
		Tags:        []string{"sensors"},
		Errors:      []int{400, 401, 404, 500},
		Security:    withAuth(),
	}, func(ctx context.Context, input *models.SensorUpdateRequest) (*models.SensorResponse, error) {
		patch := SensorPatch{
			Source:        input.Body.Source,
			Detector:      input.Body.Detector,
			ModelID:       input.Body.ModelID,
			Mode:          input.Body.Mode,
			Margin:        input.Body.Margin,
			MinConfidence: input.Body.MinConfidence,
			TickMs:        input.Body.TickMs,
		}
		updated, err := s.sensorService.Update(ctx, input.SensorID, patch)
		if err != nil {
			return nil, mapSensorError(err)
		}
		apiSensor := sensorToAPI(*updated)
		if s.sensorEntity != nil {
			s.sensorEntity.PublishUpdated(apiSensor)
		}
		return &models.SensorResponse{Body: apiSensor}, nil
	})

	huma.Register(s.api, huma.Operation{
		OperationID: "delete-sensor",
		Method:      http.MethodDelete,
		Path:        "/api/sensors/{sensor_id}",
		Summary:     "Delete Sensor",
		Description: "Delete a sensor. Refused with 409 if any composer input still selects it.",
		Tags:        []string{"sensors"},
		Errors:      []int{401, 404, 409, 500},
		Security:    withAuth(),
	}, func(ctx context.Context, input *struct {
		SensorID string `path:"sensor_id" example:"playfield" doc:"Sensor identifier"`
	},
	) (*struct{}, error) {
		if err := s.sensorService.Delete(ctx, input.SensorID); err != nil {
			return nil, mapSensorError(err)
		}
		if s.sensorEntity != nil {
			s.sensorEntity.PublishDeleted(input.SensorID)
		}
		return &struct{}{}, nil
	})
}

// sensorToAPI converts the internal sensor descriptor to the wire model.
func sensorToAPI(sn Sensor) models.SensorData {
	return models.SensorData{
		SensorID:      sn.ID,
		Source:        sn.Source,
		Detector:      sn.Detector,
		ModelID:       sn.ModelID,
		Mode:          sn.Mode,
		Margin:        sn.Margin,
		MinConfidence: sn.MinConfidence,
		TickMs:        sn.TickMs,
		Bindings:      sn.Bindings,
		Status:        sn.Status,
		CreatedAt:     sn.CreatedAt,
		UpdatedAt:     sn.UpdatedAt,
	}
}

// mapSensorError translates service-layer errors into huma StatusErrors.
func mapSensorError(err error) error {
	var notFound *SensorNotFoundError
	if errors.As(err, &notFound) {
		return huma.Error404NotFound(notFound.Error(), err)
	}
	var exists *SensorExistsError
	if errors.As(err, &exists) {
		return huma.Error409Conflict(exists.Error(), err)
	}
	var invalid *SensorInvalidError
	if errors.As(err, &invalid) {
		return huma.Error400BadRequest(invalid.Error(), err)
	}
	var inUse *SensorInUseError
	if errors.As(err, &inUse) {
		details := make([]error, len(inUse.References))
		for i, ref := range inUse.References {
			details[i] = &huma.ErrorDetail{
				Message:  fmt.Sprintf("composer %q input %q still selects this sensor", ref.ID, ref.Input),
				Location: fmt.Sprintf("composer:%s", ref.ID),
				Value:    ref.ID,
			}
		}
		return huma.Error409Conflict(inUse.Error(), details...)
	}
	return huma.Error500InternalServerError("internal server error", err)
}
