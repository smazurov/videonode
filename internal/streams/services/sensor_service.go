package services

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/smazurov/videonode/internal/api"
	"github.com/smazurov/videonode/internal/api/models"
	"github.com/smazurov/videonode/internal/logging"
	"github.com/smazurov/videonode/internal/process"
	"github.com/smazurov/videonode/internal/streams"
	"github.com/smazurov/videonode/internal/streams/pipeline"
)

// SensorLifecycle is the perception subsystem's reconcile surface the sensor
// service drives on every mutation: it provisions/tears down the analysis tap +
// videonode-sensor process + router commit policy for a sensor id. Satisfied by
// *sensors.Lifecycle; declared here at the point of use.
type SensorLifecycle interface {
	ReconcileSensor(id string)
	Remove(id string)
}

// sensorPool is the subset of the pipeline the sensor service reads for live
// status. Declared at the point of use so the service unit-tests with a tiny
// mock instead of a real supervised pipeline.
type sensorPool interface {
	Pool() process.Pool
}

// SensorServiceOptions wires the SensorService to persistence, the perception
// lifecycle, and the pipeline (for live status).
type SensorServiceOptions struct {
	Store          streams.EntityStore
	Lifecycle      SensorLifecycle
	Pipeline       sensorPool
	PipelineSwitch PipelineSwitch
}

// sensorService implements api.SensorService backed by the v2 EntityStore plus
// the perception lifecycle. Sensors are pipeline-gated: persisted always,
// reconciled into running processes only while the switch is on.
type sensorService struct {
	store     streams.EntityStore
	lifecycle SensorLifecycle
	pipe      sensorPool
	psw       PipelineSwitch
	logger    logging.Logger
	mu        sync.Mutex
}

// NewSensorService constructs a SensorService. Store is required; Lifecycle and
// Pipeline are optional (nil = persistence-only / no live status).
func NewSensorService(opts SensorServiceOptions) api.SensorService {
	if opts.Store == nil {
		panic("services.NewSensorService: Store is required")
	}
	return &sensorService{
		store:     opts.Store,
		lifecycle: opts.Lifecycle,
		pipe:      opts.Pipeline,
		psw:       opts.PipelineSwitch,
		logger:    logging.GetLogger("sensor_svc"),
	}
}

func (s *sensorService) pipelineSwitchEnabled() bool {
	if s.psw == nil {
		return true
	}
	return s.psw.GetPipeline().Enabled
}

// List returns all configured sensors, each with Bindings denormalized.
func (s *sensorService) List(_ context.Context) ([]api.Sensor, error) {
	entries := s.store.ListSensorEntities()
	out := make([]api.Sensor, len(entries))
	for i, e := range entries {
		out[i] = sensorToInternal(e)
		out[i].Bindings = s.findBindings(e.ID)
		s.enrichStatus(&out[i])
	}
	return out, nil
}

// Get returns one sensor by id, with Bindings denormalized.
func (s *sensorService) Get(_ context.Context, id string) (*api.Sensor, error) {
	sn, ok := s.store.GetSensorEntity(id)
	if !ok {
		return nil, &api.SensorNotFoundError{SensorID: id}
	}
	out := sensorToInternal(sn)
	out.Bindings = s.findBindings(id)
	s.enrichStatus(&out)
	return &out, nil
}

