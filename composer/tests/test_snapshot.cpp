#include "src/snapshot/snapshot.hpp"

#include <gtest/gtest.h>

#include <atomic>
#include <cerrno>
#include <chrono>
#include <cstdint>
#include <cstring>
#include <span>
#include <sys/mman.h>
#include <sys/syscall.h>
#include <thread>
#include <unistd.h>
#include <vector>

namespace {

// Wrap memfd_create directly so we don't depend on a glibc that may or may
// not surface a wrapper on the test host.
int memfd(const char* name) {
    return static_cast<int>(::syscall(SYS_memfd_create, name, 0u));
} // namespace

int make_memfd_with(std::span<const uint8_t> contents) {
    int fd = memfd("vn_snapshot_test");
    if (fd < 0)
        return -1;
    if (::ftruncate(fd, static_cast<off_t>(contents.size())) != 0) {
        ::close(fd);
        return -1;
    }
    ssize_t written = ::write(fd, contents.data(), contents.size());
    if (written != static_cast<ssize_t>(contents.size())) {
        ::close(fd);
        return -1;
    }
    return fd;
} // namespace

// Fill `out` (sized rows*pitch) with a deterministic pattern; only the
// first `row_bytes` of each row carry meaningful data, the trailing
// pitch-row_bytes bytes are 0xAA (padding marker, must not survive packing).
void fill_padded_(std::vector<uint8_t>& out, size_t rows, size_t row_bytes, size_t pitch,
                  uint8_t seed) {
    out.assign(rows * pitch, 0xAA);
    for (size_t r = 0; r < rows; ++r) {
        for (size_t c = 0; c < row_bytes; ++c) {
            out[r * pitch + c] = static_cast<uint8_t>(seed + r * 7 + c * 13);
        }
    }
} // namespace

std::vector<uint8_t> packed_expected_(size_t rows, size_t row_bytes, uint8_t seed) {
    std::vector<uint8_t> v(rows * row_bytes);
    for (size_t r = 0; r < rows; ++r)
        for (size_t c = 0; c < row_bytes; ++c)
            v[r * row_bytes + c] = static_cast<uint8_t>(seed + r * 7 + c * 13);
    return v;
} // namespace

void* failing_mmap(int /*fd*/, size_t /*length*/, off_t /*offset*/) {
    return MAP_FAILED;
} // namespace

} // namespace

TEST(MmapAndPack, PitchEqualsWidth) {
    std::vector<uint8_t> src;
    fill_padded_(src, /*rows=*/4, /*row_bytes=*/8, /*pitch=*/8, /*seed=*/3);
    const int fd = make_memfd_with(src);
    ASSERT_GE(fd, 0);

    vn::snapshot::Plane p{.fd = fd, .offset = 0, .pitch = 8, .row_bytes = 8, .rows = 4};
    std::vector<uint8_t> dst(32);
    EXPECT_TRUE(vn::snapshot::MmapAndPack(p, dst, 0));
    EXPECT_EQ(dst, packed_expected_(4, 8, 3));
    ::close(fd);
} // namespace

TEST(MmapAndPack, PitchExceedsWidth) {
    std::vector<uint8_t> src;
    fill_padded_(src, /*rows=*/3, /*row_bytes=*/5, /*pitch=*/11, /*seed=*/9);
    const int fd = make_memfd_with(src);
    ASSERT_GE(fd, 0);

    vn::snapshot::Plane p{.fd = fd, .offset = 0, .pitch = 11, .row_bytes = 5, .rows = 3};
    std::vector<uint8_t> dst(15);
    EXPECT_TRUE(vn::snapshot::MmapAndPack(p, dst, 0));
    EXPECT_EQ(dst, packed_expected_(3, 5, 9));
    ::close(fd);
} // namespace

TEST(MmapAndPack, RejectsPitchSmallerThanRowBytes) {
    std::vector<uint8_t> src(64, 0);
    const int fd = make_memfd_with(src);
    ASSERT_GE(fd, 0);

    vn::snapshot::Plane p{.fd = fd, .offset = 0, .pitch = 4, .row_bytes = 8, .rows = 2};
    std::vector<uint8_t> dst(16, 0x55);
    EXPECT_FALSE(vn::snapshot::MmapAndPack(p, dst, 0));
    for (auto b : dst)
        EXPECT_EQ(b, 0x55);
    ::close(fd);
} // namespace

