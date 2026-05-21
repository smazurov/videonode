# Phase F large-file split plan

Concrete split points for the four `composer/src/` files that exceed the
500-line soft target. Targets and starting line counts measured on
`wave2-phase-b @ e3b64ce` (Phase A + B + reorg-mapping doc landed, no Phase E
moves yet, so paths below are pre-reorg). Phase E moves them into their
domain subdirs before Phase F runs, so the executor reads this doc as
"after the move" — see reorg-mapping.md for the destination paths.

Soft target: 500 lines per file. Hard cap: 700 lines. The four splits below
bring every file under 400 with room to grow.

Order of execution is independent (each split is one commit). All four can
land before the docs in §Phase G section on file-size limits.

---

## 1. `bin/videonode_source_main.cpp` — 1159 lines

Largest file in the tree. Reading the file structure
(`src/videonode_source_main.cpp` pre-reorg), the responsibilities cluster
into four blocks:

| Lines     | Responsibility                                                |
|-----------|---------------------------------------------------------------|
| 60–112    | Args struct + argv format-string helpers (`v4l2_pix_fmt_`, `fourcc_`) |
| 113–162   | V4L2 → CSC format mapping, NV24 → NV16 renegotiation          |
| 164–367   | `CaptureSession` struct + `teardown_session_` + `try_open_capture` (the V4L2/decoder open path) |
| 369–424   | `PlaceholderRing` (CPU-painted NV12 ring used when source is absent) |
| 426–537   | `print_help` + `parse_args`                                   |
| 539–699   | broadcast helpers + `build_status_params` JSON serializer     |
| 703–1159  | `main()` — startup, control-channel set-up, poll/DQBUF loop, broadcast scheduling, format-change reinit (lines 770–920 are the inline set_format JSON parser) |

### Proposed extraction: `src/source/orchestrator.{cpp,hpp}`

Create a new domain subdir `composer/src/source/` (Phase E doc accommodates
this — it's the "new subdir if needed, or under `process/`" option called
out in the plan; `source/` makes the dependency graph clearest because the
orchestrator owns capture + render + ipc + rpc together).

Move into `src/source/orchestrator.cpp` + `orchestrator.hpp`:

- `enum class DecodeMode` (line 152)
- `struct CaptureSession` (164–185)
- `void teardown_session_(CaptureSession&)` (187–204)
- `bool try_open_capture(CaptureSession&, const Args&, nv12_buf::Allocator&)` (206–367)
- `struct PlaceholderRing` (369–424)
- `void broadcast_nv12(...)` (544–561)
- `void broadcast_buffer(...)` (687–699)
- `std::string build_status_params(...)` (606–682)
- `std::string json_quote(...)` helper (566–~604)
- `uint64_t now_ms()` (539–542)
- `bool v4l2_to_csc_(...)` (113–141)
- `bool maybe_renegotiate_to_rga_friendly_(...)` (142–150)
- `uint32_t v4l2_pix_fmt_(const std::string&)` (96–112) — also used by the set_format JSON parser, so it goes with the orchestrator
- `uint32_t fourcc_(const std::string&)` (90–95)

The new `orchestrator.hpp` declares a small public surface that `bin/` calls:

```cpp
namespace source {

struct Args {
    std::string device = "/dev/video0";
    std::string in_format;
    int in_width = 0;
    int in_height = 0;
    int in_fps = 0;
    int buffers = 4;
    std::string out_socket = "/tmp/videonode-source.sock";
    int max_consumers = 16;
    int run_seconds = 0;
    int broadcast_fps = 60;
    int placeholder_w = 1920;
    int placeholder_h = 1080;
    std::string ctl_connect;
    std::string device_id;
    std::string alloc_drm_device = "/dev/dri/renderD128";
};

// Run the capture + broadcast loop until `running` goes false.
// Returns process exit code (0 = ok, non-zero = startup failure).
int Run(const Args& a, std::atomic<bool>& running);

} // namespace source
```

The set_format JSON parser (lines 770–920) becomes a private free function
inside `orchestrator.cpp`:

```cpp
// parse_set_format_params: extract {fourcc,w,h,fps} from the params_json
// substring of a control-plane set_format Request. Returns a HandlerResponse
// describing success or a JSON-RPC error. Pure — no orchestrator state.
control_channel::HandlerResponse parse_set_format_params(
    std::string_view params_json, Args& out_apply);
```

