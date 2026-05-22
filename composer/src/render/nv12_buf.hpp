// nv12_buf — backend-agnostic NV12 frame buffer for videonode-source.
//
// Same producer code works on both hosts:
//   - Rig (RK3588, librga): single dma_heap bo, Y at offset 0, UV at W*H.
//     RGA's imcvtcolor expects a single dma-buf with both planes inside.
//   - Fedora / generic Mesa box (libgbm, no librga): two GBM bos (R8 for Y,
//     GR88 for UV). amdgpu/radeonsi can't sample sub-region imports at
//     non-zero PLANE0_OFFSET; the two-bo layout is the canonical AMD path.
//
// Backend chosen at compile time:
//   HAVE_RGA  → dma_heap single-buffer (the rig)
//   HAVE_GBM (and not HAVE_RGA) → gbm split-buffer (Mesa hosts)
//
// The wire format (dmabuf_msg.hpp) already supports both: plane0_fd +
// plane1_fd may be the same fd (rig) or different fds (Fedora), each
// with its own offset + pitch. Consumers (videonode-composer,
// videonode-sink) handle both.

#pragma once

#include <cstddef>
#include <cstdint>
#include <span>
#include <utility>

struct gbm_device;

namespace nv12_buf {

// One NV12 frame storage. Owns its dma-buf fd(s); destructor closes.
struct Buffer {
    // Public layout (also the wire-format fields broadcast_frame fills in).
    int y_fd = -1;
    int uv_fd = -1; // = y_fd on single-buffer (rig); separate on split (Fedora).
    uint32_t y_offset = 0;
    uint32_t uv_offset = 0;
    uint32_t y_pitch = 0;
    uint32_t uv_pitch = 0;
    int width = 0;
    int height = 0;

    // Backend-private pointers (filled by alloc(), released by free()).
    void* impl = nullptr;

    Buffer() = default;
    Buffer(const Buffer&) = delete;
    Buffer& operator=(const Buffer&) = delete;
    Buffer(Buffer&& o) noexcept { *this = std::move(o); }
    Buffer& operator=(Buffer&& o) noexcept;
    ~Buffer();

    [[nodiscard]] bool valid() const { return y_fd >= 0 && uv_fd >= 0; }
};

// Backend selector + per-process state holder. On Fedora it owns a
// gbm_device borrowed from the caller's EglCtx; on the rig it's stateless.
class Allocator {
  public:
    Allocator() = default;
    ~Allocator();

    // Bring up the backend. On the gbm backend, `gbm` must be non-null
    // (borrow EglCtx::gbm()). On the dma_heap backend, `gbm` is ignored
    // and may be null.
    [[nodiscard]] bool init(gbm_device* gbm = nullptr);

    // Allocate one NV12 frame. Width and height must be even.
    Buffer alloc(int width, int height);

  private:
    gbm_device* gbm_ = nullptr;
};

// Per-frame CPU access. On both backends this maps the underlying bo(s)
// and returns pointers to plane bytes. The caller MUST unmap before the
// GPU samples the buffer — radeonsi keeps a mapped bo in CPU-coherent
// state and reads of stale memory if the map is held.
struct Mapping {
    void* y = nullptr;
    void* uv = nullptr;
    int height = 0;
    uint32_t y_pitch = 0;
    uint32_t uv_pitch = 0;

    [[nodiscard]] std::span<uint8_t> y_bytes() const;
    [[nodiscard]] std::span<uint8_t> uv_bytes() const;
};
Mapping map_rw(Buffer& b);
void unmap(Buffer& b);

// dma-buf sync_start / sync_end around CPU writes (forwarded to
// DMA_BUF_IOCTL_SYNC). Cheap to call on both backends; on the gbm
// backend the unmap implicitly covers it, but explicit calls are fine.
enum class SyncDir { Read, Write, ReadWrite };
void sync_start(const Buffer& b, SyncDir dir);
void sync_end(const Buffer& b, SyncDir dir);

} // namespace nv12_buf