TEST(MmapAndPack, RejectsBadFd) {
    vn::snapshot::Plane p{.fd = -1, .offset = 0, .pitch = 4, .row_bytes = 4, .rows = 1};
    std::vector<uint8_t> dst(4);
    EXPECT_FALSE(vn::snapshot::MmapAndPack(p, dst, 0));
} // namespace

TEST(MmapAndPack, RejectsSmallDst) {
    std::vector<uint8_t> src(32, 0);
    const int fd = make_memfd_with(src);
    ASSERT_GE(fd, 0);

    vn::snapshot::Plane p{.fd = fd, .offset = 0, .pitch = 8, .row_bytes = 8, .rows = 4};
    std::vector<uint8_t> dst(8); // need 32, have 8
    EXPECT_FALSE(vn::snapshot::MmapAndPack(p, dst, 0));
    ::close(fd);
} // namespace

TEST(MmapAndPack, OffsetRespected) {
    // Lay out: 24 bytes pre-plane padding, then 3 rows of 5 row_bytes at
    // pitch 7. Verify only the post-offset region surfaces in dst.
    std::vector<uint8_t> src(24, 0xEE);
    std::vector<uint8_t> plane;
    fill_padded_(plane, 3, 5, 7, /*seed=*/17);
    src.insert(src.end(), plane.begin(), plane.end());

    const int fd = make_memfd_with(src);
    ASSERT_GE(fd, 0);

    vn::snapshot::Plane p{.fd = fd, .offset = 24, .pitch = 7, .row_bytes = 5, .rows = 3};
    std::vector<uint8_t> dst(15);
    EXPECT_TRUE(vn::snapshot::MmapAndPack(p, dst, 0));
    EXPECT_EQ(dst, packed_expected_(3, 5, 17));
    ::close(fd);
} // namespace

TEST(MmapAndPack, FailingMmapReturnsFalse) {
    std::vector<uint8_t> src(64, 0);
    const int fd = make_memfd_with(src);
    ASSERT_GE(fd, 0);

    vn::snapshot::Plane p{.fd = fd, .offset = 0, .pitch = 8, .row_bytes = 8, .rows = 4};
    std::vector<uint8_t> dst(32);
    EXPECT_FALSE(vn::snapshot::MmapAndPack(p, dst, 0, &failing_mmap));
    ::close(fd);
} // namespace

TEST(LatestFrameHolder, EmptyReturnsFalse) {
    vn::snapshot::LatestFrameHolder h;
    vn::snapshot::FrameBytes out;
    EXPECT_FALSE(h.Snapshot(out));
} // namespace

TEST(LatestFrameHolder, RoundTripNv12SinglePlanedFd) {
    // NV12 frame: Y plane (4x4) followed by UV plane (4x2, interleaved Cb/Cr)
    // in the same fd at offset W*H. Pitch == width so packing is bulk.
    const uint32_t W = 4, H = 4;
    const size_t y_bytes = size_t(W) * H;
    const size_t uv_bytes = size_t(W) * (H / 2);

    std::vector<uint8_t> buf(y_bytes + uv_bytes);
    for (size_t i = 0; i < y_bytes; ++i)
        buf[i] = static_cast<uint8_t>(0x10 + i);
    for (size_t i = 0; i < uv_bytes; ++i)
        buf[y_bytes + i] = static_cast<uint8_t>(0x80 + i);

    const int fd = make_memfd_with(buf);
    ASSERT_GE(fd, 0);

    vn::snapshot::FrameRef ref{};
    ref.format = vn::snapshot::Format::Nv12;
    ref.width = W;
    ref.height = H;
    ref.pitch_y = W;
    ref.pitch_uv = W;
    ref.planes[0] = {.fd = fd, .offset = 0, .pitch = W, .row_bytes = W, .rows = H};           // Y
    ref.planes[1] = {.fd = fd, .offset = y_bytes, .pitch = W, .row_bytes = W, .rows = H / 2}; // UV
    ref.frame_idx = 42;
    ref.captured_at_ns = 1'000'000;

    vn::snapshot::LatestFrameHolder h;
    h.Update(ref);

    vn::snapshot::FrameBytes out;
    ASSERT_TRUE(h.Snapshot(out));
    EXPECT_EQ(out.format, vn::snapshot::Format::Nv12);
    EXPECT_EQ(out.width, W);
    EXPECT_EQ(out.height, H);
    EXPECT_EQ(out.pitch_y, W);
    EXPECT_EQ(out.pitch_uv, W);
    EXPECT_EQ(out.frame_idx, 42u);
    EXPECT_EQ(out.captured_at_ns, 1'000'000u);
    ASSERT_EQ(out.bytes.size(), y_bytes + uv_bytes);
    EXPECT_EQ(0, std::memcmp(out.bytes.data(), buf.data(), buf.size()));
    ::close(fd);
} // namespace

