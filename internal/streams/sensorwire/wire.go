// Package sensorwire connects the decoupled sensors subsystem (lifecycle,
// binding reconciler, router, follow/commit policy) to the live daemon: it
// adapts api.ComposerService into the Provisioner + CropApplier the subsystem
// needs, adapts the pipeline into the SensorProcess, and adapts pipelinectl
// Findings into the router's input. main.go calls Build once and injects the
// results: the Lifecycle into the SensorService, the BindingReconciler into the
// pipeline (SetSensorReconciler), and the finding handler into pipelinectl.
package sensorwire

import (
	"context"
	"fmt"

	"github.com/smazurov/videonode/internal/api"
	"github.com/smazurov/videonode/internal/api/models"
	"github.com/smazurov/videonode/internal/logging"
	"github.com/smazurov/videonode/internal/streams/pipeline"
	"github.com/smazurov/videonode/internal/streams/pipelinectl"
	"github.com/smazurov/videonode/internal/streams/sensors"
)

// Config carries the sensor subsystem's static settings.
type Config struct {
	SensorBin string // videonode-sensor path
	Detector  string // daemon-default detector child command
	ModelID   string // daemon-default model id
	AnalysisW int    // analysis composer canvas width (default 480)
	AnalysisH int    // analysis composer canvas height (default 270)
}

// EntityReader is the read surface the subsystem needs over the entity store:
// sensors (lifecycle) + composers (binding).
type EntityReader interface {
	sensors.SensorReader
	sensors.ComposerReader
}

// procAdapter satisfies sensors.SensorProcess over the pipeline.
type procAdapter struct{ pipe *pipeline.Pipeline }

// StartSensor spawns + registers the sensor process.
func (a procAdapter) StartSensor(s *pipeline.SensorStage) error { return a.pipe.ApplySensor(s) }

// StopSensor stops + unregisters the sensor process.
func (a procAdapter) StopSensor(id string) error { return a.pipe.RemoveSensor(id) }

// provAdapter satisfies sensors.Provisioner: it manages a daemon-owned
// passthrough analysis composer (id "sensor-<id>") tapping the observed ref.
type provAdapter struct {
	svc  api.ComposerService
	w, h int
}

// EnsureAnalysisComposer creates the passthrough analysis composer if absent
// and returns its canvas SCM socket path.
func (p provAdapter) EnsureAnalysisComposer(sensorID, sourceRef string) (string, error) {
	ctx := context.Background()
	tapID := sensors.AnalysisComposerID(sensorID)
	scm := pipeline.SCMSocketPathFor("composer-" + tapID)
	if _, err := p.svc.GetComposer(ctx, tapID); err == nil {
		return scm, nil
	}
	_, err := p.svc.CreateComposer(ctx, models.ComposerCreateRequestData{
		ID:     tapID,
		Canvas: models.CanvasDimsData{W: p.w, H: p.h, FPS: 5},
		Inputs: []models.ComposerInputData{{Ref: sourceRef}},
		Layout: []models.LayoutSlotData{{
			Input: sourceRef, X: 0, Y: 0, W: p.w, H: p.h, AspectRatioMode: "stretch",
		}},
	})
	if err != nil {
		return "", err
	}
	return scm, nil
}

// DeleteAnalysisComposer removes the analysis composer for a sensor.
func (p provAdapter) DeleteAnalysisComposer(sensorID string) error {
	return p.svc.DeleteComposer(context.Background(), sensors.AnalysisComposerID(sensorID))
}

// cropAdapter satisfies sensors.CropApplier: it patches the display
// composer's slot for the input into aspect_ratio_mode=crop with the committed
// crop, via ReplaceLayout.
type cropAdapter struct{ svc api.ComposerService }

// ApplyCrop patches the display composer's slot for inputRef into crop mode.
func (c cropAdapter) ApplyCrop(composerID, inputRef string, crop sensors.Crop) error {
	ctx := context.Background()
	comp, err := c.svc.GetComposer(ctx, composerID)
	if err != nil {
		return err
	}
	layout := append([]models.LayoutSlotData(nil), comp.Layout...)
	found := false
	for i := range layout {
		if layout[i].Input != inputRef {
			continue
		}
		layout[i].AspectRatioMode = "crop"
		layout[i].Crop = &models.CropConfigData{X: crop.X, Y: crop.Y, Scale: crop.Scale}
		found = true
	}
	if !found {
		return fmt.Errorf("sensorwire: no layout slot for input %q in composer %q", inputRef, composerID)
	}
	_, err = c.svc.ReplaceLayout(ctx, composerID, layout)
	return err
}

// Build wires the subsystem and returns the Lifecycle (inject into the
// SensorService), the BindingReconciler (inject via
// pipeline.SetSensorReconciler), and the finding handler (inject via
// pipelinectl.Manager.SetFindingHandler). The observe hook, when non-nil,
// receives a FindingEvent for every processed finding — main wiring points it
// at the event registry so the UI can watch a sensor live.
func Build(svc api.ComposerService, pipe *pipeline.Pipeline, reader EntityReader,
	cfg Config, observe sensors.FindingObserver, log logging.Logger,
) (*sensors.Lifecycle, *sensors.BindingReconciler, func(pipelinectl.FindingParams)) {
	if cfg.AnalysisW <= 0 {
		cfg.AnalysisW = 480
	}
	if cfg.AnalysisH <= 0 {
		cfg.AnalysisH = 270
	}
	if cfg.Detector == "" {
		cfg.Detector = sensors.DefaultDetector
	}
	router := sensors.NewRouter(cropAdapter{svc}, nil, observe, log)
	lifecycle := sensors.NewLifecycle(reader, provAdapter{svc, cfg.AnalysisW, cfg.AnalysisH},
		procAdapter{pipe}, router, cfg.SensorBin, cfg.Detector, cfg.ModelID, log)
	bindings := sensors.NewBindingReconciler(reader, router, log)

	handler := func(fp pipelinectl.FindingParams) {
		router.OnFinding(sensors.Finding{
			SensorID:   fp.SensorID,
			ModelID:    fp.ModelID,
			TargetRef:  fp.TargetRef,
			FrameIdx:   fp.FrameIdx,
			Confidence: fp.Confidence,
			Kind:       fp.Kind,
			BBox:       sensors.BBox{X: fp.BBox.X, Y: fp.BBox.Y, W: fp.BBox.W, H: fp.BBox.H},
		})
	}
	return lifecycle, bindings, handler
}
