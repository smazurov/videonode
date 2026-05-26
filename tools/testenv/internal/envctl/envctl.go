// Package envctl is the single source of truth for testenv business
// logic. Both the CLI (cmd/) and the MCP server (internal/mcpsrv/)
// call into this package — neither may import store, slots, spawn,
// reaper, or config directly. An import-graph test enforces this.
package envctl

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/sys/unix"

	"github.com/smazurov/videonode/tools/testenv/internal/config"
	"github.com/smazurov/videonode/tools/testenv/internal/reaper"
	"github.com/smazurov/videonode/tools/testenv/internal/slots"
	"github.com/smazurov/videonode/tools/testenv/internal/spawn"
	"github.com/smazurov/videonode/tools/testenv/internal/store"
)

// --- params / results ---

type UpParams struct {
	StatePath string
	Session   string
	Locks     []string // exclusive resource leases
}

type UpResult struct {
	EnvID         string
	Slot          int
	Ports         map[string]int // port name → number
	HTTPURL       string
	Auth          string
	LocalOverride string // basename of local config override, empty if none
	DataDir       string
	PID           int
}

type DownParams struct {
	StatePath string
	EnvID     string
	Session   string
}

type DownResult struct {
	EnvID string
	PID   int
}

type ListParams struct {
	StatePath string
	Mine      bool
	Session   string
}

type EnvInfo struct {
	ID         string
	Slot       int
	Target     string
	Source     string
	HTTPURL    string
	HealthURL  string
	HealthAuth string
	Worktree   string
	PID        int
	Leases     []string
	CreatedAt  time.Time
}

type LeaseParams struct {
	StatePath  string
	Session    string
	ResourceID string
}

type ReapResult struct {
	Released []string
}

// --- operations ---

func Up(ctx context.Context, p UpParams) (UpResult, error) {
	if p.Session == "" {
		p.Session = "unattached-" + randHex(4)
	}

	worktree := resolveWorktree(p.StatePath, p.Session)

	if !isInWorktree(worktree) {
		return UpResult{}, fmt.Errorf(
			"testenv up requires a worktree (path must be under .claude/worktrees/); "+
				"resolved build dir: %s", worktree)
	}

	cfg, err := config.Load(worktree)
	if err != nil {
		return UpResult{}, err
	}

	s, err := openStore(p.StatePath)
	if err != nil {
		return UpResult{}, err
	}
	defer s.Close()
	reapAndClean(s)

	localOverride := filepath.Base(cfg.LocalPath)
	if cfg.LocalPath == "" {
		localOverride = ""
	}

	if existing, err := s.GetEnvBySession(p.Session); err == nil {
		return UpResult{
			EnvID: existing.ID, Slot: existing.Slot,
			HTTPURL: existing.HTTPURL, Auth: existing.HealthAuth,
			LocalOverride: localOverride,
			DataDir: existing.DataDir, PID: existing.OwnerPID,
		}, nil
	}

	envID := "env-" + randHex(4)
	dataDir := filepath.Join(worktree, cfg.DataDir)

	var held *slots.Held
	err = s.WithLock(func() error {
		held, err = slots.Pick(s, cfg)
		if err != nil {
			return err
		}
		for _, lock := range p.Locks {
			if holder, hErr := s.LeaseHolder(lock); hErr == nil && holder != "" {
				held.Release()
				return formatLeaseConflict(s, lock, holder)
			}
		}
		primaryPort := held.Ports[cfg.PrimaryPortName()]
		vars := cfg.BuildVars(held.Slot, envID, dataDir, worktree, p.Locks)
		env := store.Env{
			ID: envID, OwnerSession: p.Session, OwnerPID: os.Getpid(),
			OwnerWorktree: worktree, Target: "host", SourceMode: derivedSourceMode(p.Locks),
			Slot: held.Slot, HTTPURL: fmt.Sprintf("http://localhost:%d", primaryPort),
			RTSPURL: "", SRTURL: "",
			HealthURL: config.ExpandVars(cfg.Spawn.HealthURL, vars),
			HealthAuth: cfg.Spawn.HealthAuth,
			DataDir: dataDir, StreamsTOML: filepath.Join(dataDir, "streams.toml"),
		}
		if err := s.CreateEnv(env); err != nil {
			held.Release()
			return err
		}
		for _, lock := range p.Locks {
			if err := s.LeaseAcquire(lock, envID); err != nil {
				s.DeleteEnv(envID)
				held.Release()
				return err
			}
		}
		return nil
	})
	if err != nil {
		return UpResult{}, err
	}

	res, err := spawn.Spawn(ctx, spawn.Request{
		Config:   cfg,
		EnvID:    envID,
		DataDir:  dataDir,
		Locks:    p.Locks,
		Worktree: worktree,
		Held:     held,
	})
	if err != nil {
		s.DeleteEnv(envID)
		return UpResult{}, fmt.Errorf("spawn: %w", err)
	}
	s.UpdateEnvAfterSpawn(envID, res.PID, "")

	httpURL := fmt.Sprintf("http://localhost:%d", held.Ports[cfg.PrimaryPortName()])
	return UpResult{
		EnvID: envID, Slot: held.Slot,
		Ports: held.Ports, HTTPURL: httpURL, Auth: cfg.Spawn.HealthAuth,
		LocalOverride: localOverride, DataDir: dataDir, PID: res.PID,
	}, nil
}

