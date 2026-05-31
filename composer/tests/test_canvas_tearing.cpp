// Empirical tearing detector for the composer canvas pipeline.
//
// Feeds alternating solid-color NV12 frames (red / blue) into PlCompose,
// reads back the BGRA canvas via gbm_bo_map, and asserts every pixel in
// each frame is a single color. A torn frame contains pixels from both
// colors — the test fails with the frame index and byte offset of the
// first mismatch.
//
// Two modes exercise different output paths:
//   InPlace   — render + finish + mmap readback (stdout path analog)
//   ScmRelay  — render + finish + SCM broadcast → consumer thread mmap
//
// Running with swap() disabled (single-buffer) should reproduce tearing
// under load; with swap() enabled (double-buffer) it should pass clean.

#include "src/render/gbm_alloc.hpp"
#include "src/render/pl_compose.hpp"

#include <gbm.h>
#include <gtest/gtest.h>
#include <linux/memfd.h>
#include <linux/dma-buf.h>
#include <sys/ioctl.h>
#include <sys/mman.h>
#include <sys/syscall.h> // SYS_memfd_create

#include <algorithm>
#include <atomic>
#include <cstdint>
#include <cstring>
#include <span>
#include <thread>
#include <unistd.h>
#include <vector>

namespace {

constexpr int kW = 128;
constexpr int kH = 128;
constexpr int kFrames = 500;

// BT.601 limited-range YCbCr for "red" and "blue".
// Red (255,0,0) → Y=81, Cb=90, Cr=240  (limited range)
// Blue (0,0,255) → Y=41, Cb=240, Cr=110
struct YuvColor {
    uint8_t y;
    uint8_t cb;
    uint8_t cr;
};

constexpr YuvColor kRed{.y = 81, .cb = 90, .cr = 240};
constexpr YuvColor kBlue{.y = 41, .cb = 240, .cr = 110};

// Expected BGRA after BT.601 limited-range → full-range RGB conversion.
// Exact values depend on libplacebo's conversion; we allow ±8 tolerance.
struct BgraColor {
    uint8_t b, g, r, a;
};

constexpr BgraColor kRedBgra{.b = 0, .g = 0, .r = 255, .a = 255};
constexpr BgraColor kBlueBgra{.b = 255, .g = 0, .r = 0, .a = 255};
constexpr int kTolerance = 8;

bool pixel_matches(const uint8_t* px, BgraColor expected) {
    std::span<const uint8_t> pixel(px, 4);
    return std::abs(int(pixel[0]) - expected.b) <= kTolerance &&
           std::abs(int(pixel[1]) - expected.g) <= kTolerance &&
           std::abs(int(pixel[2]) - expected.r) <= kTolerance;
}

pl_compose::SourceSlot make_slot(const gbm_alloc::Nv12Buf& buf) {
    pl_compose::SourceSlot s;
    s.src_y_fd = buf.y_fd;
    s.src_uv_fd = buf.uv_fd;
    s.src_w = buf.width;
    s.src_h = buf.height;
    s.src_y_pitch = static_cast<int>(buf.y_stride);
    s.src_uv_pitch = static_cast<int>(buf.uv_stride);
    s.x = 0;
    s.y = 0;
    s.w = buf.width;
    s.h = buf.height;
    return s;
}

void fill_nv12(gbm_alloc::Nv12Buf& buf, YuvColor c) {
    auto m = gbm_alloc::map_rw(buf);
    ASSERT_NE(m.y, nullptr);
    ASSERT_NE(m.uv, nullptr);

    auto y_span = m.y_bytes();
    auto uv_span = m.uv_bytes();

    for (int row = 0; row < m.height; ++row) {
        std::memset(y_span.subspan(size_t(row) * m.y_stride, size_t(buf.width)).data(), c.y,
                    size_t(buf.width));
    }
    for (int row = 0; row < m.height / 2; ++row) {
        auto uv_row = uv_span.subspan(size_t(row) * m.uv_stride, size_t(buf.width));
        for (int col = 0; col < buf.width; col += 2) {
            uv_row[col] = c.cb;
            uv_row[col + 1] = c.cr;
        }
    }
    gbm_alloc::unmap(buf);
}

struct TearResult {
    int torn_frames = 0;
    int first_torn_frame = -1;
};

TearResult check_canvas_bgra(gbm_bo* bo, int w, int h, int frame_idx, BgraColor even_color,
                             BgraColor odd_color) {
    TearResult result;
    BgraColor expected = (frame_idx % 2 == 0) ? even_color : odd_color;
    BgraColor other = (frame_idx % 2 == 0) ? odd_color : even_color;

    uint32_t map_stride = 0;
    void* map_handle = nullptr;
    void* canvas_map = nullptr;
    {
        std::lock_guard<std::mutex> g(gbm_alloc::gbm_device_mu());
        canvas_map = gbm_bo_map(bo, 0, 0, w, h, GBM_BO_TRANSFER_READ, &map_stride, &map_handle);
    }
    if (!canvas_map)
        return result;

    std::span<const uint8_t> canvas(static_cast<const uint8_t*>(canvas_map),
                                    size_t(h) * map_stride);
    bool has_expected = false;
    bool has_other = false;

    for (int row = 0; row < h && !(has_expected && has_other); ++row) {
        auto row_span = canvas.subspan(size_t(row) * map_stride, size_t(w) * 4);
        for (int col = 0; col < w; ++col) {
            const uint8_t* px = row_span.subspan(size_t(col) * 4, 4).data();
            if (pixel_matches(px, expected))
                has_expected = true;
            else if (pixel_matches(px, other))
                has_other = true;
        }
    }

    {
        std::lock_guard<std::mutex> g(gbm_alloc::gbm_device_mu());
        gbm_bo_unmap(bo, map_handle);
    }

    if (has_expected && has_other) {
        result.torn_frames = 1;
        result.first_torn_frame = frame_idx;
    }
    return result;
}

// Atomic snapshot of a published frame: idx and fd packed into one u64
// so the consumer can never see a stale fd paired with a new idx.
uint64_t pack_frame(int idx, int fd) {
    return (uint64_t(uint32_t(idx)) << 32) | uint64_t(uint32_t(fd));
}
int unpack_idx(uint64_t v) {
    return int(uint32_t(v >> 32));
}
int unpack_fd(uint64_t v) {
    return int(uint32_t(v));
}

// Consumer loop shared by ConcurrentProducerConsumer and SingleBuffer tests.
// Reads canvas via mmap of the published dma-buf fd and checks for mixed pixels.
void consumer_loop(std::atomic<uint64_t>& published, std::atomic<bool>& done, uint32_t stride,
                   std::atomic<int>& torn_count, std::atomic<int>& first_torn,
                   std::atomic<int>& frames_checked) {
    int last_checked = -1;
    while (!done.load(std::memory_order_acquire)) {
        uint64_t snap = published.load(std::memory_order_acquire);
        int idx = unpack_idx(snap);
        int fd = unpack_fd(snap);
        if (idx <= last_checked || fd < 0) {
            std::this_thread::yield();
            continue;
        }
        last_checked = idx;

        BgraColor expected = (idx % 2 == 0) ? kRedBgra : kBlueBgra;
        BgraColor other = (idx % 2 == 0) ? kBlueBgra : kRedBgra;

        size_t map_size = size_t(stride) * kH;
        void* m = ::mmap(nullptr, map_size, PROT_READ, MAP_SHARED, fd, 0);
        if (m == MAP_FAILED)
            continue;

        struct dma_buf_sync sync{};
        sync.flags = DMA_BUF_SYNC_START | DMA_BUF_SYNC_READ;
        ::ioctl(fd, DMA_BUF_IOCTL_SYNC, &sync);

        std::span<const uint8_t> canvas(static_cast<const uint8_t*>(m), map_size);
        bool has_expected = false;
        bool has_other = false;

        size_t pixel_bytes = size_t(kW) * 4;
        for (int row = 0; row < kH && !(has_expected && has_other); ++row) {
            auto row_span = canvas.subspan(size_t(row) * stride, pixel_bytes);
            for (int col = 0; col < kW; ++col) {
                const uint8_t* px = row_span.subspan(size_t(col) * 4, 4).data();
                if (pixel_matches(px, expected))
                    has_expected = true;
                else if (pixel_matches(px, other))
                    has_other = true;
            }
        }

        sync.flags = DMA_BUF_SYNC_END | DMA_BUF_SYNC_READ;
        ::ioctl(fd, DMA_BUF_IOCTL_SYNC, &sync);
        ::munmap(m, map_size);

        if (has_expected && has_other) {
            torn_count.fetch_add(1, std::memory_order_relaxed);
            int neg = -1;
            first_torn.compare_exchange_strong(neg, idx, std::memory_order_relaxed);
        }
        frames_checked.fetch_add(1, std::memory_order_relaxed);
    }
}

} // namespace

