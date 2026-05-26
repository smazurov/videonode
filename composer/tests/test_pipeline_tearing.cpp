// End-to-end pipeline tearing test.
//
// Simulates the real frame path: source broadcasts alternating-color NV12
// frames via ScmRightsProducer → composer reads via ScmRightsSource,
// GPU-renders into a double-buffered BGRA canvas with the pipelined loop
// pattern (submit N, finish N-1), broadcasts canvas via ScmRightsProducer
// → consumer reads via ScmRightsSource, mmaps, and checks for mixed pixels.
//
// This exercises every layer that could tear: staged memfd reuse, fd
// borrow gap, canvas double-buffer race, pipelined GPU timing, and
// consumer mmap vs producer overwrite.

#include "src/ipc/dmabuf_header.hpp"
#include "src/ipc/scm_rights_producer.hpp"
#include "src/ipc/scm_rights_source.hpp"
#include "src/render/gbm_alloc.hpp"
#include "src/render/nv12_buf.hpp"
#include "src/render/pl_compose.hpp"

#include <gbm.h>
#include <gtest/gtest.h>
#include <linux/dma-buf.h>
#include <sys/ioctl.h>
#include <sys/mman.h>

#include <atomic>
#include <chrono>
#include <cstdint>
#include <cstring>
#include <linux/memfd.h>
#include <span>
#include <string>
#include <sys/syscall.h>
#include <thread>
#include <unistd.h>

namespace {

constexpr int kW = 64;
constexpr int kH = 64;
constexpr int kFrames = 200;

struct YuvColor {
    uint8_t y, cb, cr;
};
constexpr YuvColor kRed{81, 90, 240};
constexpr YuvColor kBlue{41, 240, 110};

struct BgraColor {
    uint8_t b, g, r, a;
};
constexpr BgraColor kRedBgra{0, 0, 255, 255};
constexpr BgraColor kBlueBgra{255, 0, 0, 255};
constexpr int kTolerance = 8;

bool pixel_matches(const uint8_t* px, BgraColor c) {
    return std::abs(int(px[0]) - c.b) <= kTolerance && std::abs(int(px[1]) - c.g) <= kTolerance &&
           std::abs(int(px[2]) - c.r) <= kTolerance;
}

std::string make_sock(const char* tag) {
    return std::string("/tmp/test-pipe-tear-") + tag + "-" + std::to_string(getpid()) + ".sock";
}

void fill_nv12_gbm(gbm_alloc::Nv12Buf& buf, YuvColor c) {
    auto m = gbm_alloc::map_rw(buf);
    if (!m.y || !m.uv)
        return;
    for (int row = 0; row < m.height; ++row)
        std::memset(m.y_bytes().subspan(size_t(row) * m.y_stride, size_t(buf.width)).data(), c.y,
                    size_t(buf.width));
    for (int row = 0; row < m.height / 2; ++row) {
        auto uv_row = m.uv_bytes().subspan(size_t(row) * m.uv_stride, size_t(buf.width));
        for (int col = 0; col < buf.width; col += 2) {
            uv_row[col] = c.cb;
            uv_row[col + 1] = c.cr;
        }
    }
    gbm_alloc::unmap(buf);
}

dmabuf_header::Header make_nv12_header(int w, int h, uint32_t y_pitch, uint32_t uv_pitch,
                                       uint64_t frame_idx) {
    dmabuf_header::Header hdr;
    hdr.width = uint32_t(w);
    hdr.height = uint32_t(h);
    hdr.format = "NV12";
    hdr.plane_pitches = {y_pitch, uv_pitch};
    hdr.plane_offsets = {0, 0};
    hdr.color_matrix = dmabuf_header::ColorMatrix::Bt601;
    hdr.color_range = dmabuf_header::ColorRange::Limited;
    hdr.chroma_siting = dmabuf_header::ChromaSiting::Mpeg2;
    hdr.frame_idx = frame_idx;
    return hdr;
}

} // namespace

