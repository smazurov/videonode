package sensors

import (
	"strings"
	"sync"

	"github.com/smazurov/videonode/internal/logging"
	"github.com/smazurov/videonode/internal/streams/pipeline"
)

// SensorReader reads first-class sensor entities. Satisfied by the TOML entity
// store.
type SensorReader interface {
	GetSensorEntity(id string) (pipeline.Sensor, bool)
	ListSensorEntities() []pipeline.Sensor
}

// Provisioner manages the daemon-owned analysis composer that taps a sensor's
// observed ref into a small passthrough canvas the sensor consumes. The
// returned scmPath is the analysis composer's canvas SCM socket the sensor
// dials.
type Provisioner interface {
	EnsureAnalysisComposer(sensorID, sourceRef string) (scmPath string, err error)
	DeleteAnalysisComposer(sensorID string) error
}

// SensorProcess spawns/stops the videonode-sensor process and its control
// plane registration. Satisfied by the pipeline (Pool + control manager).
type SensorProcess interface {
	StartSensor(stage *pipeline.SensorStage) error
	StopSensor(sensorID string) error
}

// DefaultDetector is the detector child command used when a sensor (and the
// daemon config) leaves it unset. Resolved against the daemon cwd. This is the
// "see/replace the Python runtime" knob, now editable per-sensor.
const DefaultDetector = "uv run sensors/playfield/detect.py"

// AnalysisComposerID is the daemon-owned tap composer id for a sensor. Distinct
// namespace ("sensor-<id>") so it can't collide with a user composer id.
func AnalysisComposerID(sensorID string) string { return "sensor-" + sensorID }

// Lifecycle reconciles a first-class sensor's runtime: the analysis-composer
// tap on its observed ref, the videonode-sensor process, and its router commit
// policy. It is declarative + idempotent: ReconcileSensor ensures the wiring
// for a sensor that exists in the store and tears down wiring for one that no
// longer does. A sensor runs whenever the pipeline switch is on — unattached it
// still emits findings + status; bindings are layered on separately by the
// BindingReconciler.
type Lifecycle struct {
	sensors   SensorReader
	prov      Provisioner
	proc      SensorProcess
	router    *Router
	sensorBin string
	detector  string // daemon default detector fallback
	modelID   string // daemon default model id fallback

	log logging.Logger

	mu      sync.Mutex
	running map[string]bool
}

// NewLifecycle builds a Lifecycle from the videonode-sensor path plus the
// daemon-default detector command and model id used when a sensor leaves them
// unset.
func NewLifecycle(sensors SensorReader, prov Provisioner, proc SensorProcess, router *Router,
	sensorBin, detector, modelID string, log logging.Logger,
) *Lifecycle {
	if log == nil {
		log = logging.GetLogger("sensors")
	}
	if modelID == "" {
		modelID = "playfield-classical-v0"
	}
	return &Lifecycle{
		sensors: sensors, prov: prov, proc: proc, router: router,
		sensorBin: sensorBin, detector: detector, modelID: modelID,
		log: log, running: make(map[string]bool),
	}
}

// ReconcileAll reconciles every sensor currently in the store (startup +
// pipeline-switch-on). Sensors absent from the store that this Lifecycle still
// runs are torn down.
func (l *Lifecycle) ReconcileAll() {
	want := map[string]bool{}
	for _, sn := range l.sensors.ListSensorEntities() {
		want[sn.ID] = true
	}
	l.mu.Lock()
	stale := make([]string, 0)
	for id := range l.running {
		if !want[id] {
			stale = append(stale, id)
		}
	}
	l.mu.Unlock()
	for id := range want {
		l.ReconcileSensor(id)
	}
	for _, id := range stale {
		l.Remove(id)
	}
}

// ReconcileSensor ensures (sensor present) or tears down (sensor gone) the
// runtime for one sensor id.
func (l *Lifecycle) ReconcileSensor(id string) {
	sn, ok := l.sensors.GetSensorEntity(id)
	if !ok {
		l.Remove(id)
		return
	}

	scmPath, err := l.prov.EnsureAnalysisComposer(id, sn.Source)
	if err != nil {
		l.log.Warn("sensors: ensure analysis composer failed",
			logging.KeySensorID, id, logging.KeyError, err)
		return
	}
	stage := &pipeline.SensorStage{
		SensorID:   id,
		BinaryPath: l.sensorBin,
		GrpcUds:    pipeline.GrpcSocketPathFor("sensor", id),
		ScmPath:    scmPath,
		TargetRef:  sn.Source,
		ModelID:    modelIDOf(sn, l.modelID),
		Detector:   detectorOf(sn, l.detector),
		TickMs:     sn.TickMs,
	}
	if err := l.proc.StartSensor(stage); err != nil {
		l.log.Warn("sensors: start sensor failed", logging.KeySensorID, id, logging.KeyError, err)
		_ = l.prov.DeleteAnalysisComposer(id)
		return
	}
	l.router.Configure(id, committerFromSensor(sn), modeOf(sn))

	l.mu.Lock()
	l.running[id] = true
	l.mu.Unlock()
}

// Remove tears down a sensor's process, analysis-composer tap, and router
// policy. No-op for unknown ids.
func (l *Lifecycle) Remove(id string) {
	l.router.RemoveSensor(id)
	if err := l.proc.StopSensor(id); err != nil {
		l.log.Warn("sensors: stop sensor failed", logging.KeySensorID, id, logging.KeyError, err)
	}
	if err := l.prov.DeleteAnalysisComposer(id); err != nil {
		l.log.Warn("sensors: delete analysis composer failed",
			logging.KeySensorID, id, logging.KeyError, err)
	}
	l.mu.Lock()
	delete(l.running, id)
	l.mu.Unlock()
}

// detectorOf resolves the detector command: per-sensor override > daemon
// default > DefaultDetector.
func detectorOf(sn pipeline.Sensor, fallback string) string {
	if strings.TrimSpace(sn.Detector) != "" {
		return sn.Detector
	}
	if fallback != "" {
		return fallback
	}
	return DefaultDetector
}

func modelIDOf(sn pipeline.Sensor, fallback string) string {
	if sn.ModelID != "" {
		return sn.ModelID
	}
	return fallback
}

func modeOf(sn pipeline.Sensor) string {
	if sn.Mode != "" {
		return sn.Mode
	}
	return "auto"
}

func committerFromSensor(sn pipeline.Sensor) *Committer {
	c := DefaultCommitter()
	if sn.Margin > 0 {
		c.Margin = sn.Margin
	}
	if sn.MinConfidence > 0 {
		c.MinConfidence = sn.MinConfidence
	}
	return c
}
