package sourcectl

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func tempSocket(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "ctl.sock")
}

// fakeSidecar represents a videonode-source connecting to the daemon.
type fakeSidecar struct {
	conn net.Conn
	rdr  *bufio.Reader
}

func dialFake(t *testing.T, sock string) *fakeSidecar {
	t.Helper()
	var c net.Conn
	var err error
	for range 50 {
		c, err = net.Dial("unix", sock)
		if err == nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	return &fakeSidecar{conn: c, rdr: bufio.NewReader(c)}
}

func (f *fakeSidecar) close() { _ = f.conn.Close() }

func (f *fakeSidecar) sendLine(t *testing.T, msg any) {
	t.Helper()
	b, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	b = append(b, '\n')
	if _, err := f.conn.Write(b); err != nil {
		t.Fatalf("write: %v", err)
	}
}

func (f *fakeSidecar) readLine(t *testing.T, timeout time.Duration) ([]byte, error) {
	t.Helper()
	if err := f.conn.SetReadDeadline(time.Now().Add(timeout)); err != nil {
		t.Fatalf("set deadline: %v", err)
	}
	line, err := f.rdr.ReadBytes('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}
	return line, nil
}

func TestServer_IdentifyAndStatus(t *testing.T) {
	sock := tempSocket(t)
	srv := New(sock, nil)
	if err := srv.Start(context.Background()); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer func() { _ = srv.Stop() }()

	side := dialFake(t, sock)
	defer side.close()

	// Identify.
	side.sendLine(t, map[string]any{
		"jsonrpc": "2.0",
		"method":  "identify",
		"params": map[string]any{
			"device_id": "test-dev-1",
			"pid":       1234,
			"version":   "v0",
		},
	})

	// Wait for the server to register us.
	deadline := time.Now().Add(2 * time.Second)
	for {
		if devs := srv.ConnectedDevices(); len(devs) == 1 && devs[0] == "test-dev-1" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("device not registered; connected=%v", srv.ConnectedDevices())
		}
		time.Sleep(20 * time.Millisecond)
	}

	// Send a status notification.
	side.sendLine(t, map[string]any{
		"jsonrpc": "2.0",
		"method":  "status",
		"params": map[string]any{
			"device_id": "test-dev-1",
			"ts_ms":     0,
			"health":    "LIVE",
			"device":    map[string]any{"path": "/dev/video0", "multiplanar": false},
			"signal":    map[string]any{"has_dv_timings": true, "cable_present": true, "signal_locked": true, "dv_timings": "locked"},
			"format":    map[string]any{"fourcc": "NV12", "w": 1920, "h": 1080, "fps": 30, "buffers": 4, "mode": "rga"},
			"broadcast": map[string]any{"target_fps": 60, "real_frames": 1, "placeholder_frames": 0, "last_seq": 0},
			"consumers": map[string]any{"count": 0, "live": []any{}, "evicted": []any{}},
		},
	})

	select {
	case got := <-srv.StatusFeed():
		if got.DeviceID != "test-dev-1" {
			t.Fatalf("device_id: want test-dev-1, got %s", got.DeviceID)
		}
		if got.Health != "LIVE" {
			t.Fatalf("health: want LIVE, got %s", got.Health)
		}
		if got.Format.FourCC != "NV12" {
			t.Fatalf("fourcc: want NV12, got %s", got.Format.FourCC)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("no status event after 2s")
	}
}

func TestServer_SendSetFormat(t *testing.T) {
	sock := tempSocket(t)
	srv := New(sock, nil)
	if err := srv.Start(context.Background()); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer func() { _ = srv.Stop() }()

	side := dialFake(t, sock)
	defer side.close()

	side.sendLine(t, map[string]any{
		"jsonrpc": "2.0",
		"method":  "identify",
		"params":  map[string]any{"device_id": "dev-x", "pid": 1},
	})

	// Wait for registration.
	deadline := time.Now().Add(2 * time.Second)
	for len(srv.ConnectedDevices()) != 1 {
		if time.Now().After(deadline) {
			t.Fatal("registration timeout")
		}
		time.Sleep(20 * time.Millisecond)
	}

	// Read server-pushed request in a goroutine and reply.
	gotReq := make(chan string, 1)
	go func() {
		line, err := side.readLine(t, 2*time.Second)
		if err != nil {
			gotReq <- ""
			return
		}
		// Parse the line we got.
		var req struct {
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
			ID     json.RawMessage `json:"id"`
		}
		if err := json.Unmarshal(line, &req); err != nil {
			gotReq <- ""
			return
		}
		gotReq <- req.Method
		// Send a Response back.
		resp := map[string]any{
			"jsonrpc": "2.0",
			"result":  map[string]bool{"applied": true},
		}
		// re-attach the same id verbatim
		rb, _ := json.Marshal(resp)
		// Inject the id key — easier to just rewrite the JSON.
		// jrpc2 needs a properly correlated id; the easiest path is to
		// build the response from scratch with the same ID.
		respFinal := strings.TrimSuffix(string(rb), "}") + `,"id":` + string(req.ID) + `}` + "\n"
		_, _ = side.conn.Write([]byte(respFinal))
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	result, err := srv.SendSetFormat(ctx, "dev-x", SetFormatParams{
		FourCC: "YUYV", W: 1920, H: 1080, FPS: 30,
	})
	if err != nil {
		t.Fatalf("SendSetFormat: %v", err)
	}
	if !result.Applied {
		t.Fatalf("Applied: want true")
	}
	if method := <-gotReq; method != "set_format" {
		t.Fatalf("wire method: want set_format, got %q", method)
	}
}

func TestServer_HeartbeatWatchdogDisconnects(t *testing.T) {
	// Shorten the timeout for the test by using a manual lastSeen poke.
	sock := tempSocket(t)
	srv := New(sock, nil)
	if err := srv.Start(context.Background()); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer func() { _ = srv.Stop() }()

	side := dialFake(t, sock)
	defer side.close()

	side.sendLine(t, map[string]any{
		"jsonrpc": "2.0",
		"method":  "identify",
		"params":  map[string]any{"device_id": "dev-y", "pid": 1},
	})

	// Wait for registration.
	deadline := time.Now().Add(2 * time.Second)
	for len(srv.ConnectedDevices()) != 1 {
		if time.Now().After(deadline) {
			t.Fatal("registration timeout")
		}
		time.Sleep(20 * time.Millisecond)
	}

	// Manually backdate lastSeen so the watchdog disconnects on its
	// next tick (~1s). The watchdog reads via atomic.Int64.Load.
	srv.mu.RLock()
	conn := srv.sidecars["dev-y"]
	srv.mu.RUnlock()
	if conn == nil {
		t.Fatal("missing connection record")
	}
	conn.lastSeen.Store(time.Now().Add(-5 * time.Second).UnixNano())

	// Expect disconnection within ~2 ticks.
	deadline = time.Now().Add(3 * time.Second)
	for len(srv.ConnectedDevices()) != 0 {
		if time.Now().After(deadline) {
			t.Fatal("watchdog did not disconnect")
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func TestServer_RejectsDuplicateIdentify(t *testing.T) {
	sock := tempSocket(t)
	srv := New(sock, nil)
	if err := srv.Start(context.Background()); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer func() { _ = srv.Stop() }()

	side := dialFake(t, sock)
	defer side.close()

	send := func(id any) {
		side.sendLine(t, map[string]any{
			"jsonrpc": "2.0",
			"method":  "identify",
			"params":  map[string]any{"device_id": "dev-z", "pid": 1},
			"id":      id,
		})
	}
	// First identify as a Request (with id) — we want a response back.
	send(1)
	if _, err := side.readLine(t, 1*time.Second); err != nil {
		t.Fatalf("read first identify ack: %v", err)
	}
	// Second identify should fail.
	send(2)
	line, err := side.readLine(t, 1*time.Second)
	if err != nil {
		t.Fatalf("read second identify reply: %v", err)
	}
	if !strings.Contains(string(line), "already identified") {
		t.Fatalf("expected duplicate-identify error, got %q", line)
	}
}

func TestServer_DeviceReplacement(t *testing.T) {
	// Two sidecars claiming the same device — the newer one wins.
	sock := tempSocket(t)
	srv := New(sock, nil)
	if err := srv.Start(context.Background()); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer func() { _ = srv.Stop() }()

	old := dialFake(t, sock)
	defer old.close()
	old.sendLine(t, map[string]any{
		"jsonrpc": "2.0",
		"method":  "identify",
		"params":  map[string]any{"device_id": "dup", "pid": 1},
	})

	// Wait for first registration.
	deadline := time.Now().Add(2 * time.Second)
	for len(srv.ConnectedDevices()) != 1 {
		if time.Now().After(deadline) {
			t.Fatal("first registration timeout")
		}
		time.Sleep(20 * time.Millisecond)
	}

	newer := dialFake(t, sock)
	defer newer.close()
	newer.sendLine(t, map[string]any{
		"jsonrpc": "2.0",
		"method":  "identify",
		"params":  map[string]any{"device_id": "dup", "pid": 2},
	})

	// Eventually exactly one connection should remain — the newer one.
	deadline = time.Now().Add(3 * time.Second)
	for {
		count := len(srv.ConnectedDevices())
		if count == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("expected one connection after replacement, have %d", count)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// Smoke test: a fast batch of status notifications must not deadlock.
func TestServer_StatusFanOutNonBlocking(t *testing.T) {
	sock := tempSocket(t)
	srv := New(sock, nil)
	if err := srv.Start(context.Background()); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer func() { _ = srv.Stop() }()

	side := dialFake(t, sock)
	defer side.close()

	side.sendLine(t, map[string]any{
		"jsonrpc": "2.0",
		"method":  "identify",
		"params":  map[string]any{"device_id": "fast", "pid": 1},
	})

	deadline := time.Now().Add(2 * time.Second)
	for len(srv.ConnectedDevices()) != 1 {
		if time.Now().After(deadline) {
			t.Fatal("registration timeout")
		}
		time.Sleep(20 * time.Millisecond)
	}

	// Drain the channel concurrently.
	var seen atomic.Int64
	stopDrain := make(chan struct{})
	go func() {
		for {
			select {
			case <-stopDrain:
				return
			case <-srv.StatusFeed():
				seen.Add(1)
			}
		}
	}()

	for i := range 200 {
		side.sendLine(t, map[string]any{
			"jsonrpc": "2.0",
			"method":  "status",
			"params": map[string]any{
				"device_id": "fast",
				"ts_ms":     i,
				"health":    "LIVE",
				"device":    map[string]any{},
				"signal":    map[string]any{},
				"format":    map[string]any{},
				"broadcast": map[string]any{},
				"consumers": map[string]any{"count": 0, "live": []any{}, "evicted": []any{}},
			},
		})
	}
	// Allow time for the server to process.
	time.Sleep(300 * time.Millisecond)
	close(stopDrain)
	if seen.Load() == 0 {
		t.Fatal("no status events received under load")
	}
}
