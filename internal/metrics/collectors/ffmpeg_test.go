package collectors

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/smazurov/videonode/internal/metrics"
)

func skipOnMacOS(t *testing.T) {
	if runtime.GOOS == "darwin" {
		t.Skip("Unix socket path too long on macOS")
	}
}

func TestFFmpegCollectorProgressParsing(t *testing.T) {
	skipOnMacOS(t)
	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "ffmpeg.sock")
	streamID := "test-stream-ffmpeg"

	metrics.DeleteFFmpegMetrics(streamID)

	collector := NewFFmpegCollector(socketPath, streamID)
	ctx := t.Context()

	if err := collector.Start(ctx); err != nil {
		t.Fatalf("failed to start collector: %v", err)
	}
	defer collector.Stop()

	// Wait for socket to be created
	var conn net.Conn
	var err error
	for range 50 {
		conn, err = net.Dial("unix", socketPath)
		if err == nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("failed to connect to socket: %v", err)
	}
	defer conn.Close()

	// Send FFmpeg progress data
	progressData := `fps=29.97
drop_frames=3
dup_frames=1
speed=1.25x
progress=continue
`
	_, err = conn.Write([]byte(progressData))
	if err != nil {
		t.Fatalf("failed to write progress data: %v", err)
	}

	// Wait for metrics to be processed
	time.Sleep(50 * time.Millisecond)

	all, err := metrics.GetFFmpegMetricsFromRegistry()
	if err != nil {
		t.Fatalf("failed to get metrics: %v", err)
	}
	m := all[streamID]
	if m == nil {
		t.Fatal("expected metrics to be set")
	}

	if m.FPS != 29.97 {
		t.Errorf("FPS = %v, want 29.97", m.FPS)
	}
	if m.DroppedFrames != 3 {
		t.Errorf("DroppedFrames = %v, want 3", m.DroppedFrames)
	}
	if m.DuplicateFrames != 1 {
		t.Errorf("DuplicateFrames = %v, want 1", m.DuplicateFrames)
	}
	if m.Speed != 1.25 {
		t.Errorf("Speed = %v, want 1.25", m.Speed)
	}
}

func TestFFmpegCollectorMultipleProgressUpdates(t *testing.T) {
	skipOnMacOS(t)
	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "ffmpeg2.sock")
	streamID := "test-stream-ffmpeg-multi"

	metrics.DeleteFFmpegMetrics(streamID)

	collector := NewFFmpegCollector(socketPath, streamID)
	ctx := t.Context()

	if err := collector.Start(ctx); err != nil {
		t.Fatalf("failed to start collector: %v", err)
	}
	defer collector.Stop()

	var conn net.Conn
	var err error
	for range 50 {
		conn, err = net.Dial("unix", socketPath)
		if err == nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("failed to connect to socket: %v", err)
	}
	defer conn.Close()

	// First progress update
	n, err := conn.Write([]byte("fps=30\nprogress=continue\n"))
	if err != nil {
		t.Fatalf("failed to write first update: %v", err)
	}
	if n == 0 {
		t.Fatal("wrote 0 bytes")
	}
	time.Sleep(30 * time.Millisecond)

	all, err := metrics.GetFFmpegMetricsFromRegistry()
	if err != nil {
		t.Fatalf("failed to get metrics: %v", err)
	}
	m := all[streamID]
	if m == nil || m.FPS != 30 {
		t.Errorf("first update: FPS = %v, want 30", m)
	}

	// Second progress update
	n, err = conn.Write([]byte("fps=60\nprogress=continue\n"))
	if err != nil {
		t.Fatalf("failed to write second update: %v", err)
	}
	if n == 0 {
		t.Fatal("wrote 0 bytes")
	}
	time.Sleep(30 * time.Millisecond)

	all, err = metrics.GetFFmpegMetricsFromRegistry()
	if err != nil {
		t.Fatalf("failed to get metrics: %v", err)
	}
	m = all[streamID]
	if m == nil || m.FPS != 60 {
		t.Errorf("second update: FPS = %v, want 60", m)
	}
}