// Full pipeline: source → SCM → composer (pipelined GPU) → SCM → consumer mmap check.
TEST(PipelineTearing, SourceToComposerToConsumer) {
    pl_compose::PlCompose comp;
    if (!comp.init("/dev/dri/renderD128", kW, kH))
        GTEST_SKIP() << "No DRM render node";

    gbm_device* gbm = comp.gbm();
    ASSERT_NE(gbm, nullptr);

    // Allocate two NV12 source frames (red and blue) with staging.
    nv12_buf::Allocator alloc;
    ASSERT_TRUE(alloc.init(gbm));
    nv12_buf::Buffer src_bufs[2] = {alloc.alloc(kW, kH), alloc.alloc(kW, kH)};
    ASSERT_TRUE(src_bufs[0].valid());
    ASSERT_TRUE(src_bufs[1].valid());

    // Fill via gbm_alloc (the underlying NV12 bos).
    gbm_alloc::Nv12Buf gbm_red = gbm_alloc::alloc(gbm, kW, kH);
    gbm_alloc::Nv12Buf gbm_blue = gbm_alloc::alloc(gbm, kW, kH);
    ASSERT_TRUE(gbm_red.valid());
    ASSERT_TRUE(gbm_blue.valid());
    fill_nv12_gbm(gbm_red, kRed);
    fill_nv12_gbm(gbm_blue, kBlue);

    // Source SCM producer (simulates videonode-source).
    std::string src_sock = make_sock("src");
    scm_rights_producer::ScmRightsProducer src_prod;
    scm_rights_producer::InitParams spp;
    spp.socket_path = src_sock;
    ASSERT_TRUE(src_prod.init(spp));
    ASSERT_TRUE(src_prod.start());

    // Composer's source input (simulates ScmRightsSource in canvas_loop).
    scm_rights_source::ScmRightsSource comp_in;
    scm_rights_source::InitParams cip;
    cip.socket_path = src_sock;
    cip.dial = true;
    ASSERT_TRUE(comp_in.init(cip));
    ASSERT_TRUE(comp_in.start());

    // Composer SCM output (simulates ScmRightsProducer in canvas_loop).
    std::string out_sock = make_sock("out");
    scm_rights_producer::ScmRightsProducer comp_out;
    scm_rights_producer::InitParams opp;
    opp.socket_path = out_sock;
    ASSERT_TRUE(comp_out.init(opp));
    ASSERT_TRUE(comp_out.start());

    // Consumer (simulates vn-sink).
    scm_rights_source::ScmRightsSource consumer;
    scm_rights_source::InitParams conp;
    conp.socket_path = out_sock;
    conp.dial = true;
    ASSERT_TRUE(consumer.init(conp));
    ASSERT_TRUE(consumer.start());

    std::this_thread::sleep_for(std::chrono::milliseconds(100));

    // Consumer thread: read BGRA canvas frames and check for tearing.
    std::atomic<bool> done{false};
    std::atomic<int> torn_count{0};
    std::atomic<int> frames_checked{0};
    uint32_t canvas_stride = comp.canvas_stride();

    std::thread consumer_thread([&] {
        uint64_t last_idx = 0;
        while (!done.load(std::memory_order_acquire)) {
            auto fv = consumer.latest_frame();
            if (fv.fd.get() < 0 || fv.frame_idx == 0 || fv.frame_idx == last_idx) {
                std::this_thread::yield();
                continue;
            }
            last_idx = fv.frame_idx;

            size_t map_size = size_t(canvas_stride) * kH;
            void* m = ::mmap(nullptr, map_size, PROT_READ, MAP_SHARED, fv.fd.get(), 0);
            if (m == MAP_FAILED)
                continue;

            struct dma_buf_sync sync{};
            sync.flags = DMA_BUF_SYNC_START | DMA_BUF_SYNC_READ;
            ::ioctl(fv.fd.get(), DMA_BUF_IOCTL_SYNC, &sync);

            // Check that every pixel is ONE solid color — don't predict which.
            // Mixed colors in one frame = tearing.
            std::span<const uint8_t> canvas(static_cast<const uint8_t*>(m), map_size);
            bool has_red = false;
            bool has_blue = false;
            bool has_black = false;

            for (int row = 0; row < kH; ++row) {
                auto row_span = canvas.subspan(size_t(row) * canvas_stride, size_t(kW) * 4);
                for (int col = 0; col < kW; ++col) {
                    const uint8_t* px = row_span.subspan(size_t(col) * 4, 4).data();
                    if (pixel_matches(px, kRedBgra))
                        has_red = true;
                    else if (pixel_matches(px, kBlueBgra))
                        has_blue = true;
                    else if (px[0] == 0 && px[1] == 0 && px[2] == 0 && px[3] == 0)
                        has_black = true;
                }
            }

            sync.flags = DMA_BUF_SYNC_END | DMA_BUF_SYNC_READ;
            ::ioctl(fv.fd.get(), DMA_BUF_IOCTL_SYNC, &sync);
            ::munmap(m, map_size);

            int colors = int(has_red) + int(has_blue) + int(has_black);
            bool torn = (colors > 1);
            if (torn) {
                printf("  frame %llu: MIXED (%s%s%s)\n",
                       static_cast<unsigned long long>(fv.frame_idx), has_red ? "red " : "",
                       has_blue ? "blue " : "", has_black ? "black " : "");
            }
            if (torn)
                torn_count.fetch_add(1, std::memory_order_relaxed);
            frames_checked.fetch_add(1, std::memory_order_relaxed);
        }
    });

    // Synchronous producer: source broadcasts NV12, composer renders BGRA
    // (render+finish+broadcast+swap in one step), consumer checks.
    uint64_t broadcast_idx = 0;

    for (int i = 0; i < kFrames; ++i) {
        auto& src_gbm = (i % 2 == 0) ? gbm_red : gbm_blue;
        auto hdr = make_nv12_header(kW, kH, src_gbm.y_stride, src_gbm.uv_stride,
                                    static_cast<uint64_t>(i + 1));
        src_prod.broadcast(hdr, {src_gbm.y_fd, src_gbm.uv_fd});

        std::this_thread::sleep_for(std::chrono::microseconds(500));

        auto fv = comp_in.latest_frame();
        if (fv.fd.get() < 0 || fv.frame_idx == 0)
            continue;

        pl_compose::SourceSlot slot;
        slot.src_y_fd = fv.fd.get();
        slot.src_uv_fd = fv.plane1_fd.get();
        slot.src_w = kW;
        slot.src_h = kH;
        slot.src_y_pitch = fv.plane0_pitch ? int(fv.plane0_pitch) : kW;
        slot.src_uv_pitch = fv.plane1_pitch ? int(fv.plane1_pitch) : kW;
        slot.x = 0;
        slot.y = 0;
        slot.w = kW;
        slot.h = kH;
        if (!comp.render({slot}))
            continue;
        comp.finish();

        // Snapshot the canvas into a fresh memfd (same as production code).
        size_t frame_bytes = size_t(canvas_stride) * kH;
        int snap_fd = static_cast<int>(
            ::syscall(SYS_memfd_create, "snap", static_cast<unsigned int>(MFD_CLOEXEC)));
        ASSERT_GE(snap_fd, 0);
        ASSERT_EQ(::ftruncate(snap_fd, static_cast<off_t>(frame_bytes)), 0);
        void* snap = ::mmap(nullptr, frame_bytes, PROT_WRITE, MAP_SHARED, snap_fd, 0);
        ASSERT_NE(snap, MAP_FAILED);
        {
            std::lock_guard<std::mutex> g(gbm_alloc::gbm_device_mu());
            uint32_t s = 0;
            void* mh = nullptr;
            void* src = gbm_bo_map(comp.canvas_bo(), 0, 0, kW, kH, GBM_BO_TRANSFER_READ, &s, &mh);
            if (src) {
                std::memcpy(snap, src, frame_bytes);
                gbm_bo_unmap(comp.canvas_bo(), mh);
            }
        }
        ::munmap(snap, frame_bytes);

        dmabuf_header::Header ch;
        ch.width = uint32_t(kW);
        ch.height = uint32_t(kH);
        ch.format = "BGRA";
        ch.plane_pitches = {canvas_stride};
        ch.plane_offsets = {0};
        ch.frame_idx = ++broadcast_idx;
        comp_out.broadcast(ch, {snap_fd});
        ::close(snap_fd);
        comp.swap();
    }

    std::this_thread::sleep_for(std::chrono::milliseconds(100));
    done.store(true, std::memory_order_release);
    consumer_thread.join();

    int checked = frames_checked.load();
    int torn = torn_count.load();

    printf("  [pipeline] %d/%d frames torn\n", torn, checked);
    EXPECT_GT(checked, 0) << "Consumer never checked any frames";
    EXPECT_EQ(torn, 0) << "Pipeline tearing detected in " << torn << "/" << checked << " frames";

    consumer.stop();
    comp_out.stop();
    comp_in.stop();
    src_prod.stop();
    gbm_alloc::free(gbm_red);
    gbm_alloc::free(gbm_blue);
}
