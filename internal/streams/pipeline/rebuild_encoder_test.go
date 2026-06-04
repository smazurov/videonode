package pipeline

import (
	"testing"

	"github.com/smazurov/videonode/internal/process"
)

// recordingPool is a process.Pool test double: it records Start/Stop/Restart
// calls and reports a controllable running set (and optional explicit states),
// so lazy restart behavior can be asserted without spawning real processes.
type recordingPool struct {
	running  map[string]bool
	states   map[string]process.State // explicit GetStatus override per id
	starts   []string
	stops    []string
	restarts []string
}

func newRecordingPool() *recordingPool {
	return &recordingPool{running: map[string]bool{}, states: map[string]process.State{}}
}

func (p *recordingPool) Start(id string) error {
	p.starts = append(p.starts, id)
	p.running[id] = true
	return nil
}

func (p *recordingPool) Stop(id string) error {
	p.stops = append(p.stops, id)
	p.running[id] = false
	return nil
}

func (p *recordingPool) Restart(id string) error {
	p.restarts = append(p.restarts, id)
	p.running[id] = true
	return nil
}

func (p *recordingPool) GetStatus(id string) *process.Info {
	if st, ok := p.states[id]; ok {
		return &process.Info{ID: id, State: st}
	}
	if p.running[id] {
		return &process.Info{ID: id, State: process.StateRunning}
	}
	return &process.Info{ID: id, State: process.StateIdle}
}
func (p *recordingPool) IsRunning(id string) bool { return p.running[id] }
func (p *recordingPool) SetKind(_, _ string)      {}
func (p *recordingPool) IDs() []string            { return nil }
func (p *recordingPool) StopAll()                 {}

func newPipelineWithPool(pool process.Pool) *Pipeline {
	store := newFakeStore()
	store.sources["cam"] = Source{ID: "cam", Format: &SourceFormat{Width: 1920, Height: 1080, FPS: 30}}
	p := New(Config{RTSPPort: ":8554", EntityStore: store}, nil)
	p.pool = pool
	return p
}

// storeOf reaches the fake read-through store a test pipeline was built
// with, so tests seed entities where the pipeline actually reads them.
func storeOf(p *Pipeline) *fakeEntityStore {
	return p.cfg.EntityStore.(*fakeEntityStore)
}

func rebuildStream() Stream {
	return Stream{ID: "s1", Upstream: "source:cam", Encoder: EncoderConfig{Codec: "h264"}}
}

// An idle encoder (no reader attached) must NOT be force-started — only its
// cached stage is refreshed so the next lazy spawn uses the new dims.
func TestRebuildStreamEncoder_IdleRefreshesStageWithoutStarting(t *testing.T) {
	pool := newRecordingPool()
	p := newPipelineWithPool(pool)

	if err := p.RebuildStreamEncoder(rebuildStream()); err != nil {
		t.Fatalf("RebuildStreamEncoder: %v", err)
	}
	if len(pool.starts) != 0 {
		t.Errorf("idle encoder must not be started; starts=%v", pool.starts)
	}
	if _, ok := p.stages[EncoderIDFor("s1")]; !ok {
		t.Error("cached encoder stage was not refreshed")
	}
}

// A running encoder (reader attached) must be bounced (stop+start) so the new
// `-s WxH` takes effect on the live process.
func TestRebuildStreamEncoder_RunningGetsBounced(t *testing.T) {
	pool := newRecordingPool()
	p := newPipelineWithPool(pool)
	pool.running[EncoderIDFor("s1")] = true

	if err := p.RebuildStreamEncoder(rebuildStream()); err != nil {
		t.Fatalf("RebuildStreamEncoder: %v", err)
	}
	if len(pool.stops) != 1 || len(pool.starts) != 1 {
		t.Errorf("running encoder must be bounced; stops=%v starts=%v", pool.stops, pool.starts)
	}
}
