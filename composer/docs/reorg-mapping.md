# `composer/src/` domain organization (historical playbook)

**Status: executed in commit `2b5a8dd` (Wave 4).** Current layout matches
the destinations below; the include-rewrite plan ran via the script in the
"Include rewrites" section. This document is preserved as the rationale
trail — the table below doubles as the canonical domain reference if you
need to know *why* a particular file lives where it does.

For a short present-tense summary of what's in each subdir, see
`composer/AGENTS.md` → "Architecture" → "Library layout under `src/`",
or the `README.md` inside each subdir.

---

## Original Phase E plan (now executed)

Concrete file-by-file mapping for the `composer/src/` flat → domain
reorg. Authored from worktree `wave2-phase-b` at HEAD `8339ef6`, after Phase A
and Phase B landed. File inventory is exactly what's on disk now (Phase 1 + Phase A).

The reorg moves every `composer/src/*.cpp` / `*.hpp` into one of six
subdirectories: `ipc/`, `rpc/`, `capture/`, `render/`, `process/`, `bin/`. The
existing `composer/src/CMakeLists.txt` (one `vn_add_library` per lib) splits
one-to-one across the new subdirs; each subdir gets a tiny `CMakeLists.txt`
plus a `README.md`.

## File moves

### `composer/src/ipc/`

ctest LABEL: `ipc`

| from | to |
|------|----|
| `src/dma_heap.cpp` | `src/ipc/dma_heap.cpp` |
| `src/dma_heap.hpp` | `src/ipc/dma_heap.hpp` |
| `src/scm_socket.cpp` | `src/ipc/scm_socket.cpp` |
| `src/scm_socket.hpp` | `src/ipc/scm_socket.hpp` |
| `src/scm_rights_source.cpp` | `src/ipc/scm_rights_source.cpp` |
| `src/scm_rights_source.hpp` | `src/ipc/scm_rights_source.hpp` |
| `src/scm_rights_producer.cpp` | `src/ipc/scm_rights_producer.cpp` |
| `src/scm_rights_producer.hpp` | `src/ipc/scm_rights_producer.hpp` |

Draft `src/ipc/CMakeLists.txt` (lifted verbatim from `src/CMakeLists.txt`
lines 11, 21–31, with sources now relative to this subdir):

```cmake
vn_add_library(dma_heap      SOURCES dma_heap.cpp)

vn_add_library(scm_socket
    SOURCES scm_socket.cpp
    PUBLIC_DEPS dmabuf_msg)

vn_add_library(scm_rights_source
    SOURCES scm_rights_source.cpp
    PUBLIC_DEPS scm_socket dmabuf_msg Threads::Threads)

vn_add_library(scm_rights_producer
    SOURCES scm_rights_producer.cpp
    PUBLIC_DEPS scm_socket dmabuf_msg Threads::Threads)
```

`README.md` (≤10 lines):

> Unix-domain fd passing. dma-buf allocator + SCM_RIGHTS socket transport
> for fanning frame fds out from a producer to ≤16 consumers.
>
> ctest label: `ipc`
>
> Invariant: dma-buf fds are passed by SCM_RIGHTS; never `sendmsg` from
> outside `ipc/`. Always `dup()` before storing and set `FD_CLOEXEC` on
> every export.

### `composer/src/rpc/`

ctest LABEL: `rpc`

| from | to |
|------|----|
| `src/jsonrpc_msg.cpp` | `src/rpc/jsonrpc_msg.cpp` (split in Phase F — see large-file-split-plan.md) |
| `src/jsonrpc_msg.hpp` | `src/rpc/jsonrpc_msg.hpp` |
| `src/dmabuf_msg.cpp` | `src/rpc/dmabuf_msg.cpp` |
| `src/dmabuf_msg.hpp` | `src/rpc/dmabuf_msg.hpp` |
| `src/control_channel.cpp` | `src/rpc/control_channel.cpp` |
| `src/control_channel.hpp` | `src/rpc/control_channel.hpp` |

Draft `src/rpc/CMakeLists.txt`:

```cmake
vn_add_library(jsonrpc_msg   SOURCES jsonrpc_msg.cpp)

vn_add_library(dmabuf_msg
    SOURCES dmabuf_msg.cpp
    PUBLIC_DEPS jsonrpc_msg)

vn_add_library(control_channel
    SOURCES control_channel.cpp
    PUBLIC_DEPS jsonrpc_msg)
```

Note: Phase F will rewrite the `jsonrpc_msg` library's `SOURCES` to
`jsonrpc_parse.cpp jsonrpc_serialize.cpp` once the file split lands. Phase E
ships the unmodified single-file form.

`README.md`:

