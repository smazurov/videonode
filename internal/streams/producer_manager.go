package streams

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/smazurov/videonode/internal/logging"
	"github.com/smazurov/videonode/internal/process"
)

// ProducerKeyPrefix is the pool-key namespace for device producer processes.
// Pool keys for producers look like "producer:<deviceID>". Stream/canvas IDs
// must not collide with this prefix.
const ProducerKeyPrefix = "producer:"

// ProducerSpec describes how to launch one device producer (videonode-source
// sidecar). First-wins per device: callers that Acquire with a different
// spec for the same device get a warning and the first spec stays in
// effect for the lifetime of that producer.
//
// Stdio is NOT redirected to a file — the daemon's process.SetLogParser
// captures the producer's stderr line-by-line and emits it into journald
// tagged stream_id=producer:<deviceID>.
type ProducerSpec struct {
	DeviceID   string // logical source-stream ID (canvas-ownership key)
	DevicePath string // absolute V4L2 path (post-resolution)
	BinaryPath string // path to videonode-source binary
	// SocketPath is filled in by the manager when the producer is started;
	// callers read it back via SocketPath(deviceID).
}

// ProducerHandle is what a sink dials.
type ProducerHandle struct {
	DeviceID   string
	SocketPath string
}

// ProducerManager refcounts producer processes per device. Acquire spins up
// a sidecar on first use; Release tears it down when refcount hits zero.
// Many sinks may Acquire the same device; only one sidecar exists.
//
// The manager owns the producer's pool entry — generateCommand for any pool
// id starting with ProducerKeyPrefix MUST delegate to ProducerManager.Command.
type ProducerManager struct {
	pool   process.Pool
	logger logging.Logger
	mu     sync.Mutex
	// keyed by deviceID
	entries map[string]*producerEntry
}

type producerEntry struct {
	spec       ProducerSpec
	socketPath string
	refcount   int
}

// NewProducerManager constructs a manager bound to the given pool. The pool
// must be configured with a CommandProvider that delegates producer:* keys
// back to (*ProducerManager).Command.
func NewProducerManager(pool process.Pool) *ProducerManager {
	return &ProducerManager{
		pool:    pool,
		logger:  logging.GetLogger("producer_manager"),
		entries: make(map[string]*producerEntry),
	}
}

// ProducerProcessID returns the pool key for a producer of the given device.
func ProducerProcessID(deviceID string) string {
	return ProducerKeyPrefix + deviceID
}

// SocketPathFor returns the well-known per-device socket location.
// Sinks use this to dial; the sidecar binds it on startup.
func SocketPathFor(deviceID string) string {
	return filepath.Join("/tmp", "vn-bus-"+sanitizeForFilename(deviceID)+".sock")
}

// Acquire increments the refcount for deviceID. If this is the first Acquire,
// the sidecar process is started and we wait up to socketReadyTimeout for the
// socket file to appear. On wait timeout the refcount is rolled back.
func (pm *ProducerManager) Acquire(spec ProducerSpec) (*ProducerHandle, error) {
	if spec.DeviceID == "" {
		return nil, errors.New("producer Acquire: empty DeviceID")
	}
	if spec.DevicePath == "" {
		return nil, fmt.Errorf("producer Acquire: device %s has no resolved DevicePath", spec.DeviceID)
	}

	pm.mu.Lock()
	entry, exists := pm.entries[spec.DeviceID]
	if exists {
		entry.refcount++
		ref := entry.refcount
		sock := entry.socketPath
		if !specEquivalent(entry.spec, spec) {
			pm.logger.Warn("producer Acquire: differing spec for already-active device; first-wins",
				"device_id", spec.DeviceID, "existing_path", entry.spec.DevicePath,
				"new_path", spec.DevicePath)
		}
		pm.mu.Unlock()
		pm.logger.Debug("producer Acquire: existing", "device_id", spec.DeviceID, "refcount", ref)
		return &ProducerHandle{DeviceID: spec.DeviceID, SocketPath: sock}, nil
	}

	socketPath := SocketPathFor(spec.DeviceID)
	entry = &producerEntry{
		spec:       spec,
		socketPath: socketPath,
		refcount:   1,
	}
	pm.entries[spec.DeviceID] = entry
	pm.mu.Unlock()

	// Best-effort socket cleanup from a prior run; sidecar also rm -f's on start.
	_ = os.Remove(socketPath)

	if err := pm.pool.Start(ProducerProcessID(spec.DeviceID)); err != nil {
		pm.mu.Lock()
		delete(pm.entries, spec.DeviceID)
		pm.mu.Unlock()
		return nil, fmt.Errorf("producer Acquire: pool.Start failed: %w", err)
	}

	if err := waitForSocket(socketPath, socketReadyTimeout); err != nil {
		pm.logger.Warn("producer Acquire: socket not ready, continuing anyway (sink will retry-dial)",
			"device_id", spec.DeviceID, "socket", socketPath, "error", err)
	}

	pm.logger.Info("producer started", "device_id", spec.DeviceID, "socket", socketPath)
	return &ProducerHandle{DeviceID: spec.DeviceID, SocketPath: socketPath}, nil
}

