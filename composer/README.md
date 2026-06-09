# videonode-native

Native dma-buf video pipeline for RK3588. Zero-copy V4L2 → Mali-GPU compose →
ffmpeg encode → RTSP, with hardware acceleration end-to-end.

The Go daemon (videonode) supervises the binaries here as separately
managed processes:

```
        ┌───────────────────────┐
        │   videonode-source    │   one per V4L2 device
        │  (sidecar / producer) │   publishes NV12 dma-bufs over SCM_RIGHTS
        └───────────┬───────────┘   /tmp/vn-bus-<deviceID>.sock
                    │ N consumers (cap 16)
             ┌──────┴──────────────────┐
             ▼                          ▼
  ┌──────────────────────┐  ┌──────────────────────┐
  │     videonode-sink   │  │   videonode-composer │
  │      (NV12 pass)     │  │      (GPU compose)   │
  └──────────┬───────────┘  └──────────┬───────────┘
             │ NV12                     │ BGRA
             ▼                          ▼
          ffmpeg                     ffmpeg
        (transcode)                (transcode)
             │                          │
             └────────────┬─────────────┘
                          ▼
            rtsp://localhost:8554/<stream-id>
                          ↓ external fan-out (videonode-daemon)
            WebRTC / RTSP / SRT consumers
```

## Binaries

| Binary | Purpose |
|---|---|
| `videonode-source` | V4L2 capture + RGA color-space conversion. Publishes NV12 dma-bufs to N consumers over a Unix socket via SCM_RIGHTS. Self-supervises (`PR_SET_PDEATHSIG`), auto-detects format, paints "NO SIGNAL" placeholder when the cable's out. |
| `videonode-sink` | Single-stream NV12 carrier. Dials a `videonode-source` socket, mmaps each dma-buf and writes raw NV12 bytes to stdout. Pipe into ffmpeg for transcoding. |
| `videonode-composer` | GPU canvas compositor. Reads NV12 dma-bufs from N `videonode-source` sockets (daemon requires ≥1), composites a BGRA canvas via libplacebo (Vulkan primary, OpenGL fallback). Default mode writes raw BGRA to stdout; with `--scm_out` it converts the canvas back to NV12 and broadcasts the dma-bufs to consumers over SCM_RIGHTS. |

All three respond to `--help` (defaults rendered from the actual `Args`
struct, never hardcoded) and `--version`.

## Build

```bash
# host (Fedora / Ubuntu dev box):
sudo dnf install clang-tools-extra cmake ninja-build pkgconf-pkg-config \
                 mesa-libEGL-devel mesa-libGLES-devel mesa-libgbm-devel \
                 libdrm-devel
# (RGA / MPP are optional: on a host build without them, those code paths
# are compiled out via HAVE_RGA / HAVE_MPP. On the rig the real .so's link in.)

# Run all commands from composer/. Presets live in CMakePresets.json.
cmake --preset dev
cmake --build --preset dev
ctest --preset dev --output-on-failure
```

## Install

The `relwithdebinfo` preset is the daily install build: optimized, with
readable stack traces. It installs to `$HOME/.local` by default (no sudo).

```bash
cmake --preset relwithdebinfo
cmake --build --preset relwithdebinfo
cmake --install build/relwithdebinfo   # → ~/.local/bin/videonode-source etc.
```

For a system-wide install, override the prefix at configure time:
`cmake --preset relwithdebinfo -DCMAKE_INSTALL_PREFIX=/usr`.

## Packaging

The release `.deb` (arm64 only) is built by the GitHub Actions release
workflow: `cmake --install` lays the binaries into a staging dir inside an
arm64 Debian trixie environment (`composer/scripts/build-deb-arm64.sh`,
`MODE=release-nfpm`), then nfpm (`nfpm.yaml` at the repo root) assembles the
package.

Runtime dependencies are derived from the actual arm64 binaries via
`dpkg-shlibdeps` (see `composer/scripts/gen-deb-depends.sh`). Rockchip
RGA / MPP are vendor-shipped on RK3588 distros and not listed.

## Linting

```bash
cmake --build build/dev --target lint       # clang-format dry-run (fails on diffs)
cmake --build build/dev --target format     # rewrite in place
cmake --build build/dev --target tidy-diff  # clang-tidy on lines changed vs origin
cmake --build build/dev --target tidy-all   # clang-tidy on the whole tree (slow)
```

The lint and format targets gracefully no-op when `clang-format` isn't installed.

## Layout

```
composer/
├── CMakeLists.txt              # top-level: project, deps, subdirs
├── CMakePresets.json           # dev / relwithdebinfo / asan / tsan / fuzz presets
├── cmake/
│   ├── vn.cmake                # helpers: vn_add_library / vn_add_executable
│   ├── Dependencies.cmake      # find_package (RGA/MPP optional: HAVE_RGA/HAVE_MPP)
│   ├── GenerateVersion.cmake   # version stamping
│   └── Lint.cmake              # clang-format / clang-tidy integration
├── src/                        # libraries + the three binaries
│   ├── ipc/                    # SCM_RIGHTS fd passing, dma-buf header codec
│   ├── rpc/                    # gRPC server lifecycle + composer request structs
│   ├── capture/                # V4L2 wrapper, MJPEG decoders, signal probe
│   ├── render/                 # CSC + GPU compositor, EGL/GBM, canvas loop
│   ├── process/                # child-process supervision, ffmpeg pipe
│   ├── source/                 # videonode-source orchestrator + services
│   ├── snapshot/               # NV12 frame snapshot (Snapshot RPC)
│   ├── proto/                  # generated gRPC/protobuf (build tree only)
│   ├── common/                 # shared helpers
│   └── bin/                    # the three binary entry points
├── tools/                      # diagnostic probes (not installed)
├── tests/                      # ctest unit suite
├── shaders/                    # GLSL for videonode-composer
├── scripts/                    # rig sync / build helpers
└── docs/                       # design and migration notes
```

Adding a new library, binary, probe, or test is a one-liner — see the helpers
in `cmake/vn.cmake` and the examples in `src/`, `tools/`, `tests/`
CMakeLists.

## SCM_RIGHTS wire format

See [Zero-copy frame passing with SCM_RIGHTS](../website/docs/development/architecture.md)
for the producer/consumer protocol and why it is not the GStreamer `unixfd`
format. Implementation: the header codec lives in `src/ipc/dmabuf_header.cpp`;
the consumer protocol lives in `src/ipc/scm_rights_source.cpp` and is symmetric
to the producer side at `src/ipc/scm_rights_producer.cpp`. To tap frames, use
`videonode-sink` or build a custom consumer against `libscm_rights_source.a` —
tapping with `gst-launch-1.0 unixfdsrc` would misparse the header.

## Process supervision contract

See [Process supervision contract](../website/docs/development/architecture.md)
for the producer/sink lifecycle (`PR_SET_PDEATHSIG`, the 100 ms / 30 s
consumer re-dial, and the daemon decoupling that keeps a sink crash from
taking the producer down). The daemon side lives in `internal/process`
(the supervised process pool).