func Down(ctx context.Context, p DownParams) (DownResult, error) {
	s, err := openStore(p.StatePath)
	if err != nil {
		return DownResult{}, err
	}
	defer s.Close()
	reapAndClean(s)

	envID := p.EnvID
	if envID == "" {
		if p.Session == "" {
			return DownResult{}, errors.New("no env id and no session to resolve")
		}
		e, err := s.GetEnvBySession(p.Session)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return DownResult{}, fmt.Errorf("no env owned by session %s", p.Session)
			}
			return DownResult{}, err
		}
		envID = e.ID
	}

	e, err := s.GetEnv(envID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return DownResult{}, fmt.Errorf("no such env: %s", envID)
		}
		return DownResult{}, err
	}
	signalDaemon(e.OwnerPID)
	if err := s.DeleteEnv(envID); err != nil {
		return DownResult{}, err
	}
	removeSpawnFiles(e.OwnerWorktree, e.DataDir)
	return DownResult{EnvID: envID, PID: e.OwnerPID}, nil
}

func List(ctx context.Context, p ListParams) ([]EnvInfo, error) {
	s, err := openStore(p.StatePath)
	if err != nil {
		return nil, err
	}
	defer s.Close()
	reapAndClean(s)

	envs, err := s.ListEnvs()
	if err != nil {
		return nil, err
	}
	var out []EnvInfo
	for _, e := range envs {
		if p.Mine && e.OwnerSession != p.Session {
			continue
		}
		leases, _ := s.ListLeasesFor(e.ID)
		var ids []string
		for _, l := range leases {
			ids = append(ids, l.ResourceID)
		}
		out = append(out, EnvInfo{
			ID: e.ID, Slot: e.Slot, Target: e.Target, Source: e.SourceMode,
			HTTPURL: e.HTTPURL, HealthURL: e.HealthURL, HealthAuth: e.HealthAuth,
			Worktree: DisplayWorktree(e.OwnerWorktree),
			PID: e.OwnerPID, Leases: ids, CreatedAt: e.CreatedAt,
		})
	}
	return out, nil
}

func Lease(ctx context.Context, p LeaseParams) error {
	s, err := openStore(p.StatePath)
	if err != nil {
		return err
	}
	defer s.Close()
	reapAndClean(s)

	if p.Session == "" {
		return errors.New("no session to resolve env for lease")
	}
	env, err := s.GetEnvBySession(p.Session)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("no env for session %s — run up first", p.Session)
		}
		return err
	}
	if holder, hErr := s.LeaseHolder(p.ResourceID); hErr == nil && holder != "" {
		return formatLeaseConflict(s, p.ResourceID, holder)
	}
	return s.LeaseAcquire(p.ResourceID, env.ID)
}

