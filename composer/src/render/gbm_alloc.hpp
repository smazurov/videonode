// gbm_alloc — Mesa GBM-backed NV12 allocator.
//
// On Fedora / generic Mesa boxes, radeonsi (and other Mesa drivers) reject
// EGLImage imports of dma_heap-backed NV12 buffers — the driver wants the
// buffer to carry a known modifier. GBM creates buffers with the right
// metadata, and the dma-buf fd it exports is importable.
//
// On the rig (RK3588 with Panthor / Mali), dma_heap works fine and we
// keep using it. The producer chooses its allocator; the consumer
// doesn't care — both paths look like "a dma-buf fd with NV12 planes" on
// the wire.
//
// We allocate the underlying bo as a R8 surface of width = W,
// height = H * 3/2 so the linear byte layout matches NV12: Y plane in
// rows [0, H), UV plane in rows [H, H + H/2). Stride is what GBM
// returns; the caller must use that as the plane0_pitch (and the chroma
// row size).

#pragma once

#include <cstddef>
#include <cstdint>
#include <mutex>
#include <span>

struct gbm_device;
struct gbm_bo;

namespace gbm_alloc {

// Process-wide mutex serializing all gbm_bo_map / gbm_bo_unmap calls on
// this process's shared gbm_device. Mesa's gallium "threaded context" is
// single-threaded by design; concurrent gbm_bo_map/unmap from
// ffmpeg_pipe_source capture threads and canvas_loop's main-thread
// readback raced inside si_texture_transfer_unmap and crashed during
// deferred-unmap execution. Every caller that touches a gbm_bo's CPU
// mapping must take this lock for as long as a single gbm_device backs
// all the BOs (which it does in videonode-composer).
std::mutex& gbm_device_mu();

// Two-bo NV12: separate Y (R8) and UV (GR88) GBM bos, each at its own
// PLANE0_OFFSET=0. This is the layout radeonsi/amdgpu reliably imports
// — single-fd multi-plane with non-zero PLANE1_OFFSET silently samples
// as zero (per minigbm/Chromium pattern for AMD). On other drivers
// (Panthor / iris) the layout works just as well.
struct Nv12Buf {
    gbm_bo* y_bo = nullptr;
    int y_fd = -1;
    uint32_t y_stride = 0;
    gbm_bo* uv_bo = nullptr;
    int uv_fd = -1;
    uint32_t uv_stride = 0;
    int width = 0;
    int height = 0;
    uint64_t modifier = 0;

    bool valid() const { return y_bo != nullptr && y_fd >= 0 && uv_bo != nullptr && uv_fd >= 0; }
};

// Allocate a GBM-backed NV12 buffer. Width and height must be even.
// Returns an invalid Nv12Buf on failure (logs to stderr).
Nv12Buf alloc(gbm_device* gbm, int width, int height);

// Map both planes for CPU read-write. Caller gets a pointer to Y plane
// memory and a pointer to UV plane memory.
struct Mapped {
    void* y = nullptr;
    void* uv = nullptr;
    int height = 0;
    uint32_t y_stride = 0;
    uint32_t uv_stride = 0;

    std::span<uint8_t> y_bytes() const;
    std::span<uint8_t> uv_bytes() const;
};
Mapped map_rw(Nv12Buf& b);
void unmap(Nv12Buf& b);

void free(Nv12Buf& b);

} // namespace gbm_alloc
