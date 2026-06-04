package sensors

import (
	"testing"

	"github.com/smazurov/videonode/internal/streams/pipeline"
)

type fakeSensorReader struct{ sensors map[string]pipeline.Sensor }

func (f *fakeSensorReader) GetSensorEntity(id string) (pipeline.Sensor, bool) {
	s, ok := f.sensors[id]
	return s, ok
}

func (f *fakeSensorReader) ListSensorEntities() []pipeline.Sensor {
	out := make([]pipeline.Sensor, 0, len(f.sensors))
	for _, s := range f.sensors {
		out = append(out, s)
	}
	return out
}

type fakeProv struct {
	ensured map[string]bool
	deleted map[string]bool
}

func (f *fakeProv) EnsureAnalysisComposer(sensorID, _ string) (string, error) {
	if f.ensured == nil {
		f.ensured = map[string]bool{}
	}
	f.ensured[sensorID] = true
	return "/tmp/vn-bus-composer-" + sensorID + ".sock", nil
}

func (f *fakeProv) DeleteAnalysisComposer(sensorID string) error {
	if f.deleted == nil {
		f.deleted = map[string]bool{}
	}
	f.deleted[sensorID] = true
	return nil
}

type fakeProc struct {
	started map[string]*pipeline.SensorStage
	stopped map[string]bool
}

func (f *fakeProc) StartSensor(s *pipeline.SensorStage) error {
	if f.started == nil {
		f.started = map[string]*pipeline.SensorStage{}
	}
	f.started[s.SensorID] = s
	return nil
}

func (f *fakeProc) StopSensor(id string) error {
	if f.stopped == nil {
		f.stopped = map[string]bool{}
	}
	f.stopped[id] = true
	return nil
}

func TestLifecycleEnsuresAndConfigures(t *testing.T) {
	reader := &fakeSensorReader{sensors: map[string]pipeline.Sensor{
		"playfield": {ID: "playfield", Source: "source:cam", Detector: "uv run d.py", Mode: "auto"},
	}}
	prov := &fakeProv{}
	proc := &fakeProc{}
	router := NewRouter(&fakeApplier{}, nil, nil, nil)
	lc := NewLifecycle(reader, prov, proc, router, "/bin/videonode-sensor", "uv run fallback.py", "model-x", nil)

	lc.ReconcileSensor("playfield")
	if !prov.ensured["playfield"] {
		t.Fatal("analysis composer not provisioned")
	}
	stage, ok := proc.started["playfield"]
	if !ok {
		t.Fatal("sensor process not started")
	}
	if stage.ScmPath == "" || stage.TargetRef != "source:cam" || stage.Detector != "uv run d.py" {
		t.Fatalf("stage mis-wired: %+v", stage)
	}
	if router.state["playfield"] == nil {
		t.Fatal("router policy not configured")
	}
}

func TestLifecycleDetectorFallback(t *testing.T) {
	reader := &fakeSensorReader{sensors: map[string]pipeline.Sensor{
		"s1": {ID: "s1", Source: "composer:tap"},
	}}
	proc := &fakeProc{}
	lc := NewLifecycle(reader, &fakeProv{}, proc, NewRouter(&fakeApplier{}, nil, nil, nil),
		"bin", "uv run fallback.py", "", nil)

	lc.ReconcileSensor("s1")
	if got := proc.started["s1"].Detector; got != "uv run fallback.py" {
		t.Fatalf("expected daemon-default detector, got %q", got)
	}
}

func TestLifecycleRemoveTearsDown(t *testing.T) {
	reader := &fakeSensorReader{sensors: map[string]pipeline.Sensor{
		"s1": {ID: "s1", Source: "source:cam"},
	}}
	prov := &fakeProv{}
	proc := &fakeProc{}
	router := NewRouter(&fakeApplier{}, nil, nil, nil)
	lc := NewLifecycle(reader, prov, proc, router, "bin", "det", "", nil)

	lc.ReconcileSensor("s1")
	lc.Remove("s1")
	if !proc.stopped["s1"] || !prov.deleted["s1"] {
		t.Fatalf("teardown incomplete: stopped=%v deleted=%v", proc.stopped["s1"], prov.deleted["s1"])
	}
	if router.state["s1"] != nil {
		t.Fatal("router policy not removed")
	}
}

// A sensor absent from the store is torn down by ReconcileSensor.
func TestLifecycleReconcileGoneTearsDown(t *testing.T) {
	reader := &fakeSensorReader{sensors: map[string]pipeline.Sensor{
		"s1": {ID: "s1", Source: "source:cam"},
	}}
	prov := &fakeProv{}
	proc := &fakeProc{}
	lc := NewLifecycle(reader, prov, proc, NewRouter(&fakeApplier{}, nil, nil, nil), "bin", "det", "", nil)

	lc.ReconcileSensor("s1")
	delete(reader.sensors, "s1")
	lc.ReconcileSensor("s1")
	if !proc.stopped["s1"] {
		t.Fatal("sensor not stopped when removed from store")
	}
}