func Release(ctx context.Context, statePath, resourceID string) error {
	s, err := openStore(statePath)
	if err != nil {
		return err
	}
	defer s.Close()
	return s.LeaseRelease(resourceID)
}

func ReleaseSession(ctx context.Context, statePath, session string) ([]string, error) {
	if session == "" {
		return nil, errors.New("no session id")
	}
	s, err := openStore(statePath)
	if err != nil {
		return nil, err
	}
	defer s.Close()
	envs, _ := s.ListEnvs()
	for _, e := range envs {
		if e.OwnerSession == session {
			signalDaemon(e.OwnerPID)
			removeSpawnFiles(e.OwnerWorktree, e.DataDir)
		}
	}
	return s.DeleteEnvsForSession(session)
}

func ReleaseWorktree(ctx context.Context, statePath, worktreeDir string) ([]string, error) {
	if worktreeDir == "" {
		return nil, nil
	}
	s, err := openStore(statePath)
	if err != nil {
		return nil, err
	}
	defer s.Close()
	envs, err := s.ListEnvs()
	if err != nil {
		return nil, err
	}
	var released []string
	for _, e := range envs {
		if e.OwnerWorktree == worktreeDir {
			signalDaemon(e.OwnerPID)
			if s.DeleteEnv(e.ID) == nil {
				removeSpawnFiles(e.OwnerWorktree, e.DataDir)
				released = append(released, e.ID)
			}
		}
	}
	return released, nil
}

func Reap(ctx context.Context, statePath string) (ReapResult, error) {
	s, err := openStore(statePath)
	if err != nil {
		return ReapResult{}, err
	}
	defer s.Close()
	ids := reapAndClean(s)
	return ReapResult{Released: ids}, nil
}

type ValidateResult struct {
	LocalOverride string // basename of local override file, empty if none
}

func Validate(dir string) (ValidateResult, error) {
	cfg, err := config.Load(dir)
	if err != nil {
		return ValidateResult{}, err
	}
	var local string
	if cfg.LocalPath != "" {
		local = filepath.Base(cfg.LocalPath)
	}
	return ValidateResult{LocalOverride: local}, cfg.Validate()
}

// --- doctor ---

type DoctorFact struct {
	Key   string
	Value string
}

func Doctor(statePath, session string) []DoctorFact {
	var facts []DoctorFact
	add := func(k, v string) { facts = append(facts, DoctorFact{Key: k, Value: v}) }

	cwd, _ := os.Getwd()
	add("cwd", cwd)

	if session != "" {
		add("session", session)
	} else {
		add("session", "(none)")
	}

	resolvedHow := "flag/env"
	sp := statePath
	if sp == "" {
		sp = store.DefaultPath()
		resolvedHow = "default"
	}
	add("state_path", sp+" ("+resolvedHow+")")

	if _, err := os.Stat(sp); err == nil {
		add("state_db", "exists")
	} else {
		add("state_db", "missing")
	}

	// Config is per-worktree (tracked file), but .mcp.json is per-project.
	configRoot, err := config.FindProjectRoot()
	if err != nil {
		add("config", "not found")
	} else {
		add("config", filepath.Join(configRoot, config.FileName))
		localPath := filepath.Join(configRoot, config.LocalFileName)
		if _, err := os.Stat(localPath); err == nil {
			add("local_config", localPath)
		}
	}

	projectRoot := resolveProjectRoot(configRoot)
	if configRoot != projectRoot {
		add("project_root", projectRoot)
	}

	wt := resolveWorktree(sp, session)
	if session != "" {
		if sessionCwd, _ := LookupSession(sp, session); sessionCwd != "" {
			add("build_dir", sessionCwd+" (session registry)")
		} else {
			add("build_dir", wt+" (fallback)")
		}
	} else {
		add("build_dir", wt+" (no session)")
	}

	if projectRoot != "" {
		mcpPath := filepath.Join(projectRoot, ".mcp.json")
		if data, err := os.ReadFile(mcpPath); err == nil {
			if strings.Contains(string(data), "TESTENV_STATE") {
				add("mcp_json", "ok (TESTENV_STATE set)")
			} else {
				add("mcp_json", "TESTENV_STATE missing — run `testenv install`")
			}
		} else {
			add("mcp_json", "not found — run `testenv install`")
		}
	}

	s, err := openStore(sp)
	if err == nil {
		envs, _ := s.ListEnvs()
		add("envs", fmt.Sprintf("%d", len(envs)))
		s.Close()
	}

	return facts
}

