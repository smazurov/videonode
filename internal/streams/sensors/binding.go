package sensors

import (
	"strings"
	"sync"

	"github.com/smazurov/videonode/internal/logging"
	"github.com/smazurov/videonode/internal/streams/pipeline"
)

// ComposerReader reads composer specs (their inputs + auto_crop bindings).
// Satisfied by the TOML entity store.
type ComposerReader interface {
	GetComposerEntity(id string) (pipeline.Composer, bool)
}

type binding struct {
	sensorID string
	inputRef string
}

func bindKey(sensorID, inputRef string) string { return sensorID + "\x00" + inputRef }

// BindingReconciler turns composer inputs' auto_crop{sensor} effects into
// router targets: when an input selects a sensor, the sensor's crop is applied
// to that input. It is declarative + idempotent and implements
// pipeline.SensorReconciler — the pipeline calls ReconcileComposer on any
// composer change, and it adds/removes the targets that differ. It never
// touches sensor process lifecycle (that's the Lifecycle's job); a binding to a
// sensor that doesn't exist yet simply stays idle until the sensor runs.
type BindingReconciler struct {
	reader ComposerReader
	router *Router

	log logging.Logger

	mu    sync.Mutex
	owned map[string]map[string]binding // composerID -> bindKey -> binding
}

// NewBindingReconciler builds a BindingReconciler over the composer store and
// the router whose targets it maintains.
func NewBindingReconciler(reader ComposerReader, router *Router, log logging.Logger) *BindingReconciler {
	if log == nil {
		log = logging.GetLogger("sensors")
	}
	return &BindingReconciler{
		reader: reader, router: router, log: log,
		owned: make(map[string]map[string]binding),
	}
}

// sensorIDFromRef extracts "<id>" from a "sensor:<id>" ref, or "" if malformed.
func sensorIDFromRef(ref string) string {
	id := strings.TrimPrefix(ref, "sensor:")
	if id == ref || id == "" {
		return ""
	}
	return id
}

// ReconcileComposer reconciles the auto_crop bindings for one composer against
// the router.
func (b *BindingReconciler) ReconcileComposer(composerID string) {
	comp, ok := b.reader.GetComposerEntity(composerID)
	desired := map[string]binding{}
	if ok {
		for _, in := range comp.Inputs {
			if !in.Effect.IsAutoCrop() {
				continue
			}
			sid := sensorIDFromRef(in.Effect.AutoCrop.Sensor)
			if sid == "" {
				b.log.Warn("sensors: auto_crop effect missing sensor ref",
					logging.KeyComposerID, composerID, logging.KeyRef, in.Ref)
				continue
			}
			desired[bindKey(sid, in.Ref)] = binding{sensorID: sid, inputRef: in.Ref}
		}
	}

	b.mu.Lock()
	defer b.mu.Unlock()
	current := b.owned[composerID]
	if current == nil {
		current = map[string]binding{}
	}
	for k, bd := range current {
		if _, want := desired[k]; !want {
			b.router.RemoveTarget(bd.sensorID, composerID, bd.inputRef)
			delete(current, k)
		}
	}
	for k, bd := range desired {
		if _, have := current[k]; have {
			continue
		}
		b.router.AddTarget(bd.sensorID, composerID, bd.inputRef)
		current[k] = bd
	}
	if len(current) == 0 {
		delete(b.owned, composerID)
	} else {
		b.owned[composerID] = current
	}
}
