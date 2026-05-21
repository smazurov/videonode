// live-compose-probe — slice that wires real V4L2 sources (via FfmpegPipeSource)
// into GlCompose, runs the render loop for N seconds, and dumps the last
// composed canvas frame as a PPM.
//
// Layout: 2-up side by side at canvas 1920x1080.
//   - Source A (HDMI /dev/video0, NV12 4K) at (0..960, 0..1080) WITH perspective
//   - Source B (Lyra /dev/video1, MJPEG 1080p) at (960..1920, 0..1080) IDENTITY
//
// Usage: ./live-compose-probe [seconds] [out.ppm]

#include "src/render/egl_ctx.hpp"
#include "src/process/ffmpeg_pipe_source.hpp"
#include "src/render/gl_compose.hpp"

#include <EGL/egl.h>
#include <GLES2/gl2.h>
#include <drm_fourcc.h>
#include <gbm.h>

#include <chrono>
#include <cstdint>
#include <cstdio>
#include <cstdlib>
#include <cstring>
#include <map>
#include <thread>
#include <vector>

#define CHECK(expr, msg)                                                                           \
    do {                                                                                           \
        if (!(expr)) {                                                                             \
            fprintf(stderr, "FAIL: %s\n", msg);                                                    \
            return 1;                                                                              \
        }                                                                                          \
    } while (0)

namespace {

EGLImage import_nv12_(const egl_ctx::EglCtx& ctx, const ffmpeg_pipe_source::FrameView& v) {
    egl_ctx::EglCtx::ImageDesc d;
    d.fd = v.fd;
    d.fourcc = DRM_FORMAT_NV12;
    d.modifier = DRM_FORMAT_MOD_LINEAR;
    d.width = v.width;
    d.height = v.height;
    d.plane0_offset = v.plane0_offset;
    d.plane0_pitch = v.plane0_pitch;
    d.plane1_offset = v.plane1_offset;
    d.plane1_pitch = v.plane1_pitch;
    return ctx.import_dmabuf(d);
}

} // namespace

