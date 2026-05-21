# videonode-native

Native dma-buf video pipeline for RK3588. Zero-copy V4L2 → Mali-GPU compose →
ffmpeg encode → RTSP, with hardware acceleration end-to-end.

The Go daemon (videonode) supervises the binaries here as separately
managed processes:

```
        ┌──────────────────────┐
        │ videonode-source          │   one per V4L2 device
        │ (sidecar / producer) │   publishes NV12 dma-bufs over SCM_RIGHTS
        └──────────┬───────────┘   /tmp/vn-bus-<deviceID>.sock
                   │ N consumers (cap 16)
       ┌───────────┴──────────────────┬────────────────────────┐
       ▼                              ▼                        ▼
  ┌─────────┐                  ┌────────────┐           ┌──────────────┐
  │ videonode-sink │                  │ videonode- │           │   AI sink    │
  │  (NV12  │                  │  composer  │           │ (planned)    │
  │   pass) │                  │ (GPU comp) │           │              │
  └────┬────┘                  └──────┬─────┘           └──────┬───────┘
       │ NV12                         │ BGRA                   │
       ▼                              ▼                        ▼
     ffmpeg                         ffmpeg                  detector
     (transcode)                    (transcode)             (broker)
       │                              │                        │
       └──────────┬───────────────────┴────────────────────────┘
                  ▼
            rtsp://127.0.0.1:8554/<stream-id>
                  ↓ external fan-out (videonode-daemon)
            WebRTC / RTSP / SRT consumers
```

## Binaries

| Binary | Purpose |
|---|---|
| `videonode-source` | V4L2 capture + RGA color-space conversion. Publishes NV12 dma-bufs to N consumers over a Unix socket via SCM_RIGHTS. Self-supervises (`PR_SET_PDEATHSIG`), auto-detects format, paints "NO SIGNAL" placeholder when the cable's out. |
| `videonode-sink` | Single-stream NV12 carrier. Dials a `videonode-source` socket, mmaps each dma-buf and writes raw NV12 bytes to stdout. Pipe into ffmpeg for transcoding. |
| `videonode-composer` | GPU canvas compositor. Reads NV12 dma-bufs from up to two `videonode-source` sockets, composes BGRA via EGL/GLES on Mali-G610, writes the canvas frames to stdout. |

All three respond to `--help` (defaults rendered from the actual `Args`
struct, never hardcoded) and `--version`.

## Build

```bash
# host (Fedora / Ubuntu dev box):
sudo dnf install clang-tools-extra cmake ninja-build pkgconf-pkg-config \
                 mesa-libEGL-devel mesa-libGLES-devel mesa-libgbm-devel \
                 libdrm-devel
# (RGA / MPP live behind vendored stubs on host; runtime calls return
# failure cleanly. On the rig the real .so's win.)

cmake -B build -S . -G Ninja
cmake --build build
ctest --test-dir build --output-on-failure
```

## Install

Default install prefix is `$HOME/.local` (no sudo needed). Override with
`-DCMAKE_INSTALL_PREFIX=/usr` for a system-wide install.

```bash
cmake --install build              # → ~/.local/bin/videonode-source etc.
cmake --install build --prefix /usr/local
```

## Packaging

CPack generates `.deb` and `.rpm` automatically when `dpkg-deb` / `rpmbuild`
are on PATH; always generates `.tar.gz`.

```bash
cd build
cpack                              # produces videonode-native-X.Y.Z-Linux.{deb,rpm,tar.gz}
sudo dpkg -i videonode-native-*.deb        # or
sudo rpm -ivh videonode-native-*.rpm
```

Runtime dependencies are declared in the package metadata (libegl1,
libgles2, libgbm1, libdrm2, ffmpeg on Debian; mesa equivalents on RPM).
Rockchip RGA / MPP are vendor-shipped on RK3588 distros and not listed.

## Linting

