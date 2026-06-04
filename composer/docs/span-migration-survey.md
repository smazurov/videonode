# `std::span` migration survey (Wave 3 / D.3)

Inventory of every `(pointer, size)` callsite in `composer/src/` that is a
candidate for migration to `std::span<uint8_t>` (or `std::span<const uint8_t>`).
Grouped by the Phase E domain the file will land in so D.3 fan-out can hand
each teammate a precise punch list.

Classification per row:

- **syscall boundary** — the pair is required by libc/Linux/EGL/GL/MPP/RGA
  ABI. Keep the raw pointer at the call edge; if the function is a thin
  wrapper, expose `std::span` on the inside and reduce to `data()/size()`
  on the way out.
- **internal API** — pure C++ surface owned by composer. Convert the
  signature to `std::span`. Callers can keep passing raw buffers via
  `std::span(ptr, n)` until they too migrate.
- **member field** — a `void*` / `uint8_t*` member used as a buffer cursor
  or mmap handle. Often paired with a separate `size_t` field nearby;
  bundle into `std::span` if the lifetime model allows, otherwise leave as
  raw pointer with a `bytes()` accessor returning a `std::span` view.

Search seed: `rg -n -e 'void\s*\*' -e 'uint8_t\s*\*' -e 'char\s*\*' composer/src/`
(filtered for buffer-passing contexts; C-strings, `argv`, function
pointers, GL/EGL handles, and string literals dropped).

Out of scope: `tools/`, `tests/` — those will get the cleanup for free
once the producing APIs change.

---

## ipc (Phase E `composer/src/ipc/`)

Files: `scm_socket.{cpp,hpp}`, `scm_rights_source.{cpp,hpp}`,
`scm_rights_producer.{cpp,hpp}`, `dma_heap.{cpp,hpp}`, `dmabuf_msg.{cpp,hpp}`.

| Site | Signature | Class |
| --- | --- | --- |
| `scm_socket.cpp:28` | `bool read_full(int fd, void* buf, size_t n)` | internal API — only called inside `scm_socket.cpp`; convert to `std::span<uint8_t>`. Keeps raw `void*` in the `::read` syscall at the bottom. |
| `scm_socket.cpp:47` | `bool write_full(int fd, const void* buf, size_t n)` | internal API — symmetric with `read_full`; convert to `std::span<const uint8_t>`. |
| `scm_socket.cpp:170` | `read_full(sock_fd, body.data(), body_len)` | callsite — auto-fixed once `read_full` takes `std::span`; `std::span(body)` works. |
| `scm_socket.cpp:227` | `iov[1].iov_base = const_cast<char*>(body.data())` | syscall boundary — `iovec` is libc, must stay raw. No change. |
| `dma_heap.hpp:63` / `dma_heap.cpp:89` | `void* mmap_rw(const Buffer&)` | syscall boundary — return type mirrors `::mmap`. Optionally add a `std::span<std::byte> mmap_rw_span(const Buffer&)` overload for callers, leave raw for FFI. |
| `dma_heap.hpp:64` / `dma_heap.cpp:100` | `void munmap_rw(void* ptr, size_t size)` | syscall boundary — pairs with `::munmap`. Could accept `std::span` for ergonomics but the body unconditionally calls `::munmap(p.data(), p.size())`. |
| `dmabuf_msg.{hpp,cpp}` | none — JSON-only surface (`std::string`/`std::string_view`). | n/a |
| `scm_rights_source.{cpp,hpp}` | none in the header; impl uses `recvmsg` directly via `scm_socket::RecvMessage`. | n/a — surface is already typed; migration is transitive via `scm_socket`. |
| `scm_rights_producer.{cpp,hpp}` | none in the header; impl uses `sendmsg` via `scm_socket::SendMessage`. | n/a — same transitive story. |

**Punch list for ipc teammate:** two function signatures to flip
(`read_full`, `write_full`) and one optional ergonomics overload on
`mmap_rw`. Net diff is small; the value is killing the `void*` cast in
`scm_socket.cpp:29` and `:48`.

---

## capture (Phase E `composer/src/capture/`)

Files: `v4l2_capture.{cpp,hpp}`, `mpp_jpeg_dec.{cpp,hpp}`,
`jpeg_dec_turbo.{cpp,hpp}`, `jpeg_dec.hpp`, `source_probe.{cpp,hpp}`,
`fake_source.{cpp,hpp}`.

