// vn_snapshot — lazy "latest frame" reference for sources and composers.
//
// Producers (videonode-source's orchestrator, videonode-composer's canvas
// loop) call Update(FrameRef) cheaply on every produced frame — the call
// only stashes a plane descriptor under a mutex, no mmap or copy. RPC
// handlers call Snapshot(FrameBytes&) which performs the mmap + tight-
// packed copy out of the dma-buf on demand.
//
// Used by:
//   - composer/src/source/source_service       (NV12 sources, 2 planes)
//   - composer/src/render/composer_service     (NV12 canvas, 2 planes)
//
// Lifetime: the fds referenced by a stashed FrameRef must remain valid for
// the duration of Snapshot(); in practice both producers hold their slot
// rings alive across the gRPC call.

#pragma once

#include <array>
#include <cstddef>
#include <cstdint>
#include <memory>
#include <mutex>
#include <optional>
#include <span>
#include <sys/types.h>
#include <vector>

namespace vn::snapshot {

enum class Format { Nv12 };

// Sentinel slot_index for frames not backed by a refcounted ring slot
// (placeholder / MJPEG frames). Matches the uint32 wire sentinel widened
// to uint64, so a frame's slot survives the DecodedNv12 → Header →
// FrameView → FrameRef round-trip as kNoSlot. pin()/release() ignore it.
inline constexpr uint64_t kNoSlot = 0xFFFFFFFFull;

// Gate for recycling a producer ring slot. SlotOwner (source/) implements
// it; the holder pins the slot it is about to read so the producer cannot
// recycle it mid-read. Defined here to keep the snapshot lib free of a
// source/ dependency.
struct SlotPinner {
    SlotPinner() = default;
    SlotPinner(const SlotPinner&) = delete;
    SlotPinner& operator=(const SlotPinner&) = delete;
    virtual ~SlotPinner() = default;
    // Take one ref iff the slot still holds `generation`. Returns false if
    // the slot was already recycled (or slot is kNoSlot).
    [[nodiscard]] virtual bool pin(uint64_t slot, uint64_t generation) = 0;
    virtual void release(uint64_t slot, uint64_t generation) = 0;
};

// One source plane in a dma-buf. pitch >= row_bytes; rows of row_bytes
// each are tight-packed into the destination.
struct Plane {
    int fd = -1;
    size_t offset = 0;
    size_t pitch = 0;
    size_t row_bytes = 0;
    size_t rows = 0;
};

// A point-in-time reference to a producer's frame. Pixel data lives in
// dma-bufs reachable via the planes' fds; metadata is cheap to copy.
struct FrameRef {
    Format format = Format::Nv12;
    uint32_t width = 0;
    uint32_t height = 0;
    std::array<Plane, 2> planes{}; // [0]=Y; [1]=UV
    uint32_t pitch_y = 0;          // surfaces pitches for the response metadata
    uint32_t pitch_uv = 0;
    uint64_t frame_idx = 0;
    uint64_t captured_at_ns = 0;
    uint64_t slot_index = kNoSlot;
    uint64_t generation = 0;
};

// Output of Snapshot(): tight-packed bytes plus metadata. The bytes are
// the Y plane (width*height) followed by the interleaved UV plane
// (width*height/2).
struct FrameBytes {
    Format format = Format::Nv12;
    uint32_t width = 0;
    uint32_t height = 0;
    uint32_t pitch_y = 0;
    uint32_t pitch_uv = 0;
    uint64_t frame_idx = 0;
    uint64_t captured_at_ns = 0;
    std::vector<uint8_t> bytes;
};

// Test seam for MmapAndPack / LatestFrameHolder. Default uses ::mmap with
// PROT_READ + MAP_SHARED; returning MAP_FAILED simulates a kernel-side
// failure. Caller owns the returned mapping and must munmap.
using MmapFn = void* (*)(int fd, size_t length, off_t offset);

// Mmap `fd` at `plane.offset` for `plane.pitch * plane.rows` bytes, copy
// `plane.rows` rows of `plane.row_bytes` each into `dst.subspan(dst_offset)`,
// munmap. Returns false on:
//   - plane.fd < 0
//   - plane.pitch < plane.row_bytes
//   - dst.size() < dst_offset + row_bytes * rows
//   - mmap failure
// `mmap_fn` is the test seam; pass nullptr to use ::mmap.
[[nodiscard]] bool MmapAndPack(const Plane& plane, std::span<uint8_t> dst, size_t dst_offset,
                               MmapFn mmap_fn = nullptr);

// Thread-safe holder. Single producer + single consumer is the common
// case but multi-consumer Snapshot() is also safe (each gets its own copy).
class LatestFrameHolder {
  public:
    // Replace any previously stashed ref. Cheap (just copy a small struct
    // under a mutex). Pins the new ref's slot and releases the prior one so
    // the latest frame's buffer is never recycled while it is the latest.
    void Update(FrameRef ref);

    // Pack the latest frame's pixels into `out`. Returns false if no frame
    // has been stashed yet, if MmapAndPack fails for any plane, or if the
    // stashed ref has zero dimensions. On false, `out` is left in a
    // partially-filled state and should be discarded. Holds an extra slot
    // pin across the mmap+pack so a concurrent producer cannot recycle the
    // slot mid-read.
    [[nodiscard]] bool Snapshot(FrameBytes& out);

    // Point the holder at the producer's slot gate. Replaces any previous
    // pinner, dropping the stale pin. Pass nullptr to detach (e.g. on
    // capture teardown). The holder keeps the pinner alive while pinned.
    void SetSlotPinner(std::shared_ptr<SlotPinner> pinner);

    // Override the mmap used by Snapshot(). Default (nullptr) uses ::mmap.
    void SetMmapFnForTest(MmapFn fn);

    ~LatestFrameHolder();

  private:
    std::mutex mu_;
    std::optional<FrameRef> ref_;
    MmapFn mmap_fn_ = nullptr;
    std::shared_ptr<SlotPinner> pinner_;
    bool have_pin_ = false; // pinner_ holds a ref on ref_'s slot
};

} // namespace vn::snapshot
