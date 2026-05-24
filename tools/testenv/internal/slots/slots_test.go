package slots

import (
	"fmt"
	"net"
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

func TestPickReturnsValidSlot(t *testing.T) {
	s := tempStore(t)
	held, err := Pick(s)
	if err != nil {
		t.Fatal(err)
	}
	defer held.Release()
	if held.Triple.Slot < 1 || held.Triple.Slot > MaxSlot {
		t.Errorf("slot %d out of range [1..%d]", held.Triple.Slot, MaxSlot)
	}
	if len(held.Listeners) != 3 {
		t.Errorf("expected 3 held listeners, got %d", len(held.Listeners))
	}
}

func TestPortsForSlot(t *testing.T) {
	http, rtsp, srt := PortsForSlot(1)
	if http != 8100 || rtsp != 8564 || srt != 6011 {
		t.Errorf("unexpected ports for slot 1: %d %d %d", http, rtsp, srt)
	}
	http, rtsp, srt = PortsForSlot(9)
	if http != 8180 || rtsp != 8644 || srt != 6091 {
		t.Errorf("unexpected ports for slot 9: %d %d %d", http, rtsp, srt)
	}
}

func TestPickSkipsBlockedPort(t *testing.T) {
	s := tempStore(t)

	// Block slot 1's HTTP port externally.
	http1, _, _ := PortsForSlot(1)
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", http1))
	if err != nil {
		t.Skipf("cannot bind :%d for test setup: %v", http1, err)
	}
	defer ln.Close()

	held, err := Pick(s)
	if err != nil {
		t.Fatal(err)
	}
	defer held.Release()
	if held.Triple.Slot != 2 {
		t.Errorf("expected slot 2 (slot 1 blocked), got %d", held.Triple.Slot)
	}
}

func TestPickSkipsTakenSlot(t *testing.T) {
	s := tempStore(t)
	e := store.Env{
		ID: "e1", OwnerSession: "s", OwnerPID: 1, OwnerWorktree: "/tmp",
		Target: "host", SourceMode: "fake", Slot: 1,
		HTTPURL: "h", RTSPURL: "r", SRTURL: "s", DataDir: "/d", StreamsTOML: "/t",
	}
	if err := s.CreateEnv(e); err != nil {
		t.Fatal(err)
	}
	held, err := Pick(s)
	if err != nil {
		t.Fatal(err)
	}
	defer held.Release()
	if held.Triple.Slot != 2 {
		t.Errorf("expected slot 2 (slot 1 taken in DB), got %d", held.Triple.Slot)
	}
}