> JSON-RPC 2.0 framing + dma-buf metadata envelopes + bidirectional control
> channel between Go daemon and C++ sidecars.
>
> ctest label: `rpc`
>
> Invariant: every decoder rejects truncated/malformed input and never
> reads past the bytes it was given (fuzz-target shape). No allocations
> outside the result struct on the error path.

### `composer/src/capture/`

ctest LABEL: `capture`

| from | to |
|------|----|
| `src/v4l2_capture.cpp` | `src/capture/v4l2_capture.cpp` (split in Phase F) |
| `src/v4l2_capture.hpp` | `src/capture/v4l2_capture.hpp` |
| `src/jpeg_dec.hpp` | `src/capture/jpeg_dec.hpp` |
| `src/jpeg_dec_turbo.cpp` | `src/capture/jpeg_dec_turbo.cpp` |
| `src/jpeg_dec_turbo.hpp` | `src/capture/jpeg_dec_turbo.hpp` |
| `src/mpp_jpeg_dec.cpp` | `src/capture/mpp_jpeg_dec.cpp` |
| `src/mpp_jpeg_dec.hpp` | `src/capture/mpp_jpeg_dec.hpp` |
| `src/source_probe.cpp` | `src/capture/source_probe.cpp` |
| `src/source_probe.hpp` | `src/capture/source_probe.hpp` |

Draft `src/capture/CMakeLists.txt`:

```cmake
vn_add_library(v4l2_capture SOURCES v4l2_capture.cpp)

vn_add_library(source_probe
    SOURCES source_probe.cpp
    PUBLIC_DEPS v4l2_capture)

vn_add_library(jpeg_dec_turbo
    SOURCES jpeg_dec_turbo.cpp
    PUBLIC_DEPS PkgConfig::TURBOJPEG)

if(HAVE_MPP)
    vn_add_library(mpp_jpeg_dec
        SOURCES mpp_jpeg_dec.cpp
        PUBLIC_DEPS mpp_iface)
endif()
```

After the Phase F v4l2 split, the `v4l2_capture` call becomes
`SOURCES v4l2_capture.cpp v4l2_format.cpp`.

`README.md`:

> V4L2 capture + MJPEG decode + source-health probe. Sits between the
> kernel driver and the CSC stage.
>
> ctest label: `capture`
>
> Invariant: V4L2 fds use `O_CLOEXEC`; every mmap region is wrapped in
> `std::span<uint8_t>` before crossing back out of capture/. Format
> negotiation is fallible — never assume a `set_format` succeeded without
> a follow-up `get_format`.

### `composer/src/render/`

ctest LABEL: `render`

| from | to |
|------|----|
| `src/csc.cpp` | `src/render/csc.cpp` |
| `src/csc.hpp` | `src/render/csc.hpp` |
| `src/csc_gles.cpp` | `src/render/csc_gles.cpp` |
| `src/csc_gles.hpp` | `src/render/csc_gles.hpp` |
| `src/rga_csc.cpp` | `src/render/rga_csc.cpp` |
| `src/rga_csc.hpp` | `src/render/rga_csc.hpp` |
| `src/egl_ctx.cpp` | `src/render/egl_ctx.cpp` |
| `src/egl_ctx.hpp` | `src/render/egl_ctx.hpp` |
| `src/format_dispatch.cpp` | `src/render/format_dispatch.cpp` |
| `src/format_dispatch.hpp` | `src/render/format_dispatch.hpp` |
| `src/gbm_alloc.cpp` | `src/render/gbm_alloc.cpp` |
| `src/gbm_alloc.hpp` | `src/render/gbm_alloc.hpp` |
| `src/gl_compose.cpp` | `src/render/gl_compose.cpp` |
| `src/gl_compose.hpp` | `src/render/gl_compose.hpp` |
| `src/nv12_buf.cpp` | `src/render/nv12_buf.cpp` |
| `src/nv12_buf.hpp` | `src/render/nv12_buf.hpp` |
| `src/placeholder_painter.cpp` | `src/render/placeholder_painter.cpp` |
| `src/placeholder_painter.hpp` | `src/render/placeholder_painter.hpp` |
| `src/font8x8.h` | `src/render/font8x8.h` |
| `src/fake_source.cpp` | `src/render/fake_source.cpp` |
| `src/fake_source.hpp` | `src/render/fake_source.hpp` |

Note: `nv12_buf` straddles `ipc/` (it owns dma-buf fds) and `render/` (the
GBM allocator path lives in render). The plan keeps it in `render/` because
its public interface is the buffer struct consumed by the CSC + composer,
not the dma_heap details — `dma_heap` is the lower IPC primitive.

Draft `src/render/CMakeLists.txt` (preserves the existing `HAVE_RGA` /
`HAVE_GLES_CSC` / `HAVE_GBM` conditionals from `src/CMakeLists.txt:33-116`):

