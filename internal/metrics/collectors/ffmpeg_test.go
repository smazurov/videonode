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

	m := metrics.GetFFmpegMetrics(streamID)
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

	m := metrics.GetFFmpegMetrics(streamID)
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

	m = metrics.GetFFmpegMetrics(streamID)
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
	if m := metrics.GetFFmpegMetrics(streamID); m != nil {
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
