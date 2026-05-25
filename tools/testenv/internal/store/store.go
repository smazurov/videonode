// Package store is the SQLite-backed registry for testenv.
//
// Single state file at $XDG_STATE_HOME/testenv/state.db (defaulting to
// ~/.local/state/testenv/state.db). All cross-session coordination
// happens through it: envs holds one row per active test environment;
// leases holds named resource leases held by those envs.
//
// Concurrent access is safe by SQLite WAL semantics; a sibling
// state.db.lock advisory flock under gofrs/flock serializes the write
// paths that must be atomic across processes (slot allocation
// commit, lease acquisition).
package store

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/gofrs/flock"
	_ "modernc.org/sqlite"
)

// Env is a registered test environment.
type Env struct {
	ID            string
	OwnerSession  string
	OwnerPID      int
	OwnerWorktree string
	Target        string // "host" | "sbc"
	SourceMode    string // "real" | "fake"
	Slot          int
	HTTPURL       string
	RTSPURL       string
	SRTURL        string
	HealthURL     string
	HealthAuth    string
	NativeBinDir  string
	DataDir       string
	StreamsTOML   string
	CreatedAt     time.Time
}

// Lease is a held resource lease (device, sbc, etc.).
type Lease struct {
	ResourceID string
	EnvID      string
	AcquiredAt time.Time
}

// ErrSlotTaken is returned when CreateEnv collides on the slot uniqueness constraint.
var ErrSlotTaken = errors.New("slot already taken")

// ErrLeaseHeld is returned when LeaseAcquire collides on the resource_id primary key.
var ErrLeaseHeld = errors.New("lease already held")

// Store wraps the SQLite database plus its cross-process write lock.
type Store struct {
	db   *sql.DB
	lock *flock.Flock
	path string
}

// DefaultPath returns the canonical state file path under XDG_STATE_HOME
// (or ~/.local/state as the XDG fallback).
func DefaultPath() string {
	dir := os.Getenv("XDG_STATE_HOME")
	if dir == "" {
		home, _ := os.UserHomeDir()
		dir = filepath.Join(home, ".local", "state")
	}
	return filepath.Join(dir, "testenv", "state.db")
}

