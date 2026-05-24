// vn_snapshot implementation. See snapshot.hpp.

#include "src/snapshot/snapshot.hpp"

#include <cstring>
#include <sys/mman.h>

namespace vn::snapshot {

namespace {

void* default_mmap_(int fd, size_t length, off_t offset) {
    return ::mmap(nullptr, length, PROT_READ, MAP_SHARED, fd, offset);
}

// Pack `rows` rows of `row_bytes` from `src` (pitch-strided) into `dst`
// (tight-packed). pitch == row_bytes short-circuits to a single memcpy.
void pack_rows_(const uint8_t* src, size_t pitch, uint8_t* dst, size_t row_bytes, size_t rows) {
    if (pitch == row_bytes) {
        std::memcpy(dst, src, row_bytes * rows);
        return;
    }
    for (size_t r = 0; r < rows; ++r) {
        std::memcpy(dst + r * row_bytes, src + r * pitch, row_bytes);
    }
}

} // namespace

bool MmapAndPack(const Plane& plane, std::span<uint8_t> dst, size_t dst_offset, MmapFn mmap_fn) {
    if (plane.fd < 0 || plane.pitch < plane.row_bytes || plane.rows == 0 || plane.row_bytes == 0)
        return false;
    const size_t need = plane.row_bytes * plane.rows;
    if (dst.size() < dst_offset + need)
        return false;

    // Map from fd start to cover both `offset` and the rows. mmap requires
    // a page-aligned offset; passing zero and indexing in by `offset`
    // sidesteps that constraint at the cost of mapping more bytes. Both
    // sources today produce small dma-bufs (≤8 MB) so this is fine.
    const size_t map_size = plane.offset + plane.pitch * plane.rows;
    MmapFn mfn = mmap_fn != nullptr ? mmap_fn : &default_mmap_;
    void* mapped = mfn(plane.fd, map_size, 0);
    if (mapped == MAP_FAILED || mapped == nullptr)
        return false;

    const auto* src = static_cast<const uint8_t*>(mapped) + plane.offset;
    pack_rows_(src, plane.pitch, dst.data() + dst_offset, plane.row_bytes, plane.rows);

    ::munmap(mapped, map_size);
    return true;
}

void LatestFrameHolder::Update(FrameRef ref) {
    std::lock_guard<std::mutex> lock(mu_);
    ref_ = ref;
}

void LatestFrameHolder::SetMmapFnForTest(MmapFn fn) {
    std::lock_guard<std::mutex> lock(mu_);
    mmap_fn_ = fn;
}

bool LatestFrameHolder::Snapshot(FrameBytes& out) {
    FrameRef ref;
    MmapFn mfn = nullptr;
    {
        std::lock_guard<std::mutex> lock(mu_);
        if (!ref_)
            return false;
        ref = *ref_;
        mfn = mmap_fn_;
    }

    if (ref.width == 0 || ref.height == 0)
        return false;

    out.format = ref.format;
    out.width = ref.width;
    out.height = ref.height;
    out.pitch_y = ref.pitch_y;
    out.pitch_uv = ref.pitch_uv;
    out.frame_idx = ref.frame_idx;
    out.captured_at_ns = ref.captured_at_ns;

    if (ref.format == Format::Nv12) {
        const size_t y_bytes = size_t(ref.width) * ref.height;
        const size_t uv_bytes = size_t(ref.width) * (ref.height / 2);
        out.bytes.resize(y_bytes + uv_bytes);
        if (!MmapAndPack(ref.planes[0], out.bytes, 0, mfn))
            return false;
        if (!MmapAndPack(ref.planes[1], out.bytes, y_bytes, mfn))
            return false;
    } else { // BGRA
        const size_t bytes = size_t(ref.width) * ref.height * 4;
        out.bytes.resize(bytes);
        if (!MmapAndPack(ref.planes[0], out.bytes, 0, mfn))
            return false;
    }
    return true;
}

} // namespace vn::snapshot
