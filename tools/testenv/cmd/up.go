package cmd

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/smazurov/videonode/tools/testenv/internal/slots"
	"github.com/smazurov/videonode/tools/testenv/internal/spawn"
	"github.com/smazurov/videonode/tools/testenv/internal/store"
)

// UpCmd brings up a test env.
type UpCmd struct {
	Target string `enum:"host,sbc" default:"host" help:"Where to spawn the env."`
	Source string `enum:"fake,real" default:"fake" help:"Source mode."`
	Device string `default:"/dev/video0" help:"Device path when --source real."`
}

// Run executes the up command.
func (c *UpCmd) Run(ctx *Context) error {
	if c.Target == "sbc" {
		return errors.New("--target sbc not implemented yet")
	}

	session := ctx.SessionID
	if session == "" {
		session = "unattached-" + randHex(4)
	}

	worktree, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getwd: %w", err)
	}
	if !isVideonodeRoot(worktree) {
		return fmt.Errorf("cwd %s is not a videonode worktree (no go.mod with module videonode found)", worktree)
	}

	s, err := ctx.OpenStore()
	if err != nil {
		return err
	}
	defer s.Close()

	ReapBefore(s)

	// One env per session — re-use if the session already owns one.
	if existing, err := s.GetEnvBySession(session); err == nil {
		fmt.Fprintf(stdout(), "session %s already owns env %s at %s\n", session, existing.ID, existing.HTTPURL)
		return nil
	}

	envID := "env-" + randHex(4)
	dataDir := filepath.Join(envDataDirRoot(s.Path()), envID)

	var (
		held *slots.Held
		res  spawn.Result
	)
	err = s.WithLock(func() error {
		held, err = slots.Pick(s)
		if err != nil {
			return err
		}

		// Device lease before commit, so a conflict is fast-fail before
		// we burn a slot allocation.
		var resID string
		if c.Source == "real" && c.Target == "host" {
			resID = "device:" + c.Device
			if holder, hErr := s.LeaseHolder(resID); hErr == nil && holder != "" {
				held.Release()
				return formatLeaseConflict(s, resID, holder)
			}
		}

		// Commit env row first (so the slot is taken), then the lease.
		env := store.Env{
			ID:            envID,
			OwnerSession:  session,
			OwnerPID:      os.Getpid(),
			OwnerWorktree: worktree,
			Target:        c.Target,
			SourceMode:    c.Source,
			Slot:          held.Triple.Slot,
			HTTPURL:       held.Triple.HTTP,
			RTSPURL:       held.Triple.RTSP,
			SRTURL:        held.Triple.SRT,
			DataDir:       dataDir,
			StreamsTOML:   filepath.Join(dataDir, "streams.toml"),
		}
		if err := s.CreateEnv(env); err != nil {
			held.Release()
			return err
		}
		if resID != "" {
			if err := s.LeaseAcquire(resID, envID); err != nil {
				_ = s.DeleteEnv(envID)
				held.Release()
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}

	res, err = spawn.Spawn(ctx.Ctx, spawn.Request{
		EnvID:      envID,
		Worktree:   worktree,
		Target:     c.Target,
		SourceMode: c.Source,
		Device:     c.Device,
		DataDir:    dataDir,
		Triple:     held.Triple,
		HeldPorts:  held,
	})
	if err != nil {
		// spawn failed — back out the registry rows so the next caller
		// sees a clean slate.
		_ = s.DeleteEnv(envID)
		return fmt.Errorf("spawn: %w", err)
	}

	// Update env with the daemon's PID + native bin dir.
	if updateErr := s.UpdateEnvAfterSpawn(envID, res.PID, res.NativeBinDir); updateErr != nil {
		fmt.Fprintf(stderr(), "warn: post-spawn registry update failed: %v\n", updateErr)
	}

	fmt.Fprintf(stdout(), "env %s up · slot %d · %s\n", envID, held.Triple.Slot, held.Triple.HTTP)
	fmt.Fprintf(stdout(), "  rtsp: %s\n  srt:  %s\n  data: %s\n  pid:  %d\n",
		held.Triple.RTSP, held.Triple.SRT, dataDir, res.PID)
	return nil
}

// envDataDirRoot derives the per-env data dir root from the state-db
// path so TESTENV_STATE controls both: state.db and envs/ siblings.
func envDataDirRoot(statePath string) string {
	return filepath.Join(filepath.Dir(statePath), "envs")
}

func isVideonodeRoot(dir string) bool {
	// Three signals only the videonode repo root has: go.mod, main.go,
	// and a composer/ subdir. This rejects nested go modules like
	// tools/testenv/ even though they're inside the same repo.
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
	// Confirm the go.mod is the videonode top-level module, not a
	// nested tools/* module that happens to contain "videonode".
	return strings.Contains(string(b), "module github.com/smazurov/videonode\n")
}

func randHex(nBytes int) string {
	b := make([]byte, nBytes)
	if _, err := rand.Read(b); err != nil {
		// crypto/rand failing is exceptional; fall back to a fixed seed
		// so the call doesn't blow up the entire CLI invocation.
		return "00000000"[:2*nBytes]
	}
	return hex.EncodeToString(b)
}

func formatLeaseConflict(s *store.Store, resID, holderEnvID string) error {
	holder, err := s.GetEnv(holderEnvID)
	if err != nil {
		return fmt.Errorf("resource %s held by env %s (lookup failed: %v)", resID, holderEnvID, err)
	}
	return fmt.Errorf("resource %s held by env %s (worktree=%s pid=%d since %s)",
		resID, holder.ID, holder.OwnerWorktree, holder.OwnerPID, holder.CreatedAt.Format("15:04:05"))
}
