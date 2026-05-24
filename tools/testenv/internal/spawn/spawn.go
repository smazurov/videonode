// Package spawn brings up a videonode daemon for a test environment.
//
// Host target: assemble env vars, exec the worktree's videonode
// binary, write an isolated streams.toml, health-poll /api/health.
// SBC target: cross-compile, rsync, ssh-spawn (see sbc.go — TODO).
package spawn

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/smazurov/videonode/tools/testenv/internal/slots"
)

// Request is the input to a spawn.
type Request struct {
	EnvID      string
	Worktree   string // absolute path to the videonode worktree
	Target     string // "host" | "sbc"
	SourceMode string // "fake" | "real"
	Device     string // /dev/video0 when SourceMode == "real"
	DataDir    string // per-env data dir (recordings, streams.toml live here)
	Triple     slots.Triple
	HeldPorts  *slots.Held // closed by Spawn just before exec
}

// Result is what the spawn produced.
type Result struct {
	PID          int
	NativeBinDir string
	StreamsTOML  string
}

// Spawn brings up the env. Returns when the daemon's /api/health
// answers 200, or an error.
func Spawn(ctx context.Context, req Request) (Result, error) {
	if req.Target != "host" {
		return Result{}, fmt.Errorf("spawn: only --target host implemented; got %q", req.Target)
	}
	if err := os.MkdirAll(req.DataDir, 0o755); err != nil {
		return Result{}, fmt.Errorf("mkdir data dir: %w", err)
	}

	binPath := filepath.Join(req.Worktree, "videonode")
	if _, err := os.Stat(binPath); err != nil {
		return Result{}, fmt.Errorf("videonode binary not found at %s — build it first (go build .)", binPath)
	}

	streamsTOML := filepath.Join(req.DataDir, "streams.toml")
	if err := writeStreamsTOML(streamsTOML, req); err != nil {
		return Result{}, fmt.Errorf("write streams.toml: %w", err)
	}

	recDir := filepath.Join(req.DataDir, "recording")
	if err := os.MkdirAll(recDir, 0o755); err != nil {
		return Result{}, fmt.Errorf("mkdir recording: %w", err)
	}

	httpPort, rtspPort, srtPort := slots.PortsForSlot(req.Triple.Slot)
	logPath := filepath.Join(req.DataDir, "videonode.log")
	logFile, err := os.Create(logPath)
	if err != nil {
		return Result{}, fmt.Errorf("create log: %w", err)
	}

	// Hand off the ports: close the held listeners RIGHT BEFORE exec so
	// the kernel releases the binds within microseconds of the daemon
	// re-binding them.
	req.HeldPorts.Release()

	cmd := exec.Command(binPath)
	cmd.Dir = req.Worktree
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.Env = append(os.Environ(),
		"VIDEONODE_SERVER_PORT=:"+strconv.Itoa(httpPort),
		"VIDEONODE_STREAMING_RTSP_PORT=:"+strconv.Itoa(rtspPort),
		"VIDEONODE_SRT_ADDR=:"+strconv.Itoa(srtPort),
		"VIDEONODE_STREAMS_CONFIG_FILE="+streamsTOML,
		"VIDEONODE_RECORDING_DATA_DIR="+recDir,
		"VIDEONODE_AUTH_TYPE=basic",
		"VIDEONODE_AUTH_USERNAME=testenv",
		"VIDEONODE_AUTH_PASSWORD=testenv",
	)
	nativeBinDir := findNativeBinDir(req.Worktree)
	if nativeBinDir != "" {
		cmd.Env = append(cmd.Env,
			"VIDEONODE_NATIVE_PIPELINE_SOURCE="+filepath.Join(nativeBinDir, "videonode-source"),
			"VIDEONODE_NATIVE_PIPELINE_SINK="+filepath.Join(nativeBinDir, "videonode-sink"),
			"VIDEONODE_NATIVE_PIPELINE_COMPOSER="+filepath.Join(nativeBinDir, "videonode-composer"),
		)
	}

	if err := cmd.Start(); err != nil {
		_ = logFile.Close()
		return Result{}, fmt.Errorf("start videonode: %w", err)
	}

	// Capture PID before Release — Release zeros out cmd.Process.Pid.
	pid := cmd.Process.Pid

	// Detach: we don't want the daemon to die when the testenv binary exits.
	if err := cmd.Process.Release(); err != nil {
		_ = logFile.Close()
		return Result{}, fmt.Errorf("release process: %w", err)
	}

	healthURL := req.Triple.HTTP + "/api/health"
	if err := waitHealthy(ctx, healthURL, 15*time.Second); err != nil {
		tail := tailFile(logPath, 4096)
		return Result{}, fmt.Errorf("daemon did not become healthy at %s within 15s: %w\n--- log tail ---\n%s", healthURL, err, tail)
	}
	return Result{
		PID:          pid,
		NativeBinDir: nativeBinDir,
		StreamsTOML:  streamsTOML,
	}, nil
}

