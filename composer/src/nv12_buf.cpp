#include "nv12_buf.hpp"

#include "dma_heap.hpp"

#include <cstdio>
#include <cstring>
#include <sys/mman.h>
#include <unistd.h>
#include <utility>

#if defined(HAVE_GBM) && !defined(HAVE_RGA)
#include "gbm_alloc.hpp"
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
// gbm backend: impl is a pointer to a heap-allocated gbm_alloc::Nv12Buf.
struct GbmImpl {
    gbm_alloc::Nv12Buf nv;
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
    auto* g = static_cast<GbmImpl*>(impl);
    gbm_alloc::free(g->nv);
    delete g;
#else
    auto* d = static_cast<DmaImpl*>(impl);
    if (d->mapped) {
        dmaheap::munmap_rw(d->mapped, d->bo.size);
        d->mapped = nullptr;
    }
    // dmaheap::Buffer's destructor closes its fd. Setting y_fd/uv_fd above
    // to -1 (via the move-from path) means we don't double-close.
    delete d;
#endif
    impl = nullptr;
}

// ── Allocator ──────────────────────────────────────────────────────────

Allocator::~Allocator() = default;

bool Allocator::init(gbm_device* gbm) {
#if defined(HAVE_GBM) && !defined(HAVE_RGA)
    if (!gbm) {
        std::fprintf(stderr, "nv12_buf: gbm backend requires a gbm_device\n");
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
        std::fprintf(stderr, "nv12_buf::alloc: gbm backend not initialized\n");
        return out;
    }
    auto* impl = new GbmImpl();
    impl->nv = gbm_alloc::alloc(gbm_, width, height);
    if (!impl->nv.valid()) {
        delete impl;
        return out;
    }
    out.y_fd = impl->nv.y_fd;
    out.uv_fd = impl->nv.uv_fd;
    out.y_offset = 0;
    out.uv_offset = 0;
    out.y_pitch = impl->nv.y_stride;
    out.uv_pitch = impl->nv.uv_stride;
    out.width = width;
    out.height = height;
    out.impl = impl;
    return out;
}

Mapping map_rw(Buffer& b) {
    Mapping out;
    if (!b.valid() || !b.impl)
        return out;
    auto* impl = static_cast<GbmImpl*>(b.impl);
    auto m = gbm_alloc::map_rw(impl->nv);
    out.y = m.y;
    out.uv = m.uv;
    out.height = b.height;
    out.y_pitch = b.y_pitch;
    out.uv_pitch = b.uv_pitch;
    return out;
}

void unmap(Buffer& b) {
    if (!b.valid() || !b.impl)
        return;
    auto* impl = static_cast<GbmImpl*>(b.impl);
    gbm_alloc::unmap(impl->nv);
}

#else // dma_heap backend (rig + RGA)

Buffer Allocator::alloc(int width, int height) {
    Buffer out;
    if (width <= 0 || height <= 0 || (width & 1) || (height & 1))
        return out;
    auto* impl = new DmaImpl();
    const size_t sz = size_t(width) * height * 3 / 2;
    // Try "system-uncached" first (RK3588 prefers it for output buffers
    // RGA writes to without CPU readback); fall back to plain "system".
    impl->bo = dmaheap::alloc(dmaheap::kHeapUncached, sz);
    if (!impl->bo.valid())
        impl->bo = dmaheap::alloc(dmaheap::kHeapSystem, sz);
    if (!impl->bo.valid()) {
        delete impl;
        return out;
    }
    out.y_fd = impl->bo.fd;
    out.uv_fd = impl->bo.fd; // same fd, different offsets
    out.y_offset = 0;
    out.uv_offset = uint32_t(width) * uint32_t(height);
    out.y_pitch = uint32_t(width);
    out.uv_pitch = uint32_t(width);
    out.width = width;
    out.height = height;
    out.impl = impl;
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