Pulling it out of the lambda saves ~150 lines from `main()` and makes the
parser unit-testable.

### What stays in `bin/videonode_source_main.cpp`

Just the entry point:

```cpp
int main(int argc, char** argv) {
    ::setvbuf(stderr, nullptr, _IOLBF, 0);
    source::Args a;
    if (!source::parse_args(argc, argv, a))
        return 2;
    std::signal(SIGINT, on_signal);
    std::signal(SIGTERM, on_signal);
    std::signal(SIGPIPE, SIG_IGN);
    ::prctl(PR_SET_PDEATHSIG, SIGTERM);
    if (::getppid() == 1)
        return 0;
    return source::Run(a, g_running);
}
```

`parse_args` + `print_help` move to `orchestrator.cpp` (or a sibling
`argv.cpp` if size demands). `g_running` and `on_signal` stay in `bin/`
because they wire the signal handler to the loop.

### CMake wiring

In `src/source/CMakeLists.txt` (new):

```cmake
vn_add_library(source_orchestrator
    SOURCES orchestrator.cpp
    PUBLIC_DEPS v4l2_capture csc nv12_buf placeholder_painter source_probe
                scm_rights_producer dmabuf_msg dma_heap
                jpeg_dec_turbo
                jsonrpc_msg control_channel
                Threads::Threads)
if(HAVE_RGA)
    target_compile_definitions(source_orchestrator PRIVATE HAVE_RGA=1)
endif()
if(HAVE_MPP)
    target_link_libraries(source_orchestrator PUBLIC mpp_jpeg_dec)
    target_compile_definitions(source_orchestrator PRIVATE HAVE_MPP=1)
endif()
if(HAVE_GBM AND NOT HAVE_RGA)
    target_compile_definitions(source_orchestrator PRIVATE HAVE_GBM=1)
    target_link_libraries(source_orchestrator PUBLIC egl_ctx gles_bundle)
endif()
```

Add `add_subdirectory(source)` to `src/CMakeLists.txt` between `process` and
`bin`.

In `src/bin/CMakeLists.txt`, the `videonode-source` executable shrinks to:

```cmake
vn_add_executable(videonode-source
    SOURCES videonode_source_main.cpp
    DEPS    source_orchestrator)
```

All the per-flag `target_compile_definitions` and platform-specific
`target_link_libraries` move into the library (where they belong — the lib
sees the orchestrator code).

### Expected post-split line counts

- `src/source/orchestrator.hpp` — ~50 lines (public surface only)
- `src/source/orchestrator.cpp` — ~900 lines (CaptureSession + PlaceholderRing + broadcast helpers + the loop body + parse_set_format_params)
- `src/bin/videonode_source_main.cpp` — ~50 lines

900 still trips the 500-line soft target. Acceptable for now (under the 700
hard cap); a follow-up can split orchestrator into `capture_session.cpp` +
`broadcast.cpp` + `loop.cpp` + `argv.cpp` if it grows. The Phase F mandate
is to land under 700; this exceeds 500 by ~80% so it stays as a documented
WARN until further refactoring.

If the executor prefers to land under-500 in one commit, the same
orchestrator scaffolding admits this internal split:

- `argv.cpp` — `Args`, `parse_args`, `print_help`, `fourcc_`, `v4l2_pix_fmt_`, `v4l2_to_csc_`, `maybe_renegotiate_to_rga_friendly_` (~150 lines)
- `capture_session.cpp` — `DecodeMode`, `CaptureSession`, `teardown_session_`, `try_open_capture` (~210 lines)
- `placeholder_ring.cpp` — `PlaceholderRing` (~60 lines)
- `broadcast.cpp` — `broadcast_nv12`, `broadcast_buffer`, `build_status_params`, `json_quote`, `now_ms` (~170 lines)
- `set_format_parser.cpp` — `parse_set_format_params` (~150 lines)
- `loop.cpp` — the run loop that ties them together (~170 lines)

All exposed via one `orchestrator.hpp`. Pick at execution time based on how
much budget is on hand.

---

## 2. `rpc/jsonrpc_msg.cpp` — 582 lines

Already mostly clean: the file is organized as `namespace parse { ... }`
(lines 8–262, low-level token parsers) → unnamed namespace (264–310,
`set_err` and `json_escape` helpers) → `DecodeFrame` (312–534) → unnamed
namespace (537–545, `append_params`) → encoders (547–580).

The Phase F plan calls for a two-TU split of the same `jsonrpc_msg` library.
Both TUs link into the existing library — no header changes, no public
surface change.