| Site | Signature | Class |
| --- | --- | --- |
| `v4l2_capture.hpp:193` / `v4l2_capture.cpp:479` | `bool mmap_buffer(uint32_t index, void*& out_ptr, size_t& out_size)` | internal API — out-params are the prime offender; return `std::optional<std::span<std::byte>>` instead. Hot loop in `videonode_source_main.cpp:274` consumes this. |
| `v4l2_capture.hpp:205` | `std::vector<std::pair<void*, size_t>> in_maps_` | member field — replace with `std::vector<std::span<std::byte>>`; the matching `munmap` in the dtor already loops over both fields. |
| `v4l2_capture.cpp:18` | `int xioctl(int fd, unsigned long req, void* arg)` | syscall boundary — `ioctl` ABI. No change. |
| `v4l2_capture.cpp:498` | `void* m = ::mmap(...)` | syscall boundary. No change. |
| `jpeg_dec.hpp:43` | `virtual bool decode(const uint8_t* jpeg, std::size_t size, DecodedNv12& out) = 0` | internal API — base interface. Convert to `std::span<const uint8_t>`; both impls (turbo + mpp) follow. |
| `mpp_jpeg_dec.hpp:89` | `FrameRef decode(const uint8_t* jpeg_data, size_t jpeg_size)` | internal API — convert to `std::span<const uint8_t>`. |
| `mpp_jpeg_dec.hpp:95` / `mpp_jpeg_dec.cpp:159` | `bool decode(const uint8_t* jpeg, std::size_t size, jpeg_dec::DecodedNv12&)` | internal API — same conversion; this is the `jpeg_dec::Decoder` override. |
| `mpp_jpeg_dec.cpp:108` | `mpp_packet_init(&pkt, const_cast<uint8_t*>(jpeg_data), jpeg_size)` | syscall boundary — rockchip-mpp ABI. Reduce to `data()/size()` at the edge. |
| `jpeg_dec_turbo.hpp:29` | `uint8_t* mapped` (member) | member field — points into a dma-heap mmap with a known size stored next to it. Keep raw or expose a `std::span<uint8_t> mapped_bytes()` accessor. |
| `jpeg_dec_turbo.hpp:43` / `jpeg_dec_turbo.cpp:49` | `bool decode(const uint8_t* jpeg, std::size_t size, DecodedNv12&)` | internal API — convert with the base. |
| `jpeg_dec_turbo.cpp:87` | `unsigned char* planes[3]` | syscall boundary — libjpeg-turbo's `tj3DecompressToYUVPlanes` ABI. |
| `jpeg_dec_turbo.cpp:100..116` | `uint8_t*` cursors (`uv`, `up`, `vp`, `u0`, `u1`, `v0`, `v1`, `row`) | internal — local cursors in a tight pixel loop. Leave raw; converting to `std::span` per row adds noise without safety, since the loop already validates strides. |
| `fake_source.hpp:63` / `fake_source.cpp:19,79,80` | `uint8_t* map_` (member) + cursors | member field — back the member with `std::span<uint8_t>`; `fill_y`/`fill_uv` already take `uint8_t*` (see below). |
| `fake_source.cpp:43,57,66,79,80` | `void fill_y(uint8_t* y_plane, int stride, ...)`, `fill_uv(uint8_t* uv_plane, ...)` | internal API — file-local pixel painters; convert to `std::span<uint8_t>` so the bounds (`stride * img_h`) are carried alongside. |
| `source_probe.{cpp,hpp}` | none — surface is health enums + strings. | n/a |

**Punch list for capture teammate:** the `jpeg_dec::Decoder`
inheritance chain is the big win — one virtual + two impls + every
caller goes from `(ptr, size)` to `std::span`. `v4l2_capture::mmap_buffer`
is the next-most-valuable change (kills two out-params). Skip the
inner pixel-loop cursors.

---

## render (Phase E `composer/src/render/`)

Files: `gl_compose.{cpp,hpp}`, `egl_ctx.{cpp,hpp}`, `csc.{cpp,hpp}`,
`csc_gles.{cpp,hpp}`, `rga_csc.{cpp,hpp}`, `format_dispatch.{cpp,hpp}`,
`gbm_alloc.{cpp,hpp}`, `nv12_buf.{cpp,hpp}`, `placeholder_painter.{cpp,hpp}`.

