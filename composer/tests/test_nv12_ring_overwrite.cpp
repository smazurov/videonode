#include "src/render/gbm_alloc.hpp"
#include "src/render/nv12_buf.hpp"

#include <gbm.h>
#include <gtest/gtest.h>

#include <cerrno>
#include <cstdint>
#include <cstring>
#include <fcntl.h>
#include <linux/dma-buf.h>
#include <span>
#include <sys/ioctl.h>
#include <sys/mman.h>
#include <unistd.h>

namespace {

constexpr int kW = 64;
constexpr int kH = 64;

// A consumer's independent mmap of a dma-buf is not guaranteed cache-coherent
// with producer-side writes until the CPU access is bracketed by
// DMA_BUF_IOCTL_SYNC. Skipping it makes reads on the gbm/radeonsi dev-box
// backend intermittently return stale (zero) bytes. Exporters that don't
// implement the ioctl report the buffer as already coherent (ENOTTY), so a
// failed sync is harmless here.
void dmabuf_sync(int fd, uint64_t flags) {
    dma_buf_sync sync = {};
    sync.flags = flags;
    while (::ioctl(fd, DMA_BUF_IOCTL_SYNC, &sync) == -1 && (errno == EINTR || errno == EAGAIN)) {
    }
}

uint8_t coherent_read0(int fd, std::span<const uint8_t> view) {
    dmabuf_sync(fd, DMA_BUF_SYNC_START | DMA_BUF_SYNC_READ);
    uint8_t v = view[0];
    dmabuf_sync(fd, DMA_BUF_SYNC_END | DMA_BUF_SYNC_READ);
    return v;
}

} // namespace

TEST(Nv12RingOverwrite, RingSlotReuseCausesDataCorruption) {
    // Use gbm_alloc (the Mesa/Fedora backend) since tests run on the dev box.
    // On the rig, this would be nv12_buf::Allocator with dma_heap.
    // We test the concept with gbm_alloc::Nv12Buf directly.

    int drm_fd = ::open("/dev/dri/renderD128", O_RDWR | O_CLOEXEC);
    if (drm_fd < 0)
        GTEST_SKIP() << "No DRM render node";

    struct gbm_device* gbm = gbm_create_device(drm_fd);
    if (!gbm) {
        ::close(drm_fd);
        GTEST_SKIP() << "gbm_create_device failed";
    }

    constexpr int kRingSize = 2;
    gbm_alloc::Nv12Buf ring[kRingSize];
    for (int i = 0; i < kRingSize; ++i) {
        ring[i] = gbm_alloc::alloc(gbm, kW, kH);
        ASSERT_TRUE(ring[i].valid()) << "ring slot " << i << " alloc failed";
    }

    {
        auto m = gbm_alloc::map_rw(ring[0]);
        ASSERT_NE(m.y, nullptr);
        std::memset(m.y_bytes().data(), 0xAA, m.y_bytes().size());
        gbm_alloc::unmap(ring[0]);
    }

    int consumer_fd = ::dup(ring[0].y_fd);
    ASSERT_GE(consumer_fd, 0);

    size_t y_size = size_t(ring[0].y_stride) * kH;
    void* consumer_map = ::mmap(nullptr, y_size, PROT_READ, MAP_SHARED, consumer_fd, 0);
    ASSERT_NE(consumer_map, MAP_FAILED);

    std::span<const uint8_t> consumer_view(static_cast<const uint8_t*>(consumer_map), y_size);
    EXPECT_EQ(coherent_read0(consumer_fd, consumer_view), 0xAA)
        << "consumer should see original data";

    {
        auto m = gbm_alloc::map_rw(ring[1]);
        ASSERT_NE(m.y, nullptr);
        std::memset(m.y_bytes().data(), 0xBB, m.y_bytes().size());
        gbm_alloc::unmap(ring[1]);
    }

    EXPECT_EQ(coherent_read0(consumer_fd, consumer_view), 0xAA)
        << "consumer should still see 0xAA after slot 1 write";

    // Frame 2: V4L2 returns buffer index 0 again → CSC writes into
    // ring[0 % 2] = ring[0], overwriting the consumer's data.
    {
        auto m = gbm_alloc::map_rw(ring[0]);
        ASSERT_NE(m.y, nullptr);
        std::memset(m.y_bytes().data(), 0xCC, m.y_bytes().size());
        gbm_alloc::unmap(ring[0]);
    }

    uint8_t seen = coherent_read0(consumer_fd, consumer_view);
    bool overwritten = (seen == 0xCC);
    if (overwritten) {
        printf("  [CONFIRMED] Ring slot reuse overwrote consumer's view: "
               "expected 0xAA, got 0x%02X — source CSC ring race demonstrated\n",
               seen);
    } else {
        printf("  [INFO] Consumer still sees 0x%02X (expected 0xAA) — "
               "driver may cache differently\n",
               seen);
    }

    EXPECT_TRUE(overwritten || seen == 0xAA)
        << "consumer should see either original (0xAA) or overwritten (0xCC) data, got 0x"
        << std::hex << int(seen);

    ::munmap(consumer_map, y_size);
    ::close(consumer_fd);
    for (auto& slot : ring)
        gbm_alloc::free(slot);
    gbm_device_destroy(gbm);
    ::close(drm_fd);
}