```cmake
vn_add_library(fake_source
    SOURCES fake_source.cpp
    PUBLIC_DEPS dma_heap)

vn_add_library(placeholder_painter SOURCES placeholder_painter.cpp)

vn_add_library(nv12_buf
    SOURCES nv12_buf.cpp
    PUBLIC_DEPS dma_heap)
if(HAVE_RGA)
    target_compile_definitions(nv12_buf PRIVATE HAVE_RGA=1)
endif()
if(HAVE_GBM AND NOT HAVE_RGA)
    target_compile_definitions(nv12_buf PRIVATE HAVE_GBM=1)
endif()

vn_add_library(csc SOURCES csc.cpp)
if(HAVE_RGA)
    vn_add_library(rga_csc
        SOURCES rga_csc.cpp
        PUBLIC_DEPS rga_iface)
    target_link_libraries(csc PUBLIC rga_csc)
    target_compile_definitions(csc PRIVATE HAVE_RGA=1)
endif()
if(HAVE_GLES_CSC)
    vn_add_library(csc_gles
        SOURCES csc_gles.cpp
        PUBLIC_DEPS egl_ctx gles_bundle)
    target_link_libraries(csc PUBLIC csc_gles)
    target_compile_definitions(csc PRIVATE HAVE_GLES_CSC=1)
endif()

if(HAVE_GBM)
    vn_add_library(gbm_alloc
        SOURCES gbm_alloc.cpp
        PUBLIC_DEPS gles_bundle)
    if(NOT HAVE_RGA)
        target_link_libraries(nv12_buf PUBLIC gbm_alloc)
    endif()

    vn_add_library(egl_ctx
        SOURCES egl_ctx.cpp
        PUBLIC_DEPS gles_bundle)

    vn_add_library(format_dispatch
        SOURCES format_dispatch.cpp
        PUBLIC_DEPS egl_ctx)

    vn_add_library(gl_compose
        SOURCES gl_compose.cpp
        PUBLIC_DEPS egl_ctx gles_bundle)
endif()
```

`README.md`:

> GPU compose + colorspace conversion + NV12 allocator. CSC dispatcher
> picks RGA on rig (HAVE_RGA) or GLES2 two-pass MRT on host (HAVE_GLES_CSC).
>
> ctest label: `render`
>
> Invariant: NV12 output buffers are always 64-byte-aligned (RGA
> requirement) and use the single-bo dma_heap path on rig vs two-bo GBM
> split on radeonsi. Never `gbm_bo_map` for a long-lived mapping —
> radeonsi treats it as single-shot snapshot.

### `composer/src/process/`

ctest LABEL: `process`

| from | to |
|------|----|
| `src/child_process.cpp` | `src/process/child_process.cpp` |
| `src/child_process.hpp` | `src/process/child_process.hpp` |
| `src/ffmpeg_pipe_source.cpp` | `src/process/ffmpeg_pipe_source.cpp` |
| `src/ffmpeg_pipe_source.hpp` | `src/process/ffmpeg_pipe_source.hpp` |

Draft `src/process/CMakeLists.txt`:

```cmake
vn_add_library(child_process SOURCES child_process.cpp)

if(HAVE_GBM)
    vn_add_library(ffmpeg_pipe_source
        SOURCES ffmpeg_pipe_source.cpp
        PUBLIC_DEPS gbm_alloc dma_heap
        PRIVATE_DEPS child_process Threads::Threads)
endif()
```

`README.md`:

> Child-process spawn/lifecycle + ffmpeg pipe-source wrapper used by the
> composer when a `videonode-source` socket isn't available.
>
> ctest label: `process`
>
> Invariant: every spawned child gets `PR_SET_PDEATHSIG`; orphan pids
> from crashed parents are unacceptable. Stdout/stderr fds set `FD_CLOEXEC`
> before fork.

### `composer/src/bin/`

| from | to |
|------|----|
| `src/videonode_source_main.cpp` | `src/bin/videonode_source_main.cpp` (and split in Phase F) |
| `src/videonode_sink_main.cpp` | `src/bin/videonode_sink_main.cpp` |
| `src/main.cpp` | `src/bin/videonode_composer_main.cpp` (rename during the move; Phase F extracts canvas-loop) |

`src/version.hpp.in` stays at `src/version.hpp.in` (it's a configure_file input,
referenced from `composer/CMakeLists.txt:49-52`; not source code).

Draft `src/bin/CMakeLists.txt` (lifted from `src/CMakeLists.txt:134-174`,
post-rename of `main.cpp`):