// Create validates, persists, and reconciles a new sensor.
func (s *sensorService) Create(_ context.Context, sn api.Sensor) (*api.Sensor, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := validateSensor(sn.ID, sn.Source, sn.Mode); err != nil {
		return nil, err
	}
	if _, ok := s.store.GetSensorEntity(sn.ID); ok {
		return nil, &api.SensorExistsError{SensorID: sn.ID}
	}

	now := time.Now()
	entity := pipeline.Sensor{
		ID:            sn.ID,
		Source:        sn.Source,
		Detector:      sn.Detector,
		ModelID:       sn.ModelID,
		Mode:          sn.Mode,
		Margin:        sn.Margin,
		MinConfidence: sn.MinConfidence,
		TickMs:        sn.TickMs,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := s.store.AddSensorEntity(entity); err != nil {
		return nil, fmt.Errorf("add sensor: %w", err)
	}
	if s.lifecycle != nil && s.pipelineSwitchEnabled() {
		s.lifecycle.ReconcileSensor(entity.ID)
	}

	out := sensorToInternal(entity)
	s.enrichStatus(&out)
	return &out, nil
}

// Update applies a partial patch, persists, and reconciles.
func (s *sensorService) Update(_ context.Context, id string, patch api.SensorPatch) (*api.Sensor, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	entity, ok := s.store.GetSensorEntity(id)
	if !ok {
		return nil, &api.SensorNotFoundError{SensorID: id}
	}
	if patch.Source != nil {
		entity.Source = *patch.Source
	}
	if patch.Detector != nil {
		entity.Detector = *patch.Detector
	}
	if patch.ModelID != nil {
		entity.ModelID = *patch.ModelID
	}
	if patch.Mode != nil {
		entity.Mode = *patch.Mode
	}
	if patch.Margin != nil {
		entity.Margin = *patch.Margin
	}
	if patch.MinConfidence != nil {
		entity.MinConfidence = *patch.MinConfidence
	}
	if patch.TickMs != nil {
		entity.TickMs = *patch.TickMs
	}
	if err := validateSensor(entity.ID, entity.Source, entity.Mode); err != nil {
		return nil, err
	}
	entity.UpdatedAt = time.Now()
	if err := s.store.UpdateSensorEntity(id, entity); err != nil {
		return nil, fmt.Errorf("update sensor: %w", err)
	}
	if s.lifecycle != nil && s.pipelineSwitchEnabled() {
		s.lifecycle.ReconcileSensor(id)
	}

	out := sensorToInternal(entity)
	out.Bindings = s.findBindings(id)
	s.enrichStatus(&out)
	return &out, nil
}

// Delete refuses while any composer input still selects the sensor, then tears
// down its runtime and removes it from the store.
func (s *sensorService) Delete(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.store.GetSensorEntity(id); !ok {
		return &api.SensorNotFoundError{SensorID: id}
	}
	refs := s.findBindings(id)
	if len(refs) > 0 {
		return &api.SensorInUseError{SensorID: id, References: refs}
	}

	if s.lifecycle != nil {
		s.lifecycle.Remove(id)
	}
	if err := s.store.RemoveSensorEntity(id); err != nil {
		return fmt.Errorf("remove sensor: %w", err)
	}
	return nil
}

// findBindings scans composer inputs for auto_crop effects selecting this
// sensor, reporting each as a (composer, input) reference.
func (s *sensorService) findBindings(id string) []models.SensorReference {
	target := pipeline.SensorRef(id)
	var refs []models.SensorReference
	for _, c := range s.store.ListComposerEntities() {
		for _, in := range c.Inputs {
			if in.Effect.IsAutoCrop() && in.Effect.AutoCrop.Sensor == target {
				refs = append(refs, models.SensorReference{
					Kind:  "composer",
					ID:    c.ID,
					Input: in.Ref,
				})
			}
		}
	}
	return refs
}

func (s *sensorService) enrichStatus(out *api.Sensor) {
	if s.pipe != nil {
		out.Status = models.ProcessStatus(s.pipe.Pool().GetStatus(pipeline.SensorPoolKey(out.ID)).State)
	}
}

func sensorToInternal(sn pipeline.Sensor) api.Sensor {
	return api.Sensor{
		ID:            sn.ID,
		Source:        sn.Source,
		Detector:      sn.Detector,
		ModelID:       sn.ModelID,
		Mode:          sn.Mode,
		Margin:        sn.Margin,
		MinConfidence: sn.MinConfidence,
		TickMs:        sn.TickMs,
		CreatedAt:     sn.CreatedAt,
		UpdatedAt:     sn.UpdatedAt,
	}
}

func validateSensor(id, source, mode string) error {
	if strings.TrimSpace(id) == "" {
		return &api.SensorInvalidError{Message: "id is required"}
	}
	src := strings.TrimSpace(source)
	if src == "" {
		return &api.SensorInvalidError{Message: "source is required"}
	}
	if !strings.HasPrefix(src, "source:") && !strings.HasPrefix(src, "composer:") {
		return &api.SensorInvalidError{Message: "source must be a source:<id> or composer:<id> ref"}
	}
	if after, ok := strings.CutPrefix(src, "source:"); ok && after == "" {
		return &api.SensorInvalidError{Message: "source ref is missing an id"}
	}
	if after, ok := strings.CutPrefix(src, "composer:"); ok && after == "" {
		return &api.SensorInvalidError{Message: "source ref is missing an id"}
	}
	switch mode {
	case "", "auto", "propose":
	default:
		return &api.SensorInvalidError{Message: "mode must be auto or propose"}
	}
	return nil
}
