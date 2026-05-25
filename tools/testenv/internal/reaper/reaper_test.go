package reaper

import (
	"os"
	"testing"

	"github.com/smazurov/videonode/tools/testenv/internal/store"
)

func tempStore(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.Open(t.TempDir() + "/state.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestReapRemovesDeadPID(t *testing.T) {
	s := tempStore(t)
	e := store.Env{
		ID: "dead", OwnerSession: "s", OwnerPID: 999999999, // unlikely to exist
		OwnerWorktree: "/tmp", Target: "host", SourceMode: "fake",
		Slot: 1, HTTPURL: "h", RTSPURL: "r", SRTURL: "s",
		DataDir: "/d", StreamsTOML: "/t",
	}
	if err := s.CreateEnv(e); err != nil {
		t.Fatal(err)
	}
	released, err := Reap(s)
	if err != nil {
		t.Fatal(err)
	}
	if len(released) != 1 || released[0].ID != "dead" {
		t.Errorf("expected [dead], got %v", released)
	}
}

func TestReapKeepsAlivePID(t *testing.T) {
	s := tempStore(t)
	e := store.Env{
		ID: "alive", OwnerSession: "s", OwnerPID: os.Getpid(),
		OwnerWorktree: "/tmp", Target: "host", SourceMode: "fake",
		Slot: 1, HTTPURL: "h", RTSPURL: "r", SRTURL: "s",
		DataDir: "/d", StreamsTOML: "/t",
	}
	if err := s.CreateEnv(e); err != nil {
		t.Fatal(err)
	}
	released, err := Reap(s)
	if err != nil {
		t.Fatal(err)
	}
	if len(released) != 0 {
		t.Errorf("expected nothing reaped, got %v", released)
	}
}
