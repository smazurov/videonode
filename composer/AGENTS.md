# AGENTS.md — composer/

Native C++ dma-buf video pipeline for RK3588. Sibling to the Go daemon at
the repo root (see `/AGENTS.md` for that).

## Build / Test / Lint

Presets live in `composer/CMakePresets.json`. Always invoke from
`composer/`.

```bash
# Dev build (Debug, compile_commands.json exported)
cmake --preset dev
cmake --build --preset dev

# Test
ctest --preset dev --output-on-failure
ctest --preset dev -R scm_socket           # filter by name
ctest --preset dev -L ipc                  # filter by label (post-reorg)

# Sanitizers — separate build dirs, never modify dev
cmake --preset dev-asan && cmake --build --preset dev-asan
ASAN_OPTIONS=detect_leaks=1 ctest --preset dev-asan --output-on-failure

cmake --preset dev-tsan && cmake --build --preset dev-tsan    # manual only
ctest --preset dev-tsan --output-on-failure

# Lint
cmake --build build/dev --target lint       # clang-format dry-run
cmake --build build/dev --target format     # clang-format -i
cmake --build build/dev --target tidy-diff  # clang-tidy on changed lines vs origin/native
cmake --build build/dev --target tidy-all   # clang-tidy whole tree (slow)
```

**When to run TSan:** any change to `scm_rights_*`, anything threaded in
`process/`, anything touching shared fd state.

## Environment contract

- `SCCACHE_DIR=$HOME/.cache/videonode-sccache` — shared across worktrees.
  Each developer sets this once (e.g., `.envrc`). Install with
  `sudo dnf install sccache` (Fedora) or `apt install sccache` (Debian).
- gRPC + protobuf for the daemon ↔ native control plane. Fedora:
  `sudo dnf install protobuf-compiler protobuf-devel grpc-devel grpc-plugins`.
  Debian (rig): `sudo apt install libgrpc++-dev libprotobuf-dev
  protobuf-compiler protobuf-compiler-grpc`. The deb-build Docker
  image installs these automatically (see
  `composer/scripts/build-deb-arm64-docker.sh`).