TEST(LatestFrameHolder, RoundTripNv12PaddedPitch) {
    const uint32_t W = 6, H = 4;
    const size_t y_pitch = 8;
    const size_t uv_pitch = 8;
    std::vector<uint8_t> y_src;
    fill_padded_(y_src, H, W, y_pitch, /*seed=*/0x10);
    std::vector<uint8_t> uv_src;
    fill_padded_(uv_src, H / 2, W, uv_pitch, /*seed=*/0x80);

    std::vector<uint8_t> buf;
    buf.insert(buf.end(), y_src.begin(), y_src.end());
    buf.insert(buf.end(), uv_src.begin(), uv_src.end());

    const int fd = make_memfd_with(buf);
    ASSERT_GE(fd, 0);

    vn::snapshot::FrameRef ref{};
    ref.format = vn::snapshot::Format::Nv12;
    ref.width = W;
    ref.height = H;
    ref.pitch_y = static_cast<uint32_t>(y_pitch);
    ref.pitch_uv = static_cast<uint32_t>(uv_pitch);
    ref.planes[0] = {.fd = fd, .offset = 0, .pitch = y_pitch, .row_bytes = W, .rows = H};
    ref.planes[1] = {
        .fd = fd, .offset = y_src.size(), .pitch = uv_pitch, .row_bytes = W, .rows = H / 2};

    vn::snapshot::LatestFrameHolder h;
    h.Update(ref);

    vn::snapshot::FrameBytes out;
    ASSERT_TRUE(h.Snapshot(out));
    ASSERT_EQ(out.bytes.size(), size_t(W) * H + size_t(W) * (H / 2));

    auto expected_y = packed_expected_(H, W, 0x10);
    auto expected_uv = packed_expected_(H / 2, W, 0x80);
    EXPECT_EQ(0, std::memcmp(out.bytes.data(), expected_y.data(), expected_y.size()));
    EXPECT_EQ(0,
              std::memcmp(&out.bytes[expected_y.size()], expected_uv.data(), expected_uv.size()));
    ::close(fd);
} // namespace

TEST(LatestFrameHolder, UpdateOverwrites) {
    const uint32_t W = 2, H = 2;
    std::vector<uint8_t> bufA(W * H, 0xAA);
    std::vector<uint8_t> bufB(W * H, 0xBB);
    bufA.resize(W * H + W * (H / 2), 0xAA);
    bufB.resize(W * H + W * (H / 2), 0xBB);
    int fdA = make_memfd_with(bufA);
    int fdB = make_memfd_with(bufB);
    ASSERT_GE(fdA, 0);
    ASSERT_GE(fdB, 0);

    auto mk = [&](int fd, uint64_t idx) {
        vn::snapshot::FrameRef r{};
        r.format = vn::snapshot::Format::Nv12;
        r.width = W;
        r.height = H;
        r.pitch_y = W;
        r.pitch_uv = W;
        r.planes[0] = {.fd = fd, .offset = 0, .pitch = W, .row_bytes = W, .rows = H};
        r.planes[1] = {.fd = fd, .offset = W * H, .pitch = W, .row_bytes = W, .rows = H / 2};
        r.frame_idx = idx;
        return r;
    };

    vn::snapshot::LatestFrameHolder h;
    h.Update(mk(fdA, 1));
    h.Update(mk(fdB, 2));
    vn::snapshot::FrameBytes out;
    ASSERT_TRUE(h.Snapshot(out));
    EXPECT_EQ(out.frame_idx, 2u);
    EXPECT_EQ(out.bytes[0], 0xBB);
    ::close(fdA);
    ::close(fdB);
} // namespace

