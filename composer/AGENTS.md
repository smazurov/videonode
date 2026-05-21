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
cmake --build build/dev --target tidy-diff  # clang-tidy on changed lines vs origin/main
cmake --build build/dev --target tidy-all   # clang-tidy whole tree (slow)
```

**When to run TSan:** any change to `scm_rights_*`, anything threaded in
`process/`, anything touching shared fd state.

## Environment contract

- `SCCACHE_DIR=$HOME/.cache/videonode-sccache` — shared across worktrees.
  Each developer sets this once (e.g., `.envrc`). Install with
  `sudo dnf install sccache` (Fedora) or `apt install sccache` (Debian).
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
- File size: soft target 500 lines, hard cap 700 lines. CI enforces via
  `scripts/check-file-size.sh`. Per-function limits set in `.clang-tidy`
  (`readability-function-size`).

## Architecture (one paragraph)

Three binaries supervised by the Go daemon:
- **`videonode-source`** (one per V4L2 device) — captures frames, converts
  to NV12 via RGA or GLES CSC backend, fans dma-buf fds out to ≤16
  consumers over a Unix socket using SCM_RIGHTS.
- **`videonode-sink`** — single consumer, mmaps each dma-buf and writes
  raw NV12 to stdout. Piped into ffmpeg for transcoding.
- **`videonode-composer`** — GPU compositor on Mali-G610 (Panthor on rig,
  radeonsi on Fedora dev box). Reads up to two source sockets, composes
  BGRA via EGL/GLES, writes to stdout.

Vendored Rockchip stubs are gone — host builds either link real librga /
librockchip_mpp or skip those code paths via `HAVE_RGA` / `HAVE_MPP`.

## Known gaps (don't re-propose)

- **Fuzzing**: `rpc/jsonrpc_msg::DecodeFrameNotification` and
  `rpc/dmabuf_msg::DecodeFrameNotification` are prime targets. Preset
  `fuzz` exists; harness not written. See `composer/README.md` follow-ups.
- **C++ modules**: deferred until clangd module-navigation support
  stabilizes. Spike candidate: `rpc/jsonrpc_msg`. See README follow-ups.
- **Hardened libstdc++**: enable `_GLIBCXX_ASSERTIONS` in `dev` preset
  only. See README follow-ups.