// Open opens (creating if necessary) the state DB at path. Parent dirs
// are created. WAL is enabled. The schema is applied idempotently.
func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("mkdir state dir: %w", err)
	}
	dsn := fmt.Sprintf("file:%s?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(ON)", path)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	db.SetMaxOpenConns(1) // serialize via Go too; SQLite WAL handles cross-proc.
	s := &Store{
		db:   db,
		lock: flock.New(path + ".lock"),
		path: path,
	}
	if err := s.migrate(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

// Close releases the DB handle. The advisory lock file persists on disk
// but is harmless when no process is holding it.
func (s *Store) Close() error { return s.db.Close() }

// Path returns the state file path.
func (s *Store) Path() string { return s.path }

// WithLock runs fn under the cross-process write lock. Use for any
// multi-statement mutation that must be atomic across processes (slot
// allocation + env insert, lease acquire-check + insert).
func (s *Store) WithLock(fn func() error) error {
	if err := s.lock.Lock(); err != nil {
		return fmt.Errorf("acquire flock: %w", err)
	}
	defer func() { _ = s.lock.Unlock() }()
	return fn()
}

func (s *Store) migrate() error {
	_, err := s.db.Exec(`
CREATE TABLE IF NOT EXISTS envs (
  id              TEXT PRIMARY KEY,
  owner_session   TEXT NOT NULL,
  owner_pid       INTEGER NOT NULL,
  owner_worktree  TEXT NOT NULL,
  target          TEXT NOT NULL,
  source_mode     TEXT NOT NULL,
  slot            INTEGER NOT NULL UNIQUE,
  http_url        TEXT NOT NULL,
  rtsp_url        TEXT NOT NULL,
  srt_url         TEXT NOT NULL,
  native_bin_dir  TEXT NOT NULL DEFAULT '',
  data_dir        TEXT NOT NULL,
  streams_toml    TEXT NOT NULL,
  created_at      INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS leases (
  resource_id  TEXT PRIMARY KEY,
  env_id       TEXT NOT NULL REFERENCES envs(id) ON DELETE CASCADE,
  acquired_at  INTEGER NOT NULL
);
`)
	if err != nil {
		return err
	}
	for _, col := range []string{
		"health_url TEXT NOT NULL DEFAULT ''",
		"health_auth TEXT NOT NULL DEFAULT ''",
	} {
		_, _ = s.db.Exec("ALTER TABLE envs ADD COLUMN " + col)
	}
	return nil
}

// CreateEnv inserts a new env row. Returns ErrSlotTaken if the slot is in use.
func (s *Store) CreateEnv(e Env) error {
	_, err := s.db.Exec(`
INSERT INTO envs (id, owner_session, owner_pid, owner_worktree, target, source_mode,
                  slot, http_url, rtsp_url, srt_url, health_url, health_auth,
                  native_bin_dir, data_dir, streams_toml, created_at)
VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		e.ID, e.OwnerSession, e.OwnerPID, e.OwnerWorktree, e.Target, e.SourceMode,
		e.Slot, e.HTTPURL, e.RTSPURL, e.SRTURL, e.HealthURL, e.HealthAuth,
		e.NativeBinDir, e.DataDir, e.StreamsTOML, time.Now().Unix(),
	)
	if err != nil && isUniqueErr(err) {
		return ErrSlotTaken
	}
	return err
}

// DeleteEnv removes an env (cascades to its leases).
func (s *Store) DeleteEnv(id string) error {
	_, err := s.db.Exec(`DELETE FROM envs WHERE id = ?`, id)
	return err
}

// UpdateEnvAfterSpawn sets the daemon PID and native-binary dir on an
// env that has just been spawned.
func (s *Store) UpdateEnvAfterSpawn(id string, pid int, nativeBinDir string) error {
	_, err := s.db.Exec(
		`UPDATE envs SET owner_pid = ?, native_bin_dir = ? WHERE id = ?`,
		pid, nativeBinDir, id,
	)
	return err
}

// UpdateEnvAfterRestart sets PID + health fields after a restart.
func (s *Store) UpdateEnvAfterRestart(id string, pid int, healthURL, healthAuth string) error {
	_, err := s.db.Exec(
		`UPDATE envs SET owner_pid = ?, health_url = ?, health_auth = ? WHERE id = ?`,
		pid, healthURL, healthAuth, id,
	)
	return err
}

// DeleteEnvsForSession removes every env owned by session. Returns the
// removed env ids.
func (s *Store) DeleteEnvsForSession(session string) ([]string, error) {
	rows, err := s.db.Query(`SELECT id FROM envs WHERE owner_session = ?`, session)
	if err != nil {
		return nil, err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	rows.Close()
	for _, id := range ids {
		if delErr := s.DeleteEnv(id); delErr != nil {
			return ids, delErr
		}
	}
	return ids, nil
}

// GetEnv returns the env by id, or sql.ErrNoRows if missing.
func (s *Store) GetEnv(id string) (Env, error) {
	row := s.db.QueryRow(`SELECT id, owner_session, owner_pid, owner_worktree, target,
		source_mode, slot, http_url, rtsp_url, srt_url, health_url, health_auth,
		native_bin_dir, data_dir, streams_toml, created_at FROM envs WHERE id = ?`, id)
	return scanEnv(row)
}

// GetEnvBySession returns the env owned by the given session, or sql.ErrNoRows.
func (s *Store) GetEnvBySession(session string) (Env, error) {
	row := s.db.QueryRow(`SELECT id, owner_session, owner_pid, owner_worktree, target,
		source_mode, slot, http_url, rtsp_url, srt_url, health_url, health_auth,
		native_bin_dir, data_dir, streams_toml, created_at FROM envs WHERE owner_session = ? LIMIT 1`, session)
	return scanEnv(row)
}

// ListEnvs returns all envs ordered by slot.
func (s *Store) ListEnvs() ([]Env, error) {
	rows, err := s.db.Query(`SELECT id, owner_session, owner_pid, owner_worktree, target,
		source_mode, slot, http_url, rtsp_url, srt_url, health_url, health_auth,
		native_bin_dir, data_dir, streams_toml, created_at FROM envs ORDER BY slot`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Env
	for rows.Next() {
		e, err := scanEnv(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// TakenSlots returns the slots currently held by registered envs.
func (s *Store) TakenSlots() ([]int, error) {
	rows, err := s.db.Query(`SELECT slot FROM envs ORDER BY slot`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []int
	for rows.Next() {
		var n int
		if err := rows.Scan(&n); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

// LeaseAcquire claims a named resource for env. Returns ErrLeaseHeld if taken.
func (s *Store) LeaseAcquire(resourceID, envID string) error {
	_, err := s.db.Exec(
		`INSERT INTO leases (resource_id, env_id, acquired_at) VALUES (?,?,?)`,
		resourceID, envID, time.Now().Unix(),
	)
	if err != nil && isUniqueErr(err) {
		return ErrLeaseHeld
	}
	return err
}

// LeaseRelease drops a resource lease.
func (s *Store) LeaseRelease(resourceID string) error {
	_, err := s.db.Exec(`DELETE FROM leases WHERE resource_id = ?`, resourceID)
	return err
}

// LeaseHolder returns the env_id holding resource_id, or sql.ErrNoRows.
func (s *Store) LeaseHolder(resourceID string) (string, error) {
	var envID string
	err := s.db.QueryRow(`SELECT env_id FROM leases WHERE resource_id = ?`, resourceID).Scan(&envID)
	return envID, err
}

// ListLeasesFor returns all leases held by env_id.
func (s *Store) ListLeasesFor(envID string) ([]Lease, error) {
	rows, err := s.db.Query(
		`SELECT resource_id, env_id, acquired_at FROM leases WHERE env_id = ? ORDER BY resource_id`,
		envID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Lease
	for rows.Next() {
		var l Lease
		var ts int64
		if err := rows.Scan(&l.ResourceID, &l.EnvID, &ts); err != nil {
			return nil, err
		}
		l.AcquiredAt = time.Unix(ts, 0)
		out = append(out, l)
	}
	return out, rows.Err()
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanEnv(r rowScanner) (Env, error) {
	var e Env
	var ts int64
	err := r.Scan(&e.ID, &e.OwnerSession, &e.OwnerPID, &e.OwnerWorktree, &e.Target,
		&e.SourceMode, &e.Slot, &e.HTTPURL, &e.RTSPURL, &e.SRTURL, &e.HealthURL, &e.HealthAuth,
		&e.NativeBinDir, &e.DataDir, &e.StreamsTOML, &ts)
	if err != nil {
		return Env{}, err
	}
	e.CreatedAt = time.Unix(ts, 0)
	return e, nil
}

// isUniqueErr returns true when err is a SQLite UNIQUE-constraint failure.
// modernc.org/sqlite returns errors whose string contains "UNIQUE constraint failed".
func isUniqueErr(err error) bool {
	if err == nil {
		return false
	}
	return containsCI(err.Error(), "UNIQUE constraint failed")
}

func containsCI(haystack, needle string) bool {
	if len(needle) > len(haystack) {
		return false
	}
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if eqFold(haystack[i:i+len(needle)], needle) {
			return true
		}
	}
	return false
}

func eqFold(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		ca, cb := a[i], b[i]
		if ca == cb {
			continue
		}
		if ca >= 'A' && ca <= 'Z' {
			ca += 32
		}
		if cb >= 'A' && cb <= 'Z' {
			cb += 32
		}
		if ca != cb {
			return false
		}
	}
	return true
}