```cmake
vn_add_executable(videonode-source
    SOURCES videonode_source_main.cpp
    DEPS    v4l2_capture csc nv12_buf placeholder_painter source_probe
            scm_rights_producer dmabuf_msg dma_heap
            jpeg_dec_turbo
            jsonrpc_msg control_channel
            Threads::Threads)
if(HAVE_RGA)
    target_compile_definitions(videonode-source PRIVATE HAVE_RGA=1)
endif()
if(HAVE_MPP)
    target_link_libraries(videonode-source PRIVATE mpp_jpeg_dec)
    target_compile_definitions(videonode-source PRIVATE HAVE_MPP=1)
endif()
if(HAVE_GBM AND NOT HAVE_RGA)
    target_compile_definitions(videonode-source PRIVATE HAVE_GBM=1)
    target_link_libraries(videonode-source PRIVATE egl_ctx gles_bundle)
endif()

vn_add_executable(videonode-sink
    SOURCES videonode_sink_main.cpp
    DEPS    scm_rights_source)

if(HAVE_GBM)
    vn_add_executable(videonode-composer
        SOURCES videonode_composer_main.cpp
        DEPS    dma_heap ffmpeg_pipe_source scm_rights_source
                format_dispatch egl_ctx gl_compose gles_bundle)
endif()
```

After the Phase F source/composer/v4l2 splits, the executable wiring grows
extra `SOURCES` entries (or new library deps); see large-file-split-plan.md.

`bin/` has no `README.md` (binaries are documented in `composer/AGENTS.md`).

### `composer/src/CMakeLists.txt` (the umbrella)

After the moves, the umbrella becomes:

```cmake
add_subdirectory(ipc)
add_subdirectory(rpc)
add_subdirectory(capture)
add_subdirectory(render)
add_subdirectory(process)
add_subdirectory(bin)
```

## Tests-directory rewrite

`composer/tests/*.cpp` stay flat (per plan §Phase E), but their `#include`
paths need rewriting too. Per-test changes are itemized in the next section.

`tests/CMakeLists.txt` needs new ctest LABELS per test so `ctest -L ipc`
works. Recommended labels:

| test | label |
|------|-------|
| `test_child_process` | `process` |
| `test_dmabuf_msg` | `rpc` |
| `test_ffmpeg_pipe_source_argv` | `process` |
| `test_format_dispatch` | `render` |
| `test_jsonrpc_msg` | `rpc` |
| `test_placeholder_painter` | `render` |
| `test_scm_rights_producer` | `ipc` |
| `test_scm_rights_source` | `ipc` |
| `test_scm_socket` | `ipc` |
| `test_source_probe` | `capture` |

Phase C will land `gtest_discover_tests`; the LABELS go on the
`set_tests_properties(... PROPERTIES LABELS <label>)` call (or via
`gtest_discover_tests(... PROPERTIES LABELS <label>)`).

## Include rewrite plan

Source of truth: `git grep -nE '^#include "[a-z_]+\.(h|hpp)"' composer/`
on `wave2-phase-b @ 8339ef6`. Test includes use the same pattern; tool
includes use `"../src/..."` and need the longer rewrite.

Grouped by source file. Each row: current → target. The mechanical rewrite
for src/ + tests/ is `git grep -l '#include "...' composer/src composer/tests
| xargs sed -i ...`; the `tools/` rewrite is a different sed.

### `src/`-internal rewrites

The rule: an include like `"foo.hpp"` becomes `"src/<destination_subdir>/foo.hpp"`
because `include_directories(${CMAKE_SOURCE_DIR})` is set at the composer
root (per `composer/CMakeLists.txt:45`), so quote-includes resolve from
the composer root after the move.

**`src/control_channel.cpp` (now `src/rpc/control_channel.cpp`)**
- `"control_channel.hpp"` → `"src/rpc/control_channel.hpp"`
- `"jsonrpc_msg.hpp"` → `"src/rpc/jsonrpc_msg.hpp"`

**`src/csc.cpp` (now `src/render/csc.cpp`)**
- `"csc.hpp"` → `"src/render/csc.hpp"`
- `"rga_csc.hpp"` → `"src/render/rga_csc.hpp"`
- `"csc_gles.hpp"` → `"src/render/csc_gles.hpp"`

**`src/csc_gles.cpp` (now `src/render/csc_gles.cpp`)**
- `"csc_gles.hpp"` → `"src/render/csc_gles.hpp"`
- `"egl_ctx.hpp"` → `"src/render/egl_ctx.hpp"`

**`src/csc_gles.hpp` (now `src/render/csc_gles.hpp`)**
- `"csc.hpp"` → `"src/render/csc.hpp"`

**`src/dma_heap.cpp` (now `src/ipc/dma_heap.cpp`)**
- `"dma_heap.hpp"` → `"src/ipc/dma_heap.hpp"`