// writeStreamsTOML emits the per-env stream definitions. For
// SourceMode=fake we declare a single test_mode source named
// "<envID>-fake-source" and a single stream attached to it; for
// SourceMode=real we point at req.Device. Entity IDs are env-prefixed
// to guarantee no namespace collision with another concurrent env.
func writeStreamsTOML(path string, req Request) error {
	srcID := req.EnvID + "-src"
	streamID := req.EnvID + "-stream"
	var sourceBlock string
	if req.SourceMode == "fake" {
		sourceBlock = fmt.Sprintf(`[[sources]]
id = "%s"
test_mode = true
width = 1280
height = 720
fps = 30
`, srcID)
	} else {
		device := req.Device
		if device == "" {
			device = "/dev/video0"
		}
		sourceBlock = fmt.Sprintf(`[[sources]]
id = "%s"
device = "%s"
width = 1920
height = 1080
fps = 30
`, srcID, device)
	}

	contents := "version = 2\n\n" + sourceBlock + fmt.Sprintf(`
[[streams]]
id = "%s"
upstream = "source:%s"

[streams.encoder]
codec = "h264"
bitrate_kbps = 2000
preset = "veryfast"
`, streamID, srcID)
	return os.WriteFile(path, []byte(contents), 0o644)
}

func waitHealthy(ctx context.Context, url string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	ctx, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()
	client := &http.Client{Timeout: 1 * time.Second}
	var lastErr error
	for time.Now().Before(deadline) {
		req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
		if err != nil {
			return err
		}
		req.SetBasicAuth("testenv", "testenv")
		resp, err := client.Do(req)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == 200 {
				return nil
			}
			lastErr = fmt.Errorf("status %d", resp.StatusCode)
		} else {
			lastErr = err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(200 * time.Millisecond):
		}
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("timeout")
	}
	return lastErr
}

func dirExists(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && fi.IsDir()
}

// findNativeBinDir returns the worktree's freshest C++ build output
// dir, or "" if none exists. Prefers relwithdebinfo (the documented
// daily-driver preset per CLAUDE.md) and falls back to dev.
func findNativeBinDir(worktree string) string {
	candidates := []string{
		filepath.Join(worktree, "composer", "build", "relwithdebinfo", "src", "bin"),
		filepath.Join(worktree, "composer", "build", "dev", "src", "bin"),
	}
	for _, d := range candidates {
		if dirExists(d) {
			return d
		}
	}
	return ""
}

func tailFile(path string, n int) string {
	f, err := os.Open(path)
	if err != nil {
		return "(no log: " + err.Error() + ")"
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		return ""
	}
	off := fi.Size() - int64(n)
	if off < 0 {
		off = 0
	}
	_, err = f.Seek(off, io.SeekStart)
	if err != nil {
		return ""
	}
	b, _ := io.ReadAll(f)
	return strings.TrimRight(string(b), "\n")
}