// Release decrements the refcount. At zero, the producer's pool process is
// stopped and the entry is removed. Releasing an unknown device is a no-op
// with a debug log.
func (pm *ProducerManager) Release(deviceID string) {
	pm.mu.Lock()
	entry, ok := pm.entries[deviceID]
	if !ok {
		pm.mu.Unlock()
		pm.logger.Debug("producer Release: no entry", "device_id", deviceID)
		return
	}
	entry.refcount--
	ref := entry.refcount
	if ref > 0 {
		pm.mu.Unlock()
		pm.logger.Debug("producer Release: still referenced", "device_id", deviceID, "refcount", ref)
		return
	}
	delete(pm.entries, deviceID)
	pm.mu.Unlock()

	if err := pm.pool.Stop(ProducerProcessID(deviceID)); err != nil {
		pm.logger.Warn("producer Release: pool.Stop failed", "device_id", deviceID, "error", err)
	}
	pm.logger.Info("producer stopped", "device_id", deviceID)
}

// SocketPath returns the bound socket for a currently-Acquired producer.
// Returns ("", false) if no producer is active for that device.
func (pm *ProducerManager) SocketPath(deviceID string) (string, bool) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	if e, ok := pm.entries[deviceID]; ok {
		return e.socketPath, true
	}
	return "", false
}

// Command returns the shell command for a producer:<deviceID> pool key. The
// streamProcessManager's CommandProvider must delegate here for producer:*.
func (pm *ProducerManager) Command(processID string) (string, error) {
	if len(processID) <= len(ProducerKeyPrefix) || processID[:len(ProducerKeyPrefix)] != ProducerKeyPrefix {
		return "", fmt.Errorf("producer Command: %q is not a producer key", processID)
	}
	deviceID := processID[len(ProducerKeyPrefix):]

	pm.mu.Lock()
	entry, ok := pm.entries[deviceID]
	pm.mu.Unlock()
	if !ok {
		return "", fmt.Errorf("producer Command: no entry for device %s", deviceID)
	}

	// Plain exec — no sh wrapper, no trap, no background. The pool
	// supervises this process directly so its stdout/stderr stream back
	// through process.SetLogParser into journald (tagged stream_id=
	// producer:<deviceID>). PR_SET_PDEATHSIG inside the sidecar handles
	// cleanup if the daemon vanishes.
	argv := []string{
		entry.spec.BinaryPath,
		"--device", entry.spec.DevicePath,
		"--out-socket", entry.socketPath,
	}
	return shellJoin(argv), nil
}

// IsProducerKey reports whether a pool id refers to a producer process.
func IsProducerKey(id string) bool {
	return len(id) > len(ProducerKeyPrefix) && id[:len(ProducerKeyPrefix)] == ProducerKeyPrefix
}

// socketReadyTimeout caps how long Acquire waits for the producer to bind its
// socket. The composer-spike scm_rights_source has its own 30 s dial-retry,
// so a sink will recover even if we time out here — this is just a UX nicety
// for early dial avoidance. Tests override via SetSocketReadyTimeout.
var socketReadyTimeout = 3 * time.Second

// SetSocketReadyTimeout overrides the Acquire socket-wait window. Test-only.
func SetSocketReadyTimeout(d time.Duration) { socketReadyTimeout = d }

func waitForSocket(path string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		info, err := os.Stat(path)
		if err == nil && info.Mode()&os.ModeSocket != 0 {
			return nil
		}
		time.Sleep(50 * time.Millisecond)
	}
	return fmt.Errorf("socket %s not ready after %s", path, timeout)
}

func specEquivalent(a, b ProducerSpec) bool {
	return a.DevicePath == b.DevicePath && a.BinaryPath == b.BinaryPath
}

// sanitizeForFilename strips characters that aren't safe in a /tmp filename.
// Conservative: keep alnum, dash, underscore; anything else becomes '_'.
func sanitizeForFilename(s string) string {
	out := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'a' && c <= 'z',
			c >= 'A' && c <= 'Z',
			c >= '0' && c <= '9',
			c == '-' || c == '_':
			out = append(out, c)
		default:
			out = append(out, '_')
		}
	}
	return string(out)
}
