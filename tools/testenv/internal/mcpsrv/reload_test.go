package mcpsrv

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func withReloadStubs(t *testing.T, updated bool) *atomic.Int64 {
	t.Helper()
	prevUpdated, prevExec, prevGrace := binaryUpdatedFn, execReloadFn, reloadGrace
	prevServer := mcpServer

	mcpServer = mcp.NewServer(&mcp.Implementation{Name: "testenv-test", Version: "0"}, nil)
	reloadArmed.Store(false)
	inflight.Store(0)
	reloadGrace = 5 * time.Millisecond
	binaryUpdatedFn = func() bool { return updated }
	var execs atomic.Int64
	execReloadFn = func() bool { execs.Add(1); return true }

	t.Cleanup(func() {
		binaryUpdatedFn = prevUpdated
		execReloadFn = prevExec
		reloadGrace = prevGrace
		mcpServer = prevServer
		reloadArmed.Store(false)
		inflight.Store(0)
	})
	return &execs
}

func waitFor(t *testing.T, cond func() bool) bool {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return true
		}
		time.Sleep(5 * time.Millisecond)
	}
	return false
}

func TestReloadIfUpdated_NoReloadWhenUnchanged(t *testing.T) {
	execs := withReloadStubs(t, false)
	reloadIfUpdated()
	time.Sleep(50 * time.Millisecond)
	if got := execs.Load(); got != 0 {
		t.Fatalf("reload fired with unchanged binary: execs=%d", got)
	}
}

func TestReloadIfUpdated_WaitsForInflightDrain(t *testing.T) {
	execs := withReloadStubs(t, true)

	inflight.Add(1) // simulate a request still in flight
	reloadIfUpdated()

	time.Sleep(50 * time.Millisecond)
	if got := execs.Load(); got != 0 {
		t.Fatalf("reloaded while a request was in flight: execs=%d", got)
	}

	inflight.Add(-1)
	if !waitFor(t, func() bool { return execs.Load() == 1 }) {
		t.Fatalf("reload did not fire after drain: execs=%d", execs.Load())
	}
}

func TestReloadIfUpdated_ArmsOnce(t *testing.T) {
	execs := withReloadStubs(t, true)

	inflight.Add(1)
	reloadIfUpdated()
	reloadIfUpdated()
	reloadIfUpdated()
	inflight.Add(-1)

	if !waitFor(t, func() bool { return execs.Load() >= 1 }) {
		t.Fatalf("reload never fired: execs=%d", execs.Load())
	}
	time.Sleep(50 * time.Millisecond)
	if got := execs.Load(); got != 1 {
		t.Fatalf("reload fired more than once: execs=%d", got)
	}
}
