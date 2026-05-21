// canvas_loop — composer compose-render-stdout loop, factored out of
// videonode_composer_main.cpp so the binary stays argv-only.
//
// Owns:
//   - EGLImage cache keyed by source dma-buf fd (one Y + one UV image per fd)
//   - per-frame compose.render() + finish()
//   - per-frame gbm_bo_map() of the canvas + write() to stdout
//   - frame-rate sleep
//
// Does NOT own the EglCtx, GlCompose, or the source objects — those are
// constructed in main() so their destruction order stays correct
// (sources → GlCompose → EglCtx).
//
// The source-helper functions (`start_scm_source_`, `start_ffmpeg_source_`)
// live here too because they share the wait_first_frame_ template; main()
// calls them after constructing the source objects.

#pragma once

#include "src/render/egl_ctx.hpp"
#include "src/render/gl_compose.hpp"

#include <atomic>
#include <string>

struct gbm_device;

// Forward-declare the source types. Callers that actually construct
// ScmRightsSource / FfmpegPipeSource and pass them in already pull
// the full headers; declaring them here keeps render/ from depending
// on ipc/ and process/ for the function signatures alone.
namespace scm_rights_source {
class ScmRightsSource;
}
namespace ffmpeg_pipe_source {
class FfmpegPipeSource;
}

namespace render {

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

struct CanvasLoopArgs {
    int canvas_w = 1920;
    int canvas_h = 1080;
    int fps = 60;
    int run_seconds = 0; // 0 = run until `running` goes false
    bool a_enabled = false;
    bool a_is_scm = false;
    bool b_enabled = false;
    bool b_is_scm = false;
};

// Start an SCM-rights source: init + start + wait up to 30s for first frame.
// Returns false on any failure; the producer (videonode-source sidecar)
// listens on `a.scm_socket_path` and we dial it as one of N consumers.
bool StartScmSource(scm_rights_source::ScmRightsSource& s, const SourceArgs& a,
                    const char* tag);

// Start a child-ffmpeg source: init + start + wait up to 10s for first frame.
// Picks V4L2 or lavfi testsrc2 based on `a.testsrc`.
bool StartFfmpegSource(ffmpeg_pipe_source::FfmpegPipeSource& s, const SourceArgs& a,
                       const char* tag, gbm_device* gbm);

// Run the compose-render-stdout loop until run_seconds elapses, stdout EPIPE
// is hit, or `running` goes false. Returns the number of frames composed.
// Caller owns `ctx`, `compose`, and the source objects; only the source
// slots flagged enabled in `args` are read.
int RunCanvasLoop(const CanvasLoopArgs& args,
                  egl_ctx::EglCtx& ctx,
                  gl_compose::GlCompose& compose,
                  scm_rights_source::ScmRightsSource& scm_a,
                  scm_rights_source::ScmRightsSource& scm_b,
                  ffmpeg_pipe_source::FfmpegPipeSource& ff_a,
                  ffmpeg_pipe_source::FfmpegPipeSource& ff_b,
                  std::atomic<bool>& running);

} // namespace render
