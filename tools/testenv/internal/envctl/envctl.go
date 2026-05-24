// Package envctl is the single source of truth for testenv business
// logic. Both the CLI (cmd/) and the MCP server (internal/mcpsrv/)
// call into this package — neither may import store, slots, spawn, or
// reaper directly. A compile-time import-graph test enforces this.
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
	"time"

	"golang.org/x/sys/unix"

	"github.com/smazurov/videonode/tools/testenv/internal/reaper"
	"github.com/smazurov/videonode/tools/testenv/internal/slots"
	"github.com/smazurov/videonode/tools/testenv/internal/spawn"
	"github.com/smazurov/videonode/tools/testenv/internal/store"
)

// --- params / results ---

type UpParams struct {
	StatePath string
	Session   string
	Target    string // "host" | "sbc"
	Source    string // "fake" | "real"
	Device    string // e.g. "/dev/video0"
}

type UpResult struct {
	EnvID   string
	Slot    int
	HTTPURL string
	RTSPURL string
	SRTURL  string
	DataDir string
	PID     int
}

type DownParams struct {
	StatePath string
	EnvID     string // if empty, resolved from Session
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
	if p.Target == "" {
		p.Target = "host"
	}
	if p.Source == "" {
		p.Source = "fake"
	}
	if p.Target == "sbc" {
		return UpResult{}, errors.New("--target sbc not implemented yet")
	}
	if p.Session == "" {
		p.Session = "unattached-" + randHex(4)
	}

	worktree, err := os.Getwd()
	if err != nil {
		return UpResult{}, fmt.Errorf("getwd: %w", err)
	}
	if !isVideonodeRoot(worktree) {
		return UpResult{}, fmt.Errorf("cwd %s is not a videonode worktree", worktree)
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
			HTTPURL: existing.HTTPURL, RTSPURL: existing.RTSPURL, SRTURL: existing.SRTURL,
			DataDir: existing.DataDir, PID: existing.OwnerPID,
		}, nil
	}

	envID := "env-" + randHex(4)
	dataDir := filepath.Join(filepath.Dir(s.Path()), "envs", envID)

	var held *slots.Held
	err = s.WithLock(func() error {
		held, err = slots.Pick(s)
		if err != nil {
			return err
		}
		if p.Source == "real" {
			resID := deviceResource(p.Device)
			if holder, hErr := s.LeaseHolder(resID); hErr == nil && holder != "" {
				held.Release()
				return formatLeaseConflict(s, resID, holder)
			}
		}
		env := store.Env{
			ID: envID, OwnerSession: p.Session, OwnerPID: os.Getpid(),
			OwnerWorktree: worktree, Target: p.Target, SourceMode: p.Source,
			Slot: held.Triple.Slot, HTTPURL: held.Triple.HTTP,
			RTSPURL: held.Triple.RTSP, SRTURL: held.Triple.SRT,
			DataDir: dataDir, StreamsTOML: filepath.Join(dataDir, "streams.toml"),
		}
		if err := s.CreateEnv(env); err != nil {
			held.Release()
			return err
		}
		if p.Source == "real" {
			resID := deviceResource(p.Device)
			if err := s.LeaseAcquire(resID, envID); err != nil {
				_ = s.DeleteEnv(envID)
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
		EnvID: envID, Worktree: worktree, Target: p.Target,
		SourceMode: p.Source, Device: p.Device,
		DataDir: dataDir, Triple: held.Triple, HeldPorts: held,
	})
	if err != nil {
		_ = s.DeleteEnv(envID)
		return UpResult{}, fmt.Errorf("spawn: %w", err)
	}
	_ = s.UpdateEnvAfterSpawn(envID, res.PID, res.NativeBinDir)

	return UpResult{
		EnvID: envID, Slot: held.Triple.Slot,
		HTTPURL: held.Triple.HTTP, RTSPURL: held.Triple.RTSP, SRTURL: held.Triple.SRT,
		DataDir: dataDir, PID: res.PID,
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
			HTTPURL: e.HTTPURL, Worktree: filepath.Base(e.OwnerWorktree),
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

func Reap(ctx context.Context, statePath string) (ReapResult, error) {
	s, err := openStore(statePath)
	if err != nil {
		return ReapResult{}, err
	}
	defer s.Close()
	released, err := reaper.Reap(s)
	return ReapResult{Released: released}, err
}

// --- helpers ---

func openStore(statePath string) (*store.Store, error) {
	if statePath == "" {
		statePath = store.DefaultPath()
	}
	return store.Open(statePath)
}

func deviceResource(device string) string {
	if device == "" {
		device = "/dev/video0"
	}
	return "device:" + device
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

func isVideonodeRoot(dir string) bool {
	for _, name := range []string{"go.mod", "main.go"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			return false
		}
	}
	fi, err := os.Stat(filepath.Join(dir, "composer"))
	if err != nil || !fi.IsDir() {
		return false
	}
	b, err := os.ReadFile(filepath.Join(dir, "go.mod"))
	if err != nil {
		return false
	}
	for _, line := range splitLines(string(b)) {
		if line == "module github.com/smazurov/videonode" {
			return true
		}
	}
	return false
}

func splitLines(s string) []string {
	var out []string
	for s != "" {
		i := 0
		for i < len(s) && s[i] != '\n' {
			i++
		}
		out = append(out, s[:i])
		if i < len(s) {
			i++
		}
		s = s[i:]
	}
	return out
}

func randHex(nBytes int) string {
	b := make([]byte, nBytes)
	if _, err := rand.Read(b); err != nil {
		return "00000000"[:2*nBytes]
	}
	return hex.EncodeToString(b)
}
