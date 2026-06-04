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
	"syscall"
	"time"

	"github.com/smazurov/videonode/tools/testenv/internal/config"
	"github.com/smazurov/videonode/tools/testenv/internal/slots"
)

// Request is the input to a spawn.
type Request struct {
	Config   *config.V1
	EnvID    string
	DataDir  string
	Locks    []string
	Worktree string
	Held     *slots.Held
}

// Result is what the spawn produced.
type Result struct {
	PID int
}

// Spawn builds and starts the daemon per the config. Returns when the
// health check passes or an error.
func Spawn(ctx context.Context, req Request) (Result, error) {
	cfg := req.Config
	worktree := req.Worktree

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

	if err := Build(ctx, cfg, worktree, vars, env); err != nil {
		return Result{}, err
	}

	// Release held ports right before exec.
	req.Held.Release()

	pid, err := Start(ctx, cfg, worktree, vars, env, req.DataDir)
	if err != nil {
		return Result{}, err
	}

	return Result{PID: pid}, nil
}

// Build runs the spawn.build command from the config.
func Build(ctx context.Context, cfg *config.V1, worktree string, vars map[string]string, env []string) error {
	if cfg.Spawn.Build == "" {
		return nil
	}
	buildCmd := config.ExpandVars(cfg.Spawn.Build, vars)
	cmd := exec.CommandContext(ctx, "sh", "-c", buildCmd)
	cmd.Dir = worktree
	cmd.Env = env
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("build step failed: %w", err)
	}
	return nil
}

// Start spawns the daemon and waits for the health check. Returns the PID.
func Start(ctx context.Context, cfg *config.V1, worktree string, vars map[string]string, env []string, dataDir string) (int, error) {
	command := config.ExpandVars(cfg.Spawn.Command, vars)

	var logPath string
	var logFile *os.File
	if cfg.Spawn.LogsEnabled() {
		logPath = filepath.Join(dataDir, "daemon.log")
		var err error
		logFile, err = os.Create(logPath)
		if err != nil {
			return 0, fmt.Errorf("create log: %w", err)
		}
	}

	cmd := exec.Command("sh", "-c", command)
	cmd.Dir = worktree
	cmd.Env = env
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if logFile != nil {
		cmd.Stdout = logFile
		cmd.Stderr = logFile
	}
	if err := cmd.Start(); err != nil {
		if logFile != nil {
			logFile.Close()
		}
		return 0, fmt.Errorf("start daemon: %w", err)
	}
	pid := cmd.Process.Pid
	if err := cmd.Process.Release(); err != nil {
		if logFile != nil {
			logFile.Close()
		}
		return 0, fmt.Errorf("release process: %w", err)
	}

	if cfg.Spawn.HealthURL != "" {
		healthURL := config.ExpandVars(cfg.Spawn.HealthURL, vars)
		timeout := 15 * time.Second
		if cfg.Spawn.HealthTimeout != "" {
			if d, err := time.ParseDuration(cfg.Spawn.HealthTimeout); err == nil {
				timeout = d
			}
		}
		if err := waitHealthy(ctx, healthURL, cfg.Spawn.HealthAuth, timeout); err != nil {
			if logPath != "" {
				tail := tailFile(logPath, 4096)
				return 0, fmt.Errorf("health check failed at %s: %w\n--- log tail ---\n%s", healthURL, err, tail)
			}
			return 0, fmt.Errorf("health check failed at %s: %w", healthURL, err)
		}
	}

	return pid, nil
}

// BuildVarsForSlot constructs TESTENV_* vars for an existing slot (used by restart).
func BuildVarsForSlot(cfg *config.V1, slot int, envID, dataDir, worktree string, locks []string) map[string]string {
	return cfg.BuildVars(slot, envID, dataDir, worktree, locks)
}

// BuildEnv constructs the environment for build/spawn commands.
func BuildEnv(cfg *config.V1, vars map[string]string) []string {
	return buildEnv(cfg, vars)
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
