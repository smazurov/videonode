package store

import (
	"database/sql"
	"path/filepath"
	"testing"
)

func tempStore(t *testing.T) *Store {
	t.Helper()
	path := filepath.Join(t.TempDir(), "state.db")
	s, err := Open(path)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestCreateAndGetEnv(t *testing.T) {
	s := tempStore(t)
	e := Env{
		ID: "env-1", OwnerSession: "sess-1", OwnerPID: 12345,
		OwnerWorktree: "/tmp/wt", Target: "host", SourceMode: "fake",
		Slot: 1, HTTPURL: "http://localhost:8100", RTSPURL: "rtsp://localhost:8564",
		SRTURL: "srt://localhost:6011", DataDir: "/tmp/d", StreamsTOML: "/tmp/s.toml",
	}
	if err := s.CreateEnv(e); err != nil {
		t.Fatalf("create: %v", err)
	}
	got, err := s.GetEnv("env-1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Slot != 1 || got.HTTPURL != "http://localhost:8100" {
		t.Errorf("got slot=%d url=%s", got.Slot, got.HTTPURL)
	}
}

func TestSlotUniqueness(t *testing.T) {
	s := tempStore(t)
	e := Env{
		ID: "env-1", OwnerSession: "s1", OwnerPID: 1,
		OwnerWorktree: "/tmp", Target: "host", SourceMode: "fake",
		Slot: 1, HTTPURL: "h", RTSPURL: "r", SRTURL: "s",
		DataDir: "/d", StreamsTOML: "/t",
	}
	if err := s.CreateEnv(e); err != nil {
		t.Fatalf("first create: %v", err)
	}
	e.ID = "env-2"
	e.OwnerSession = "s2"
	if err := s.CreateEnv(e); err != ErrSlotTaken {
		t.Errorf("expected ErrSlotTaken, got %v", err)
	}
}

func TestDeleteCascadesLeases(t *testing.T) {
	s := tempStore(t)
	e := Env{
		ID: "env-1", OwnerSession: "s", OwnerPID: 1,
		OwnerWorktree: "/tmp", Target: "host", SourceMode: "fake",
		Slot: 1, HTTPURL: "h", RTSPURL: "r", SRTURL: "s",
		DataDir: "/d", StreamsTOML: "/t",
	}
	if err := s.CreateEnv(e); err != nil {
		t.Fatal(err)
	}
	if err := s.LeaseAcquire("device:/dev/video0", "env-1"); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteEnv("env-1"); err != nil {
		t.Fatal(err)
	}
	_, err := s.LeaseHolder("device:/dev/video0")
	if err != sql.ErrNoRows {
		t.Errorf("expected ErrNoRows after cascade delete, got %v", err)
	}
}

func TestLeaseConflict(t *testing.T) {
	s := tempStore(t)
	for _, id := range []string{"env-1", "env-2"} {
		e := Env{
			ID: id, OwnerSession: "s-" + id, OwnerPID: 1,
			OwnerWorktree: "/tmp", Target: "host", SourceMode: "fake",
			HTTPURL: "h", RTSPURL: "r", SRTURL: "s",
			DataDir: "/d", StreamsTOML: "/t",
		}
		if id == "env-1" {
			e.Slot = 1
		} else {
			e.Slot = 2
		}
		if err := s.CreateEnv(e); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.LeaseAcquire("device:/dev/video0", "env-1"); err != nil {
		t.Fatal(err)
	}
	if err := s.LeaseAcquire("device:/dev/video0", "env-2"); err != ErrLeaseHeld {
		t.Errorf("expected ErrLeaseHeld, got %v", err)
	}
}

func TestTakenSlots(t *testing.T) {
	s := tempStore(t)
	for i, id := range []string{"env-a", "env-b"} {
		e := Env{
			ID: id, OwnerSession: "s", OwnerPID: 1,
			OwnerWorktree: "/tmp", Target: "host", SourceMode: "fake",
			Slot: i + 1, HTTPURL: "h", RTSPURL: "r", SRTURL: "s",
			DataDir: "/d", StreamsTOML: "/t",
		}
		if err := s.CreateEnv(e); err != nil {
			t.Fatal(err)
		}
	}
	taken, err := s.TakenSlots()
	if err != nil {
		t.Fatal(err)
	}
	if len(taken) != 2 || taken[0] != 1 || taken[1] != 2 {
		t.Errorf("expected [1 2], got %v", taken)
	}
}

func TestDeleteEnvsForSession(t *testing.T) {
	s := tempStore(t)
	for i, e := range []Env{
		{ID: "e1", OwnerSession: "sess-A", Slot: 1},
		{ID: "e2", OwnerSession: "sess-A", Slot: 2},
		{ID: "e3", OwnerSession: "sess-B", Slot: 3},
	} {
		e.OwnerPID = 1
		e.OwnerWorktree = "/tmp"
		e.Target = "host"
		e.SourceMode = "fake"
		e.HTTPURL = "h"
		e.RTSPURL = "r"
		e.SRTURL = "s"
		e.DataDir = "/d"
		e.StreamsTOML = "/t"
		_ = i
		if err := s.CreateEnv(e); err != nil {
			t.Fatal(err)
		}
	}
	released, err := s.DeleteEnvsForSession("sess-A")
	if err != nil {
		t.Fatal(err)
	}
	if len(released) != 2 {
		t.Errorf("expected 2 released, got %d", len(released))
	}
	envs, _ := s.ListEnvs()
	if len(envs) != 1 || envs[0].ID != "e3" {
		t.Errorf("expected only e3 remaining, got %v", envs)
	}
}
