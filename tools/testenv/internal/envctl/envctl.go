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
	EnvID   string
	Slot    int
	Ports   map[string]int // port name → number
	DataDir string
	PID     int
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
	ID        string
	Slot      int
	Target    string
	Source    string
	HTTPURL   string
	Worktree  string
	PID       int
	Leases    []string
	CreatedAt time.Time
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

	cfg, err := config.Load(".")
	if err != nil {
		return UpResult{}, err
	}

	s, err := openStore(p.StatePath)
	if err != nil {
		return UpResult{}, err
	}
	defer s.Close()
	reaper.Reap(s)

	if existing, err := s.GetEnvBySession(p.Session); err == nil {
		return UpResult{
			EnvID: existing.ID, Slot: existing.Slot,
			DataDir: existing.DataDir, PID: existing.OwnerPID,
		}, nil
	}

	envID := "env-" + randHex(4)
	dataDir := filepath.Join(filepath.Dir(s.Path()), "envs", envID)
	worktree := filepath.Dir(cfg.Path)
	if sessionCwd, _ := LookupSession(p.StatePath, p.Session); sessionCwd != "" {
		worktree = sessionCwd
	}

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
		// First port in sorted order is used as the primary HTTP URL.
		firstPort := held.Ports[cfg.PortNames()[0]]
		env := store.Env{
			ID: envID, OwnerSession: p.Session, OwnerPID: os.Getpid(),
			OwnerWorktree: worktree, Target: "host", SourceMode: derivedSourceMode(p.Locks),
			Slot: held.Slot, HTTPURL: fmt.Sprintf("http://localhost:%d", firstPort),
			RTSPURL: "", SRTURL: "",
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
		Config:  cfg,
		EnvID:   envID,
		DataDir: dataDir,
		Locks:   p.Locks,
		Held:    held,
	})
	if err != nil {
		s.DeleteEnv(envID)
		return UpResult{}, fmt.Errorf("spawn: %w", err)
	}
	s.UpdateEnvAfterSpawn(envID, res.PID, "")

	return UpResult{
		EnvID: envID, Slot: held.Slot,
		Ports: held.Ports, DataDir: dataDir, PID: res.PID,
	}, nil
}

func Down(ctx context.Context, p DownParams) (DownResult, error) {
	s, err := openStore(p.StatePath)
	if err != nil {
		return DownResult{}, err
	}
	defer s.Close()
	reaper.Reap(s)

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
	return DownResult{EnvID: envID, PID: e.OwnerPID}, nil
}

func List(ctx context.Context, p ListParams) ([]EnvInfo, error) {
	s, err := openStore(p.StatePath)
	if err != nil {
		return nil, err
	}
	defer s.Close()
	reaper.Reap(s)

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
			HTTPURL: e.HTTPURL, Worktree: DisplayWorktree(e.OwnerWorktree),
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
	reaper.Reap(s)

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
	released, err := reaper.Reap(s)
	return ReapResult{Released: released}, err
}

func Validate(dir string) error {
	cfg, err := config.Load(dir)
	if err != nil {
		return err
	}
	return cfg.Validate()
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
}

func randHex(nBytes int) string {
	b := make([]byte, nBytes)
	if _, err := rand.Read(b); err != nil {
		return "00000000"[:2*nBytes]
	}
	return hex.EncodeToString(b)
}
