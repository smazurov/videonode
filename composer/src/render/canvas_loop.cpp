#include "src/render/canvas_loop.hpp"

#include "src/ipc/scm_rights_source.hpp"
#include "src/process/ffmpeg_pipe_source.hpp"

#include <EGL/egl.h>
#include <GLES2/gl2.h>
#include <drm_fourcc.h>
#include <gbm.h>

#include <cerrno>
#include <chrono>
#include <cstdint>
#include <cstdio>
#include <cstring>
#include <map>
#include <span>
#include <thread>
#include <unistd.h>
#include <vector>

namespace render {

namespace {

// Canonical frame view: a thin re-export of the fields both FfmpegPipeSource
// and ScmRightsSource expose with identical names. Lets the render loop and
// the EGL import stay source-agnostic.
struct FrameView {
    int fd = -1;
    int width = 0;
    int height = 0;
    uint32_t plane0_pitch = 0;
    uint32_t plane0_offset = 0;
    uint32_t plane1_pitch = 0;
    uint32_t plane1_offset = 0;
    int plane1_fd = -1;
    std::string format;
    uint64_t frame_idx = 0;
};

template <typename FV> FrameView to_canonical_(const FV& v) {
    FrameView c;
    c.fd = v.fd;
    c.width = v.width;
    c.height = v.height;
    c.plane0_pitch = v.plane0_pitch;
    c.plane0_offset = v.plane0_offset;
    c.plane1_pitch = v.plane1_pitch;
    c.plane1_offset = v.plane1_offset;
    c.plane1_fd = v.plane1_fd;
    c.format = v.format;
    c.frame_idx = v.frame_idx;
    return c;
}

// Two single-plane EGLImages per NV12 frame: Y as R8, UV as GR88. Both
// planes use the offset the producer supplied on the wire — for the
// host gbm split-buffer path the UV fd is distinct from the Y fd and
// both offsets are 0; for the rig dma_heap single-buffer path the UV
// fd aliases the Y fd and the UV offset is non-zero (typically
// y_pitch * height). Trusting the wire offsets covers both cleanly,
// and matches what csc-probe / minigbm do on radeonsi.
struct SourceImagePair {
    EGLImage y = EGL_NO_IMAGE;
    EGLImage uv = EGL_NO_IMAGE;
};

SourceImagePair import_frame_(const egl_ctx::EglCtx& ctx, const FrameView& v) {
    SourceImagePair p;
    if (v.fd < 0 || v.plane1_fd < 0 || v.width <= 0 || v.height <= 0)
        return p;

    // MOD_INVALID = omit MODIFIER_* attribs entirely. dri2_create_image_dma_buf
    // rejects MOD_LINEAR explicitly because gbm_bo_get_modifier() returns
    // MOD_INVALID for GBM_BO_USE_LINEAR bos and the values don't match.
    // csc-probe takes this same path.
    constexpr uint64_t kModInvalid = (uint64_t{1} << 56) - 1;

    egl_ctx::EglCtx::ImageDesc dy;
    dy.fd = v.fd;
    dy.fourcc = DRM_FORMAT_R8;
    dy.modifier = kModInvalid;
    dy.width = v.width;
    dy.height = v.height;
    dy.plane0_offset = v.plane0_offset;
    dy.plane0_pitch = v.plane0_pitch ? v.plane0_pitch : uint32_t(v.width);
    p.y = ctx.import_dmabuf(dy);
    if (p.y == EGL_NO_IMAGE)
        return p;

    egl_ctx::EglCtx::ImageDesc duv;
    duv.fd = v.plane1_fd;
    duv.fourcc = DRM_FORMAT_GR88;
    duv.modifier = kModInvalid;
    duv.width = v.width / 2;
    duv.height = v.height / 2;
    duv.plane0_offset = v.plane1_offset;
    duv.plane0_pitch = v.plane1_pitch ? v.plane1_pitch : uint32_t(v.width);
    p.uv = ctx.import_dmabuf(duv);
    if (p.uv == EGL_NO_IMAGE) {
        eglDestroyImage(ctx.display(), p.y);
        p.y = EGL_NO_IMAGE;
    }
    return p;
}

template <typename Src> bool wait_first_frame_(Src& s, int timeout_seconds, const char* tag) {
    auto deadline = std::chrono::steady_clock::now() + std::chrono::seconds(timeout_seconds);
    while (std::chrono::steady_clock::now() < deadline) {
        if (s.latest_frame().frame_idx > 0) {
            fprintf(stderr, "ok: %s first frame\n", tag);
            return true;
        }
        std::this_thread::sleep_for(std::chrono::milliseconds(50));
    }
    fprintf(stderr, "FAIL: %s no frame in %ds\n", tag, timeout_seconds);
    return false;
}

bool write_full_(int fd, std::span<const uint8_t> buf) {
    while (!buf.empty()) {
        ssize_t w = ::write(fd, buf.data(), buf.size());
        if (w < 0) {
            if (errno == EINTR)
                continue;
            if (errno == EPIPE)
                return false; // consumer hung up; clean exit
            fprintf(stderr, "write stdout: %s\n", strerror(errno));
            return false;
        }
        buf = buf.subspan(static_cast<size_t>(w));
    }
    return true;
}

} // namespace

bool StartScmSource(scm_rights_source::ScmRightsSource& s, const SourceArgs& a,
                    const char* tag) {
    scm_rights_source::InitParams p;
    p.socket_path = a.scm_socket_path;
    // Dial mode: the producer (videonode-source sidecar) listens on the
    // socket; we connect to it as one of N consumers. This is the new
    // multi-consumer architecture; the listen-mode path (where the
    // daemon dials in) is kept for the older single-source smoke tests.
    p.dial = true;
    if (!s.init(p))
        return false;
    if (!s.start())
        return false;
    return wait_first_frame_(s, 30, tag);
}

bool StartFfmpegSource(ffmpeg_pipe_source::FfmpegPipeSource& s, const SourceArgs& a,
                       const char* tag, gbm_device* gbm) {
    ffmpeg_pipe_source::InitParams p{};
    p.width = a.width;
    p.height = a.height;
    p.fps = a.fps;
    p.gbm = gbm;
    if (a.testsrc) {
        p.kind = ffmpeg_pipe_source::SourceKind::Lavfi;
        char expr[160];
        std::snprintf(expr, sizeof(expr), "testsrc2=size=%dx%d:rate=%d", a.width, a.height, a.fps);
        p.device = expr;
    } else {
        p.kind = ffmpeg_pipe_source::SourceKind::V4L2;
        p.device = a.device;
        p.input_format = a.input_format;
        p.extra_input_args = {"-thread_queue_size", "1024"};
    }
    if (!s.init(p))
        return false;
    if (!s.start())
        return false;
    return wait_first_frame_(s, 10, tag);
}

int RunCanvasLoop(const CanvasLoopArgs& args,
                  egl_ctx::EglCtx& ctx,
                  gl_compose::GlCompose& compose,
                  scm_rights_source::ScmRightsSource& scm_a,
                  scm_rights_source::ScmRightsSource& scm_b,
                  ffmpeg_pipe_source::FfmpegPipeSource& ff_a,
                  ffmpeg_pipe_source::FfmpegPipeSource& ff_b,
                  std::atomic<bool>& running) {
    // CPU read-out plan. We gbm_bo_map per frame after rendering; some Mesa
    // drivers (radeonsi) treat the mapping as a single-shot snapshot, so
    // reusing a one-time map returns stale data. egl-probe.cpp uses the
    // same per-frame pattern.
    const size_t bytes_per_frame = static_cast<size_t>(args.canvas_w) * args.canvas_h * 4;
    fprintf(stderr, "ok: canvas %dx%d ready, %zu bytes/frame\n", args.canvas_w, args.canvas_h,
            bytes_per_frame);

    std::map<int, SourceImagePair> img_cache;
    auto get_img = [&](const FrameView& v) -> SourceImagePair {
        if (v.fd < 0)
            return {};
        auto it = img_cache.find(v.fd);
        if (it != img_cache.end())
            return it->second;
        SourceImagePair p = import_frame_(ctx, v);
        if (p.y != EGL_NO_IMAGE && p.uv != EGL_NO_IMAGE)
            img_cache[v.fd] = p;
        return p;
    };

    auto start = std::chrono::steady_clock::now();
    auto frame_period = std::chrono::nanoseconds(1'000'000'000LL / args.fps);
    int frames_rendered = 0;

    while (running.load()) {
        if (args.run_seconds > 0 &&
            std::chrono::steady_clock::now() - start > std::chrono::seconds(args.run_seconds))
            break;

        std::vector<gl_compose::SourceSlot> slots;
        FrameView fv_a{}, fv_b{};
        if (args.a_enabled) {
            fv_a = args.a_is_scm ? to_canonical_(scm_a.latest_frame())
                                 : to_canonical_(ff_a.latest_frame());
        }
        if (args.b_enabled) {
            fv_b = args.b_is_scm ? to_canonical_(scm_b.latest_frame())
                                 : to_canonical_(ff_b.latest_frame());
        }

        if (args.a_enabled && args.b_enabled) {
            SourceImagePair pa = get_img(fv_a);
            SourceImagePair pb = get_img(fv_b);
            gl_compose::SourceSlot s0{};
            s0.src_y_image = pa.y;
            s0.src_uv_image = pa.uv;
            s0.x = 0;
            s0.y = 0;
            s0.w = args.canvas_w / 2;
            s0.h = args.canvas_h;
            s0.warp = {{1.0f, 0.0f, 0.0f, 0.0f, 1.0f, 0.0f, 0.0f, -0.3f, 1.0f}};
            slots.push_back(s0);
            gl_compose::SourceSlot s1{};
            s1.src_y_image = pb.y;
            s1.src_uv_image = pb.uv;
            s1.x = args.canvas_w / 2;
            s1.y = 0;
            s1.w = args.canvas_w / 2;
            s1.h = args.canvas_h;
            slots.push_back(s1);
        } else {
            SourceImagePair p = args.a_enabled ? get_img(fv_a) : get_img(fv_b);
            gl_compose::SourceSlot s0{};
            s0.src_y_image = p.y;
            s0.src_uv_image = p.uv;
            s0.x = 0;
            s0.y = 0;
            s0.w = args.canvas_w;
            s0.h = args.canvas_h;
            slots.push_back(s0);
        }

        if (!compose.render(slots)) {
            fprintf(stderr, "FAIL render\n");
            break;
        }
        compose.finish();

        // Map → read → unmap per frame. radeonsi treats gbm_bo_map as a
        // single-shot snapshot; a stale long-lived map returns zeros even
        // with glFinish + DMA_BUF_IOCTL_SYNC. Cost: one ioctl roundtrip
        // per frame, acceptable for this CPU read-out path (the rig uses
        // RGA encode-direct from the same dma-buf and doesn't go through
        // here).
        uint32_t map_stride = 0;
        void* map_data = nullptr;
        void* canvas_map = gbm_bo_map(compose.canvas_bo(), 0, 0, args.canvas_w, args.canvas_h,
                                      GBM_BO_TRANSFER_READ, &map_stride, &map_data);
        if (!canvas_map) {
            fprintf(stderr, "FAIL gbm_bo_map canvas\n");
            break;
        }
        bool write_ok = true;
        if (map_stride == static_cast<uint32_t>(args.canvas_w) * 4) {
            write_ok = write_full_(
                STDOUT_FILENO,
                std::span(static_cast<const uint8_t*>(canvas_map), bytes_per_frame));
        } else {
            for (int y = 0; y < args.canvas_h; ++y) {
                const uint8_t* row = static_cast<const uint8_t*>(canvas_map) + y * map_stride;
                if (!write_full_(STDOUT_FILENO,
                                 std::span(row, static_cast<size_t>(args.canvas_w) * 4))) {
                    write_ok = false;
                    break;
                }
            }
        }
        gbm_bo_unmap(compose.canvas_bo(), map_data);
        if (!write_ok) {
            running.store(false);
            break;
        }
        ++frames_rendered;

        if ((frames_rendered % args.fps) == 0) {
            auto now = std::chrono::steady_clock::now();
            double elapsed = std::chrono::duration<double>(now - start).count();
            fprintf(stderr, "[%6.1fs] rendered=%d (%.1f fps)\n", elapsed, frames_rendered,
                    frames_rendered / elapsed);
        }

        auto next = start + frame_period * frames_rendered;
        std::this_thread::sleep_until(next);
    }

    for (auto& kv : img_cache) {
        if (kv.second.y != EGL_NO_IMAGE)
            eglDestroyImage(ctx.display(), kv.second.y);
        if (kv.second.uv != EGL_NO_IMAGE)
            eglDestroyImage(ctx.display(), kv.second.uv);
    }

    return frames_rendered;
}

} // namespace render
