# AGENTS.md — composer/

Native C++ dma-buf video pipeline for RK3588. Sibling to the Go daemon at
the repo root (see `/AGENTS.md` for that).

## Build / Test / Lint

Presets live in `composer/CMakePresets.json`. Always invoke from
`composer/`.

```bash
# Dev build (Debug, compile_commands.json exported) — use this for
# stepping with lldb. ~2x runtime CPU vs RelWithDebInfo because of
# no inlining; do not install to ~/.local/bin for day-to-day work.
cmake --preset dev
cmake --build --preset dev

# Daily install build (RelWithDebInfo, no tests) — realistic CPU,
# readable stack traces. This is what should land in ~/.local/bin.
cmake --preset relwithdebinfo
cmake --build --preset relwithdebinfo
cmake --install composer/build/relwithdebinfo

# Test (always against dev preset — keeps asserts on)
ctest --preset dev --output-on-failure
ctest --preset dev -R scm_socket           # filter by name
ctest --preset dev -L ipc                  # filter by label (post-reorg)

# Sanitizers — separate build dirs, never modify dev
cmake --preset dev-asan && cmake --build --preset dev-asan
ASAN_OPTIONS=detect_leaks=1 ctest --preset dev-asan --output-on-failure

cmake --preset dev-tsan && cmake --build --preset dev-tsan    # manual only
ctest --preset dev-tsan --output-on-failure

# Fuzzing — libFuzzer harnesses under fuzz/ (clang only, manual lane)
cmake --preset fuzz && cmake --build --preset fuzz
ctest --preset fuzz -L fuzz --output-on-failure              # bounded seed replay
./build/fuzz/fuzz/fuzz_dmabuf_header_decode \
    fuzz/corpus/dmabuf_header_decode -max_total_time=60      # real campaign

# Lint
cmake --build build/dev --target lint       # clang-format dry-run
cmake --build build/dev --target format     # clang-format -i
cmake --build build/dev --target tidy-diff  # clang-tidy on changed lines vs origin/native
cmake --build build/dev --target tidy-all   # clang-tidy whole tree (slow)

# Strict-deps — catch under-declared library link deps. Builds every
# vn_add_library target as a shared object with -Wl,--no-undefined; a lib that
# uses a symbol without declaring the owning lib fails to link. OFF by default
# (normal builds stay static). Run when you add/move a lib or edit DEPS.
cmake -B build/strict -G Ninja -DCMAKE_BUILD_TYPE=Debug -DVN_STRICT_DEPS=ON -DBUILD_TESTS=OFF
cmake --build build/strict
```

**After any composer/ change, the full quality sweep is:**

1. `cmake --build --preset dev`
2. `ctest --preset dev --output-on-failure`
3. `cmake --build build/dev --target lint`       # clang-format must be clean
4. `cmake --build build/dev --target tidy-diff`  # clang-tidy must be clean

All four are gates — don't ship if any fails. `tidy-diff` enforces the
checks listed in `composer/.clang-tidy` (nodiscard, span at syscall
boundaries, owning-memory, function-size limits, etc.) on every line a
PR touches.

**When to run TSan:** any change to `scm_rights_*`, anything threaded in
`process/`, anything touching shared fd state.

**When to run strict-deps (`VN_STRICT_DEPS=ON`):** any change that adds or
moves a `vn_add_library`, or edits a target's `DEPS`/`PUBLIC_DEPS`/
`PRIVATE_DEPS`. The shared + `-Wl,--no-undefined` link fails on a dependency a
lib uses but doesn't declare — which the static build silently tolerates via
transitive linking (the linker names the exact symbol + lib to add). It does
NOT catch over-declared/dead deps. Host config can't see `HAVE_RGA`/`HAVE_MPP`
libs, so run it on the rig too for full coverage. Not a CI gate — a local check
you run when touching library structure.

## Environment contract

- `SCCACHE_DIR=$HOME/.cache/videonode-sccache` — shared across worktrees.
  Each developer sets this once (e.g., `.envrc`). Install with
  `sudo dnf install sccache` (Fedora) or `apt install sccache` (Debian).
- gRPC + protobuf for the daemon ↔ native control plane. Fedora:
  `sudo dnf install protobuf-compiler protobuf-devel grpc-devel grpc-plugins`.
  Debian (rig): `sudo apt install libgrpc++-dev libprotobuf-dev
  protobuf-compiler protobuf-compiler-grpc`. CI's `deb-arm64` lane
  installs the same set in its native arm64 runner; there is no
  qemu-based local Docker build (it was too slow to be useful).
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
- Annotate owning raw pointers with `gsl::owner<T*>` (from
  `src/common/owner.hpp`). Prefer `std::make_unique` or stack allocation
  when possible; use `gsl::owner` for pimpl or cases where `unique_ptr`
  doesn't work (GCC incomplete-type limitations).
- Template error walls: fix the call site. Do not refactor templates to
  silence diagnostics.
- Untrusted-bytes decoders get a libFuzzer harness via `vn_add_fuzzer`.
  `ipc/dmabuf_header::Decode` is the only one today
  (`fuzz/fuzz_dmabuf_header_decode.cpp`, round-trip oracle); the gRPC
  control plane needs none — libprotobuf parses it. Add a sibling harness
  for any new decoder.
- **Never add `NOLINT` comments.** Fix the code instead. If clang-tidy
  flags pointer arithmetic, use `std::span` and `.subspan()`. If it flags
  function size, split the function. If it flags designated initializers,
  use them. The linter is right; suppressing it hides real problems.
- **Comments: default to ZERO. This is a hard gate, not a preference.**
  Code is self-documenting — names, types, and structure carry the meaning.
  A comment is justified ONLY to explain a non-obvious WHY (a workaround, an
  external constraint, a counterintuitive choice), and then it is exactly ONE
  short line. NEVER restate what the code does, narrate a block, label a
  section, or write multi-line / banner / ASCII comments. Treat every comment
  beyond one line like a `NOLINT`: justify it before writing, on every line,
  every time — recalling the rule once at session start is not enough, because
  comment and code emit together and the check decays. If you can't justify
  it, delete it. An instruction that says "comment the WHY" licenses ONE line,
  never a paragraph. Strip redundant comments from any code you touch.

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
- **When a file approaches the cap:** split rather than thicken.

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