TEST(LatestFrameHolder, MmapFailureLeavesRefIntact) {
    std::vector<uint8_t> buf(16, 0x77);
    const int fd = make_memfd_with(buf);
    ASSERT_GE(fd, 0);

    vn::snapshot::FrameRef ref{};
    ref.format = vn::snapshot::Format::Nv12;
    ref.width = 2;
    ref.height = 2;
    ref.pitch_y = 2;
    ref.pitch_uv = 2;
    ref.planes[0] = {.fd = fd, .offset = 0, .pitch = 2, .row_bytes = 2, .rows = 2};
    ref.planes[1] = {.fd = fd, .offset = 4, .pitch = 2, .row_bytes = 2, .rows = 1};

    vn::snapshot::LatestFrameHolder h;
    h.Update(ref);
    h.SetMmapFnForTest(&failing_mmap);

    vn::snapshot::FrameBytes out;
    EXPECT_FALSE(h.Snapshot(out));

    h.SetMmapFnForTest(nullptr);
    EXPECT_TRUE(h.Snapshot(out));
    ::close(fd);
} // namespace

TEST(LatestFrameHolder, ConcurrentUpdateAndSnapshot) {
    const uint32_t W = 4, H = 2;
    std::vector<uint8_t> buf(W * H + W * (H / 2), 0);
    const int fd = make_memfd_with(buf);
    ASSERT_GE(fd, 0);

    vn::snapshot::LatestFrameHolder h;

    constexpr uint64_t N = 10'000;
    std::atomic<bool> stop{false};
    std::atomic<uint64_t> last_published{0};

    // jthread so a fatal ASSERT failure in the consumer (which returns from
    // the test body, skipping join()) auto-joins on destruction instead of
    // tripping std::terminate() — that abort otherwise masks the real
    // assertion message behind a bare SIGABRT.
    std::jthread producer([&] {
        for (uint64_t i = 1; i <= N; ++i) {
            vn::snapshot::FrameRef r{};
            r.format = vn::snapshot::Format::Nv12;
            r.width = W;
            r.height = H;
            r.pitch_y = W;
            r.pitch_uv = W;
            r.planes[0] = {.fd = fd, .offset = 0, .pitch = W, .row_bytes = W, .rows = H};
            r.planes[1] = {.fd = fd, .offset = W * H, .pitch = W, .row_bytes = W, .rows = H / 2};
            r.frame_idx = i;
            // Advance the published-counter BEFORE making the frame visible,
            // so last_published is always an upper bound on what a consumer can
            // Snapshot. Update() publishes under a mutex; a consumer that
            // observes frame i therefore also observes last_published >= i.
            last_published.store(i, std::memory_order_release);
            h.Update(r);
        }
        stop.store(true, std::memory_order_release);
    });

    uint64_t consumer_seen = 0;
    while (!stop.load(std::memory_order_acquire)) {
        vn::snapshot::FrameBytes out;
        if (!h.Snapshot(out))
            continue;
        const uint64_t pub_at_least = last_published.load(std::memory_order_acquire);
        ASSERT_GE(out.frame_idx, consumer_seen) << "frame_idx went backward";
        ASSERT_LE(out.frame_idx, pub_at_least) << "saw frame_idx producer hasn't published yet";
        consumer_seen = out.frame_idx;
    }
    producer.join();

    vn::snapshot::FrameBytes final_out;
    ASSERT_TRUE(h.Snapshot(final_out));
    EXPECT_EQ(final_out.frame_idx, N);
    ::close(fd);
} // namespace