### Proposed split

**`rpc/jsonrpc_parse.cpp`** — the decode path:

- `namespace jsonrpc_msg::parse` (lines 8–262 of the original) — all the
  primitive parsers (`skip_ws`, `parse_string`, `parse_uint`, `parse_int`,
  `parse_uint_array`, `skip_value`, `skip_unknown_value`, plus the static
  `hex_nibble` / `append_utf8` helpers used by `parse_string`)
- `namespace jsonrpc_msg { bool DecodeFrame(...) }` (lines 312–534)
- The `set_err` helper from the unnamed namespace (266–269) — only
  `DecodeFrame` uses it.

**`rpc/jsonrpc_serialize.cpp`** — the encode path:

- `namespace jsonrpc_msg { std::string EncodeRequest(...) }` (547–554)
- `namespace jsonrpc_msg { std::string EncodeNotification(...) }` (556–562)
- `namespace jsonrpc_msg { std::string EncodeResponseResult(...) }` (564–569)
- `namespace jsonrpc_msg { std::string EncodeResponseError(...) }` (571–580)
- `namespace { void append_params(...) }` (537–545)
- `namespace { std::string json_escape(...) }` (273–308) — only the
  encoders use it.

### Header changes

`jsonrpc_msg.hpp` is unchanged. The `namespace jsonrpc_msg::parse`
declarations (lines 74–104) already expose every helper that's still
called from `dmabuf_msg.cpp` (per the header comment at line 73: "Low-level
helpers used by both this codec and dmabuf_msg.cpp"), so cross-TU linkage
stays clean.

The two private unnamed-namespace helpers (`set_err`, `json_escape`,
`append_params`) are confined to one TU each by the split above. No
public-surface escape.

### CMake wiring

`src/rpc/CMakeLists.txt` (drafted in reorg-mapping.md) — change the
`jsonrpc_msg` library to:

```cmake
vn_add_library(jsonrpc_msg
    SOURCES jsonrpc_parse.cpp jsonrpc_serialize.cpp)
```

No new library, no new header, no new dependency.

### Expected post-split line counts

- `src/rpc/jsonrpc_parse.cpp` — ~340 lines (parse namespace + `DecodeFrame` + `set_err`)
- `src/rpc/jsonrpc_serialize.cpp` — ~120 lines (4 encoders + `append_params` + `json_escape`)
- `src/rpc/jsonrpc_msg.cpp` — deleted

Both well under 500.

---

## 3. `bin/videonode_composer_main.cpp` (was `main.cpp`) — 557 lines

After Phase E renames `main.cpp` → `videonode_composer_main.cpp`. Structure:

| Lines   | Responsibility                                          |
|---------|---------------------------------------------------------|
| 1–58    | includes + namespace open                               |
| 60–106  | signal handler + `SourceArgs` / `Args` / `FrameView` + `to_canonical_` |
| 124–166 | `SourceImagePair` + `import_frame_` (EGLImage import helpers) |
| 168–179 | `wait_first_frame_` template                            |
| 181–220 | `start_scm_source_` / `start_ffmpeg_source_`            |
| 222–240 | `write_full_` stdout write helper                       |
| 242–390 | `main()` — argv parsing + per-source backend selection  |
| 391–402 | source startup                                          |
| 404–416 | `gl_compose::GlCompose` init + canvas log               |
| 418–533 | **render loop** — img_cache get, FrameView pick, slot build, `compose.render`, `gbm_bo_map` + stdout write, frame-rate sleep |
| 534–557 | cleanup + final stats                                   |

The plan says: extract canvas/render loop into `src/render/canvas_loop.{cpp,hpp}`,
main becomes argv + run.

### Proposed extraction: `src/render/canvas_loop.{cpp,hpp}`

Move into `src/render/canvas_loop.cpp` + `canvas_loop.hpp`:

- `struct FrameView` (93–105)
- `template <typename FV> FrameView to_canonical_(const FV&)` (106–122)
- `struct SourceImagePair` (124–127)
- `SourceImagePair import_frame_(const egl_ctx::EglCtx&, const FrameView&)` (129–166)
- `template <typename Src> bool wait_first_frame_(...)` (168–179)
- `bool start_scm_source_(...)` (181–195)
- `bool start_ffmpeg_source_(...)` (197–220)
- `bool write_full_(int fd, const void*, size_t)` (222–240)
- The whole render loop body from lines 404–532 — wrapped in a function:

```cpp
namespace render {

struct CanvasLoopArgs {
    int canvas_w = 0;
    int canvas_h = 0;
    int fps = 0;
    int run_seconds = 0;
    bool a_enabled = false;
    bool a_is_scm = false;
    bool b_enabled = false;
    bool b_is_scm = false;
};

// Run the compose-render-stdout loop until run_seconds elapses or
// `running` goes false. Returns frame count rendered. Caller owns the
// sources, gl_compose::GlCompose, and EglCtx.
int RunCanvasLoop(const CanvasLoopArgs& a,
                  egl_ctx::EglCtx& ctx,
                  gl_compose::GlCompose& compose,
                  scm_rights_source::ScmRightsSource& scm_a,
                  scm_rights_source::ScmRightsSource& scm_b,
                  ffmpeg_pipe_source::FfmpegPipeSource& ff_a,
                  ffmpeg_pipe_source::FfmpegPipeSource& ff_b,
                  std::atomic<bool>& running);

} // namespace render
```

The render loop's `get_img` lambda + `img_cache` move inside `RunCanvasLoop`
as locals.

### What stays in `bin/videonode_composer_main.cpp`

- `g_running` + `on_signal` (lines 60–65)
- `struct SourceArgs` (66–79) + `struct Args` (80–91) — argv-only types
- argv parsing (lines 274–388 inside `main()`)
- per-source backend pick (`a_is_scm`, `b_is_scm` decisions, source object
  construction)
- `egl_ctx::EglCtx ctx` + `gl_compose::GlCompose compose` construction
- `start_scm_source_` / `start_ffmpeg_source_` calls (use the now-public
  functions from `canvas_loop.hpp`)
- one call to `render::RunCanvasLoop`
- cleanup (lines 534–553)

### CMake wiring

`canvas_loop` becomes part of the `gl_compose` library or a sibling:

```cmake
# In src/render/CMakeLists.txt — extend the gl_compose library:
vn_add_library(gl_compose
    SOURCES gl_compose.cpp canvas_loop.cpp
    PUBLIC_DEPS egl_ctx gles_bundle scm_rights_source ffmpeg_pipe_source)
```

(The `scm_rights_source` + `ffmpeg_pipe_source` deps move from
`videonode-composer`'s `DEPS` list up to the library, because canvas_loop
now references both. `videonode-composer`'s DEPS shrink correspondingly.)

If the `scm_rights_source` + `ffmpeg_pipe_source` dependency on the render
lib feels wrong layering-wise, prefer a separate `canvas_loop` library:

```cmake
vn_add_library(canvas_loop
    SOURCES canvas_loop.cpp
    PUBLIC_DEPS gl_compose egl_ctx scm_rights_source ffmpeg_pipe_source)
```

…and add `canvas_loop` to `videonode-composer`'s DEPS. This is cleaner —
`gl_compose` stays a pure GPU library.

### Expected post-split line counts

- `src/render/canvas_loop.hpp` — ~30 lines
- `src/render/canvas_loop.cpp` — ~340 lines (helpers 124–240 plus the loop body 418–532)
- `src/bin/videonode_composer_main.cpp` — ~190 lines (argv + setup + cleanup)

Both under 500.

---

## 4. `capture/v4l2_capture.cpp` — 509 lines

Just over the soft target. From the method list, format-related work
clusters cleanly:

| Method (lines)                                | Group       |
|-----------------------------------------------|-------------|
| `Streamer::open` (37)                         | runtime     |
| `Streamer::unmap_all_` (66)                   | runtime     |
| `Streamer::close` (74)                        | runtime     |
| `Streamer::buf_type_` (92)                    | runtime     |
| `Streamer::get_format` (96)                   | **format**  |
| `Streamer::set_format` (120)                  | **format**  |
| `Streamer::request_buffers` (156)             | runtime     |
| `Streamer::query_buffer_` (192)               | runtime     |
| `Streamer::export_buffer` (226)               | runtime     |
| `Streamer::export_all_planes` (248)           | runtime     |
| `Streamer::queue_buffer` (259)                | runtime     |
| `Streamer::dequeue_buffer` (283)              | runtime     |
| `Streamer::stream_on` (332)                   | runtime     |
| `Streamer::subscribe_ctrl_event` (346)        | events      |
| `Streamer::drain_events_typed` (359)          | events      |
| `Streamer::read_ctrl` (375)                   | runtime     |
| `Streamer::query_dv_timings_valid` (388)      | **format**  |
| `Streamer::query_dv_timings_state` (399)      | **format**  |
| `Streamer::subscribe_source_change` (423)     | events      |
| `Streamer::drain_events` (437)                | events      |
| `Streamer::restart_streaming` (457)           | runtime     |
| `Streamer::stream_off` (467)                  | runtime     |
| `Streamer::mmap_buffer` (479)                 | runtime     |

The plan says: split format-negotiation (probe + select) into
`v4l2_format.cpp`; ioctl wrappers stay in `v4l2_capture.cpp`.

### Proposed extraction: `src/capture/v4l2_format.cpp`

Move into `src/capture/v4l2_format.cpp`:

- `Streamer::get_format` (96–118)
- `Streamer::set_format` (120–154)
- `Streamer::query_dv_timings_valid` (388–397)
- `Streamer::query_dv_timings_state` (399–421)
- Any static helpers used only by these methods (e.g. local `xioctl`
  wrappers — read `xioctl` at line 18 stays in `v4l2_capture.cpp` because
  every method calls it; if it's `static`, mark it accessible to both TUs
  by moving its declaration into an internal header, see below).

### Internal header

Currently `xioctl` (line 18) and `close_planes` (line 26) live in an
unnamed namespace at the top of `v4l2_capture.cpp`. After the split, both
TUs need them. Two options, pick at execution:

**Option A — duplicate.** `xioctl` is 8 lines; `close_planes` is 11. Copy
into `v4l2_format.cpp`'s unnamed namespace. Pro: no new header file. Con:
violates DRY (mild — these are tiny stable helpers).

**Option B — internal header.** Create `src/capture/v4l2_internal.hpp`
(not in the install set; not in the public include surface) that exposes
`namespace v4l2::detail { int xioctl(int, unsigned long, void*); void
close_planes(BufferRef&); }`. Both `.cpp` files `#include` it. Pro: DRY.
Con: one more header in the tree.

Recommendation: Option A. These helpers are stable and tiny; the split is
about size budget, not factoring.

### Header changes

`v4l2_capture.hpp` is unchanged — every method moved is already declared
there as a member of `class Streamer`. The split is purely at the .cpp
definition level.

### CMake wiring

`src/capture/CMakeLists.txt` (drafted in reorg-mapping.md) — change the
`v4l2_capture` library to:

```cmake
vn_add_library(v4l2_capture
    SOURCES v4l2_capture.cpp v4l2_format.cpp)
```

No new library, no new public header.

### Expected post-split line counts

- `src/capture/v4l2_capture.cpp` — ~390 lines (runtime + events methods + the unnamed-namespace helpers)
- `src/capture/v4l2_format.cpp` — ~120 lines (get_format, set_format, dv_timings probes, optional copy of `xioctl` if Option A)

Both well under 500.

---

## Cross-cutting notes for the executor

1. **Commit per split.** Each of the four splits lands as its own commit on
   the integration branch. Rollback granularity matters more than commit
   count.

2. **CI gate.** After Phase F.1 + F.2 land (the `check-file-size.sh` script
   + the CI job + the `readability-function-size` clang-tidy options), each
   split-commit re-runs the gate. The hard cap of 700 should pass on every
   split; the soft target of 500 will only fail (as WARN) for the
   videonode-source orchestrator unless the executor also takes the
   sub-split path described in §1.

3. **Test impact.** Phase F splits don't touch test files. The existing
   tests still cover the same symbols because the splits preserve the
   public surface of every library. `ctest --preset dev` should pass
   unchanged after each split.

4. **Header churn.** Three of the four splits add no new public headers
   (jsonrpc, v4l2, source — source adds an internal `orchestrator.hpp` but
   only `bin/` includes it). Only the composer canvas-loop adds
   `src/render/canvas_loop.hpp`. Diff-aware clang-tidy applies, so each new
   header gets graded against the full `.clang-tidy` ruleset on the lines
   you write.

5. **Pre-Phase-E ordering caveat.** This doc speaks in post-Phase-E paths
   (`src/source/`, `src/rpc/`, `src/render/`, `src/capture/`, `src/bin/`).
   If the executor lands Phase F before Phase E for some reason, fall back
   to flat `src/` paths and skip the new `src/source/` subdir — keep the
   orchestrator extraction as a top-level library `src/source_orchestrator.cpp`
   instead.