**`src/dmabuf_msg.cpp` (now `src/rpc/dmabuf_msg.cpp`)**
- `"dmabuf_msg.hpp"` → `"src/rpc/dmabuf_msg.hpp"`
- `"jsonrpc_msg.hpp"` → `"src/rpc/jsonrpc_msg.hpp"`

**`src/egl_ctx.cpp` (now `src/render/egl_ctx.cpp`)**
- `"egl_ctx.hpp"` → `"src/render/egl_ctx.hpp"`

**`src/fake_source.cpp` (now `src/render/fake_source.cpp`)**
- `"fake_source.hpp"` → `"src/render/fake_source.hpp"`

**`src/fake_source.hpp` (now `src/render/fake_source.hpp`)**
- `"dma_heap.hpp"` → `"src/ipc/dma_heap.hpp"`

**`src/ffmpeg_pipe_source.cpp` (now `src/process/ffmpeg_pipe_source.cpp`)**
- `"ffmpeg_pipe_source.hpp"` → `"src/process/ffmpeg_pipe_source.hpp"`
- `"child_process.hpp"` → `"src/process/child_process.hpp"`
- `"dma_heap.hpp"` → `"src/ipc/dma_heap.hpp"`

**`src/ffmpeg_pipe_source.hpp` (now `src/process/ffmpeg_pipe_source.hpp`)**
- `"gbm_alloc.hpp"` → `"src/render/gbm_alloc.hpp"`

**`src/format_dispatch.cpp` (now `src/render/format_dispatch.cpp`)**
- `"format_dispatch.hpp"` → `"src/render/format_dispatch.hpp"`

**`src/format_dispatch.hpp` (now `src/render/format_dispatch.hpp`)**
- `"egl_ctx.hpp"` → `"src/render/egl_ctx.hpp"`

**`src/gbm_alloc.cpp` (now `src/render/gbm_alloc.cpp`)**
- `"gbm_alloc.hpp"` → `"src/render/gbm_alloc.hpp"`

**`src/gl_compose.cpp` (now `src/render/gl_compose.cpp`)**
- `"gl_compose.hpp"` → `"src/render/gl_compose.hpp"`

**`src/gl_compose.hpp` (now `src/render/gl_compose.hpp`)**
- `"egl_ctx.hpp"` → `"src/render/egl_ctx.hpp"`

**`src/jpeg_dec_turbo.cpp` (now `src/capture/jpeg_dec_turbo.cpp`)**
- `"jpeg_dec_turbo.hpp"` → `"src/capture/jpeg_dec_turbo.hpp"`

**`src/jpeg_dec_turbo.hpp` (now `src/capture/jpeg_dec_turbo.hpp`)**
- `"jpeg_dec.hpp"` → `"src/capture/jpeg_dec.hpp"`

**`src/jsonrpc_msg.cpp` (now `src/rpc/jsonrpc_msg.cpp`, then split in Phase F)**
- `"jsonrpc_msg.hpp"` → `"src/rpc/jsonrpc_msg.hpp"`

**`src/main.cpp` (now `src/bin/videonode_composer_main.cpp`)**
- `"ffmpeg_pipe_source.hpp"` → `"src/process/ffmpeg_pipe_source.hpp"`
- `"scm_rights_source.hpp"` → `"src/ipc/scm_rights_source.hpp"`
- `"format_dispatch.hpp"` → `"src/render/format_dispatch.hpp"`
- `"egl_ctx.hpp"` → `"src/render/egl_ctx.hpp"`
- `"gl_compose.hpp"` → `"src/render/gl_compose.hpp"`
- `"dma_heap.hpp"` → `"src/ipc/dma_heap.hpp"`
- `"version.hpp"` → `"version.hpp"` (unchanged — generated header in build/generated/, on `include_directories`)

**`src/mpp_jpeg_dec.cpp` (now `src/capture/mpp_jpeg_dec.cpp`)**
- `"mpp_jpeg_dec.hpp"` → `"src/capture/mpp_jpeg_dec.hpp"`

**`src/mpp_jpeg_dec.hpp` (now `src/capture/mpp_jpeg_dec.hpp`)**
- `"jpeg_dec.hpp"` → `"src/capture/jpeg_dec.hpp"`

**`src/nv12_buf.cpp` (now `src/render/nv12_buf.cpp`)**
- `"dma_heap.hpp"` → `"src/ipc/dma_heap.hpp"`
- `"gbm_alloc.hpp"` → `"src/render/gbm_alloc.hpp"`

**`src/placeholder_painter.cpp` (now `src/render/placeholder_painter.cpp`)**
- `"placeholder_painter.hpp"` → `"src/render/placeholder_painter.hpp"`