int main(int argc, char** argv) {
    int seconds = (argc > 1) ? std::atoi(argv[1]) : 5;
    const char* out = (argc > 2) ? argv[2] : "/tmp/live-compose.ppm";
    constexpr int Cw = 1920, Ch = 1080;

    // 1. Start V4L2 captures.
    // Stagger the two ffmpeg subprocesses: Lyra (USB UVC) reliably fails
    // with "VIDIOC_DQBUF: No such device" when it races against the HDMI
    // capture starting at the same instant. Lyra first, wait for it to
    // produce a frame, then bring up HDMI.
    using ffmpeg_pipe_source::FfmpegPipeSource;
    using ffmpeg_pipe_source::FrameView;
    using ffmpeg_pipe_source::InitParams;

    FfmpegPipeSource lyra;
    InitParams pl{};
    pl.device = "/dev/video1";
    pl.input_format = "mjpeg";
    pl.width = 1920;
    pl.height = 1080;
    pl.fps = 30;
    // -thread_queue_size 1024 in front of -i is the canonical UVC stability fix.
    pl.extra_input_args = {"-thread_queue_size", "1024"};
    CHECK(lyra.init(pl), "lyra init");
    CHECK(lyra.start(), "lyra start");

    auto lyra_deadline = std::chrono::steady_clock::now() + std::chrono::seconds(10);
    while (std::chrono::steady_clock::now() < lyra_deadline) {
        if (lyra.latest_frame().frame_idx > 0)
            break;
        std::this_thread::sleep_for(std::chrono::milliseconds(50));
    }
    if (lyra.latest_frame().frame_idx == 0) {
        fprintf(stderr, "FAIL: lyra never produced a frame\n");
        lyra.stop();
        return 1;
    }
    fprintf(stderr, "ok: lyra streaming\n");

    FfmpegPipeSource hdmi;
    InitParams ph{};
    ph.device = "/dev/video0";
    ph.input_format = "nv12";
    ph.width = 3840;
    ph.height = 2160;
    ph.fps = 30;
    ph.extra_input_args = {"-thread_queue_size", "1024"};
    CHECK(hdmi.init(ph), "hdmi init");
    CHECK(hdmi.start(), "hdmi start");

    fprintf(stderr, "ok: captures started (lyra then hdmi)\n");

    // 2. EGL + compose.
    egl_ctx::EglCtx ctx;
    CHECK(ctx.init("/dev/dri/renderD130"), "EglCtx::init");
    gl_compose::GlCompose compose;
    CHECK(compose.init(ctx, Cw, Ch), "GlCompose::init");
    fprintf(stderr, "ok: GLES compose canvas %dx%d on Mali\n", Cw, Ch);

    // 3. Wait for first frame from each source (up to 10s).
    auto wait_deadline = std::chrono::steady_clock::now() + std::chrono::seconds(10);
    while (std::chrono::steady_clock::now() < wait_deadline) {
        auto a = hdmi.latest_frame();
        auto b = lyra.latest_frame();
        if (a.frame_idx > 0 && b.frame_idx > 0)
            break;
        std::this_thread::sleep_for(std::chrono::milliseconds(50));
    }
    {
        auto a = hdmi.latest_frame();
        auto b = lyra.latest_frame();
        fprintf(stderr, "ok: first frames hdmi=%lu lyra=%lu\n", (unsigned long)a.frame_idx,
                (unsigned long)b.frame_idx);
        if (!a.frame_idx || !b.frame_idx) {
            hdmi.stop();
            lyra.stop();
            return 1;
        }
    }

    // 4. Render loop: 30fps for `seconds`.
    // We cache EGLImages by source dma-buf fd: as long as the source keeps
    // ping-ponging through the same N dma_heap buffers, we only pay the
    // eglCreateImage cost N times total.
    std::map<int, EGLImage> img_cache;
    auto get_img = [&](const FrameView& v) -> EGLImage {
        auto it = img_cache.find(v.fd);
        if (it != img_cache.end())
            return it->second;
        EGLImage im = import_nv12_(ctx, v);
        if (im != EGL_NO_IMAGE)
            img_cache[v.fd] = im;
        return im;
    };

    auto start = std::chrono::steady_clock::now();
    auto end = start + std::chrono::seconds(seconds);
    int frames_rendered = 0;
    int next_tick_ms = 0;
    while (std::chrono::steady_clock::now() < end) {
        auto a = hdmi.latest_frame();
        auto b = lyra.latest_frame();

        EGLImage ia = get_img(a);
        EGLImage ib = get_img(b);

        std::vector<gl_compose::SourceSlot> slots(2);
        // HDMI on the left, with the perspective unlock visible.
        slots[0].src_image = ia;
        slots[0].x = 0;
        slots[0].y = 0;
        slots[0].w = 960;
        slots[0].h = 1080;
        slots[0].warp = {{1.0f, 0.0f, 0.0f, 0.0f, 1.0f, 0.0f, 0.0f, -0.3f, 1.0f}};
        // Lyra on the right, identity warp.
        slots[1].src_image = ib;
        slots[1].x = 960;
        slots[1].y = 0;
        slots[1].w = 960;
        slots[1].h = 1080;

        compose.render(slots);
        ++frames_rendered;

        // Tick at ~30Hz for the probe; a real composer would use a proper timer.
        next_tick_ms += 33;
        auto next_tick = start + std::chrono::milliseconds(next_tick_ms);
        std::this_thread::sleep_until(next_tick);
    }
    compose.finish();
    auto elapsed = std::chrono::duration<double>(std::chrono::steady_clock::now() - start).count();
    fprintf(stderr, "ok: rendered %d frames in %.2fs = %.1f fps\n", frames_rendered, elapsed,
            frames_rendered / elapsed);

    // 5. Dump last canvas to PPM.
    uint32_t stride = 0;
    void* mdata = nullptr;
    void* mapped =
        gbm_bo_map(compose.canvas_bo(), 0, 0, Cw, Ch, GBM_BO_TRANSFER_READ, &stride, &mdata);
    CHECK(mapped, "gbm_bo_map canvas");
    FILE* f = std::fopen(out, "wb");
    CHECK(f, "fopen PPM");
    std::fprintf(f, "P6\n%d %d\n255\n", Cw, Ch);
    for (int y = 0; y < Ch; ++y) {
        uint8_t* row = (uint8_t*)mapped + y * stride;
        for (int x = 0; x < Cw; ++x) {
            uint8_t bgra[3] = {row[x * 4 + 2], row[x * 4 + 1], row[x * 4 + 0]};
            std::fwrite(bgra, 1, 3, f);
        }
    }
    std::fclose(f);
    gbm_bo_unmap(compose.canvas_bo(), mdata);

    for (auto& kv : img_cache)
        eglDestroyImage(ctx.display(), kv.second);
    lyra.stop();
    hdmi.stop();

    printf("PASS: live compose %d frames; PPM at %s\n", frames_rendered, out);
    return 0;
}
