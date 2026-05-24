package pipeline

import (
	"errors"
	"log/slog"
	"path/filepath"

	"github.com/smazurov/videonode/internal/ffmpeg"
	"github.com/smazurov/videonode/internal/process"
)

// ProducerStage is the per-Source `videonode-source` process. Keyed by
// SourceID (1:1 with Source.ID). Captures V4L2 frames (or a built-in
// test-pattern when TestMode is set) and broadcasts NV12 dma-bufs via
// SCM_RIGHTS to N consumers.
//
// Sharing happens at the SCM data-plane level: any number of composers
// or encoders dial SCMSocketPathFor(sourceID) to fan out. The Pipeline's
// source registry tracks one ProducerStage per source-id; consumers
// don't refcount the producer themselves.
//
// Exactly one of DevicePath / TestMode must be non-zero. The Pipeline
// validates this at ApplySource time.
type ProducerStage struct {
	SourceID   string // logical source identity from Source.ID
	DevicePath string // resolved /dev/videoN path; empty when TestMode is true
	TestMode   bool   // swaps argv to --test-pattern when true
	BinaryPath string // path to videonode-source binary
	// GrpcUds is the per-instance gRPC UDS the daemon dials for control
	// plane RPCs (SetFormat / Snapshot / status subscription).
	GrpcUds string
}

// ProducerPoolKey returns the pool.Pool key for the producer of a given
// source id. Stable across restarts.
func ProducerPoolKey(sourceID string) string { return SourcePoolKey(sourceID) }

// ID returns the stage's process.Pool key.
func (p *ProducerStage) ID() string { return SourcePoolKey(p.SourceID) }

// Kind reports this as a Producer stage.
func (p *ProducerStage) Kind() Kind { return KindProducer }

// StreamID returns "" — producers are source-scoped, not stream-scoped.
func (p *ProducerStage) StreamID() string { return "" }

// SCMSocketPath returns the data-plane socket consumers dial.
func (p *ProducerStage) SCMSocketPath() string { return SCMSocketPathFor(p.SourceID) }

// Command returns the videonode-source argv.
func (p *ProducerStage) Command() ([]string, []string, error) {
	if p.BinaryPath == "" {
		return nil, nil, errors.New("producer: BinaryPath is required")
	}
	if p.SourceID == "" {
		return nil, nil, errors.New("producer: SourceID is required")
	}
	if p.TestMode && p.DevicePath != "" {
		return nil, nil, errors.New("producer: TestMode and DevicePath are mutually exclusive")
	}
	if !p.TestMode && p.DevicePath == "" {
		return nil, nil, errors.New("producer: one of DevicePath or TestMode is required")
	}

	argv := []string{p.BinaryPath}
	if p.TestMode {
		argv = append(argv, "--test-pattern")
	} else {
		argv = append(argv, "--device", p.DevicePath)
	}
	argv = append(argv, "--out-socket", p.SCMSocketPath())
	if p.GrpcUds != "" {
		argv = append(argv,
			"--grpc-listen", p.GrpcUds,
			"--device-id", p.SourceID,
		)
	}
	return argv, nil, nil
}

// LogParser uses the ffmpeg parser — videonode-source emits the same
// `[level] msg` format via vn::log helpers.
func (p *ProducerStage) LogParser() process.LogParser { return ffmpeg.ParseLogLevel }

// LogAttrs tags producer logs with the source id + pool-key instance.
func (p *ProducerStage) LogAttrs() []slog.Attr {
	return []slog.Attr{
		slog.String("source_id", p.SourceID),
		slog.String("stage_instance", p.ID()),
	}
}

// Reconfigure always returns ErrRequiresRestart: format changes require
// a V4L2 device reopen, which means a full restart.
func (p *ProducerStage) Reconfigure(_ any) error { return ErrRequiresRestart }

// SCMSocketPathFor returns the data-plane socket a producer binds for
// the given source id.
func SCMSocketPathFor(sourceID string) string {
	return filepath.Join("/tmp", "vn-bus-"+sanitizeForFilename(sourceID)+".sock")
}

// NativeUdsDir holds per-instance gRPC sockets for spawned native
// binaries.
const NativeUdsDir = "/tmp/videonode-native"

// GrpcSocketPathFor builds the per-instance gRPC UDS path. Kind is
// "source" or "composer"; id is the source-id or composer-id.
func GrpcSocketPathFor(kind, id string) string {
	return filepath.Join(NativeUdsDir, kind+"-"+sanitizeForFilename(id)+".sock")
}

// sanitizeForFilename strips characters that aren't safe in /tmp paths.
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
