#include "src/render/nv12_buf.hpp"

#include "src/common/log_levels.hpp"
#include "src/ipc/dma_heap.hpp"

#include <cstring>
#include <linux/memfd.h>
#include <memory>
#include <sys/mman.h>
#include <sys/syscall.h>
#include <unistd.h>
#include <utility>

#if defined(HAVE_GBM) && !defined(HAVE_RGA)
#include "src/render/gbm_alloc.hpp"
#include <gbm.h>
#endif

namespace nv12_buf {

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
    // Double-buffered staging: two memfd pairs per ring slot. stage_for_read
    // flips between them so the consumer from the previous call still has
    // valid pages while the producer writes the new snapshot into the other.
    static constexpr int kStageBufs = 2;
    int stage_idx = 0;
    int staged_y_fd[kStageBufs] = {-1, -1};
    int staged_uv_fd[kStageBufs] = {-1, -1};
    void* staged_y_map[kStageBufs] = {};
    void* staged_uv_map[kStageBufs] = {};
    size_t staged_y_size = 0;
    size_t staged_uv_size = 0;
};
#else
struct DmaImpl {
    dmaheap::Buffer bo;
    void* mapped = nullptr;
};
#endif

} // namespace

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
    staged_y_fd = o.staged_y_fd;
    staged_uv_fd = o.staged_uv_fd;
    impl = o.impl;
    o.y_fd = o.uv_fd = -1;
    o.staged_y_fd = o.staged_uv_fd = -1;
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
    for (int i = 0; i < GbmImpl::kStageBufs; ++i) {
        if (g->staged_y_map[i])
            ::munmap(g->staged_y_map[i], g->staged_y_size);
        if (g->staged_uv_map[i])
            ::munmap(g->staged_uv_map[i], g->staged_uv_size);
        if (g->staged_y_fd[i] >= 0)
            ::close(g->staged_y_fd[i]);
        if (g->staged_uv_fd[i] >= 0)
            ::close(g->staged_uv_fd[i]);
    }
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
        vn::log::error("nv12_buf::map_rw: lseek(%s fd=%d) failed: %s", tag, fd,
                       std::strerror(errno));
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
    // "system" (cached) — consumers (vn-sink) mmap and CPU-read every
    // frame; uncached pages would serialize every load through DRAM.
    // RGA writes are coherent via DMA_BUF_IOCTL_SYNC.
    impl->bo = dmaheap::alloc(dmaheap::kHeapSystem, sz);
    if (!impl->bo.valid())
        impl->bo = dmaheap::alloc(dmaheap::kHeapUncached, sz);
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

#if defined(HAVE_GBM) && !defined(HAVE_RGA)

namespace {
int create_memfd_(const char* name, size_t size) {
    int fd = static_cast<int>(::syscall(SYS_memfd_create, name, MFD_CLOEXEC));
    if (fd < 0)
        return -1;
    if (::ftruncate(fd, static_cast<off_t>(size)) < 0) {
        ::close(fd);
        return -1;
    }
    return fd;
}
} // namespace

void stage_for_read(Buffer& b) {
    if (!b.valid() || !b.impl)
        return;
    auto* g = static_cast<GbmImpl*>(b.impl);
    const size_t y_bytes = size_t(b.height) * b.y_pitch;
    const size_t uv_bytes = size_t(b.height) / 2 * b.uv_pitch;

    // Flip to the next staging slot. Consumers holding dup'd fds from
    // the previous call read from the other slot's pages — no overwrite.
    int si = g->stage_idx;
    g->stage_idx = (si + 1) % GbmImpl::kStageBufs;

    if (g->staged_y_fd[si] < 0) {
        g->staged_y_fd[si] = create_memfd_("nv12-y", y_bytes);
        g->staged_uv_fd[si] = create_memfd_("nv12-uv", uv_bytes);
        if (g->staged_y_fd[si] < 0 || g->staged_uv_fd[si] < 0) {
            vn::log::error("nv12_buf::stage_for_read: memfd_create failed");
            if (g->staged_y_fd[si] >= 0) {
                ::close(g->staged_y_fd[si]);
                g->staged_y_fd[si] = -1;
            }
            if (g->staged_uv_fd[si] >= 0) {
                ::close(g->staged_uv_fd[si]);
                g->staged_uv_fd[si] = -1;
            }
            return;
        }
        g->staged_y_size = y_bytes;
        g->staged_uv_size = uv_bytes;
        g->staged_y_map[si] =
            ::mmap(nullptr, y_bytes, PROT_READ | PROT_WRITE, MAP_SHARED, g->staged_y_fd[si], 0);
        g->staged_uv_map[si] =
            ::mmap(nullptr, uv_bytes, PROT_READ | PROT_WRITE, MAP_SHARED, g->staged_uv_fd[si], 0);
        if (g->staged_y_map[si] == MAP_FAILED || g->staged_uv_map[si] == MAP_FAILED) {
            vn::log::error("nv12_buf::stage_for_read: mmap failed");
            g->staged_y_map[si] = nullptr;
            g->staged_uv_map[si] = nullptr;
            ::close(g->staged_y_fd[si]);
            g->staged_y_fd[si] = -1;
            ::close(g->staged_uv_fd[si]);
            g->staged_uv_fd[si] = -1;
            return;
        }
    }

    {
        std::lock_guard<std::mutex> lock(gbm_alloc::gbm_device_mu());
        uint32_t stride = 0;
        void* y_map_data = nullptr;
        void* y_ptr = gbm_bo_map(g->nv.y_bo, 0, 0, b.width, b.height, GBM_BO_TRANSFER_READ, &stride,
                                 &y_map_data);
        if (y_ptr) {
            std::memcpy(g->staged_y_map[si], y_ptr, y_bytes);
            gbm_bo_unmap(g->nv.y_bo, y_map_data);
        }

        void* uv_map_data = nullptr;
        void* uv_ptr = gbm_bo_map(g->nv.uv_bo, 0, 0, b.width / 2, b.height / 2,
                                  GBM_BO_TRANSFER_READ, &stride, &uv_map_data);
        if (uv_ptr) {
            std::memcpy(g->staged_uv_map[si], uv_ptr, uv_bytes);
            gbm_bo_unmap(g->nv.uv_bo, uv_map_data);
        }
    }

    b.staged_y_fd = g->staged_y_fd[si];
    b.staged_uv_fd = g->staged_uv_fd[si];
}

#else

void stage_for_read(Buffer&) {}

#endif

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
