package pipeline

import (
	"errors"
	"log/slog"
	"path/filepath"

	"github.com/smazurov/videonode/internal/ffmpeg"
	"github.com/smazurov/videonode/internal/process"
)

// ProducerStage is the per-device videonode-source process. One
// instance per unique device, refcounted by ProducerRegistry across
// the streams that consume it. Captures V4L2 frames, broadcasts NV12
// dma-bufs via SCM_RIGHTS to N consumers.
//
// The stage exposes two sockets: the SCM data plane (where consumers
// dial for frames) and the per-instance gRPC control plane (where the
// daemon dials for SetFormat / Snapshot / status stream subscription).
// Both paths are derived from the device id; see SocketPathFor /
// GrpcSocketPathFor.
type ProducerStage struct {
	DeviceID   string // logical device identity (USB bus-port or similar)
	DevicePath string // resolved /dev/videoN path, set by Pipeline before Start
	BinaryPath string // path to videonode-source binary
	// GrpcUds is the per-instance gRPC UDS the daemon dials. Empty =
	// disabled (the smoke / R-test path; daemon-driven control plane is
	// the only sane production mode).
	GrpcUds string
}

// ProducerPoolKey returns the pool.Pool key for the producer of a
// given device. Stable; ProducerRegistry uses the same key to look up
// liveness.
func ProducerPoolKey(deviceID string) string {
	return "producer:" + deviceID
}

// ID returns the stage's process.Pool key.
func (p *ProducerStage) ID() string { return ProducerPoolKey(p.DeviceID) }

// Kind reports this as a Producer stage.
func (p *ProducerStage) Kind() Kind { return KindProducer }

// StreamID returns "" — producers are device-scoped, not stream-scoped.
// Producer logs are attributed to the device, and the consumer streams
// are visible via ProducerRegistry.ConsumersOf.
func (p *ProducerStage) StreamID() string { return "" }

// SCMSocketPath returns the data-plane socket the producer binds.
// Consumers (composer slots, single-stream vn-sink) dial this path.
// Caller of NewProducerStage is responsible for ensuring uniqueness;
// today we derive it from the device id.
func (p *ProducerStage) SCMSocketPath() string {
	return SCMSocketPathFor(p.DeviceID)
}

// Command returns the videonode-source argv. Required: BinaryPath +
// DeviceID. DevicePath is optional — when empty the binary starts with no
// `--device` argv and broadcasts the placeholder ring until a daemon
// `SetDevice` RPC assigns a path. Control-plane flags
// (--grpc-listen --device-id) are added when GrpcUds is set.
func (p *ProducerStage) Command() ([]string, []string, error) {
	if p.BinaryPath == "" {
		return nil, nil, errors.New("producer: BinaryPath is required")
	}
	if p.DeviceID == "" {
		return nil, nil, errors.New("producer: DeviceID is required")
	}

	argv := []string{
		p.BinaryPath,
		"--out-socket", p.SCMSocketPath(),
	}
	if p.DevicePath != "" {
		argv = append(argv, "--device", p.DevicePath)
	}
	if p.GrpcUds != "" {
		argv = append(argv,
			"--grpc-listen", p.GrpcUds,
			"--device-id", p.DeviceID,
		)
	}
	return argv, nil, nil
}

// LogParser uses the ffmpeg parser — videonode-source emits the same
// `[level] msg` format via vn::log helpers.
func (p *ProducerStage) LogParser() process.LogParser {
	return ffmpeg.ParseLogLevel
}

// LogAttrs tags producer logs with the device id (the stream-scoped
// stream_id field is intentionally omitted; multiple streams may
// consume the same producer).
func (p *ProducerStage) LogAttrs() []slog.Attr {
	return []slog.Attr{
		slog.String("device", p.DeviceID),
		slog.String("stage_instance", p.ID()),
	}
}

// Reconfigure: the producer has a SetFormat RPC but resolution/FPS
// changes still require the underlying V4L2 device to be reopened
// (kernel constraint), which means restart. Returns ErrRequiresRestart;
// the Pipeline orchestrates the restart, and consumers self-heal via
// scm_rights_source's 30s retry-dial.
func (p *ProducerStage) Reconfigure(_ any) error { return ErrRequiresRestart }

// SCMSocketPathFor returns the data-plane socket a producer binds for
// the given device id. Stable; consumers dial this when constructing
// a ProducerFrameSource.
func SCMSocketPathFor(deviceID string) string {
	return filepath.Join("/tmp", "vn-bus-"+sanitizeForFilename(deviceID)+".sock")
}

// NativeUdsDir holds per-instance gRPC sockets for spawned native
// binaries. The daemon mkdir's it before spawn (Pipeline.Apply).
const NativeUdsDir = "/tmp/videonode-native"

// GrpcSocketPathFor builds the per-instance gRPC UDS path the daemon
// allocates before spawning a native binary. Kind is "source" or
// "composer"; id is the device-id or composer-id.
func GrpcSocketPathFor(kind, id string) string {
	return filepath.Join(NativeUdsDir, kind+"-"+sanitizeForFilename(id)+".sock")
}

// sanitizeForFilename strips characters that aren't safe in /tmp paths.
// Conservative: keep alnum + dash + underscore; everything else → '_'.
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
