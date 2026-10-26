package collectors

import (
	"errors"
	"net"
	"path/filepath"
	"testing"
	"time"

	"github.com/smazurov/videonode/internal/metrics"
)

func TestFFmpegHealth_ReportsDoNotImplyFrames(t *testing.T) {
	c := NewFFmpegCollector("", t.Name())
	defer c.Stop()
	now := time.Now()
	c.sendProgressMetrics(map[string]string{"frame": "30", "fps": "30", "speed": "1x"}, 0, now)
	later := now.Add(metrics.HealthTimeout)
	c.sendProgressMetrics(map[string]string{"frame": "30"}, 0, later)
	h := c.Health(0, later)
	if h.Progress.State != "flowing" || h.Frames.State != "stalled" || h.Frames.Count != 30 {
		t.Fatalf("progress without frame advancement = %+v", h)
	}
	if h.Stats.Speed != 1 {
		t.Errorf("speed = %v, want 1", h.Stats.Speed)
	}
	c.sendProgressMetrics(map[string]string{"frame": "31"}, 0, later)
	if h = c.Health(0, later); h.Frames.State != "flowing" {
		t.Fatalf("new frame did not recover freshness: %+v", h)
	}
}

func TestFFmpegHealth_RunResetRejectsOldReports(t *testing.T) {
	c := NewFFmpegCollector("", t.Name())
	defer c.Stop()
	now := time.Now()
	c.sendProgressMetrics(map[string]string{"frame": "99", "fps": "30"}, 0, now)
	if err := c.BeginRun(123); err != nil {
		t.Fatal(err)
	}
	c.sendProgressMetrics(map[string]string{"frame": "100", "fps": "60"}, 0, now)
	h := c.Health(123, now)
	if h.Progress.State != "never_received" || h.Frames.Count != 0 || h.Stats.FPS != 0 {
		t.Fatalf("old run contaminated reset: %+v", h)
	}
	c.sendProgressMetrics(map[string]string{"frame": "0", "fps": "0"}, 1, now)
	h = c.Health(123, now)
	if h.Progress.State != "flowing" || h.Frames.State != "never_received" {
		t.Fatalf("frame zero must not count as advancement: %+v", h)
	}
	if c.Health(124, now).Progress.State != "never_received" {
		t.Fatal("snapshot for a different run exposed current observations")
	}
}

func TestFFmpegHealth_StopClosesSilentConnection(t *testing.T) {
	skipOnMacOS(t)
	c := NewFFmpegCollector(filepath.Join(t.TempDir(), "progress.sock"), t.Name())
	if err := c.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	conn, err := net.Dial("unix", c.socketPath)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if err := c.Stop(); err != nil {
		t.Fatal(err)
	}
	conn.SetReadDeadline(time.Now().Add(time.Second))
	var buf [1]byte
	if _, err := conn.Read(buf[:]); err == nil {
		t.Fatal("connection remained open after stopping")
	} else {
		var netErr net.Error
		if errors.As(err, &netErr) && netErr.Timeout() {
			t.Fatal("silent progress reader was not closed")
		}
	}
}

func TestFFmpegHealth_RebindDiscardsPreviousRunConnections(t *testing.T) {
	skipOnMacOS(t)
	c := NewFFmpegCollector(filepath.Join(t.TempDir(), "progress.sock"), t.Name())
	if err := c.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	defer c.Stop()
	old, err := net.Dial("unix", c.socketPath)
	if err != nil {
		t.Fatal(err)
	}
	defer old.Close()
	if _, err = old.Write([]byte("frame=999\nfps=99\nprogress=continue\n")); err != nil {
		t.Fatal(err)
	}
	if err := c.BeginRun(1); err != nil {
		t.Fatal(err)
	}
	old.SetReadDeadline(time.Now().Add(time.Second))
	var buf [1]byte
	if _, err := old.Read(buf[:]); err == nil {
		t.Fatal("old connection remained open")
	} else {
		var netErr net.Error
		if errors.As(err, &netErr) && netErr.Timeout() {
			t.Fatal("old connection timed out instead of closing")
		}
	}
	if got := c.Health(1, time.Now()); got.Progress.Count != 0 {
		t.Fatalf("old progress survived: %+v", got)
	}
	current, err := net.Dial("unix", c.socketPath)
	if err != nil {
		t.Fatal(err)
	}
	defer current.Close()
	if _, err = current.Write([]byte("frame=30\nfps=30\nprogress=continue\n")); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for c.Health(1, time.Now()).Progress.Count == 0 {
		if time.Now().After(deadline) {
			t.Fatal("new run progress never arrived")
		}
		time.Sleep(time.Millisecond)
	}
	if got := c.Health(1, time.Now()); got.Frames.Count != 30 || got.Stats.FPS != 30 {
		t.Fatalf("new run health = %+v", got)
	}
}

func TestFFmpegHealth_InvalidNumbersDoNotReplaceStats(t *testing.T) {
	c := NewFFmpegCollector("", t.Name())
	defer c.Stop()
	now := time.Now()
	c.sendProgressMetrics(map[string]string{"fps": "30", "speed": "1x"}, 0, now)
	for _, raw := range []string{"NaN", "+Inf", "-Inf", "N/A"} {
		c.sendProgressMetrics(map[string]string{"fps": raw, "speed": raw}, 0, now)
		if got := c.Health(0, now).Stats; got.FPS != 30 || got.Speed != 1 {
			t.Fatalf("invalid %q overwrote stats: %+v", raw, got)
		}
	}
}
