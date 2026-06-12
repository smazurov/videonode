#include "src/render/nv12_buf.hpp"

#include <gbm.h>
#include <gtest/gtest.h>

#include <algorithm>
#include <cstdint>
#include <fcntl.h>
#include <sys/mman.h>
#include <unistd.h>

namespace {

constexpr uint8_t kU = 0x11;
constexpr uint8_t kV = 0x22;

} // namespace

TEST(Nv12Stage, StagedUvPreservesByteOrder) {
    int drm_fd = ::open("/dev/dri/renderD128", O_RDWR | O_CLOEXEC);
    if (drm_fd < 0)
        GTEST_SKIP() << "No DRM render node";
    struct gbm_device* gbm = gbm_create_device(drm_fd);
    if (!gbm) {
        ::close(drm_fd);
        GTEST_SKIP() << "gbm_create_device failed";
    }

    {
        nv12_buf::Allocator a;
        ASSERT_TRUE(a.init(gbm));
        nv12_buf::Buffer b = a.alloc(64, 64);
        ASSERT_TRUE(b.valid());

        auto m = nv12_buf::map_rw(b);
        ASSERT_NE(m.y, nullptr);
        auto y = m.y_bytes();
        std::fill(y.begin(), y.end(), uint8_t{0x55});
        auto uv = m.uv_bytes();
        for (size_t i = 0; i + 1 < uv.size(); i += 2) {
            uv[i] = kU;
            uv[i + 1] = kV;
        }
        nv12_buf::unmap(b);

        nv12_buf::stage_for_read(b);
        if (b.staged_uv_fd < 0)
            GTEST_SKIP() << "backend does not stage";

        const size_t len = size_t(b.height) / 2 * b.uv_pitch;
        void* p = ::mmap(nullptr, len, PROT_READ, MAP_SHARED, b.staged_uv_fd, 0);
        ASSERT_NE(p, MAP_FAILED);
        std::span<const uint8_t> staged{static_cast<const uint8_t*>(p), len};
        EXPECT_EQ(staged[0], kU) << "UV byte order changed during staging";
        EXPECT_EQ(staged[1], kV) << "UV byte order changed during staging";
        ::munmap(p, len);
    }

    gbm_device_destroy(gbm);
    ::close(drm_fd);
}