**`src/rga_csc.cpp` (now `src/render/rga_csc.cpp`)**
- `"rga_csc.hpp"` → `"src/render/rga_csc.hpp"`

**`src/scm_rights_producer.cpp` (now `src/ipc/scm_rights_producer.cpp`)**
- `"scm_rights_producer.hpp"` → `"src/ipc/scm_rights_producer.hpp"`
- `"scm_socket.hpp"` → `"src/ipc/scm_socket.hpp"`

**`src/scm_rights_producer.hpp` (now `src/ipc/scm_rights_producer.hpp`)**
- `"dmabuf_msg.hpp"` → `"src/rpc/dmabuf_msg.hpp"`

**`src/scm_rights_source.cpp` (now `src/ipc/scm_rights_source.cpp`)**
- `"scm_rights_source.hpp"` → `"src/ipc/scm_rights_source.hpp"`
- `"dmabuf_msg.hpp"` → `"src/rpc/dmabuf_msg.hpp"`
- `"scm_socket.hpp"` → `"src/ipc/scm_socket.hpp"`

**`src/scm_socket.cpp` (now `src/ipc/scm_socket.cpp`)**
- `"scm_socket.hpp"` → `"src/ipc/scm_socket.hpp"`

**`src/scm_socket.hpp` (now `src/ipc/scm_socket.hpp`)**
- `"dmabuf_msg.hpp"` → `"src/rpc/dmabuf_msg.hpp"`

**`src/source_probe.cpp` (now `src/capture/source_probe.cpp`)**
- `"source_probe.hpp"` → `"src/capture/source_probe.hpp"`

**`src/videonode_sink_main.cpp` (now `src/bin/videonode_sink_main.cpp`)**
- `"scm_rights_source.hpp"` → `"src/ipc/scm_rights_source.hpp"`
- `"version.hpp"` → unchanged

**`src/videonode_source_main.cpp` (now `src/bin/videonode_source_main.cpp`, then split in Phase F)**
- `"control_channel.hpp"` → `"src/rpc/control_channel.hpp"`
- `"jsonrpc_msg.hpp"` → `"src/rpc/jsonrpc_msg.hpp"`
- `"csc.hpp"` → `"src/render/csc.hpp"`
- `"placeholder_painter.hpp"` → `"src/render/placeholder_painter.hpp"`
- `"source_probe.hpp"` → `"src/capture/source_probe.hpp"`
- `"scm_rights_producer.hpp"` → `"src/ipc/scm_rights_producer.hpp"`
- `"dmabuf_msg.hpp"` → `"src/rpc/dmabuf_msg.hpp"`
- `"dma_heap.hpp"` → `"src/ipc/dma_heap.hpp"`
- `"jpeg_dec.hpp"` → `"src/capture/jpeg_dec.hpp"`
- `"jpeg_dec_turbo.hpp"` → `"src/capture/jpeg_dec_turbo.hpp"`
- `"mpp_jpeg_dec.hpp"` → `"src/capture/mpp_jpeg_dec.hpp"`
- `"version.hpp"` → unchanged
- `"egl_ctx.hpp"` → `"src/render/egl_ctx.hpp"`

`src/nv12_buf.hpp` is included only via `csc.hpp` / `videonode_source_main.cpp`
transitive paths in code I read; if a direct include exists in code I missed,
it follows the same `nv12_buf.hpp → src/render/nv12_buf.hpp` rewrite.

### `tests/` rewrites

All tests use `"../src/<file>.hpp"`. After Phase E, tests stay flat in
`composer/tests/`, so the rewrite becomes `"src/<subdir>/<file>.hpp"`
(no leading `../` — the composer-root `include_directories` covers them).

**`tests/test_child_process.cpp`**
- `"test_runner.hpp"` → deleted by Phase C migration to gtest
- `"../src/child_process.hpp"` → `"src/process/child_process.hpp"`

**`tests/test_dmabuf_msg.cpp`**
- `"test_runner.hpp"` → deleted by Phase C
- `"../src/dmabuf_msg.hpp"` → `"src/rpc/dmabuf_msg.hpp"`

**`tests/test_ffmpeg_pipe_source_argv.cpp`**
- `"test_runner.hpp"` → deleted by Phase C
- `"../src/dma_heap.hpp"` → `"src/ipc/dma_heap.hpp"`
- `"../src/ffmpeg_pipe_source.hpp"` → `"src/process/ffmpeg_pipe_source.hpp"`

**`tests/test_format_dispatch.cpp`**
- `"test_runner.hpp"` → deleted by Phase C
- `"../src/format_dispatch.hpp"` → `"src/render/format_dispatch.hpp"`
- `"../src/egl_ctx.hpp"` → `"src/render/egl_ctx.hpp"`