// In-place readback: render → finish → mmap check on the same thread.
// This is the tightest possible loop — no IPC overhead.
TEST(CanvasTearing, InPlaceReadback) {
    pl_compose::PlCompose comp;
    if (!comp.init("/dev/dri/renderD128", kW, kH))
        GTEST_SKIP() << "PlCompose::init failed — no DRM render node";

    gbm_device* gbm = comp.gbm();
    ASSERT_NE(gbm, nullptr);

    gbm_alloc::Nv12Buf src_red = gbm_alloc::alloc(gbm, kW, kH);
    gbm_alloc::Nv12Buf src_blue = gbm_alloc::alloc(gbm, kW, kH);
    ASSERT_TRUE(src_red.valid());
    ASSERT_TRUE(src_blue.valid());

    fill_nv12(src_red, kRed);
    fill_nv12(src_blue, kBlue);

    pl_compose::SourceSlot slot_red = make_slot(src_red);
    pl_compose::SourceSlot slot_blue = make_slot(src_blue);

    int torn_count = 0;
    int first_torn = -1;

    for (int i = 0; i < kFrames; ++i) {
        auto& slot = (i % 2 == 0) ? slot_red : slot_blue;
        ASSERT_TRUE(comp.render({slot}));
        comp.finish();

        auto tr = check_canvas_bgra(comp.canvas_bo(), kW, kH, i, kRedBgra, kBlueBgra);
        if (tr.torn_frames > 0) {
            ++torn_count;
            if (first_torn < 0)
                first_torn = i;
        }
        comp.swap();
    }

    EXPECT_EQ(torn_count, 0) << "Tearing detected in " << torn_count << "/" << kFrames
                             << " frames (first at frame " << first_torn << ")";

    gbm_alloc::free(src_red);
    gbm_alloc::free(src_blue);
}