```bash
cmake --build build --target lint    # clang-format dry-run (fails on diffs)
cmake --build build --target format  # rewrite in place
cmake -B build -S . -DENABLE_CLANG_TIDY=ON   # opt-in tidy on every TU
```

Both targets gracefully no-op when `clang-format` isn't installed.

## Layout

```
composer/
├── CMakeLists.txt              # top-level: project, deps, subdirs, CPack
├── cmake/
│   ├── vn.cmake                # helpers: vn_add_library / vn_add_executable
│   │                           #          vn_add_probe / vn_add_test
│   ├── Dependencies.cmake      # find_package + rockchip stub fallback
│   ├── Lint.cmake              # clang-format / clang-tidy integration
│   └── Packaging.cmake         # CPack config for .deb / .rpm / .tar.gz
├── src/                        # libraries + production binaries
│   ├── CMakeLists.txt
│   └── *.cpp / *.hpp
├── tools/                      # diagnostic probes (not installed)
│   ├── CMakeLists.txt
│   └── *-probe.cpp
├── tests/                      # ctest unit suite
│   ├── CMakeLists.txt
│   └── test_*.cpp
├── shaders/                    # GLSL for videonode-composer
├── scripts/                    # rig sync / build helpers
└── third_party/
    └── rockchip-stubs/         # host-build fallback for librga / libmpp
```

Adding a new library, binary, probe, or test is a one-liner — see the helpers
in `cmake/vn.cmake` and the examples in `src/`, `tools/`, `tests/`
CMakeLists.

## SCM_RIGHTS wire format

A producer sends a serialized `dmabuf_msg::Header` (NV12 plane offsets +
pitches + frame index) on the byte stream, plus the dma-buf fd(s) as
`SCM_RIGHTS` ancillary data. Consumer protocol lives in
`src/scm_rights_source.cpp` and is symmetric to the producer side at
`src/scm_rights_producer.cpp`. Tooling note: this is **not** the GStreamer
`unixfd` wire format; tapping with `gst-launch-1.0 unixfdsrc` would
misparse the header. Use `videonode-sink` (or build a custom consumer against
`libscm_rights_source.a`) to read frames.

## Process supervision contract

- **`videonode-source` is a long-lived producer.** It listens on its socket
  forever; consumers come and go. It never exits because no one is
  consuming. If the daemon (its parent) dies, `PR_SET_PDEATHSIG` SIGTERMs
  it so it doesn't orphan-leak.
- **`videonode-sink` and `videonode-composer` are sinks.** They dial the
  producer's socket; on disconnect the consumer-side `scm_rights_source`
  retries dial for 30 s with 100 ms backoff. If the producer is briefly
  unavailable (restart, swap, etc.) the sink survives.
- **The daemon decouples them.** A sink crash doesn't take the producer
  down. A producer crash auto-restarts under daemon supervision and sinks
  re-dial. See `internal/streams/producer_manager.go` in the daemon repo.

## TODOs

Concrete, named, and prioritized — pick one off the top and ship it.

### Pipeline features
- **2nd source slot for canvases via Lyra MJPEG sidecar** — `videonode-composer`
  already accepts `--source-b-*`; the daemon currently asserts 1 source
  per canvas (`internal/streams/canvas_processor_gpu.go`). Loosen that and
  start a second `videonode-source` for Lyra devices.
- **Vision tap** — C++ producer side that hands NV12 fds to a Go-side
  consumer for ML inference, separate from the canvas/sink chain. The
  fanout in `scm_rights_producer` already supports it; needs daemon glue.
- **Audio side-track** — alsa loopback → ffmpeg in parallel with video,
  muxed on the RTSP egress.
- **Multi-canvas engage / release lifecycle** — when canvases share a
  source, the existing `ProducerManager` refcounts correctly. Needs
  end-to-end smoke coverage for engage / release races.
- **BGRA canvas bus** — `videonode-composer`'s stdout currently feeds one
  ffmpeg. Add an SCM_RIGHTS-style fanout so N encoders can subscribe to
  the composed canvas (each with a different egress URL / codec / bitrate).

