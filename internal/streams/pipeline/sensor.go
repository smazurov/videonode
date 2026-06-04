package pipeline

import (
	"errors"
	"log/slog"
	"strconv"

	"github.com/smazurov/videonode/internal/ffmpeg"
	"github.com/smazurov/videonode/internal/logging"
	"github.com/smazurov/videonode/internal/process"
)

// SensorStage is the per-Sensor supervised `videonode-sensor`
// process. Keyed by SensorID. Dials an analysis composer's NV12 canvas
// SCM bus (ScmPath), runs an out-of-process detector child, and streams
// normalized Findings to the daemon over the per-instance gRPC control
// plane (Sensor.StreamFindings). Perception-only — it never actuates;
// the daemon's BindingRouter maps findings to a crop on the display
// composer.
type SensorStage struct {
	SensorID   string
	BinaryPath string
	GrpcUds    string // per-instance gRPC UDS the daemon dials
	ScmPath    string // analysis composer canvas SCM socket to dial
	TargetRef  string // "source:<id>" the findings pertain to
	ModelID    string
	Detector   string // detector child shell command (run under /bin/sh -c)
	TickMs     int    // periodic re-detect cadence; 0 → binary default
}

// SensorReconciler reconciles auto_crop sensor wiring (analysis-composer
// tap + videonode-sensor process + findings→crop binding) against a
// composer's current inputs. It is declarative and idempotent: called on any
// input-effect change, it provisions wiring for inputs that carry an auto_crop
// effect and tears down wiring for inputs that no longer do. Implemented by
// internal/streams/sensors and injected via pipeline.Config.
type SensorReconciler interface {
	ReconcileComposer(composerID string)
}

// SensorPoolKey returns the pool.Pool key for an sensor id. Stable
// across restarts.
func SensorPoolKey(sensorID string) string { return "sensor:" + sensorID }

// ID returns the stage's process.Pool key.
func (a *SensorStage) ID() string { return SensorPoolKey(a.SensorID) }

// Kind reports this as an Sensor stage.
func (a *SensorStage) Kind() Kind { return KindSensor }

// StreamID returns "" — sensors are perception sidecars, not stream-scoped.
func (a *SensorStage) StreamID() string { return "" }

// Command returns the videonode-sensor argv.
func (a *SensorStage) Command() ([]string, []string, error) {
	if a.BinaryPath == "" {
		return nil, nil, errors.New("sensor: BinaryPath is required")
	}
	if a.SensorID == "" {
		return nil, nil, errors.New("sensor: SensorID is required")
	}
	if a.ScmPath == "" {
		return nil, nil, errors.New("sensor: ScmPath is required")
	}
	if a.Detector == "" {
		return nil, nil, errors.New("sensor: Detector command is required")
	}
	argv := []string{
		a.BinaryPath,
		"--sensor-id", a.SensorID,
		"--upstream-scm", a.ScmPath,
		"--detector", a.Detector,
	}
	if a.GrpcUds != "" {
		argv = append(argv, "--grpc-listen", a.GrpcUds)
	}
	if a.TargetRef != "" {
		argv = append(argv, "--target-ref", a.TargetRef)
	}
	if a.ModelID != "" {
		argv = append(argv, "--model-id", a.ModelID)
	}
	if a.TickMs > 0 {
		argv = append(argv, "--tick-ms", strconv.Itoa(a.TickMs))
	}
	return argv, nil, nil
}

// LogParser uses the ffmpeg parser — videonode-sensor emits the same
// `[level] msg` format via vn::log helpers.
func (a *SensorStage) LogParser() process.LogParser { return ffmpeg.ParseLogLine }

// LogAttrs tags sensor logs with the sensor id + pool-key instance.
func (a *SensorStage) LogAttrs() []slog.Attr {
	return []slog.Attr{
		slog.String(logging.KeySensorID, a.SensorID),
		slog.String(logging.KeyStageInstance, a.ID()),
	}
}