func TestFFmpegCollectorStop(t *testing.T) {
	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "ffmpeg3.sock")
	streamID := "test-stream-ffmpeg-stop"

	metrics.DeleteFFmpegMetrics(streamID)

	collector := NewFFmpegCollector(socketPath, streamID)
	ctx := t.Context()

	if err := collector.Start(ctx); err != nil {
		t.Fatalf("failed to start collector: %v", err)
	}

	// Wait for socket
	for range 50 {
		if _, err := os.Stat(socketPath); err == nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	// Set some metrics
	metrics.SetFFmpegFPS(streamID, 30)

	// Stop should clean up
	if err := collector.Stop(); err != nil {
		t.Errorf("stop returned error: %v", err)
	}

	// Metrics should be deleted
	all, err := metrics.GetFFmpegMetricsFromRegistry()
	if err != nil {
		t.Fatalf("failed to get metrics: %v", err)
	}
	if all[streamID] != nil {
		t.Error("expected metrics to be deleted after stop")
	}

	// Socket file should be removed
	time.Sleep(20 * time.Millisecond)
	if _, err := os.Stat(socketPath); !os.IsNotExist(err) {
		t.Error("expected socket file to be removed")
	}
}

func TestFFmpegCollectorCleanupOldSocket(t *testing.T) {
	skipOnMacOS(t)
	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "ffmpeg4.sock")
	streamID := "test-stream-ffmpeg-cleanup"

	// Create a stale socket file
	f, err := os.Create(socketPath)
	if err != nil {
		t.Fatalf("failed to create stale socket: %v", err)
	}
	f.Close()

	collector := NewFFmpegCollector(socketPath, streamID)
	ctx := t.Context()

	if err := collector.Start(ctx); err != nil {
		t.Fatalf("failed to start collector: %v", err)
	}
	defer collector.Stop()

	// Should be able to connect (old socket was cleaned up)
	var conn net.Conn
	for range 50 {
		conn, err = net.Dial("unix", socketPath)
		if err == nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("failed to connect after cleanup: %v", err)
	}
	conn.Close()
}

func TestFFmpegCollectorHandleConnection(t *testing.T) {
	skipOnMacOS(t)
	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "ffmpeg5.sock")
	streamID := "test-stream-handle-conn"

	metrics.DeleteFFmpegMetrics(streamID)

	collector := NewFFmpegCollector(socketPath, streamID)
	ctx := t.Context()

	if err := collector.Start(ctx); err != nil {
		t.Fatalf("failed to start collector: %v", err)
	}
	defer collector.Stop()

	// Wait for socket
	var conn net.Conn
	var dialErr error
	for range 50 {
		conn, dialErr = net.Dial("unix", socketPath)
		if dialErr == nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if dialErr != nil {
		t.Fatalf("failed to connect: %v", dialErr)
	}

	// Test various edge cases
	testCases := []string{
		"",                  // empty line
		"fps=invalid",       // valid key but parsed as 0
		"no_equals_sign",    // no equals sign
		"  fps = 25.0  ",    // whitespace
		"progress=continue", // trigger metrics send
	}

	for _, tc := range testCases {
		_, writeErr := fmt.Fprintln(conn, tc)
		if writeErr != nil {
			t.Logf("write failed (may be expected): %v", writeErr)
		}
	}

	conn.Close()
	time.Sleep(30 * time.Millisecond)
}

