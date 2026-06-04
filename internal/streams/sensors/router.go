package sensors

import (
	"sync"

	"github.com/smazurov/videonode/internal/logging"
)

// slog attribute keys for the per-finding debug log line.
const (
	keyFrame      = "frame"
	keyKind       = "kind"
	keyConfidence = "confidence"
	keyBBox       = "bbox"
	keyDecision   = "decision"
	keyCrop       = "crop"
	keyApplied    = "applied"
	keyTargets    = "targets"
)

// Finding is the daemon-native perception unit the router consumes; the main
// wiring adapts pipelinectl.FindingParams into this shape so the sensors
// package stays decoupled from the generated proto.
type Finding struct {
	SensorID   string
	ModelID    string
	TargetRef  string
	FrameIdx   uint64
	Confidence float64
	Kind       string // "bbox" | "none"
	BBox       BBox
}

// FindingEvent is the observable record emitted for every finding the router
// processes: the raw detection plus what the policy decided. It is the unit of
// debuggability — logged on every finding and published to the event bus so
// the UI / `curl /api/events` can watch a sensor work live, including while it
// runs unattached (no binding), in which case ComposerID/InputRef are empty.
type FindingEvent struct {
	SensorID   string  `json:"sensor_id"`
	ModelID    string  `json:"model_id"`
	ComposerID string  `json:"composer_id,omitempty"`
	InputRef   string  `json:"input_ref,omitempty"`
	TargetRef  string  `json:"target_ref"`
	FrameIdx   uint64  `json:"frame_idx"`
	Kind       string  `json:"kind"`
	Confidence float64 `json:"confidence"`
	BBox       *BBox   `json:"bbox,omitempty"`
	// Decision: "hold" | "crop" | "widen" (+ " (propose)" when not auto-applied).
	Decision string `json:"decision"`
	Crop     Crop   `json:"crop"`
	Mode     string `json:"mode"`
	Applied  bool   `json:"applied"`
}

// FindingObserver receives a FindingEvent for every processed finding; main
// wiring points it at the event registry (nil disables publishing).
type FindingObserver func(FindingEvent)

// CropApplier applies a committed crop to a display composer's input slot
// (aspect_ratio_mode=crop). Implemented over ComposerService in main wiring.
type CropApplier interface {
	ApplyCrop(composerID, inputRef string, crop Crop) error
}

// ProposalSink receives proposed crops in propose mode (the operator confirms
// before they go live). Optional; nil disables proposals (mode falls back to
// holding).
type ProposalSink func(sensorID, composerID, inputRef string, crop Crop)

// target is one composer input a sensor's crop is applied to.
type target struct {
	composerID string
	inputRef   string
}

func targetKey(composerID, inputRef string) string { return composerID + "\x00" + inputRef }

// sensorState holds a sensor's per-sensor commit policy plus the set of
// composer-input targets its crop fans out to. The committer and mode come
// from the Sensor entity (Configure); the targets come from composer auto_crop
// bindings (AddTarget/RemoveTarget). Findings for one sensor arrive on a single
// goroutine, so the committer is touched single-threaded.
type sensorState struct {
	committer *Committer
	mode      string // "auto" | "propose"
	targets   map[string]target
}

// Router maps each sensor's findings to a crop on the composer inputs bound to
// it. Policy (committer + mode) is per-sensor and owned by the sensor entity;
// bindings are plain targets. Every finding is logged + emitted as a
// FindingEvent — including for unattached sensors — so the pipeline is
// observable on every frame.
type Router struct {
	mu         sync.Mutex
	state      map[string]*sensorState
	applier    CropApplier
	onProposal ProposalSink
	observe    FindingObserver
	log        logging.Logger
}

// NewRouter builds a Router; onProposal and observe may be nil.
func NewRouter(applier CropApplier, onProposal ProposalSink, observe FindingObserver, log logging.Logger) *Router {
	if log == nil {
		log = logging.GetLogger("sensors")
	}
	return &Router{
		state:      make(map[string]*sensorState),
		applier:    applier,
		onProposal: onProposal,
		observe:    observe,
		log:        log,
	}
}

func (r *Router) ensureState(sensorID string) *sensorState {
	st := r.state[sensorID]
	if st == nil {
		st = &sensorState{committer: DefaultCommitter(), mode: "auto", targets: map[string]target{}}
		r.state[sensorID] = st
	}
	return st
}

