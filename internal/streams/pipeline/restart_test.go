package pipeline

import (
	"errors"
	"slices"
	"testing"

	"github.com/smazurov/videonode/internal/process"
)

// An idle encoder must NOT be bounced — the lazy lifecycle keeps a reader-less
// encoder down rather than spawning a consumer-less process.
func TestRestartEncoder_IdleIsNoop(t *testing.T) {
	pool := newRecordingPool()
	p := newPipelineWithPool(pool)

	if err := p.restartEncoder("s1"); err != nil {
		t.Fatalf("restartEncoder: %v", err)
	}
	if len(pool.restarts) != 0 {
		t.Errorf("idle encoder must not be restarted; restarts=%v", pool.restarts)
	}
}

// A running encoder is bounced through the pool.
func TestRestartEncoder_RunningGetsRestarted(t *testing.T) {
	pool := newRecordingPool()
	p := newPipelineWithPool(pool)
	pool.running[EncoderIDFor("s1")] = true

	if err := p.restartEncoder("s1"); err != nil {
		t.Fatalf("restartEncoder: %v", err)
	}
	if len(pool.restarts) != 1 || pool.restarts[0] != EncoderIDFor("s1") {
		t.Errorf("running encoder must be restarted; restarts=%v", pool.restarts)
	}
}

// A crashed (error-state) encoder is revived via a pool restart, not left down.
func TestRestartEncoder_ErrorGetsRevived(t *testing.T) {
	pool := newRecordingPool()
	p := newPipelineWithPool(pool)
	pool.states[EncoderIDFor("s1")] = process.StateError

	if err := p.restartEncoder("s1"); err != nil {
		t.Fatalf("restartEncoder: %v", err)
	}
	if len(pool.restarts) != 1 {
		t.Errorf("crashed encoder must be revived; restarts=%v", pool.restarts)
	}
}

// RestartProcess routes a producer id to ApplySource, which bounces the stage
// through the pool (start recorded; a test-mode source needs no device).
func TestRestartProcess_ProducerReapplies(t *testing.T) {
	pool := newRecordingPool()
	p := newPipelineWithPool(pool)
	storeOf(p).sources["cam2"] = Source{ID: "cam2", TestMode: true}

	if err := p.RestartProcess("producer:cam2"); err != nil {
		t.Fatalf("RestartProcess(producer): %v", err)
	}
	if !slices.Contains(pool.starts, SourcePoolKey("cam2")) {
		t.Errorf("producer restart must start the source stage; starts=%v", pool.starts)
	}
}

// RestartProcess routes a composer id to ApplyComposer.
func TestRestartProcess_ComposerReapplies(t *testing.T) {
	pool := newRecordingPool()
	p := newPipelineWithPool(pool)
	storeOf(p).composers["main"] = Composer{ID: "main", Canvas: CanvasDims{W: 1920, H: 1080}}

	if err := p.RestartProcess("composer:main"); err != nil {
		t.Fatalf("RestartProcess(composer): %v", err)
	}
	if !slices.Contains(pool.starts, ComposerPoolKey("main")) {
		t.Errorf("composer restart must start the composer stage; starts=%v", pool.starts)
	}
}

// RestartProcess returns ErrNoSuchProcess for ids the pipeline isn't supervising.
func TestRestartProcess_UnknownIsNotFound(t *testing.T) {
	pool := newRecordingPool()
	p := newPipelineWithPool(pool)

	for _, id := range []string{"producer:missing", "composer:missing", "encoder:missing", "bogus:x", "nocolon"} {
		if err := p.RestartProcess(id); !errors.Is(err, ErrNoSuchProcess) {
			t.Errorf("RestartProcess(%q) = %v; want ErrNoSuchProcess", id, err)
		}
	}
}
