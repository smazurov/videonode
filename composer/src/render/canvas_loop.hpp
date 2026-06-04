#pragma once

#include <array>
#include <atomic>
#include <cstdint>
#include <string>

namespace render {
class World;
} // namespace render
namespace nativerpc {
class ComposerService;
} // namespace nativerpc

namespace render {

// DRM render nodes probed in order when no usable --drm-device is given.
inline constexpr std::array<const char*, 3> kDrmRenderCandidates{
    "/dev/dri/renderD128", "/dev/dri/renderD129", "/dev/dri/renderD130"};

// Lock-free counters the canvas loop writes once per frame and the gRPC
// `Composer.GetStats` handler reads from another thread.
struct RenderStats {
    std::atomic<uint64_t> frames_rendered{0};
    std::atomic<uint32_t> fps_observed_centi{0}; // fps * 100, recomputed every ~1s
    std::atomic<uint32_t> canvas_w{0};
    std::atomic<uint32_t> canvas_h{0};
    std::atomic<uint32_t> canvas_fps{0};
    std::atomic<int32_t> consumer_count{0}; // SCM mode only; 0 in stdout mode
};

// `scm_out_path` selects the output mode: empty → BGRA to stdout (dies via
// EPIPE if the consumer exits); non-empty → SCM_RIGHTS broadcast of the
// canvas dma-buf, decoupling composer lifetime from any one consumer.
struct CanvasLoopConfig {
    World& world;
    int target_fps = 30;
    int run_seconds = 0;
    std::atomic<bool>& running;
    std::string scm_out_path;
    RenderStats* stats = nullptr;
    nativerpc::ComposerService* composer_svc = nullptr;
};

// Sentinel return from RunCanvasLoop: an unrecoverable runtime error (not
// GPU-absence; that's gated at startup). Distinct from a frame count (>= 0).
inline constexpr int kCanvasRuntimeError = -1;

int RunCanvasLoop(CanvasLoopConfig cfg);

} // namespace render