// Configure sets (or replaces) a sensor's commit policy, preserving any
// existing bindings. Called by the sensor lifecycle on create/update with a
// committer + mode derived from the Sensor entity.
func (r *Router) Configure(sensorID string, c *Committer, mode string) {
	if mode == "" {
		mode = "auto"
	}
	if c == nil {
		c = DefaultCommitter()
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	st := r.ensureState(sensorID)
	st.committer = c
	st.mode = mode
}

// RemoveSensor drops a sensor's policy + bindings entirely (on sensor delete).
func (r *Router) RemoveSensor(sensorID string) {
	r.mu.Lock()
	delete(r.state, sensorID)
	r.mu.Unlock()
}

// AddTarget binds the sensor's crop to a composer input. Idempotent. If the
// sensor has not been Configure'd yet, a default policy is installed so the
// binding still works; the lifecycle's later Configure preserves the target.
func (r *Router) AddTarget(sensorID, composerID, inputRef string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	st := r.ensureState(sensorID)
	st.targets[targetKey(composerID, inputRef)] = target{composerID: composerID, inputRef: inputRef}
}

// RemoveTarget unbinds a single composer input from the sensor. Idempotent.
func (r *Router) RemoveTarget(sensorID, composerID, inputRef string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if st := r.state[sensorID]; st != nil {
		delete(st.targets, targetKey(composerID, inputRef))
	}
}

// OnFinding is the handler wired to the control plane's finding stream. It runs
// the sensor's commit policy once, fans the result out to every bound target
// (apply or propose), and ALWAYS logs + emits a FindingEvent so the sensor is
// observable on every frame — bound or not.
func (r *Router) OnFinding(f Finding) {
	r.mu.Lock()
	st := r.state[f.SensorID]
	var (
		committer *Committer
		mode      string
		targets   []target
	)
	if st != nil {
		committer = st.committer
		mode = st.mode
		targets = make([]target, 0, len(st.targets))
		for _, t := range st.targets {
			targets = append(targets, t)
		}
	}
	r.mu.Unlock()

	if committer == nil {
		committer = DefaultCommitter()
		mode = "auto"
	}

	var (
		crop    Crop
		changed bool
	)
	if f.Kind == "bbox" {
		crop, changed = committer.Observe(f.BBox, f.Confidence)
	} else {
		crop, changed = committer.Observe(BBox{}, 0)
	}

	decision := "hold"
	if changed {
		if crop == WideCrop {
			decision = "widen"
		} else {
			decision = "crop"
		}
	}

	var bb *BBox
	if f.Kind == "bbox" {
		x := f.BBox
		bb = &x
	}

	appliedAny := false
	emit := func(t target, applied bool, dec string) {
		if r.observe == nil {
			return
		}
		r.observe(FindingEvent{
			SensorID: f.SensorID, ModelID: f.ModelID, ComposerID: t.composerID,
			InputRef: t.inputRef, TargetRef: f.TargetRef, FrameIdx: f.FrameIdx,
			Kind: f.Kind, Confidence: f.Confidence, BBox: bb,
			Decision: dec, Crop: crop, Mode: mode, Applied: applied,
		})
	}

	for _, t := range targets {
		dec := decision
		applied := false
		if changed {
			if mode == "propose" {
				dec += " (propose)"
				if r.onProposal != nil {
					r.onProposal(f.SensorID, t.composerID, t.inputRef, crop)
				}
			} else if err := r.applier.ApplyCrop(t.composerID, t.inputRef, crop); err != nil {
				r.log.Warn("sensors: apply crop failed",
					logging.KeySensorID, f.SensorID, logging.KeyComposerID, t.composerID,
					logging.KeyError, err)
			} else {
				applied = true
				appliedAny = true
			}
		}
		emit(t, applied, dec)
	}
	if len(targets) == 0 {
		emit(target{}, false, decision)
	}

	// Every frame still emits a FindingEvent on the SSE bus (above), so the
	// sensor stays fully observable live. Only an actionable finding — a crop/
	// widen decision or an applied crop — is worth an Info line; the steady-
	// state per-frame "hold" goes to Debug so it doesn't flood the log.
	logFinding := r.log.Debug
	if changed || appliedAny {
		logFinding = r.log.Info
	}
	logFinding("sensor finding",
		logging.KeySensorID, f.SensorID, keyFrame, f.FrameIdx, keyKind, f.Kind,
		keyConfidence, f.Confidence, keyBBox, bboxStr(bb), keyDecision, decision,
		keyCrop, cropStr(crop), keyTargets, len(targets), keyApplied, appliedAny)
}