**`tests/test_jsonrpc_msg.cpp`**
- `"test_runner.hpp"` → deleted by Phase C
- `"../src/jsonrpc_msg.hpp"` → `"src/rpc/jsonrpc_msg.hpp"`

**`tests/test_placeholder_painter.cpp`**
- `"test_runner.hpp"` → deleted by Phase C
- `"../src/placeholder_painter.hpp"` → `"src/render/placeholder_painter.hpp"`

**`tests/test_scm_rights_producer.cpp`**
- `"test_runner.hpp"` → deleted by Phase C
- `"../src/scm_rights_producer.hpp"` → `"src/ipc/scm_rights_producer.hpp"`
- `"../src/scm_rights_source.hpp"` → `"src/ipc/scm_rights_source.hpp"`
- `"../src/scm_socket.hpp"` → `"src/ipc/scm_socket.hpp"`
- `"../src/dmabuf_msg.hpp"` → `"src/rpc/dmabuf_msg.hpp"`

**`tests/test_scm_rights_source.cpp`**
- `"test_runner.hpp"` → deleted by Phase C
- `"../src/scm_rights_source.hpp"` → `"src/ipc/scm_rights_source.hpp"`
- `"../src/scm_socket.hpp"` → `"src/ipc/scm_socket.hpp"`
- `"../src/dmabuf_msg.hpp"` → `"src/rpc/dmabuf_msg.hpp"`

**`tests/test_scm_socket.cpp`**
- `"test_runner.hpp"` → deleted by Phase C
- `"../src/scm_socket.hpp"` → `"src/ipc/scm_socket.hpp"`
- `"../src/dmabuf_msg.hpp"` → `"src/rpc/dmabuf_msg.hpp"`

**`tests/test_source_probe.cpp`**
- `"test_runner.hpp"` → deleted by Phase C
- `"../src/source_probe.hpp"` → `"src/capture/source_probe.hpp"`
- `"../src/v4l2_capture.hpp"` → `"src/capture/v4l2_capture.hpp"`

### `tools/` rewrites

`tools/*.cpp` stay in `composer/tools/`. Their `"../src/foo.hpp"` includes
become `"src/<subdir>/foo.hpp"`.

**`tools/compose-probe.cpp`**
- `"../src/egl_ctx.hpp"` → `"src/render/egl_ctx.hpp"`
- `"../src/fake_source.hpp"` → `"src/render/fake_source.hpp"`
- `"../src/gl_compose.hpp"` → `"src/render/gl_compose.hpp"`
- `"../src/dma_heap.hpp"` → `"src/ipc/dma_heap.hpp"`

**`tools/csc-probe.cpp`**
- `"../src/egl_ctx.hpp"` → `"src/render/egl_ctx.hpp"`

**`tools/dma-probe.cpp`**
- `"../src/dma_heap.hpp"` → `"src/ipc/dma_heap.hpp"`

**`tools/dmabuf-format-probe.cpp`**
- `"../src/egl_ctx.hpp"` → `"src/render/egl_ctx.hpp"`

**`tools/import-probe.cpp`**
- `"../src/egl_ctx.hpp"` → `"src/render/egl_ctx.hpp"`
- `"../src/fake_source.hpp"` → `"src/render/fake_source.hpp"`
- `"../src/dma_heap.hpp"` → `"src/ipc/dma_heap.hpp"`

**`tools/live-compose-probe.cpp`**
- `"../src/egl_ctx.hpp"` → `"src/render/egl_ctx.hpp"`
- `"../src/ffmpeg_pipe_source.hpp"` → `"src/process/ffmpeg_pipe_source.hpp"`
- `"../src/gl_compose.hpp"` → `"src/render/gl_compose.hpp"`

**`tools/pipe-source-probe.cpp`**
- `"../src/ffmpeg_pipe_source.hpp"` → `"src/process/ffmpeg_pipe_source.hpp"`
- `"../src/dma_heap.hpp"` → `"src/ipc/dma_heap.hpp"`

**`tools/rga-csc-probe.cpp`**
- `"../src/dma_heap.hpp"` → `"src/ipc/dma_heap.hpp"`
- `"../src/rga_csc.hpp"` → `"src/render/rga_csc.hpp"`

**`tools/source-probe.cpp`**
- `"../src/fake_source.hpp"` → `"src/render/fake_source.hpp"`
- `"../src/dma_heap.hpp"` → `"src/ipc/dma_heap.hpp"`

**`tools/egl-probe.cpp`** — no `"../src/..."` includes; nothing to rewrite.

## Sed scripts (mechanical execution)

After `git mv` of every file into its destination subdir, the per-file
include rewrites collapse into one sed expression per old → new prefix
because the basenames are unique across the project (verified: no two libs
share a header name). One pattern handles src/ + tests/ + tools/ in a single
pass:

