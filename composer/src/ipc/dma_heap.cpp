#include "src/ipc/dma_heap.hpp"

#include <cerrno>
#include <cstdio>
#include <cstring>
#include <fcntl.h>
#include <linux/dma-buf.h>
#include <linux/dma-heap.h>
#include <string>
#include <sys/ioctl.h>
#include <sys/mman.h>
#include <unistd.h>

namespace dmaheap {

namespace {

int sync_flags_for(SyncDir d) {
    switch (d) {
    case SyncDir::Read:
        return DMA_BUF_SYNC_READ;
    case SyncDir::Write:
        return DMA_BUF_SYNC_WRITE;
    case SyncDir::ReadWrite:
        return DMA_BUF_SYNC_RW;
    }
    return DMA_BUF_SYNC_RW;
}

} // namespace

Buffer::~Buffer() {
    if (fd >= 0)
        ::close(fd);
}

Buffer& Buffer::operator=(Buffer&& other) noexcept {
    if (this != &other) {
        if (fd >= 0)
            ::close(fd);
        fd = other.fd;
        size = other.size;
        other.fd = -1;
        other.size = 0;
    }
    return *this;
}

Buffer alloc(std::string_view heap_name, size_t size) {
    std::string path = "/dev/dma_heap/";
    path.append(heap_name);

    int heap_fd = ::open(path.c_str(), O_RDWR | O_CLOEXEC);
    if (heap_fd < 0 && errno == ENOENT && heap_name != "system") {
        // Host kernels (mainline x86) only expose /dev/dma_heap/system; the
        // -uncached / -reserved variants are Rockchip/Android extensions.
        // Fall back to "system" so the binary runs on plain Fedora/Debian.
        static bool warned = false;
        if (!warned) {
            fprintf(stderr, "dmaheap: %s not present, falling back to /dev/dma_heap/system\n",
                    path.c_str());
            warned = true;
        }
        path = "/dev/dma_heap/system";
        heap_fd = ::open(path.c_str(), O_RDWR | O_CLOEXEC);
    }
    if (heap_fd < 0) {
        fprintf(stderr, "dmaheap: open(%s): %s\n", path.c_str(), strerror(errno));
        return {};
    }

    dma_heap_allocation_data req{};
    req.len = size;
    req.fd_flags = O_RDWR | O_CLOEXEC;
    req.heap_flags = 0;
    if (::ioctl(heap_fd, DMA_HEAP_IOCTL_ALLOC, &req) < 0) {
        fprintf(stderr, "dmaheap: ALLOC %s size=%zu: %s\n", path.c_str(), size, strerror(errno));
        ::close(heap_fd);
        return {};
    }
    ::close(heap_fd);

    Buffer out;
    out.fd = static_cast<int>(req.fd);
    out.size = size;
    return out;
}

void* mmap_rw(const Buffer& buf) {
    if (!buf.valid())
        return nullptr;
    void* p = ::mmap(nullptr, buf.size, PROT_READ | PROT_WRITE, MAP_SHARED, buf.fd, 0);
    if (p == MAP_FAILED) {
        fprintf(stderr, "dmaheap: mmap fd=%d size=%zu: %s\n", buf.fd, buf.size, strerror(errno));
        return nullptr;
    }
    return p;
}

void munmap_rw(void* ptr, size_t size) {
    if (!ptr)
        return;
    ::munmap(ptr, size);
}

void sync_start(int fd, SyncDir d) {
    dma_buf_sync s{};
    s.flags = DMA_BUF_SYNC_START | sync_flags_for(d);
    if (::ioctl(fd, DMA_BUF_IOCTL_SYNC, &s) < 0 && errno != ENOTTY) {
        fprintf(stderr, "dmaheap: SYNC_START fd=%d: %s\n", fd, strerror(errno));
    }
}

void sync_end(int fd, SyncDir d) {
    dma_buf_sync s{};
    s.flags = DMA_BUF_SYNC_END | sync_flags_for(d);
    if (::ioctl(fd, DMA_BUF_IOCTL_SYNC, &s) < 0 && errno != ENOTTY) {
        fprintf(stderr, "dmaheap: SYNC_END fd=%d: %s\n", fd, strerror(errno));
    }
}

} // namespace dmaheap
