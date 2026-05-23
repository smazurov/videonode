// live-compose-probe — slice that wires real V4L2 sources (via FfmpegPipeSource)
// into GlCompose, runs the render loop for N seconds, and dumps the last
// composed canvas frame as a PPM.
//
// Layout: 2-up side by side at canvas 1920x1080.
//   - Source A (HDMI /dev/video0, NV12 4K) at (0..960, 0..1080) WITH perspective
//   - Source B (Lyra /dev/video1, MJPEG 1080p) at (960..1920, 0..1080) IDENTITY
//
// Usage: ./live-compose-probe [seconds] [out.ppm]

#include "src/common/probe_check.hpp"
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
#include <span>
#include <thread>
#include <vector>

namespace {

struct Nv12Image {
    EGLImage y = EGL_NO_IMAGE;
    EGLImage uv = EGL_NO_IMAGE;
};

Nv12Image import_nv12_(const egl_ctx::EglCtx& ctx, const ffmpeg_pipe_source::FrameView& v) {
    Nv12Image im;
    egl_ctx::EglCtx::ImageDesc dy;
    dy.fd = v.fd;
    dy.fourcc = DRM_FORMAT_R8;
    dy.modifier = DRM_FORMAT_MOD_LINEAR;
    dy.width = v.width;
    dy.height = v.height;
    dy.plane0_offset = static_cast<int>(v.plane0_offset);
    dy.plane0_pitch = static_cast<int>(v.plane0_pitch);
    im.y = ctx.import_dmabuf(dy);
    if (im.y == EGL_NO_IMAGE)
        return im;

    egl_ctx::EglCtx::ImageDesc duv;
    duv.fd = v.fd;
    duv.fourcc = DRM_FORMAT_GR88;
    duv.modifier = DRM_FORMAT_MOD_LINEAR;
    duv.width = v.width / 2;
    duv.height = v.height / 2;
    duv.plane0_offset = static_cast<int>(v.plane1_offset);
    duv.plane0_pitch = static_cast<int>(v.plane1_pitch);
    im.uv = ctx.import_dmabuf(duv);
    if (im.uv == EGL_NO_IMAGE) {
        eglDestroyImage(ctx.display(), im.y);
        im.y = EGL_NO_IMAGE;
    }
    return im;
}

} // namespace

int main(int argc, char** argv) {
    const std::span<char*> args(argv, static_cast<size_t>(argc));
    int seconds = (args.size() > 1) ? std::atoi(args[1]) : 5;
    const char* out = (args.size() > 2) ? args[2] : "/tmp/live-compose.ppm";
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
    VN_CHECK(lyra.init(pl), "lyra init");
    VN_CHECK(lyra.start(), "lyra start");

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
    VN_CHECK(hdmi.init(ph), "hdmi init");
    VN_CHECK(hdmi.start(), "hdmi start");

    fprintf(stderr, "ok: captures started (lyra then hdmi)\n");

    // 2. EGL + compose.
    egl_ctx::EglCtx ctx;
    VN_CHECK(ctx.init("/dev/dri/renderD130"), "EglCtx::init");
    gl_compose::GlCompose compose;
    VN_CHECK(compose.init(ctx, Cw, Ch), "GlCompose::init");
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
    std::map<int, Nv12Image> img_cache;
    auto get_img = [&](const FrameView& v) -> Nv12Image {
        auto it = img_cache.find(v.fd);
        if (it != img_cache.end())
            return it->second;
        Nv12Image im = import_nv12_(ctx, v);
        if (im.y != EGL_NO_IMAGE && im.uv != EGL_NO_IMAGE)
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

        Nv12Image ia = get_img(a);
        Nv12Image ib = get_img(b);

        std::vector<gl_compose::SourceSlot> slots(2);
        // HDMI on the left, with the perspective unlock visible.
        slots[0].src_y_image = ia.y;
        slots[0].src_uv_image = ia.uv;
        slots[0].x = 0;
        slots[0].y = 0;
        slots[0].w = 960;
        slots[0].h = 1080;
        slots[0].warp = {{1.0f, 0.0f, 0.0f, 0.0f, 1.0f, 0.0f, 0.0f, -0.3f, 1.0f}};
        // Lyra on the right, identity warp.
        slots[1].src_y_image = ib.y;
        slots[1].src_uv_image = ib.uv;
        slots[1].x = 960;
        slots[1].y = 0;
        slots[1].w = 960;
        slots[1].h = 1080;

        VN_CHECK(compose.render(slots), "GlCompose::render");
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
    VN_CHECK(mapped, "gbm_bo_map canvas");
    FILE* f = std::fopen(out, "wb");
    VN_CHECK(f, "fopen PPM");
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

    for (auto& kv : img_cache) {
        eglDestroyImage(ctx.display(), kv.second.y);
        eglDestroyImage(ctx.display(), kv.second.uv);
    }
    lyra.stop();
    hdmi.stop();

    printf("PASS: live compose %d frames; PPM at %s\n", frames_rendered, out);
    return 0;
}
