package slots

import (
	"fmt"
	"net"
	"testing"

	"github.com/smazurov/videonode/tools/testenv/internal/config"
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

func testConfig() *config.V1 {
	return &config.V1{
		Version:  1,
		MaxSlots: 9,
		Ports: map[string]config.Port{
			"http": {Base: 8090, Step: 10},
			"rtsp": {Base: 8554, Step: 10},
		},
	}
}

func TestPickReturnsValidSlot(t *testing.T) {
	s := tempStore(t)
	cfg := testConfig()
	held, err := Pick(s, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer held.Release()
	if held.Slot < 1 || held.Slot > cfg.MaxSlots {
		t.Errorf("slot %d out of range [1..%d]", held.Slot, cfg.MaxSlots)
	}
	if len(held.Listeners) != 2 {
		t.Errorf("expected 2 listeners (http+rtsp), got %d", len(held.Listeners))
	}
	if held.Ports["http"] != cfg.PortForSlot("http", held.Slot) {
		t.Errorf("http port mismatch: got %d want %d", held.Ports["http"], cfg.PortForSlot("http", held.Slot))
	}
}

func TestPickSkipsBlockedPort(t *testing.T) {
	s := tempStore(t)
	cfg := testConfig()

	port1 := cfg.PortForSlot("http", 1)
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port1))
	if err != nil {
		t.Skipf("cannot bind :%d for test setup: %v", port1, err)
	}
	defer ln.Close()

	held, err := Pick(s, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer held.Release()
	if held.Slot == 1 {
		t.Error("expected slot 1 to be skipped (port blocked)")
	}
}

func TestPickSkipsTakenSlot(t *testing.T) {
	s := tempStore(t)
	cfg := testConfig()
	e := store.Env{
		ID: "e1", OwnerSession: "s", OwnerPID: 1, OwnerWorktree: "/tmp",
		Target: "host", SourceMode: "fake", Slot: 1,
		HTTPURL: "h", RTSPURL: "r", SRTURL: "s", DataDir: "/d", StreamsTOML: "/t",
	}
	if err := s.CreateEnv(e); err != nil {
		t.Fatal(err)
	}
	held, err := Pick(s, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer held.Release()
	if held.Slot == 1 {
		t.Error("expected slot 1 to be skipped (taken in DB)")
	}
}