| Site | Signature | Class |
| --- | --- | --- |
| `placeholder_painter.hpp:45` / `placeholder_painter.cpp:108` | `void paint_base(uint8_t* nv12, int w, int h, const char* device_path)` | internal API — full-frame NV12 pointer with no size. Convert to `std::span<uint8_t>`; size is `w * h * 3 / 2` and validating it at the entry would have caught past stride mismatches. |
| `placeholder_painter.hpp:51` / `placeholder_painter.cpp:149` | `void paint_tick(uint8_t* nv12, int w, int h, uint64_t tick_idx, uint64_t wallclock_ms, ...)` | internal API — same conversion as `paint_base`. |
| `placeholder_painter.cpp:33,47,66,84` | `fill_luma`, `fill_chroma`, `draw_glyph_luma`, `draw_text_luma` (`uint8_t*` plane + w/h) | internal API — file-local helpers. Convert if you're already changing `paint_base`/`paint_tick`; otherwise low priority — call surface is internal to this TU. |
| `nv12_buf.hpp:42` / `nv12_buf.cpp:42` | `void* impl` (member) | member field — opaque PIMPL handle, not a buffer. Leave as-is. |
| `nv12_buf.hpp:78,79` | `void* y; void* uv;` (member) | member field — mapped Y/UV planes returned to callers. Pair with the existing `y_pitch`/`uv_pitch` and frame dims; expose `std::span<uint8_t> y_bytes() const` and `uv_bytes() const`. Many call sites do `static_cast<uint8_t*>(m.y)` immediately (see `videonode_source_main.cpp:332,393,395,414`, `ffmpeg_pipe_source.cpp:146,154`). |
| `nv12_buf.cpp:187` | `auto* base = static_cast<uint8_t*>(impl->mapped);` | internal — local cursor; auto-fixed once `impl->mapped` is typed. |
| `gbm_alloc.hpp:57,58` / `gbm_alloc.cpp:17` | `void* y = nullptr; void* uv = nullptr;` (member) | member field — same story as `nv12_buf`. Expose `std::span<uint8_t>` accessors. |
| `gbm_alloc.cpp:76,77` | `gbm_bo_set_user_data(..., [](gbm_bo*, void* p) { delete (MapState*)p; })` | syscall boundary — gbm callback ABI. No change. |
| `gl_compose.cpp:268,271` | `glVertexAttribPointer(..., (void*)(0))` / `(void*)(2 * sizeof(float))` | syscall boundary — GL ABI quirk (pointer is actually an offset). No change. |
| `csc.{cpp,hpp}` | none — interface signatures use `NV12Buf` / GBM handles, not raw bytes. | n/a |
| `csc_gles.{cpp,hpp}` | none in the buffer-passing sense — only `const char*` shader sources and GL handles. | n/a |
| `rga_csc.{cpp,hpp}` | none — RGA APIs work on `rga_buffer_t` (fd-based). | n/a |
| `format_dispatch.{cpp,hpp}` | none — works on `Format` / `Frame` value types. | n/a |
| `egl_ctx.{cpp,hpp}` | none in scope — `EGLDisplay`/`EGLConfig` handles only. | n/a |

**Punch list for render teammate:** `placeholder_painter`'s two
public entry points + `nv12_buf` / `gbm_alloc` `y`/`uv` accessors are
the win. The rest of render lives behind `NV12Buf`, so it benefits
transitively.

---

## process (Phase E `composer/src/process/`)

Files: `child_process.{cpp,hpp}`, `ffmpeg_pipe_source.{cpp,hpp}`.

| Site | Signature | Class |
| --- | --- | --- |
| `ffmpeg_pipe_source.hpp:118` / `ffmpeg_pipe_source.cpp:17` | `bool read_full_(uint8_t* dst, size_t n)` / `bool read_exact(int fd, void* dst, size_t n)` | internal API — file-local helpers around `::read`. Convert to `std::span<uint8_t>`. |
| `ffmpeg_pipe_source.cpp:146,154` | `uint8_t* y_base = static_cast<uint8_t*>(m.y);` | internal — cursors into the destination NV12 plane; auto-fixed once `NV12Buf::y/uv` become `std::span` (see render). |
| `ffmpeg_pipe_source.cpp:148,151,156,159` | `read_exact(ffmpeg_stdout_fd_, y_base, ...)` / `... y_base + y * slot.buf.y_stride, width_` | callsite — auto-fixed once `read_exact` takes `std::span`; `std::span(y_base, n)` works. |
| `ffmpeg_pipe_source.hpp:130,131` | `void* mapped_y = nullptr; void* mapped_uv = nullptr;` (member) | member field — slot-local cursor mirrors `NV12Buf` layout. Convert with the rest of the NV12 surface. |
| `child_process.hpp` (no buffer surfaces) | n/a | — |
| `child_process.cpp:52,55` | `std::vector<char*> argv_c; argv_c.push_back(const_cast<char*>(s.c_str()));` | syscall boundary — `execvp` ABI. No change; we already accept this for argv. |
| `child_process.cpp:12` | `extern char** environ;` | syscall boundary — libc global. No change. |