- MCP setup:
  - `mcp-cpp-server`: download the prebuilt binary from
    [mpsm/mcp-cpp releases](https://github.com/mpsm/mcp-cpp/releases) to
    `~/.local/bin/` (`cargo install` is currently broken upstream — see
    rust-mcp-sdk issue with `url` crate serde feature).
  - `lldb`: needs `sudo dnf install lldb` (Fedora) or apt equivalent.
    `.mcp.json` connects via `nc localhost 59999`, which requires lldb to
    be running as a TCP listener first. Start it in a side terminal:
    `lldb -O "protocol-server start MCP listen://localhost:59999" -b`.
    Then `/mcp` reload in Claude.

## C++ invariants (most enforced by `.clang-tidy`)

- `[[nodiscard]]` on every function returning fd / status / error.
- Use `std::span<uint8_t>` at syscall boundaries (CMSG_DATA, mmap regions,
  V4L2 buffer views). Raw `(void*, size_t)` is only acceptable at the
  syscall itself; wrap on return.
- dma-buf fds: `dup()` before storing; set `FD_CLOEXEC` on every export.
- Never `new` / `delete` / `malloc` outside `gsl::owner<>` annotation.
  Prefer `std::make_unique` or stack allocation.
- Template error walls: fix the call site. Do not refactor templates to
  silence diagnostics.

## Size limits

Hard limits keep individual files reviewable and incentivize the
split-first reflex when something gets too thick.

- **Per-file:** soft target 500 lines, hard cap 700. CI fails over the
  hard cap.
- **Enforced by:** `composer/scripts/check-file-size.sh`. Globs
  `composer/src/**/*.{cpp,hpp}`, `composer/tools/*.cpp`,
  `composer/tests/*.cpp`. Exits non-zero on FAIL; WARN is advisory.
- **Per-function:** `.clang-tidy` `readability-function-size` —
  120 lines, 100 statements, 12 branches, 6 parameters, 5 nesting.
- **When a file approaches the cap:** split rather than thicken. See
  `composer/docs/large-file-split-plan.md` for the rationale behind
  the historical extractions (`src/source/`, `render/canvas_loop`,
  `capture/v4l2_format`); the JSON-RPC parse/serialize extractions
  referenced there are gone with the gRPC migration.

## Architecture

Three binaries (see `src/bin/`) supervised by the Go daemon:
- **`videonode-source`** (one per V4L2 device) — captures frames, converts
  to NV12 via RGA or GLES CSC backend, fans dma-buf fds out to ≤16
  consumers over a Unix socket using SCM_RIGHTS.
- **`videonode-sink`** — single consumer, mmaps each dma-buf and writes
  raw NV12 to stdout. Piped into ffmpeg for transcoding.
- **`videonode-composer`** — GPU compositor on Mali-G610 (Panthor on rig,
  radeonsi on Fedora dev box). Reads up to two source sockets, composes
  BGRA via EGL/GLES, writes to stdout.

Control plane: each native binary runs a gRPC server on a per-instance
Unix socket the daemon allocates before spawn (`--grpc-listen <path>
--device-id <id>` or `--composer-id <id>`). The daemon dials in, calls
`Source.Describe()` / `Composer.Describe()` to seed identity, then
issues unary RPCs (SetFormat, SetCanvas, …) and for sources opens a
server-streaming `Source.StreamStatus` for the status push. Schemas
live in `proto/control/*.proto` (repo root). Omit `--grpc-listen` to
run a binary standalone — used by the R smoke scenarios.

Data plane: `videonode-source` broadcasts NV12 dma-buf fds to consumers
(`videonode-sink`, `videonode-composer`) via SCM_RIGHTS on a separate
data socket. Wire format is a fixed-shape little-endian binary header
(see `ipc/dmabuf_header.hpp`) followed by the fds in the same recvmsg.
PipeWire / GStreamer ipcpipeline pattern.

Library layout under `src/`:
- `proto/` — protoc-generated gRPC + protobuf bindings (build tree only)
- `ipc/` — SCM_RIGHTS Unix-socket fd passing, dma_heap allocator,
  binary dma-buf header codec (`dmabuf_header`)
- `rpc/` — gRPC server lifecycle wrapper (`grpc_server`) + composer
  request struct definitions (`composer_rpc`, header-only)
- `capture/` — V4L2 ioctl wrapper (capture + format-negotiation TUs), MJPEG decoders (MPP HW + libjpeg-turbo SW), source health probe
- `render/` — CSC dispatch + GLES/RGA backends, GBM/dma_heap NV12 allocators, GPU compositor, EGL context, canvas loop, NO-SIGNAL placeholder painter, composer gRPC service impl (`composer_service`)
- `process/` — child-process supervision, ffmpeg-pipe wrapper
- `source/` — `videonode-source`'s orchestrator + capture session + broadcast helpers + source gRPC service impl (`source_service`)
- `bin/` — the three binary entry points

Vendored Rockchip stubs are gone — host builds either link real librga /
librockchip_mpp or skip those code paths via `HAVE_RGA` / `HAVE_MPP`.

## Known gaps (don't re-propose)

All three of these have a fuller writeup (rationale + sketch of the
fix) in the `## Follow-ups` section of `composer/README.md` — read
that before proposing work on any of them.

- **Fuzzing**: `ipc/dmabuf_header::Decode` is the prime target (the
  only remaining untrusted-bytes decoder; the gRPC control plane is
  parsed by libprotobuf). Preset `fuzz` exists; harness not written.
  See `composer/README.md` Follow-ups → Fuzzing.
- **C++ modules**: deferred until clangd module-navigation support
  stabilizes. Experiment candidate: `ipc/dmabuf_header`. See
  `composer/README.md` Follow-ups → C++20 modules experiment.
- **Hardened libstdc++**: enable `_GLIBCXX_ASSERTIONS` in `dev` preset
  only. See `composer/README.md` Follow-ups → Hardened libstdc++.
