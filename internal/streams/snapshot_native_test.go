package streams

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestCaptureNativeSnapshot_RoundTrip(t *testing.T) {
	// memfd backing the "dma-buf" with a known NV12 pattern (gray Y, neutral chroma).
	const w, h = 64, 32
	size := w * h * 3 / 2
	memfd, err := unix.MemfdCreate("snap-test", 0)
	if err != nil {
		t.Skipf("memfd_create unavailable: %v", err)
	}
	defer syscall.Close(memfd)
	if err := unix.Ftruncate(memfd, int64(size)); err != nil {
		t.Fatalf("ftruncate: %v", err)
	}
	mm, err := syscall.Mmap(memfd, 0, size, syscall.PROT_READ|syscall.PROT_WRITE, syscall.MAP_SHARED)
	if err != nil {
		t.Fatalf("mmap: %v", err)
	}
	for i := range size {
		mm[i] = 128
	}
	_ = syscall.Munmap(mm)

	dir := t.TempDir()
	sockPath := filepath.Join(dir, "snap.sock")
	ln, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	serverDone := make(chan error, 1)
	go func() {
		c, err := ln.Accept()
		if err != nil {
			serverDone <- err
			return
		}
		defer c.Close()
		env := frameNotification{
			JSONRPC: "2.0",
			Method:  "frame",
			Params: dmabufHeader{
				SlotIndex:    0,
				Width:        w,
				Height:       h,
				Format:       "NV12",
				PlanePitches: []uint32{w, w},
				PlaneOffsets: []uint32{0, uint32(w * h)},
				FrameIdx:     1,
			},
		}
		body, _ := json.Marshal(env)
		var buf bytes.Buffer
		_ = binary.Write(&buf, binary.BigEndian, uint32(len(body)))
		buf.Write(body)

		ucon := c.(*net.UnixConn)
		oob := unix.UnixRights(memfd)
		if _, _, err := ucon.WriteMsgUnix(buf.Bytes(), oob, nil); err != nil {
			serverDone <- err
			return
		}
		serverDone <- nil
	}()

	jpeg, err := captureNativeSnapshot(sockPath, 5*time.Second)
	if err != nil {
		t.Fatalf("captureNativeSnapshot: %v", err)
	}
	if len(jpeg) < 4 || jpeg[0] != 0xFF || jpeg[1] != 0xD8 || jpeg[2] != 0xFF {
		t.Fatalf("output is not JPEG (len=%d, prefix=% x)", len(jpeg), jpeg[:min(4, len(jpeg))])
	}

	if err := <-serverDone; err != nil {
		t.Errorf("server error: %v", err)
	}
}

func TestCaptureNativeSnapshot_DialFailure(t *testing.T) {
	dir := t.TempDir()
	missing := filepath.Join(dir, "does-not-exist.sock")
	if _, err := captureNativeSnapshot(missing, 100*time.Millisecond); err == nil {
		t.Error("expected error dialing missing socket")
	}
}

func TestCaptureNativeSnapshot_BadHeaderLength(t *testing.T) {
	dir := t.TempDir()
	sockPath := filepath.Join(dir, "bad.sock")
	ln, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()
	go func() {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		// Length = 100000 (over our 65536 cap).
		var prefix [4]byte
		binary.BigEndian.PutUint32(prefix[:], 100000)
		_, _ = c.Write(prefix[:])
		c.Close()
	}()
	if _, err := captureNativeSnapshot(sockPath, time.Second); err == nil {
		t.Error("expected bad-header-length error")
	}
	// Suppress unused import in the rare event it isn't needed.
	_ = os.Getuid
}
