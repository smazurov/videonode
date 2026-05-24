// Package spawn brings up a daemon for a test environment using the
// project's .testenv.toml config. All project-specific logic (binary
// paths, env vars, config files) comes from the config — this package
// is generic.
package spawn

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/smazurov/videonode/tools/testenv/internal/config"
	"github.com/smazurov/videonode/tools/testenv/internal/slots"
)

// Request is the input to a spawn.
type Request struct {
	Config  *config.V1
	EnvID   string
	DataDir string
	Locks   []string
	Held    *slots.Held
}

// Result is what the spawn produced.
type Result struct {
	PID int
}

// Spawn builds and starts the daemon per the config. Returns when the
// health check passes or an error.
func Spawn(ctx context.Context, req Request) (Result, error) {
	cfg := req.Config
	worktree, _ := os.Getwd()

	if err := os.MkdirAll(req.DataDir, 0o755); err != nil {
		return Result{}, fmt.Errorf("mkdir data dir: %w", err)
	}

	vars := cfg.BuildVars(req.Held.Slot, req.EnvID, req.DataDir, worktree, req.Locks)
	env := buildEnv(cfg, vars)

	// Write spawn.files before build/command.
	for _, f := range cfg.Spawn.Files {
		path := config.ExpandVars(f.Path, vars)
		content := config.ExpandVars(f.Content, vars)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return Result{}, fmt.Errorf("mkdir for %s: %w", path, err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			return Result{}, fmt.Errorf("write %s: %w", path, err)
		}
	}

	// Build step (optional).
	if cfg.Spawn.Build != "" {
		buildCmd := config.ExpandVars(cfg.Spawn.Build, vars)
		cmd := exec.CommandContext(ctx, "sh", "-c", buildCmd)
		cmd.Dir = worktree
		cmd.Env = env
		cmd.Stdout = os.Stderr
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return Result{}, fmt.Errorf("build step failed: %w", err)
		}
	}

	// Release held ports right before exec.
	req.Held.Release()

	// Spawn the daemon.
	command := config.ExpandVars(cfg.Spawn.Command, vars)
	logPath := filepath.Join(req.DataDir, "daemon.log")
	logFile, err := os.Create(logPath)
	if err != nil {
		return Result{}, fmt.Errorf("create log: %w", err)
	}

	cmd := exec.Command("sh", "-c", command)
	cmd.Dir = worktree
	cmd.Env = env
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	if err := cmd.Start(); err != nil {
		logFile.Close()
		return Result{}, fmt.Errorf("start daemon: %w", err)
	}
	pid := cmd.Process.Pid
	if err := cmd.Process.Release(); err != nil {
		logFile.Close()
		return Result{}, fmt.Errorf("release process: %w", err)
	}

	// Health check.
	if cfg.Spawn.HealthURL != "" {
		healthURL := config.ExpandVars(cfg.Spawn.HealthURL, vars)
		timeout := 15 * time.Second
		if cfg.Spawn.HealthTimeout != "" {
			if d, err := time.ParseDuration(cfg.Spawn.HealthTimeout); err == nil {
				timeout = d
			}
		}
		if err := waitHealthy(ctx, healthURL, cfg.Spawn.HealthAuth, timeout); err != nil {
			tail := tailFile(logPath, 4096)
			return Result{}, fmt.Errorf("health check failed at %s: %w\n--- log tail ---\n%s", healthURL, err, tail)
		}
	}

	return Result{PID: pid}, nil
}

func buildEnv(cfg *config.V1, vars map[string]string) []string {
	env := os.Environ()
	// Export TESTENV_* vars.
	for k, v := range vars {
		env = append(env, k+"="+v)
	}
	// Expand and export spawn.env entries.
	for k, v := range cfg.Spawn.Env {
		env = append(env, k+"="+config.ExpandVars(v, vars))
	}
	return env
}

func waitHealthy(ctx context.Context, url, auth string, timeout time.Duration) error {
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
		if auth != "" {
			parts := strings.SplitN(auth, ":", 2)
			if len(parts) == 2 {
				req.SetBasicAuth(parts[0], parts[1])
			}
		}
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

func tailFile(path string, n int) string {
	f, err := os.Open(path)
	if err != nil {
		return "(no log: " + err.Error() + ")"
	}
	defer f.Close()
	fi, _ := f.Stat()
	if fi == nil {
		return ""
	}
	off := fi.Size() - int64(n)
	if off < 0 {
		off = 0
	}
	f.Seek(off, io.SeekStart)
	b, _ := io.ReadAll(f)
	return strings.TrimRight(string(b), "\n")
}
