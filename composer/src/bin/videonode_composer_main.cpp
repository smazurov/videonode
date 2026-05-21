// videonode-composer — capture + GPU-compose, write BGRA canvas frames to stdout.
//
// The encoder is NOT part of this binary. We're a frame producer; consumers
// pipe our stdout into ffmpeg (or anything else). Example pipeline:
/*
    videonode-composer --canvas-w 1920 --canvas-h 1080 --fps 60 \
      | ffmpeg -f rawvideo -pix_fmt bgra -s 1920x1080 -framerate 30 -i pipe:0 \
               -c:v h264_rkmpp -profile:v high -level:v 5.2 -rc_mode VBR \
               -b:v 6M -g 60 -bf 0 -bsf:v dump_extra=freq=keyframe \
               -rtsp_transport tcp -f rtsp rtsp://127.0.0.1:8554/spike
*/
//
// Why no encoder here:
//   - Cross-egress isolation is the architecture's main selling point.
//     Per-egress encoders live in their own processes, supervised by the
//     parent. The composer's job is one composed frame stream; the parent
//     fans that out.
//   - h264_rkmpp on the rig + libx264 on a dev machine is a one-line shell
//     change; not worth a code branch.
//   - Backpressure is just the Unix pipe. ffmpeg slow → write() blocks →
//     compose loop sleeps. No torn frames, no fancy plumbing.
//
// Pipeline shape:
//   1. Per source: child ffmpeg captures (V4L2 or lavfi) into a ring of
//      dma-heap dma-buf NV12 buffers (FfmpegPipeSource).
//   2. Each tick, snapshot "latest frame" from each source.
//   3. GLES2 shaders compose source quads into a BGRA canvas dma-buf
//      (GlCompose), with optional per-source 3x3 perspective warp.
//   4. mmap the canvas dma-buf, fwrite BGRA bytes to stdout. Done.

#include "src/process/ffmpeg_pipe_source.hpp"
#include "src/ipc/scm_rights_source.hpp"
#include "src/render/format_dispatch.hpp"
#include "src/render/egl_ctx.hpp"
#include "src/render/gl_compose.hpp"
#include "src/ipc/dma_heap.hpp"
#include "version.hpp"

#include <EGL/egl.h>
#include <GLES2/gl2.h>
#include <drm_fourcc.h>
#include <gbm.h>

#include <atomic>
#include <chrono>
#include <csignal>
#include <cstdint>
#include <cstdio>
#include <cstdlib>
#include <cstring>
#include <map>
#include <span>
#include <string>
#include <sys/mman.h>
#include <sys/prctl.h>
#include <unistd.h>
#include <thread>
#include <unistd.h>
#include <vector>