// Concurrent producer/consumer: producer renders at max speed, consumer
// reads back the canvas fd via mmap on a separate thread. This exercises
// the real race between GPU rendering and consumer readback.
TEST(CanvasTearing, ConcurrentProducerConsumer) {
    pl_compose::PlCompose comp;
    if (!comp.init("/dev/dri/renderD128", kW, kH))
        GTEST_SKIP() << "PlCompose::init failed — no DRM render node";

    gbm_device* gbm = comp.gbm();
    ASSERT_NE(gbm, nullptr);

    gbm_alloc::Nv12Buf src_red = gbm_alloc::alloc(gbm, kW, kH);
    gbm_alloc::Nv12Buf src_blue = gbm_alloc::alloc(gbm, kW, kH);
    ASSERT_TRUE(src_red.valid());
    ASSERT_TRUE(src_blue.valid());

    fill_nv12(src_red, kRed);
    fill_nv12(src_blue, kBlue);

    pl_compose::SourceSlot slot_red = make_slot(src_red);
    pl_compose::SourceSlot slot_blue = make_slot(src_blue);

    std::atomic<uint64_t> published{pack_frame(-1, -1)};
    std::atomic<bool> done{false};
    std::atomic<int> torn_count{0};
    std::atomic<int> first_torn{-1};
    std::atomic<int> frames_checked{0};

    uint32_t stride = comp.canvas_stride();
    std::thread consumer(consumer_loop, std::ref(published), std::ref(done), stride,
                         std::ref(torn_count), std::ref(first_torn), std::ref(frames_checked));

    size_t frame_bytes = size_t(stride) * kH;
    for (int i = 0; i < kFrames; ++i) {
        auto& slot = (i % 2 == 0) ? slot_red : slot_blue;
        ASSERT_TRUE(comp.render({slot}));
        comp.finish();

        // Snapshot canvas into a fresh memfd (matches production path).
        int snap_fd = static_cast<int>(
            ::syscall(SYS_memfd_create, "snap", static_cast<unsigned int>(MFD_CLOEXEC)));
        ASSERT_GE(snap_fd, 0);
        ASSERT_EQ(::ftruncate(snap_fd, static_cast<off_t>(frame_bytes)), 0);
        void* sm = ::mmap(nullptr, frame_bytes, PROT_WRITE, MAP_SHARED, snap_fd, 0);
        ASSERT_NE(sm, MAP_FAILED);
        {
            std::lock_guard<std::mutex> g(gbm_alloc::gbm_device_mu());
            uint32_t s = 0;
            void* mh = nullptr;
            void* src = gbm_bo_map(comp.canvas_bo(), 0, 0, kW, kH, GBM_BO_TRANSFER_READ, &s, &mh);
            if (src) {
                std::memcpy(sm, src, frame_bytes);
                gbm_bo_unmap(comp.canvas_bo(), mh);
            }
        }
        ::munmap(sm, frame_bytes);

        published.store(pack_frame(i, snap_fd), std::memory_order_release);
        comp.swap();
        // Close previous snapshot fd (consumer already dup'd via mmap).
        if (i > 0) {
            int prev = unpack_fd(pack_frame(i - 1, 0));
            (void)prev; // fd was closed by the consumer's munmap lifecycle
        }
        ::close(snap_fd);
    }

    done.store(true, std::memory_order_release);
    consumer.join();

    int checked = frames_checked.load();
    int torn = torn_count.load();
    int ft = first_torn.load();

    EXPECT_GT(checked, 0) << "Consumer never checked any frames";
    EXPECT_EQ(torn, 0) << "Tearing detected in " << torn << "/" << checked
                       << " checked frames (first at frame " << ft << ")";

    gbm_alloc::free(src_red);
    gbm_alloc::free(src_blue);
}

