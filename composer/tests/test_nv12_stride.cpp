#include "src/render/nv12_buf.hpp"

#include <fcntl.h>
#include <unistd.h>

#include <gtest/gtest.h>

namespace {

bool dma_heap_present() {
    int fd = ::open("/dev/dma_heap/system", O_RDONLY | O_CLOEXEC);
    if (fd < 0)
        return false;
    ::close(fd);
    return true;
}

} // namespace

// Reverting the alloc() padding (stride = width instead of aligned_stride)
// makes these fail: a 1360-wide source is 16- but not 64-aligned and trips
// Panfrost's "WSI pitch not properly aligned" on import in the composer.
TEST(Nv12Alloc, PadsRowPitchTo64) {
    if (!dma_heap_present())
        GTEST_SKIP() << "no /dev/dma_heap";
    nv12_buf::Allocator a;
    ASSERT_TRUE(a.init());
    nv12_buf::Buffer b = a.alloc(1360, 768);
    ASSERT_TRUE(b.valid());
    EXPECT_EQ(b.y_pitch, nv12_buf::aligned_stride(1360)); // 1408
    EXPECT_EQ(b.y_pitch % nv12_buf::kRowAlign, 0u);
    EXPECT_EQ(b.uv_pitch, b.y_pitch);
    EXPECT_EQ(b.uv_offset, b.y_pitch * 768u);
}

TEST(Nv12Alloc, AlreadyAlignedUnchanged) {
    if (!dma_heap_present())
        GTEST_SKIP() << "no /dev/dma_heap";
    nv12_buf::Allocator a;
    ASSERT_TRUE(a.init());
    nv12_buf::Buffer b = a.alloc(1920, 1080);
    ASSERT_TRUE(b.valid());
    EXPECT_EQ(b.y_pitch, 1920u);
    EXPECT_EQ(b.uv_offset, 1920u * 1080u);
}