**Punch list for process teammate:** `read_exact` / `read_full_`
conversion is trivial (file-local, two call patterns). The bigger
ergonomic gain in this domain comes from finishing the NV12
surface migration in render; this domain mostly inherits.

---

## Cross-domain hot spots (not in Phase E groupings)

Files that aren't being moved by Phase E but contain buffer
plumbing that should track the migrations above.

| Site | Signature | Class |
| --- | --- | --- |
| `main.cpp:222` | `bool write_full_(int fd, const void* buf, size_t n)` | internal API — duplicate of `scm_socket::write_full` / `ffmpeg_pipe_source::read_exact`. Migrate to `std::span<const uint8_t>`; better, drop the duplicate and call the shared helper once Phase E gives us one. |
| `main.cpp:497..509` | `gbm_bo_map(..., &map_stride, &map_data)`; `const uint8_t* row = static_cast<const uint8_t*>(canvas_map) + y * map_stride;` | syscall + internal — `gbm_bo_map` is ABI; the local `row` walk benefits once `canvas_map` carries its size. |
| `videonode_sink_main.cpp:37` | `bool write_full(int fd, const void* buf, size_t n)` | internal API — third copy of the same helper. Same migration + dedup story. |
| `videonode_sink_main.cpp:63..68` | `void* m = ::mmap(...)` + `static_cast<const uint8_t*>(m)` | syscall boundary + local cursor. No change at the syscall; cursor becomes `std::span` once the helper takes one. |
| `videonode_source_main.cpp:178,183,184` | `std::vector<void*> in_maps; std::vector<void*> out_y; std::vector<void*> out_uv;` (members) | member field — three parallel vectors that should each be `std::vector<std::span<std::byte>>` after `v4l2_capture::mmap_buffer` migrates. Drives the bulk of the `static_cast<uint8_t*>(m.y)` churn at lines 332, 390-414, 1062-1063. |
| `videonode_source_main.cpp:274` | `void* ptr = nullptr;` | local — disappears with the `mmap_buffer` migration. |

The shared takeaway: there are three near-identical `write_full` /
`read_full` helpers in the tree (`scm_socket.cpp:47`,
`videonode_sink_main.cpp:37`, `main.cpp:222`) and one near-identical
`read_exact` (`ffmpeg_pipe_source.cpp:17`). Phase E should land a single
`io::read_full(int fd, std::span<uint8_t>)` / `io::write_full(int fd,
std::span<const uint8_t>)` in `process/` (or a small `util/io.hpp`)
and the four current copies collapse into one.

## Not surveyed

- GL handle types (`GLuint`, `EGLDisplay`, etc.) — not buffers.
- `const char*` shader source / format-string parameters — these stay.
- libc strerror / printf-family — out of scope.
- `argv` / `environ` — execve ABI, intentionally raw.
- `gbm_bo_set_user_data` callbacks — gbm ABI.
- `composer/tools/` and `composer/tests/` — non-shipping code; they
  auto-fix once their dependencies migrate, and changes there don't
  affect the public surface.

## Status

Executed during Wave 3 (additive `std::span` overloads + accessors) and
Wave 3.5 (dead raw-ptr surfaces dropped, callers migrated). The "ipc",
"capture", "render", "process", and cross-domain hot-spot punch lists
above were the working basis for the per-file commits in
`da7b735..dc7e16f` and the dead-surface drops in `cacd24f`/`1b5cf3c`/`91f0534`.

The single deferred item — collapsing the three duplicate
`read_full`/`write_full` helpers (`ipc/scm_socket.cpp`, `bin/main.cpp`,
`bin/videonode_sink_main.cpp`) into one shared `io::` helper — remains
open. Tracked as a Wave-4 follow-up cleanup; not yet landed.
