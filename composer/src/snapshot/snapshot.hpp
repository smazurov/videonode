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

// Gate for recycling a producer ring slot. A producer that refcounts its
// ring slots can implement it so the holder pins the slot it is about to
// read, blocking recycle mid-read. No producer wires one today (slots are
// always kNoSlot), so the holder never pins — see Update/Snapshot, which
// short-circuit on kNoSlot.
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

struct FrameRef {
    Format format = Format::Nv12;
    uint32_t width = 0;
    uint32_t height = 0;
    std::array<Plane, 2> planes{}; // [0]=Y; [1]=UV
    uint32_t pitch_y = 0;
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

using MmapFn = void* (*)(int fd, size_t length, off_t offset);

[[nodiscard]] bool MmapAndPack(const Plane& plane, std::span<uint8_t> dst, size_t dst_offset,
                               MmapFn mmap_fn = nullptr);

// Thread-safe holder. Single producer + single consumer is the common
// case but multi-consumer Snapshot() is also safe (each gets its own copy).
class LatestFrameHolder {
  public:
    void Update(FrameRef ref);

    [[nodiscard]] bool Snapshot(FrameBytes& out);

    // Point the holder at the producer's slot gate. Replaces any previous
    // pinner, dropping the stale pin. Pass nullptr to detach (e.g. on
    // capture teardown). The holder keeps the pinner alive while pinned.
    void SetSlotPinner(std::shared_ptr<SlotPinner> pinner);

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