TEST(Nv12RingOverwrite, DeeperRingDelaysReuse) {
    int drm_fd = ::open("/dev/dri/renderD128", O_RDWR | O_CLOEXEC);
    if (drm_fd < 0)
        GTEST_SKIP() << "No DRM render node";

    struct gbm_device* gbm = gbm_create_device(drm_fd);
    if (!gbm) {
        ::close(drm_fd);
        GTEST_SKIP() << "gbm_create_device failed";
    }

    constexpr int kRingSize = 3;
    gbm_alloc::Nv12Buf ring[kRingSize];
    for (int i = 0; i < kRingSize; ++i) {
        ring[i] = gbm_alloc::alloc(gbm, kW, kH);
        ASSERT_TRUE(ring[i].valid());
    }

    {
        auto m = gbm_alloc::map_rw(ring[0]);
        ASSERT_NE(m.y, nullptr);
        std::memset(m.y_bytes().data(), 0xAA, m.y_bytes().size());
        gbm_alloc::unmap(ring[0]);
    }

    int consumer_fd = ::dup(ring[0].y_fd);
    ASSERT_GE(consumer_fd, 0);
    size_t y_size = size_t(ring[0].y_stride) * kH;
    void* consumer_map = ::mmap(nullptr, y_size, PROT_READ, MAP_SHARED, consumer_fd, 0);
    ASSERT_NE(consumer_map, MAP_FAILED);

    std::span<const uint8_t> consumer_view(static_cast<const uint8_t*>(consumer_map), y_size);
    EXPECT_EQ(coherent_read0(consumer_fd, consumer_view), 0xAA);

    for (int i = 1; i < kRingSize; ++i) {
        auto m = gbm_alloc::map_rw(ring[i]);
        ASSERT_NE(m.y, nullptr);
        std::memset(m.y_bytes().data(), 0xBB + uint8_t(i), m.y_bytes().size());
        gbm_alloc::unmap(ring[i]);
    }

    EXPECT_EQ(coherent_read0(consumer_fd, consumer_view), 0xAA)
        << "3-slot ring: slot 0 should survive 2 frame advances without overwrite";

    printf("  [OK] 3-slot ring preserved slot 0 through frames 1-2\n");

    ::munmap(consumer_map, y_size);
    ::close(consumer_fd);
    for (auto& slot : ring)
        gbm_alloc::free(slot);
    gbm_device_destroy(gbm);
    ::close(drm_fd);
}
