package streams

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/smazurov/videonode/internal/logging"
)

// NativePipelineConfig points the daemon at the videonode-native binaries.
// Binary availability (each path resolved + stat'd at daemon start) decides
// whether single streams and canvases route to the native dma-buf pipeline
// or fall back to the legacy ffmpeg-direct path. There is no per-stream or
// per-canvas opt-in flag — if the binaries are present, the daemon uses
// them.
type NativePipelineConfig struct {
	// Paths to the three production binaries. Set via the
	// `[native_pipeline]` section of config.toml or the corresponding env
	// vars (NATIVE_PIPELINE_SOURCE, NATIVE_PIPELINE_SINK,
	// NATIVE_PIPELINE_COMPOSER). Empty path == component unavailable.
	V4L2Source string
	VNSink     string
	Composer   string

	// Resolved availability — populated by Resolve() at startup. The Bool
	// is true iff the path is non-empty AND points at an executable file.
	Available NativeAvailability
}

// NativeAvailability captures the result of a single startup stat() check
// for each binary. Stored on the config so the processors can consult it
// without re-statting per request.
type NativeAvailability struct {
	V4L2Source bool
	VNSink     bool
	Composer   bool
}

// Resolve checks each configured path on disk and updates the Available
// struct. Safe to call once at daemon startup; results don't change at
// runtime. Tilde-prefixed paths (~/.local/bin/X) are expanded against $HOME.
// Returns the config receiver for chaining.
func (n *NativePipelineConfig) Resolve(logger logging.Logger) *NativePipelineConfig {
	n.V4L2Source = expandHome(n.V4L2Source)
	n.VNSink = expandHome(n.VNSink)
	n.Composer = expandHome(n.Composer)
	n.Available.V4L2Source = isExecutable(n.V4L2Source)
	n.Available.VNSink = isExecutable(n.VNSink)
	n.Available.Composer = isExecutable(n.Composer)

	if logger != nil {
		logger.Info("Native pipeline resolved",
			logging.KeySourceBin, n.V4L2Source, logging.KeySourceOK, n.Available.V4L2Source,
			logging.KeySinkBin, n.VNSink, logging.KeySinkOK, n.Available.VNSink,
			logging.KeyComposerBin, n.Composer, logging.KeyComposerOK, n.Available.Composer)
	}
	return n
}

// SingleStreamReady reports whether the videonode-source + vn-sink pair is
// usable for a single V4L2 stream.
func (n *NativePipelineConfig) SingleStreamReady() bool {
	return n != nil && n.Available.V4L2Source && n.Available.VNSink
}

// CanvasReady reports whether the videonode-source + videonode-composer pair
// is usable for a GPU canvas.
func (n *NativePipelineConfig) CanvasReady() bool {
	return n != nil && n.Available.V4L2Source && n.Available.Composer
}

// isExecutable returns true if path is non-empty and the file exists
// with at least one execute bit set.
func isExecutable(path string) bool {
	if path == "" {
		return false
	}
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return false
	}
	return info.Mode().Perm()&0o111 != 0
}

// expandHome resolves a leading `~/` or bare `~` against $HOME. Leaves
// absolute or already-expanded paths alone. Empty stays empty.
func expandHome(path string) string {
	if path == "" || path[0] != '~' {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	if path == "~" {
		return home
	}
	if strings.HasPrefix(path, "~/") {
		return filepath.Join(home, path[2:])
	}
	return path
}