func TestFPSTracker(t *testing.T) {
	t.Run("first sample returns no value", func(t *testing.T) {
		var tr fpsTracker
		start := time.Unix(0, 0)
		if v, ok := tr.update(100, start); ok {
			t.Errorf("first sample should not yield a value, got %v", v)
		}
	})

	t.Run("steady 30fps converges to 30", func(t *testing.T) {
		var tr fpsTracker
		start := time.Unix(0, 0)
		tr.update(0, start)
		// 0.5s blocks, 15 frames per block = 30 fps.
		var last float64
		for i := 1; i <= 8; i++ {
			now := start.Add(time.Duration(i) * 500 * time.Millisecond)
			v, ok := tr.update(uint64(i*15), now)
			if !ok {
				t.Fatalf("update %d returned ok=false", i)
			}
			last = v
		}
		if diff := last - 30.0; diff < -0.01 || diff > 0.01 {
			t.Errorf("steady-state fps = %v, want ~30", last)
		}
	})

	t.Run("first valid sample seeds without smoothing", func(t *testing.T) {
		var tr fpsTracker
		start := time.Unix(0, 0)
		tr.update(0, start)
		v, ok := tr.update(60, start.Add(time.Second))
		if !ok || v != 60.0 {
			t.Errorf("first seeded sample = %v ok=%v, want 60 true", v, ok)
		}
	})

	t.Run("EMA reacts to drop within a few ticks", func(t *testing.T) {
		var tr fpsTracker
		start := time.Unix(0, 0)
		tr.update(0, start)
		// Warm up at 30 fps for 6 blocks.
		for i := 1; i <= 6; i++ {
			tr.update(uint64(i*15), start.Add(time.Duration(i)*500*time.Millisecond))
		}
		// Drop to 10 fps; check value after 3 more 0.5s blocks (~1.5s).
		frame := uint64(6 * 15)
		var v float64
		var ok bool
		for i := 1; i <= 3; i++ {
			frame += 5 // 5 frames in 0.5s = 10 fps
			now := start.Add(time.Duration(6+i) * 500 * time.Millisecond)
			v, ok = tr.update(frame, now)
			if !ok {
				t.Fatalf("update returned ok=false")
			}
		}
		// After 3 ticks the EMA should have moved well past the midpoint
		// toward the new 10fps target — concretely below 18.
		if v >= 18 {
			t.Errorf("after 3 ticks at 10fps, smoothed = %v, want < 18", v)
		}
	})

	t.Run("counter reset re-bases and skips bogus delta", func(t *testing.T) {
		var tr fpsTracker
		start := time.Unix(0, 0)
		tr.update(10_000, start)
		// FFmpeg restart: counter goes backwards.
		v, ok := tr.update(5, start.Add(time.Second))
		if ok {
			t.Errorf("reset sample should yield ok=false, got v=%v", v)
		}
		// Next valid sample should seed cleanly (treated as first sample).
		v, ok = tr.update(35, start.Add(2*time.Second))
		if !ok || v != 30.0 {
			t.Errorf("re-seeded sample = %v ok=%v, want 30 true", v, ok)
		}
	})

	t.Run("ceiling discards bogus huge delta", func(t *testing.T) {
		var tr fpsTracker
		start := time.Unix(0, 0)
		tr.update(0, start)
		// 10 million frames in 1s — way past the sanity ceiling.
		v, ok := tr.update(10_000_000, start.Add(time.Second))
		if ok {
			t.Errorf("bogus sample should yield ok=false, got v=%v", v)
		}
	})

	t.Run("zero or negative dt is ignored", func(t *testing.T) {
		var tr fpsTracker
		start := time.Unix(0, 0)
		tr.update(0, start)
		if _, ok := tr.update(30, start); ok {
			t.Error("dt=0 should yield ok=false")
		}
		if _, ok := tr.update(30, start.Add(-time.Second)); ok {
			t.Error("negative dt should yield ok=false")
		}
	})

	t.Run("reset clears state", func(t *testing.T) {
		var tr fpsTracker
		start := time.Unix(0, 0)
		tr.update(0, start)
		tr.update(30, start.Add(time.Second))
		tr.reset()
		if v, ok := tr.update(30, start.Add(time.Second)); ok {
			t.Errorf("after reset, first update should yield ok=false, got %v", v)
		}
	})
}