namespace {

std::atomic<bool> g_running{true};
void on_signal(int) {
    g_running.store(false);
}

struct SourceArgs {
    bool enabled = true;
    bool testsrc = false;     // use lavfi testsrc2 instead of V4L2
    std::string device;       // /dev/videoN (V4L2 mode)
    std::string input_format; // "nv12" / "mjpeg" / "yuyv422"
    int width = 1920;
    int height = 1080;
    int fps = 60;
    // If non-empty, this slot consumes dma-buf fds the Go daemon hands over
    // SCM_RIGHTS instead of spawning its own child ffmpeg. Path is the
    // Unix socket the composer listens on; daemon connects to it.
    std::string scm_socket_path;
};

struct Args {
    std::string drm_device = "/dev/dri/renderD128"; // common default; rig override below
    int canvas_w = 1920;
    int canvas_h = 1080;
    int fps = 60;
    int run_seconds = 0; // 0 = run until SIGINT or stdout EPIPE
    SourceArgs source_a;
    SourceArgs source_b;
};

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
    std::string format; // DRM fourcc string; empty → default NV12
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

// Two single-plane EGLImages per NV12 frame: Y as R8, UV as GR88. Each
// plane has its own dma-buf fd at PLANE0_OFFSET=0 — the only pattern
// that reliably samples on radeonsi (per minigbm/Chromium AMD path).
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
    dy.plane0_offset = 0;
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
    duv.plane0_offset = 0;
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

bool start_scm_source_(scm_rights_source::ScmRightsSource& s, const SourceArgs& a,
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

bool start_ffmpeg_source_(ffmpeg_pipe_source::FfmpegPipeSource& s, const SourceArgs& a,
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

int main(int argc, char** argv) {
    Args a;

    // Sensible per-platform defaults: on the rig, source A is HDMI-IN at
    // 4K NV12 and source B is the Lyra at 1080p MJPEG. Override per slot
    // via CLI flags below.
    a.source_a.device = "/dev/video0";
    a.source_a.input_format = "nv12";
    a.source_a.width = 3840;
    a.source_a.height = 2160;
    a.source_a.fps = 60;

    a.source_b.device = "/dev/video1";
    a.source_b.input_format = "mjpeg";
    a.source_b.width = 1920;
    a.source_b.height = 1080;
    a.source_b.fps = 60;

    auto eat_int = [&](int i, int& dst) -> int {
        if (i + 1 < argc) {
            dst = std::atoi(argv[i + 1]);
            return i + 1;
        }
        return i;
    };
    auto eat_str = [&](int i, std::string& dst) -> int {
        if (i + 1 < argc) {
            dst = argv[i + 1];
            return i + 1;
        }
        return i;
    };
    for (int i = 1; i < argc; ++i) {
        std::string s = argv[i];
        if (s == "--canvas-w")
            i = eat_int(i, a.canvas_w);
        else if (s == "--canvas-h")
            i = eat_int(i, a.canvas_h);
        else if (s == "--fps")
            i = eat_int(i, a.fps);
        else if (s == "--seconds")
            i = eat_int(i, a.run_seconds);
        else if (s == "--drm-device")
            i = eat_str(i, a.drm_device);

        else if (s == "--no-source-a")
            a.source_a.enabled = false;
        else if (s == "--source-a-testsrc")
            a.source_a.testsrc = true;
        else if (s == "--source-a-device")
            i = eat_str(i, a.source_a.device);
        else if (s == "--source-a-format")
            i = eat_str(i, a.source_a.input_format);
        else if (s == "--source-a-width")
            i = eat_int(i, a.source_a.width);
        else if (s == "--source-a-height")
            i = eat_int(i, a.source_a.height);
        else if (s == "--source-a-fps")
            i = eat_int(i, a.source_a.fps);
        else if (s == "--source-a-scm-path")
            i = eat_str(i, a.source_a.scm_socket_path);

        else if (s == "--no-source-b")
            a.source_b.enabled = false;
        else if (s == "--source-b-testsrc")
            a.source_b.testsrc = true;
        else if (s == "--source-b-device")
            i = eat_str(i, a.source_b.device);
        else if (s == "--source-b-format")
            i = eat_str(i, a.source_b.input_format);
        else if (s == "--source-b-width")
            i = eat_int(i, a.source_b.width);
        else if (s == "--source-b-height")
            i = eat_int(i, a.source_b.height);
        else if (s == "--source-b-fps")
            i = eat_int(i, a.source_b.fps);
        else if (s == "--source-b-scm-path")
            i = eat_str(i, a.source_b.scm_socket_path);

        else if (s == "-h" || s == "--help") {
            Args d; // defaults
            printf(
                "videonode-composer — write BGRA canvas frames to stdout.\n"
                "  --canvas-w W                          (default %d)\n"
                "  --canvas-h H                          (default %d)\n"
                "  --fps N                               (default %d)\n"
                "  --seconds N                           (default %d = until SIGINT or stdout "
                "EPIPE)\n"
                "  --drm-device PATH                     (default %s)\n"
                "  --source-{a,b}-testsrc                use lavfi testsrc2 instead of V4L2\n"
                "  --source-{a,b}-device DEV             V4L2 device path (a=%s b=%s)\n"
                "  --source-{a,b}-format FMT             V4L2 input pixel format (nv12 / mjpeg / "
                "yuyv422)\n"
                "  --source-{a,b}-width W                source width  (default a=%d b=%d)\n"
                "  --source-{a,b}-height H               source height (default a=%d b=%d)\n"
                "  --source-{a,b}-fps N                  source fps    (default a=%d b=%d)\n"
                "  --source-{a,b}-scm-path PATH          dial videonode-source SCM socket instead "
                "of "
                "V4L2\n"
                "  --no-source-{a,b}                     disable that slot\n"
                "  --version                             print version and exit\n"
                "\n"
                "Stdout: rawvideo BGRA at canvas_w*canvas_h*4 bytes per frame at canvas fps.\n"
                "Pipe to ffmpeg with -f rawvideo -pix_fmt bgra -s WxH -framerate N -i pipe:0 ...\n",
                d.canvas_w, d.canvas_h, d.fps, d.run_seconds, d.drm_device.c_str(),
                d.source_a.device.c_str(), d.source_b.device.c_str(), d.source_a.width,
                d.source_b.width, d.source_a.height, d.source_b.height, d.source_a.fps,
                d.source_b.fps);
            return 0;
        } else if (s == "--version") {
            printf("videonode-composer %s\n", vn::kVersion);
            return 0;
        } else if (!s.empty() && s[0] == '-') {
            fprintf(stderr, "unknown flag: %s (use --help)\n", s.c_str());
            return 2;
        }
    }

    std::signal(SIGINT, on_signal);
    std::signal(SIGTERM, on_signal);
    std::signal(SIGPIPE, SIG_IGN); // we handle EPIPE explicitly
    // Note: not using PR_SET_PDEATHSIG here. Composer-spike sits inside
    // a `composer | ffmpeg` shell pipeline; bash forks a transient
    // subshell that exits right after exec, so PDEATHSIG would fire
    // immediately. Composer dies naturally via stdout EPIPE when ffmpeg
    // ends; that suffices for shutdown propagation.

    // ---------- 1. Captures (staggered: source A first, source B after first frame) ----------
    // Each slot picks one of two producer types: a child ffmpeg that captures
    // and writes dma-buf-backed NV12 frames to a ring (FfmpegPipeSource), or
    // a Unix socket on which the Go daemon hands over dma-buf fds via
    // SCM_RIGHTS (ScmRightsSource). The render loop reads through whichever
    // is active per slot.
    using ffmpeg_pipe_source::FfmpegPipeSource;
    using scm_rights_source::ScmRightsSource;

    // ---------- 1. EGL first. Declared BEFORE the source objects so the
    // destruction order is correct: sources hold gbm_bo handles owned by
    // the gbm_device inside ctx; ff_a / ff_b destructors run before
    // ~EglCtx and can still call gbm_bo_destroy. ----------
    egl_ctx::EglCtx ctx;
    if (!ctx.init(a.drm_device))
        return 1;

    FfmpegPipeSource ff_a, ff_b;
    ScmRightsSource scm_a, scm_b;
    const bool a_is_scm = a.source_a.enabled && !a.source_a.scm_socket_path.empty();
    const bool b_is_scm = a.source_b.enabled && !a.source_b.scm_socket_path.empty();

    if (a.source_a.enabled) {
        bool ok = a_is_scm ? start_scm_source_(scm_a, a.source_a, "source-a")
                           : start_ffmpeg_source_(ff_a, a.source_a, "source-a", ctx.gbm());
        if (!ok)
            return 1;
    }
    if (a.source_b.enabled) {
        bool ok = b_is_scm ? start_scm_source_(scm_b, a.source_b, "source-b")
                           : start_ffmpeg_source_(ff_b, a.source_b, "source-b", ctx.gbm());
        if (!ok)
            return 1;
    }

    // ---------- 2. compose. ----------
    gl_compose::GlCompose compose;
    if (!compose.init(ctx, a.canvas_w, a.canvas_h))
        return 1;
    fprintf(stderr, "ok: GLES canvas %dx%d via %s\n", a.canvas_w, a.canvas_h, a.drm_device.c_str());

    // ---------- 3. CPU read-out plan. We gbm_bo_map per frame after
    // rendering; some Mesa drivers (radeonsi) treat the mapping as a
    // single-shot snapshot, so reusing a one-time map returns stale data.
    // egl-probe.cpp uses the same per-frame pattern. ----------
    const size_t bytes_per_frame = static_cast<size_t>(a.canvas_w) * a.canvas_h * 4;
    fprintf(stderr, "ok: canvas %dx%d ready, %zu bytes/frame\n", a.canvas_w, a.canvas_h,
            bytes_per_frame);

    // ---------- 4. Render loop. ----------
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
    auto frame_period = std::chrono::nanoseconds(1'000'000'000LL / a.fps);
    int frames_rendered = 0;

    while (g_running.load()) {
        if (a.run_seconds > 0 &&
            std::chrono::steady_clock::now() - start > std::chrono::seconds(a.run_seconds))
            break;

        std::vector<gl_compose::SourceSlot> slots;
        FrameView fv_a{}, fv_b{};
        if (a.source_a.enabled) {
            fv_a =
                a_is_scm ? to_canonical_(scm_a.latest_frame()) : to_canonical_(ff_a.latest_frame());
        }
        if (a.source_b.enabled) {
            fv_b =
                b_is_scm ? to_canonical_(scm_b.latest_frame()) : to_canonical_(ff_b.latest_frame());
        }

        if (a.source_a.enabled && a.source_b.enabled) {
            SourceImagePair pa = get_img(fv_a);
            SourceImagePair pb = get_img(fv_b);
            gl_compose::SourceSlot s0{};
            s0.src_y_image = pa.y;
            s0.src_uv_image = pa.uv;
            s0.x = 0;
            s0.y = 0;
            s0.w = a.canvas_w / 2;
            s0.h = a.canvas_h;
            s0.warp = {{1.0f, 0.0f, 0.0f, 0.0f, 1.0f, 0.0f, 0.0f, -0.3f, 1.0f}};
            slots.push_back(s0);
            gl_compose::SourceSlot s1{};
            s1.src_y_image = pb.y;
            s1.src_uv_image = pb.uv;
            s1.x = a.canvas_w / 2;
            s1.y = 0;
            s1.w = a.canvas_w / 2;
            s1.h = a.canvas_h;
            slots.push_back(s1);
        } else {
            SourceImagePair p = a.source_a.enabled ? get_img(fv_a) : get_img(fv_b);
            gl_compose::SourceSlot s0{};
            s0.src_y_image = p.y;
            s0.src_uv_image = p.uv;
            s0.x = 0;
            s0.y = 0;
            s0.w = a.canvas_w;
            s0.h = a.canvas_h;
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
        void* canvas_map = gbm_bo_map(compose.canvas_bo(), 0, 0, a.canvas_w, a.canvas_h,
                                      GBM_BO_TRANSFER_READ, &map_stride, &map_data);
        if (!canvas_map) {
            fprintf(stderr, "FAIL gbm_bo_map canvas\n");
            break;
        }
        bool write_ok = true;
        if (map_stride == static_cast<uint32_t>(a.canvas_w) * 4) {
            write_ok = write_full_(
                STDOUT_FILENO,
                std::span(static_cast<const uint8_t*>(canvas_map), bytes_per_frame));
        } else {
            for (int y = 0; y < a.canvas_h; ++y) {
                const uint8_t* row = static_cast<const uint8_t*>(canvas_map) + y * map_stride;
                if (!write_full_(STDOUT_FILENO,
                                 std::span(row, static_cast<size_t>(a.canvas_w) * 4))) {
                    write_ok = false;
                    break;
                }
            }
        }
        gbm_bo_unmap(compose.canvas_bo(), map_data);
        if (!write_ok) {
            g_running.store(false);
            break;
        }
        ++frames_rendered;

        if ((frames_rendered % a.fps) == 0) {
            auto now = std::chrono::steady_clock::now();
            double elapsed = std::chrono::duration<double>(now - start).count();
            fprintf(stderr, "[%6.1fs] rendered=%d (%.1f fps)\n", elapsed, frames_rendered,
                    frames_rendered / elapsed);
        }

        auto next = start + frame_period * frames_rendered;
        std::this_thread::sleep_until(next);
    }

    fprintf(stderr, "shutting down\n");
    // canvas map_data is now scoped per-frame; no global unmap needed.
    for (auto& kv : img_cache) {
        if (kv.second.y != EGL_NO_IMAGE)
            eglDestroyImage(ctx.display(), kv.second.y);
        if (kv.second.uv != EGL_NO_IMAGE)
            eglDestroyImage(ctx.display(), kv.second.uv);
    }
    if (a.source_b.enabled) {
        if (b_is_scm)
            scm_b.stop();
        else
            ff_b.stop();
    }
    if (a.source_a.enabled) {
        if (a_is_scm)
            scm_a.stop();
        else
            ff_a.stop();
    }

    fprintf(stderr, "PASS: %d frames composed\n", frames_rendered);
    return 0;
}