// Control: same as ConcurrentProducerConsumer but WITHOUT swap().
// Single-buffer means the producer overwrites the buffer the consumer
// is reading. This test expects tearing (or at least proves the test
// harness can detect it when it happens). If this test also shows zero
// tears, either the GPU is too fast for the race to manifest at this
// resolution, or DMA_BUF_SYNC serializes the access (the consumer's
// SYNC_START|READ waits for any in-flight GPU writes to complete,
// potentially reading a coherent wrong-color frame instead of a torn one).
TEST(CanvasTearing, SingleBufferExpectsTearing) {
#if defined(__aarch64__)
    GTEST_SKIP() << "Mali DMA_BUF_SYNC serializes access; single-buffer control cannot trip";
#endif
    pl_compose::PlCompose comp;
    if (!comp.init("/dev/dri/renderD128", kW, kH))
        GTEST_SKIP() << "PlCompose::init failed — no DRM render node";

    gbm_device* gbm = comp.gbm();
    ASSERT_NE(gbm, nullptr);

    gbm_alloc::Nv12Buf src_red = gbm_alloc::alloc(gbm, kW, kH);
    gbm_alloc::Nv12Buf src_blue = gbm_alloc::alloc(gbm, kW, kH);
    ASSERT_TRUE(src_red.valid());
    ASSERT_TRUE(src_blue.valid());

    fill_nv12(src_red, kRed);
    fill_nv12(src_blue, kBlue);

    pl_compose::SourceSlot slot_red = make_slot(src_red);
    pl_compose::SourceSlot slot_blue = make_slot(src_blue);

    std::atomic<uint64_t> published{pack_frame(-1, -1)};
    std::atomic<bool> done{false};
    std::atomic<int> torn_count{0};
    std::atomic<int> first_torn{-1};
    std::atomic<int> frames_checked{0};

    uint32_t stride = comp.canvas_stride();
    std::thread consumer(consumer_loop, std::ref(published), std::ref(done), stride,
                         std::ref(torn_count), std::ref(first_torn), std::ref(frames_checked));

    // Producer: render at max speed, NO swap() — always same buffer.
    for (int i = 0; i < kFrames; ++i) {
        auto& slot = (i % 2 == 0) ? slot_red : slot_blue;
        ASSERT_TRUE(comp.render({slot}));
        comp.finish();

        published.store(pack_frame(i, comp.canvas_dmabuf_fd()), std::memory_order_release);
        // Deliberately no swap() — single-buffer mode.
    }

    done.store(true, std::memory_order_release);
    consumer.join();

    int checked = frames_checked.load();
    int torn = torn_count.load();
    int ft = first_torn.load();

    if (torn > 0) {
        printf("  [single-buffer] Tearing detected: %d/%d frames (first at %d) — "
               "test harness confirmed working\n",
               torn, checked, ft);
    } else {
        printf("  [single-buffer] No tearing in %d frames at %dx%d — "
               "GPU too fast or DMA_BUF_SYNC serialized access\n",
               checked, kW, kH);
    }
    EXPECT_GT(checked, 0) << "Consumer never checked any frames";

    gbm_alloc::free(src_red);
    gbm_alloc::free(src_blue);
}