// resolveProjectRoot strips .claude/worktrees/<name> from a path to
// get the real project root where .mcp.json lives.
func resolveProjectRoot(dir string) string {
	const marker = "/.claude/worktrees/"
	if i := strings.Index(dir, marker); i >= 0 {
		return dir[:i]
	}
	return dir
}

// ConfigFileName is the config file name, re-exported so cmd/ doesn't
// need to import config directly.
const ConfigFileName = config.FileName

// DisplayWorktree formats a stored absolute worktree path for display.
// For .claude/worktrees/<name> paths: "<name>/<project>".
// For main checkouts: "<project>".
func DisplayWorktree(absPath string) string {
	marker := "/.claude/worktrees/"
	i := strings.Index(absPath, marker)
	if i < 0 {
		return filepath.Base(absPath)
	}
	project := filepath.Base(absPath[:i])
	rest := absPath[i+len(marker):]
	if j := strings.Index(rest, "/"); j >= 0 {
		rest = rest[:j]
	}
	return rest + "/" + project
}

// resolveWorktree returns the session's registered cwd, falling back to the
// project root (the directory containing .testenv.toml found by walking up
// from the process cwd).
func resolveWorktree(statePath, session string) string {
	if cwd, _ := LookupSession(statePath, session); cwd != "" {
		return cwd
	}
	if dir, err := config.FindProjectRoot(); err == nil {
		return dir
	}
	return "."
}

type RestartParams struct {
	StatePath string
	EnvID     string
	Session   string
}

type RestartResult struct {
	EnvID   string
	HTTPURL string
	Auth    string
	PID     int
}

func Restart(ctx context.Context, p RestartParams) (RestartResult, error) {
	worktree := resolveWorktree(p.StatePath, p.Session)

	cfg, err := config.Load(worktree)
	if err != nil {
		return RestartResult{}, err
	}

	s, err := openStore(p.StatePath)
	if err != nil {
		return RestartResult{}, err
	}
	defer s.Close()
	reaper.Reap(s)

	envID := p.EnvID
	if envID == "" {
		if p.Session == "" {
			return RestartResult{}, errors.New("no env id and no session to resolve")
		}
		e, err := s.GetEnvBySession(p.Session)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return RestartResult{}, fmt.Errorf("no env owned by session %s", p.Session)
			}
			return RestartResult{}, err
		}
		envID = e.ID
	}

	e, err := s.GetEnv(envID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return RestartResult{}, fmt.Errorf("no such env: %s", envID)
		}
		return RestartResult{}, err
	}

	leases, _ := s.ListLeasesFor(envID)
	var lockIDs []string
	for _, l := range leases {
		lockIDs = append(lockIDs, l.ResourceID)
	}

	// Use the session's worktree (not the stored one, which may be stale ".").
	buildDir := worktree
	if buildDir == "" || buildDir == "." {
		buildDir = e.OwnerWorktree
	}

	vars := spawn.BuildVarsForSlot(cfg, e.Slot, envID, e.DataDir, buildDir, lockIDs)
	env := spawn.BuildEnv(cfg, vars)

	// Park our own PID so the reaper won't delete the row while we rebuild.
	s.UpdateEnvAfterRestart(envID, os.Getpid(), e.HealthURL, e.HealthAuth)

	signalDaemon(e.OwnerPID)

	if err := spawn.Build(ctx, cfg, buildDir, vars, env); err != nil {
		return RestartResult{}, fmt.Errorf("rebuild: %w", err)
	}

	pid, err := spawn.Start(ctx, cfg, buildDir, vars, env, e.DataDir)
	if err != nil {
		return RestartResult{}, fmt.Errorf("restart: %w", err)
	}

	healthURL := config.ExpandVars(cfg.Spawn.HealthURL, vars)
	s.UpdateEnvAfterRestart(envID, pid, healthURL, cfg.Spawn.HealthAuth)

	return RestartResult{
		EnvID: envID, HTTPURL: e.HTTPURL, Auth: cfg.Spawn.HealthAuth, PID: pid,
	}, nil
}

