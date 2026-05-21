package streams

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"syscall"
	"time"

	"golang.org/x/sys/unix"

	"github.com/smazurov/videonode/internal/ffmpeg"
)

// dmabufHeader mirrors the inner `params` shape of the JSON-RPC 2.0
// `frame` notification encoded by composer/src/dmabuf_msg.cpp.
type dmabufHeader struct {
	SlotIndex    uint32   `json:"slot_index"`
	Width        uint32   `json:"width"`
	Height       uint32   `json:"height"`
	Format       string   `json:"format"`
	PlanePitches []uint32 `json:"plane_pitches"`
	PlaneOffsets []uint32 `json:"plane_offsets"`
	FrameIdx     uint64   `json:"frame_idx"`
}

// frameNotification is the JSON-RPC 2.0 envelope around dmabufHeader. The
// videonode-source producer sends it length-prefixed (4-byte big-endian)
// alongside SCM_RIGHTS ancillary fds.
type frameNotification struct {
	JSONRPC string       `json:"jsonrpc"`
	Method  string       `json:"method"`
	Params  dmabufHeader `json:"params"`
}

// captureNativeSnapshot dials a videonode-source SCM_RIGHTS socket, reads
// one frame, mmaps the NV12 plane, JPEG-encodes via ffmpeg subprocess.
// Returns JPEG bytes. Caller ensures the producer is running.
func captureNativeSnapshot(socketPath string, timeout time.Duration) ([]byte, error) {
	fd, err := syscall.Socket(syscall.AF_UNIX, syscall.SOCK_STREAM, 0)
	if err != nil {
		return nil, fmt.Errorf("snapshot socket: %w", err)
	}
	defer syscall.Close(fd)

	if err := syscall.Connect(fd, &syscall.SockaddrUnix{Name: socketPath}); err != nil {
		return nil, fmt.Errorf("snapshot dial %s: %w", socketPath, err)
	}

	deadline := time.Now().Add(timeout)
	usec := timeout.Microseconds()
	tv := syscall.Timeval{Sec: usec / 1_000_000, Usec: usec % 1_000_000}
	_ = syscall.SetsockoptTimeval(fd, syscall.SOL_SOCKET, syscall.SO_RCVTIMEO, &tv)

	// One frame: 4-byte big-endian length prefix + JSON header (data) +
	// SCM_RIGHTS ancillary carrying the plane fd(s). Producer sends it in
	// a single sendmsg; we may receive in fewer or more recvmsg calls.
	var prefix [4]byte
	planeFDs, n, err := recvAll(fd, prefix[:], deadline)
	if err != nil {
		return nil, fmt.Errorf("snapshot read prefix: %w", err)
	}
	if n != len(prefix) {
		return nil, fmt.Errorf("snapshot: short prefix read (%d)", n)
	}
	hdrLen := binary.BigEndian.Uint32(prefix[:])
	if hdrLen == 0 || hdrLen > 65536 {
		closeFDs(planeFDs)
		return nil, fmt.Errorf("snapshot: bad header length %d", hdrLen)
	}

	body := make([]byte, hdrLen)
	moreFDs, m, err := recvAll(fd, body, deadline)
	if err != nil {
		closeFDs(planeFDs)
		return nil, fmt.Errorf("snapshot read body: %w", err)
	}
	if m != int(hdrLen) {
		closeFDs(planeFDs)
		closeFDs(moreFDs)
		return nil, fmt.Errorf("snapshot: short body read (%d/%d)", m, hdrLen)
	}
	planeFDs = append(planeFDs, moreFDs...)
	defer closeFDs(planeFDs)

	var env frameNotification
	if err := json.Unmarshal(body, &env); err != nil {
		return nil, fmt.Errorf("snapshot: decode envelope: %w", err)
	}
	if env.JSONRPC != "2.0" {
		return nil, fmt.Errorf("snapshot: bad jsonrpc version %q", env.JSONRPC)
	}
	if env.Method != "frame" {
		return nil, fmt.Errorf("snapshot: unexpected method %q", env.Method)
	}
	hdr := env.Params
	if len(planeFDs) == 0 {
		return nil, fmt.Errorf("snapshot: no fds received with frame")
	}
	if hdr.Width == 0 || hdr.Height == 0 {
		return nil, fmt.Errorf("snapshot: invalid frame dims %dx%d", hdr.Width, hdr.Height)
	}
	// NV12 expected; 1 fd covers Y+UV (3/2 of plane area).
	size := int(hdr.Width) * int(hdr.Height) * 3 / 2
	mm, err := syscall.Mmap(planeFDs[0], 0, size, syscall.PROT_READ, syscall.MAP_SHARED)
	if err != nil {
		return nil, fmt.Errorf("snapshot: mmap fd=%d sz=%d: %w", planeFDs[0], size, err)
	}
	defer func() { _ = syscall.Munmap(mm) }()

	return ffmpeg.EncodeNV12ToJPEG(mm, int(hdr.Width), int(hdr.Height))
}

// recvAll repeatedly Recvmsg's until `buf` is filled, collecting any
// SCM_RIGHTS fds along the way. Returns the collected fds, bytes read,
// and any error. Honors the deadline via per-call SO_RCVTIMEO.
func recvAll(fd int, buf []byte, deadline time.Time) ([]int, int, error) {
	var collected []int
	oob := make([]byte, unix.CmsgSpace(4*4)) // up to 4 fds
	read := 0
	for read < len(buf) {
		if time.Now().After(deadline) {
			closeFDs(collected)
			return nil, read, fmt.Errorf("recvAll: deadline exceeded")
		}
		n, oobn, _, _, err := syscall.Recvmsg(fd, buf[read:], oob, 0)
		if err != nil {
			closeFDs(collected)
			return nil, read, err
		}
		if oobn > 0 {
			scms, perr := syscall.ParseSocketControlMessage(oob[:oobn])
			if perr == nil {
				for _, scm := range scms {
					if fds, ferr := syscall.ParseUnixRights(&scm); ferr == nil {
						collected = append(collected, fds...)
					}
				}
			}
		}
		if n == 0 {
			closeFDs(collected)
			return nil, read, fmt.Errorf("recvAll: peer closed")
		}
		read += n
	}
	return collected, read, nil
}

func closeFDs(fds []int) {
	for _, fd := range fds {
		_ = syscall.Close(fd)
	}
}