### Known issues
- **`h264_rkmpp` encoder crash** — `mpp_buffer: check buffer found NULL
  pointer from get_packet_async` periodically kills ffmpeg → sink
  respawn loop. Observed in videonode journal. Workaround: the
  producer/sink decoupling means the sidecar stays alive across restarts.
  Real fix needs a librockchip-mpp upstream report or codec switch.
- **Mali EGLImage snapshot cache stalls placeholder updates** — when the
  source is in a state where it keeps broadcasting the same set of
  dma-buf fds (e.g. the 2-buffer placeholder ring), Mali samples each
  EGLImage on first import and renders the cached snapshot forever. New
  pixel data written to the same memory doesn't show. Reproducible via
  `videonode-sink … | sha256sum` (fds keep advancing) vs the downstream RTSP
  output (frozen). Fixes: rotate the placeholder ring across more
  buffers so Mali sees fresh fds; or force EGLImage refresh per frame
  in `videonode-composer`.
- **Producer arg conflicts across consumers** — when multiple sinks
  Acquire the same device with different format / dimensions, the
  ProducerManager logs a warning and keeps the first-acquired args. A
  proper negotiation pass is future work.

## Follow-ups

Scoped engineering work that's been deferred deliberately. Each entry
names the why, the rough shape of the fix, and what would unblock it.

### Fuzzing

Parsers that would benefit from `libFuzzer` coverage:

- `rpc/jsonrpc_msg::DecodeFrameNotification` — JSON envelope decoder
  shared by the control channel and producer/consumer handshake.
- `rpc/dmabuf_msg::DecodeFrameNotification` — NV12 plane offset / pitch
  decoder applied to every dma-buf message before we trust the values.
- Future V4L2 input validation — once the capture domain starts
  consuming externally-supplied format descriptors instead of probing
  for them.

Bug classes targeted: malformed envelopes (truncated, mis-typed,
missing required fields), integer overflow when computing `pitch * h`
or `offset + size`, OOB reads on truncated input where the header
length disagrees with the body, and stack/heap reads past the end of
the receive buffer.

The `fuzz` CMake preset already exists (Phase B); no harness has been
written. Pattern for a new harness:

```cpp
// tests/fuzz/fuzz_jsonrpc_msg.cpp
#include "src/rpc/jsonrpc_msg.hpp"
extern "C" int LLVMFuzzerTestOneInput(const uint8_t* data, size_t size) {
    jsonrpc::FrameNotification out;
    (void)jsonrpc::DecodeFrameNotification({data, size}, out);
    return 0;
}
```

Gated on `-DENABLE_FUZZING=ON`. CI runs each harness for
`-max_total_time=60` per change.

### C++20 modules experiment

Convert `rpc/jsonrpc_msg` as a single-library experiment — it's a leaf with
one external dependency (`nlohmann/json`) and clean header surface.
Measure rebuild time on a typical edit cycle (touch the implementation
of one decoder, time `ninja`). Verify clangd jump-to-definition,
find-references, and the `mpsm/mcp-cpp` MCP navigation still work on
the consumer side.

If clean, expand leaf-by-leaf. Revert is a single commit (CMake target
flag + `.cppm` rename). Deferred because clangd modules support is the
soft spot for our agent workflow — broken navigation costs more than
the rebuild speedup currently buys.

### Hardened libstdc++

Enable `_GLIBCXX_ASSERTIONS` in the `dev` preset only (not Release).
Catches bounds violations on `std::vector`, `std::span`, `std::string`,
and friends at runtime with roughly 6% overhead in some workloads —
only acceptable in Debug. Add as a `-D_GLIBCXX_ASSERTIONS` entry in the
`dev` preset's `cacheVariables` (`CMAKE_CXX_FLAGS_DEBUG` append) so
sanitizer presets inherit it transparently.
