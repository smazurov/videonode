package pipeline

import (
	"net"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/smazurov/videonode/internal/metrics/collectors"
	"github.com/smazurov/videonode/internal/process"
)

type encoderHealthPool struct {
	*recordingPool
	info process.Info
}

func (p *encoderHealthPool) GetStatus(string) *process.Info { return &p.info }

func TestEncoderHealth_UsesCurrentRunProgress(t *testing.T) {
	if runtime.GOOS == "darwin" {
		t.Skip("Unix socket temp paths can exceed macOS limits")
	}
	start := time.Now()
	pool := &encoderHealthPool{
		recordingPool: newRecordingPool(),
		info:          process.Info{ID: EncoderIDFor("s1"), State: process.StateStarting, StartedAt: start},
	}
	p := newPipelineWithPool(pool)
	path := filepath.Join(t.TempDir(), "progress.sock")
	c := collectors.NewFFmpegCollector(path, t.Name())
	if err := c.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	defer c.Stop()
	p.collectors["s1"] = c
	p.onStateChange(pool.info.ID, process.StateIdle, process.StateStarting, nil)

	conn, err := net.Dial("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if _, err = conn.Write([]byte("frame=30\nfps=30\nprogress=continue\n")); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for p.EncoderHealth("s1", time.Now()).FFmpeg.Progress.Count == 0 {
		if time.Now().After(deadline) {
			t.Fatal("current-run progress never arrived")
		}
		time.Sleep(time.Millisecond)
	}
	pool.info.State = process.StateRunning
	if got := p.EncoderHealth("s1", time.Now()); got.State != "running" || got.StartedAtUS != start.UnixMicro() || got.FFmpeg.Frames.Count != 30 {
		t.Fatalf("encoder health = %+v", got)
	}

	// Pool status can change before the callback runs; do not expose old-run stats.
	pool.info.StartedAt = start.Add(time.Second)
	pool.info.State = process.StateStarting
	if got := p.EncoderHealth("s1", time.Now()); got.FFmpeg.Progress.Count != 0 {
		t.Fatalf("old run leaked: %+v", got)
	}
	p.onStateChange(pool.info.ID, process.StateIdle, process.StateStarting, nil)
	if got := p.EncoderHealth("s1", time.Now()); got.FFmpeg.Frames.State != "never_received" {
		t.Fatalf("run was not reset: %+v", got)
	}

	for _, state := range []process.State{process.StateIdle, process.StateStopping, process.StateError} {
		pool.info.State = state
		if got := p.EncoderHealth("s1", time.Now()); got.State != string(state) || got.FFmpeg.Progress.Count != 0 {
			t.Fatalf("inactive encoder health = %+v", got)
		}
	}
	if len(pool.starts) != 0 {
		t.Fatal("reading health started an idle encoder")
	}
}
