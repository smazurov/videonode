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
// This file is argv + startup wiring only. The compose-render-stdout loop
// and the EGLImage import path live in src/render/canvas_loop.{cpp,hpp}.

#include "src/ipc/scm_rights_source.hpp"
#include "src/process/ffmpeg_pipe_source.hpp"
#include "src/render/canvas_loop.hpp"
#include "src/render/egl_ctx.hpp"
#include "src/render/gl_compose.hpp"
#include "version.hpp"

#include <atomic>
#include <csignal>
#include <cstdio>
#include <cstdlib>
#include <string>

namespace {

std::atomic<bool> g_running{true};
void on_signal(int) {
    g_running.store(false);
}

struct Args {
    std::string drm_device = "/dev/dri/renderD128"; // common default; rig override below
    int canvas_w = 1920;
    int canvas_h = 1080;
    int fps = 60;
    int run_seconds = 0; // 0 = run until SIGINT or stdout EPIPE
    render::SourceArgs source_a;
    render::SourceArgs source_b;
};

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

    // EGL first. Declared BEFORE the source objects so the destruction order
    // is correct: sources hold gbm_bo handles owned by the gbm_device inside
    // ctx; ff_a / ff_b destructors run before ~EglCtx and can still call
    // gbm_bo_destroy.
    egl_ctx::EglCtx ctx;
    if (!ctx.init(a.drm_device))
        return 1;

    ffmpeg_pipe_source::FfmpegPipeSource ff_a, ff_b;
    scm_rights_source::ScmRightsSource scm_a, scm_b;
    const bool a_is_scm = a.source_a.enabled && !a.source_a.scm_socket_path.empty();
    const bool b_is_scm = a.source_b.enabled && !a.source_b.scm_socket_path.empty();

    if (a.source_a.enabled) {
        bool ok = a_is_scm ? render::StartScmSource(scm_a, a.source_a, "source-a")
                           : render::StartFfmpegSource(ff_a, a.source_a, "source-a", ctx.gbm());
        if (!ok)
            return 1;
    }
    if (a.source_b.enabled) {
        bool ok = b_is_scm ? render::StartScmSource(scm_b, a.source_b, "source-b")
                           : render::StartFfmpegSource(ff_b, a.source_b, "source-b", ctx.gbm());
        if (!ok)
            return 1;
    }

    gl_compose::GlCompose compose;
    if (!compose.init(ctx, a.canvas_w, a.canvas_h))
        return 1;
    fprintf(stderr, "ok: GLES canvas %dx%d via %s\n", a.canvas_w, a.canvas_h, a.drm_device.c_str());

    render::CanvasLoopArgs la;
    la.canvas_w = a.canvas_w;
    la.canvas_h = a.canvas_h;
    la.fps = a.fps;
    la.run_seconds = a.run_seconds;
    la.a_enabled = a.source_a.enabled;
    la.a_is_scm = a_is_scm;
    la.b_enabled = a.source_b.enabled;
    la.b_is_scm = b_is_scm;

    int frames_rendered =
        render::RunCanvasLoop(la, ctx, compose, scm_a, scm_b, ff_a, ff_b, g_running);

    fprintf(stderr, "shutting down\n");
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