```bash
git grep -lE '#include "(\.\./src/|src/|)' composer | xargs sed -i -E \
  -e 's|"(\.\./src/|src/|)dma_heap\.hpp"|"src/ipc/dma_heap.hpp"|g' \
  -e 's|"(\.\./src/|src/|)scm_socket\.hpp"|"src/ipc/scm_socket.hpp"|g' \
  -e 's|"(\.\./src/|src/|)scm_rights_source\.hpp"|"src/ipc/scm_rights_source.hpp"|g' \
  -e 's|"(\.\./src/|src/|)scm_rights_producer\.hpp"|"src/ipc/scm_rights_producer.hpp"|g' \
  -e 's|"(\.\./src/|src/|)jsonrpc_msg\.hpp"|"src/rpc/jsonrpc_msg.hpp"|g' \
  -e 's|"(\.\./src/|src/|)dmabuf_msg\.hpp"|"src/rpc/dmabuf_msg.hpp"|g' \
  -e 's|"(\.\./src/|src/|)control_channel\.hpp"|"src/rpc/control_channel.hpp"|g' \
  -e 's|"(\.\./src/|src/|)v4l2_capture\.hpp"|"src/capture/v4l2_capture.hpp"|g' \
  -e 's|"(\.\./src/|src/|)jpeg_dec\.hpp"|"src/capture/jpeg_dec.hpp"|g' \
  -e 's|"(\.\./src/|src/|)jpeg_dec_turbo\.hpp"|"src/capture/jpeg_dec_turbo.hpp"|g' \
  -e 's|"(\.\./src/|src/|)mpp_jpeg_dec\.hpp"|"src/capture/mpp_jpeg_dec.hpp"|g' \
  -e 's|"(\.\./src/|src/|)source_probe\.hpp"|"src/capture/source_probe.hpp"|g' \
  -e 's|"(\.\./src/|src/|)csc\.hpp"|"src/render/csc.hpp"|g' \
  -e 's|"(\.\./src/|src/|)csc_gles\.hpp"|"src/render/csc_gles.hpp"|g' \
  -e 's|"(\.\./src/|src/|)rga_csc\.hpp"|"src/render/rga_csc.hpp"|g' \
  -e 's|"(\.\./src/|src/|)egl_ctx\.hpp"|"src/render/egl_ctx.hpp"|g' \
  -e 's|"(\.\./src/|src/|)format_dispatch\.hpp"|"src/render/format_dispatch.hpp"|g' \
  -e 's|"(\.\./src/|src/|)gbm_alloc\.hpp"|"src/render/gbm_alloc.hpp"|g' \
  -e 's|"(\.\./src/|src/|)gl_compose\.hpp"|"src/render/gl_compose.hpp"|g' \
  -e 's|"(\.\./src/|src/|)nv12_buf\.hpp"|"src/render/nv12_buf.hpp"|g' \
  -e 's|"(\.\./src/|src/|)placeholder_painter\.hpp"|"src/render/placeholder_painter.hpp"|g' \
  -e 's|"(\.\./src/|src/|)fake_source\.hpp"|"src/render/fake_source.hpp"|g' \
  -e 's|"(\.\./src/|src/|)child_process\.hpp"|"src/process/child_process.hpp"|g' \
  -e 's|"(\.\./src/|src/|)ffmpeg_pipe_source\.hpp"|"src/process/ffmpeg_pipe_source.hpp"|g'
```

`font8x8.h` is included by `placeholder_painter.cpp` only as `<font8x8.h>`
or similar — check before sed; if it uses the bare `"font8x8.h"` form, add a
matching line. `version.hpp` stays unchanged (it's the generated header from
`build/generated/`).

## Validation checklist for the Phase E executor

After the moves + sed pass:

```bash
cd composer
cmake --preset dev               # configures
cmake --build --preset dev       # full build
ctest --preset dev -L ipc        # filters work
ctest --preset dev -L rpc
ctest --preset dev -L capture
ctest --preset dev -L render
ctest --preset dev -L process
ctest --preset dev               # full pass

# Sanity: no surviving flat includes pointing at the old layout.
! git grep -nE '#include "(\.\./)?(src/)?(dma_heap|scm_socket|scm_rights_|jsonrpc_msg|dmabuf_msg|control_channel|v4l2_capture|jpeg_dec|mpp_jpeg_dec|source_probe|csc|csc_gles|rga_csc|egl_ctx|format_dispatch|gbm_alloc|gl_compose|nv12_buf|placeholder_painter|fake_source|child_process|ffmpeg_pipe_source)\.hpp"' composer
```

The negated `git grep` should print nothing — every old-style include should
have been rewritten.
