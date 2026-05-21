// dma_heap — userspace allocator for DMA-BUF backing memory via /dev/dma_heap/.
//
// Why this exists: every dma-buf the composer passes between subsystems (GBM
// canvas, RGA scratch, vision tap, ffmpeg encoder input) ultimately needs a
// kernel-side allocation. dma_heap is the modern (post-ION) interface. On
// this rig /dev/dma_heap/system is the cacheable virtually-contiguous heap;
// system-uncached is the same with caching off; "reserved" is the
// physically-contiguous heap (typically CMA-backed).
//
// Allocations return a DMA-BUF fd that can be:
//   - mmap'd into the composer process for CPU read/write (fake_source uses this)
//   - imported by librga via importbuffer_fd
//   - imported by EGL via EGL_LINUX_DMA_BUF_EXT
//   - passed to ffmpeg over SCM_RIGHTS
//
// Out-of-scope here: stride/alignment logic specific to a pixel format.
// Callers compute layout (e.g., NV12 = w*h*3/2) and pass total bytes.

#pragma once

#include <cstddef>
#include <cstdint>
#include <string>
#include <string_view>

namespace dmaheap {

// Heap names available on Armbian vendor 6.1 rk35xx:
//   "system"          — cacheable, virtually-contiguous (default; works for RGA/GBM/MPP)
//   "system-uncached" — same allocation, caching disabled (use when CPU access is rare)
//   "reserved"        — physically-contiguous (use for hardware that needs phys-contig)
inline constexpr std::string_view kHeapSystem = "system";
inline constexpr std::string_view kHeapUncached = "system-uncached";
inline constexpr std::string_view kHeapReserved = "reserved";

struct Buffer {
    int fd = -1;
    size_t size = 0;

    Buffer() = default;
    Buffer(const Buffer&) = delete;
    Buffer& operator=(const Buffer&) = delete;
    Buffer(Buffer&& other) noexcept : fd(other.fd), size(other.size) {
        other.fd = -1;
        other.size = 0;
    }
    Buffer& operator=(Buffer&& other) noexcept;
    ~Buffer();

    bool valid() const { return fd >= 0; }
};

// Allocate `size` bytes from the named heap. Returns an invalid Buffer on
// failure (caller can check .valid() or rely on errno being set).
Buffer alloc(std::string_view heap_name, size_t size);

// mmap the buffer for CPU read+write. Returns nullptr on failure.
// The mapping is alive until unmap() or the Buffer is destroyed (via fd close).
// On cacheable heaps the kernel may require dma-buf-sync ioctls around each
// CPU access burst for correctness on some platforms; on RK3588 the BSP
// kernel treats CPU mmap of the system heap as coherent, but we sync anyway
// before-read / after-write for portability and to dodge subtle data races.
void* mmap_rw(const Buffer& buf);
void munmap_rw(void* ptr, size_t size);

// dma-buf-sync ioctls — call before CPU read or after CPU write to ensure
// caches are coherent with prior/next device access. No-op on UNCACHED heaps
// but cheap and correct everywhere.
enum class SyncDir { Read, Write, ReadWrite };
void sync_start(int fd, SyncDir d);
void sync_end(int fd, SyncDir d);

} // namespace dmaheap
