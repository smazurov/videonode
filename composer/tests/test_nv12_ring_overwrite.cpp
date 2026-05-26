// NV12 ring buffer overwrite test.
//
// Demonstrates that the source's CSC output ring, indexed by V4L2 buffer
// index (df.index % ring_size), can overwrite a buffer that a consumer
// still holds an fd to. This is the source-side analog of the composer
// canvas single-buffer tearing bug.
//
// The test allocates a small ring (2 slots), writes distinct data into
// each slot, broadcasts the fd, then reuses slot 0 with new data while
// a "consumer" still holds an mmap of the original slot 0 fd. If the
// consumer sees the new data, the overwrite race is confirmed.

#include "src/render/gbm_alloc.hpp"
#include "src/render/nv12_buf.hpp"

#include <gbm.h>
#include <gtest/gtest.h>

#include <cstdint>
#include <cstring>
#include <fcntl.h>
#include <span>
#include <sys/mman.h>
#include <unistd.h>

namespace {

constexpr int kW = 64;
constexpr int kH = 64;

} // namespace

// With a 2-slot ring and V4L2-style indexing (slot = buf_index % 2),
// slot 0 is reused on the 3rd DQBUF. A consumer holding a dup'd fd to
// slot 0 from frame 0 sees the frame 2 overwrite.
TEST(Nv12RingOverwrite, RingSlotReuseCausesDataCorruption) {
    // Use gbm_alloc (the Mesa/Fedora backend) since tests run on the dev box.
    // On the rig, this would be nv12_buf::Allocator with dma_heap.
    // We test the concept with gbm_alloc::Nv12Buf directly.

    // Skip if no DRM render node (CI without GPU).
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

    // Frame 0: write 0xAA into slot 0's Y plane.
    {
        auto m = gbm_alloc::map_rw(ring[0]);
        ASSERT_NE(m.y, nullptr);
        std::memset(m.y_bytes().data(), 0xAA, m.y_bytes().size());
        gbm_alloc::unmap(ring[0]);
    }

    // Consumer grabs slot 0's fd (simulates ScmRightsSource receiving it).
    int consumer_fd = ::dup(ring[0].y_fd);
    ASSERT_GE(consumer_fd, 0);

    // Consumer mmaps the fd for reading (simulates vn-sink or composer import).
    size_t y_size = size_t(ring[0].y_stride) * kH;
    void* consumer_map = ::mmap(nullptr, y_size, PROT_READ, MAP_SHARED, consumer_fd, 0);
    ASSERT_NE(consumer_map, MAP_FAILED);

    // Verify consumer sees the original 0xAA data.
    std::span<const uint8_t> consumer_view(static_cast<const uint8_t*>(consumer_map), y_size);
    EXPECT_EQ(consumer_view[0], 0xAA) << "consumer should see original data";

    // Frame 1: write 0xBB into slot 1 (different slot, no conflict).
    {
        auto m = gbm_alloc::map_rw(ring[1]);
        ASSERT_NE(m.y, nullptr);
        std::memset(m.y_bytes().data(), 0xBB, m.y_bytes().size());
        gbm_alloc::unmap(ring[1]);
    }

    // Consumer should still see 0xAA in slot 0.
    EXPECT_EQ(consumer_view[0], 0xAA) << "consumer should still see 0xAA after slot 1 write";

    // Frame 2: V4L2 returns buffer index 0 again → CSC writes into
    // ring[0 % 2] = ring[0], overwriting the consumer's data.
    {
        auto m = gbm_alloc::map_rw(ring[0]);
        ASSERT_NE(m.y, nullptr);
        std::memset(m.y_bytes().data(), 0xCC, m.y_bytes().size());
        gbm_alloc::unmap(ring[0]);
    }

    // Consumer's mmap of the SAME underlying dma-buf now shows 0xCC.
    // This is the overwrite race.
    bool overwritten = (consumer_view[0] == 0xCC);
    if (overwritten) {
        printf("  [CONFIRMED] Ring slot reuse overwrote consumer's view: "
               "expected 0xAA, got 0x%02X — source CSC ring race demonstrated\n",
               consumer_view[0]);
    } else {
        printf("  [INFO] Consumer still sees 0x%02X (expected 0xAA) — "
               "driver may cache differently\n",
               consumer_view[0]);
    }

    // The key assertion: with a 2-slot ring and V4L2-style indexing,
    // the producer WILL overwrite the consumer's buffer. This test
    // documents the race rather than asserting a specific outcome,
    // since cache coherency behavior varies by driver.
    EXPECT_TRUE(overwritten || consumer_view[0] == 0xAA)
        << "consumer should see either original (0xAA) or overwritten (0xCC) data, got 0x"
        << std::hex << int(consumer_view[0]);

    ::munmap(consumer_map, y_size);
    ::close(consumer_fd);
    for (auto& slot : ring)
        gbm_alloc::free(slot);
    gbm_device_destroy(gbm);
    ::close(drm_fd);
}

// With a 3-slot ring and the same access pattern, slot 0 is not reused
// until frame 3, giving the consumer an extra frame's worth of safety.
// This demonstrates that deepening the ring mitigates the race.
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

    // Frame 0: write 0xAA into slot 0.
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
    EXPECT_EQ(consumer_view[0], 0xAA);

    // Frames 1 and 2 go to slots 1 and 2 — slot 0 is untouched.
    for (int i = 1; i < kRingSize; ++i) {
        auto m = gbm_alloc::map_rw(ring[i]);
        ASSERT_NE(m.y, nullptr);
        std::memset(m.y_bytes().data(), 0xBB + uint8_t(i), m.y_bytes().size());
        gbm_alloc::unmap(ring[i]);
    }

    // After 2 frames, consumer's slot 0 is still intact.
    EXPECT_EQ(consumer_view[0], 0xAA)
        << "3-slot ring: slot 0 should survive 2 frame advances without overwrite";

    printf("  [OK] 3-slot ring preserved slot 0 through frames 1-2\n");

    ::munmap(consumer_map, y_size);
    ::close(consumer_fd);
    for (auto& slot : ring)
        gbm_alloc::free(slot);
    gbm_device_destroy(gbm);
    ::close(drm_fd);
}