// --- helpers ---

func openStore(statePath string) (*store.Store, error) {
	if statePath == "" {
		statePath = store.DefaultPath()
	}
	return store.Open(statePath)
}

func derivedSourceMode(locks []string) string {
	for _, l := range locks {
		if strings.HasPrefix(l, "device:") {
			return "real"
		}
	}
	return "fake"
}

func formatLeaseConflict(s *store.Store, resID, holderEnvID string) error {
	holder, err := s.GetEnv(holderEnvID)
	if err != nil {
		return fmt.Errorf("resource %s held by env %s (lookup failed: %v)", resID, holderEnvID, err)
	}
	return fmt.Errorf("resource %s held by env %s (worktree=%s pid=%d since %s)",
		resID, holder.ID, holder.OwnerWorktree, holder.OwnerPID, holder.CreatedAt.Format("15:04:05"))
}

func signalDaemon(pid int) {
	if pid <= 0 {
		return
	}
	_ = unix.Kill(-pid, unix.SIGTERM)
	_ = unix.Kill(pid, unix.SIGTERM)

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if !processAlive(pid) {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	_ = unix.Kill(-pid, unix.SIGKILL)
	_ = unix.Kill(pid, unix.SIGKILL)
}

func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := unix.Kill(pid, 0)
	return err == nil || err == unix.EPERM
}

func isInWorktree(path string) bool {
	return strings.Contains(path, "/.claude/worktrees/")
}

func reapAndClean(s *store.Store) []string {
	reaped, _ := reaper.Reap(s)
	var ids []string
	for _, r := range reaped {
		removeDataDir(r.DataDir)
		ids = append(ids, r.ID)
	}
	reaper.CleanOrphanDirs(s, filepath.Join(filepath.Dir(s.Path()), "envs"))
	return ids
}

func removeDataDir(dir string) {
	if dir == "" {
		return
	}
	if isInWorktree(dir) {
		return
	}
	os.RemoveAll(dir)
}

func removeSpawnFiles(worktree, dataDir string) {
	if worktree == "" {
		removeDataDir(dataDir)
		return
	}
	cfg, err := config.Load(worktree)
	if err != nil {
		removeDataDir(dataDir)
		return
	}
	vars := map[string]string{
		"TESTENV_WORKTREE": worktree,
		"TESTENV_DIR":      dataDir,
	}
	for _, f := range cfg.Spawn.Files {
		path := config.ExpandVars(f.Path, vars)
		os.Remove(path)
	}
	if cfg.Spawn.LogsEnabled() && dataDir != "" {
		os.Remove(filepath.Join(dataDir, "daemon.log"))
	}
}

func randHex(nBytes int) string {
	b := make([]byte, nBytes)
	if _, err := rand.Read(b); err != nil {
		return "00000000"[:2*nBytes]
	}
	return hex.EncodeToString(b)
}
