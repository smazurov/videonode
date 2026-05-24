#include "src/render/nv12_buf.hpp"

#include "src/common/log_levels.hpp"
#include "src/ipc/dma_heap.hpp"

#include <cstring>
#include <memory>
#include <sys/mman.h>
#include <unistd.h>
#include <utility>

#if defined(HAVE_GBM) && !defined(HAVE_RGA)
#include "src/render/gbm_alloc.hpp"
#endif

namespace nv12_buf {

// ── shared helpers ─────────────────────────────────────────────────────

namespace {

dmaheap::SyncDir to_dmaheap(SyncDir d) {
    switch (d) {
    case SyncDir::Read:
        return dmaheap::SyncDir::Read;
    case SyncDir::Write:
        return dmaheap::SyncDir::Write;
    case SyncDir::ReadWrite:
        return dmaheap::SyncDir::ReadWrite;
    }
    return dmaheap::SyncDir::ReadWrite;
}

#if defined(HAVE_GBM) && !defined(HAVE_RGA)
// gbm backend: impl owns the gbm_alloc::Nv12Buf plus raw mmap pointers
// onto each plane's dma-buf fd. We bypass gbm_bo_map intentionally —
// radeonsi's gbm_bo_map allocates a CPU staging buffer and only copies
// dirty pages back to the BO on gbm_bo_unmap, so a separate consumer
// process (videonode-sink) mmapping the same dma-buf fd reads the
// original zero-filled BO until the writer drops the mapping. Raw
// mmap() on the exported dma-buf fd gives every mapper a coherent
// view of the same backing pages.
struct GbmImpl {
    gbm_alloc::Nv12Buf nv;
    void* y_map = nullptr;
    size_t y_map_size = 0;
    void* uv_map = nullptr;
    size_t uv_map_size = 0;
};
#else
// dma_heap backend: impl is a heap-allocated single dmaheap::Buffer + map.
struct DmaImpl {
    dmaheap::Buffer bo;
    void* mapped = nullptr;
};
#endif

} // namespace

// ── Mapping span accessors ─────────────────────────────────────────────

std::span<uint8_t> Mapping::y_bytes() const {
    if (!y)
        return {};
    return {static_cast<uint8_t*>(y), size_t(height) * y_pitch};
}

std::span<uint8_t> Mapping::uv_bytes() const {
    if (!uv)
        return {};
    return {static_cast<uint8_t*>(uv), size_t(height) / 2 * uv_pitch};
}

// ── Buffer lifetime ────────────────────────────────────────────────────

Buffer& Buffer::operator=(Buffer&& o) noexcept {
    if (this == &o)
        return *this;
    y_fd = o.y_fd;
    uv_fd = o.uv_fd;
    y_offset = o.y_offset;
    uv_offset = o.uv_offset;
    y_pitch = o.y_pitch;
    uv_pitch = o.uv_pitch;
    width = o.width;
    height = o.height;
    impl = o.impl;
    o.y_fd = o.uv_fd = -1;
    o.impl = nullptr;
    return *this;
}

Buffer::~Buffer() {
    if (!impl)
        return;
#if defined(HAVE_GBM) && !defined(HAVE_RGA)
    // Re-take ownership through unique_ptr so deletion goes through the
    // typed deleter (cppcoreguidelines-owning-memory clean).
    std::unique_ptr<GbmImpl> g{static_cast<GbmImpl*>(impl)};
    if (g->y_map)
        ::munmap(g->y_map, g->y_map_size);
    if (g->uv_map)
        ::munmap(g->uv_map, g->uv_map_size);
    gbm_alloc::free(g->nv);
#else
    std::unique_ptr<DmaImpl> d{static_cast<DmaImpl*>(impl)};
    if (d->mapped) {
        dmaheap::munmap_rw(d->mapped, d->bo.size);
        d->mapped = nullptr;
    }
    // dmaheap::Buffer's destructor closes its fd. Setting y_fd/uv_fd above
    // to -1 (via the move-from path) means we don't double-close.
#endif
    impl = nullptr;
}

// ── Allocator ──────────────────────────────────────────────────────────

Allocator::~Allocator() = default;

bool Allocator::init(gbm_device* gbm) {
#if defined(HAVE_GBM) && !defined(HAVE_RGA)
    if (!gbm) {
        vn::log::error("nv12_buf: gbm backend requires a gbm_device");
        return false;
    }
    gbm_ = gbm;
#else
    (void)gbm;
#endif
    return true;
}

#if defined(HAVE_GBM) && !defined(HAVE_RGA)

Buffer Allocator::alloc(int width, int height) {
    Buffer out;
    if (!gbm_) {
        vn::log::error("nv12_buf::alloc: gbm backend not initialized");
        return out;
    }
    auto impl = std::make_unique<GbmImpl>();
    impl->nv = gbm_alloc::alloc(gbm_, width, height);
    if (!impl->nv.valid())
        return out;
    out.y_fd = impl->nv.y_fd;
    out.uv_fd = impl->nv.uv_fd;
    out.y_offset = 0;
    out.uv_offset = 0;
    out.y_pitch = impl->nv.y_stride;
    out.uv_pitch = impl->nv.uv_stride;
    out.width = width;
    out.height = height;
    out.impl = impl.release();
    return out;
}

namespace {
bool ensure_dmabuf_mmap_(int fd, void*& mapping, size_t& mapping_size, const char* tag) {
    if (mapping)
        return true;
    const off_t sz = ::lseek(fd, 0, SEEK_END);
    if (sz <= 0) {
        vn::log::error("nv12_buf::map_rw: lseek(%s fd=%d) failed: %s", tag, fd, std::strerror(errno));
        return false;
    }
    void* p = ::mmap(nullptr, static_cast<size_t>(sz), PROT_READ | PROT_WRITE, MAP_SHARED, fd, 0);
    if (p == MAP_FAILED) {
        vn::log::error("nv12_buf::map_rw: mmap(%s fd=%d size=%lld) failed: %s", tag, fd,
                       static_cast<long long>(sz), std::strerror(errno));
        return false;
    }
    mapping = p;
    mapping_size = static_cast<size_t>(sz);
    return true;
}
} // namespace

Mapping map_rw(Buffer& b) {
    Mapping out;
    if (!b.valid() || !b.impl)
        return out;
    auto* impl = static_cast<GbmImpl*>(b.impl);
    if (!ensure_dmabuf_mmap_(impl->nv.y_fd, impl->y_map, impl->y_map_size, "y"))
        return out;
    if (!ensure_dmabuf_mmap_(impl->nv.uv_fd, impl->uv_map, impl->uv_map_size, "uv"))
        return out;
    out.y = impl->y_map;
    out.uv = impl->uv_map;
    out.height = b.height;
    out.y_pitch = b.y_pitch;
    out.uv_pitch = b.uv_pitch;
    return out;
}

void unmap(Buffer& b) {
    if (!b.valid() || !b.impl)
        return;
    auto* impl = static_cast<GbmImpl*>(b.impl);
    if (impl->y_map) {
        ::munmap(impl->y_map, impl->y_map_size);
        impl->y_map = nullptr;
        impl->y_map_size = 0;
    }
    if (impl->uv_map) {
        ::munmap(impl->uv_map, impl->uv_map_size);
        impl->uv_map = nullptr;
        impl->uv_map_size = 0;
    }
}

#else // dma_heap backend (rig + RGA)

Buffer Allocator::alloc(int width, int height) {
    Buffer out;
    if (width <= 0 || height <= 0 || (width & 1) || (height & 1))
        return out;
    auto impl = std::make_unique<DmaImpl>();
    const size_t sz = static_cast<size_t>(width) * static_cast<size_t>(height) * 3 / 2;
    // Try "system-uncached" first (RK3588 prefers it for output buffers
    // RGA writes to without CPU readback); fall back to plain "system".
    impl->bo = dmaheap::alloc(dmaheap::kHeapUncached, sz);
    if (!impl->bo.valid())
        impl->bo = dmaheap::alloc(dmaheap::kHeapSystem, sz);
    if (!impl->bo.valid())
        return out;
    out.y_fd = impl->bo.fd.get();
    out.uv_fd = impl->bo.fd.get(); // same fd, different offsets
    out.y_offset = 0;
    out.uv_offset = static_cast<uint32_t>(width) * static_cast<uint32_t>(height);
    out.y_pitch = static_cast<uint32_t>(width);
    out.uv_pitch = static_cast<uint32_t>(width);
    out.width = width;
    out.height = height;
    out.impl = impl.release();
    return out;
}

Mapping map_rw(Buffer& b) {
    Mapping out;
    if (!b.valid() || !b.impl)
        return out;
    auto* impl = static_cast<DmaImpl*>(b.impl);
    if (!impl->mapped) {
        impl->mapped = dmaheap::mmap_rw(impl->bo);
        if (!impl->mapped)
            return out;
    }
    auto* base = static_cast<uint8_t*>(impl->mapped);
    out.y = base + b.y_offset;
    out.uv = base + b.uv_offset;
    out.height = b.height;
    out.y_pitch = b.y_pitch;
    out.uv_pitch = b.uv_pitch;
    return out;
}

void unmap(Buffer& b) {
    if (!b.valid() || !b.impl)
        return;
    auto* impl = static_cast<DmaImpl*>(b.impl);
    if (impl->mapped) {
        dmaheap::munmap_rw(impl->mapped, impl->bo.size);
        impl->mapped = nullptr;
    }
}

#endif

// ── sync_start / sync_end (shared) ─────────────────────────────────────

void sync_start(const Buffer& b, SyncDir d) {
    if (b.y_fd >= 0)
        dmaheap::sync_start(b.y_fd, to_dmaheap(d));
    if (b.uv_fd >= 0 && b.uv_fd != b.y_fd)
        dmaheap::sync_start(b.uv_fd, to_dmaheap(d));
}

void sync_end(const Buffer& b, SyncDir d) {
    if (b.y_fd >= 0)
        dmaheap::sync_end(b.y_fd, to_dmaheap(d));
    if (b.uv_fd >= 0 && b.uv_fd != b.y_fd)
        dmaheap::sync_end(b.uv_fd, to_dmaheap(d));
}

} // namespace nv12_buf
