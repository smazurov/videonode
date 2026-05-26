#include "src/render/canvas_loop.hpp"

#include "src/common/log_levels.hpp"
#include "src/ipc/dmabuf_header.hpp"
#include "src/ipc/scm_rights_producer.hpp"
#include "src/ipc/scm_rights_source.hpp"
#include "src/render/composer_service.hpp"
#include "src/render/egl_ctx.hpp"
#include "src/render/gbm_alloc.hpp"
#include "src/render/pl_compose.hpp"
#include "src/render/world.hpp"
#include "src/snapshot/snapshot.hpp"

#include <drm_fourcc.h>
#include <gbm.h>
#include <poll.h>

#include <algorithm>
#include <cerrno>
#include <chrono>
#include <cstdint>
#include <cstdio>
#include <cstring>
#include <map>
#include <memory>
#include <span>
#include <string>
#include <thread>
#include <unistd.h>
#include <vector>

namespace render {

namespace {

bool write_full_(int fd, std::span<const uint8_t> buf) {
    while (!buf.empty()) {
        ssize_t w = ::write(fd, buf.data(), buf.size());
        if (w < 0) {
            if (errno == EINTR)
                continue;
            if (errno == EPIPE)
                return false; // consumer hung up; clean exit
            vn::log::error("canvas_loop: write stdout: %s", strerror(errno));
            return false;
        }
        buf = buf.subspan(static_cast<size_t>(w));
    }
    return true;
}

// Reconcile our per-slot ScmRightsSource pool with the current snapshot.
// Constructs new sources for new slot+scm_path pairs; tears down sources
// for slots that disappeared or had their scm_path swapped. Cheap when
// the snapshot is steady (no allocation).
struct LiveSource {
    std::unique_ptr<scm_rights_source::ScmRightsSource> src;
    std::string scm_path;
};

void reconcile_sources_(std::map<std::string, LiveSource>& live,
                        const std::vector<SlotBinding>& bindings) {
    std::vector<std::string> to_drop;
    for (const auto& [slot, ls] : live) {
        auto it = std::find_if(bindings.begin(), bindings.end(),
                               [&](const SlotBinding& b) { return b.slot == slot; });
        if (it == bindings.end() || it->scm_path != ls.scm_path)
            to_drop.push_back(slot);
    }
    for (const auto& slot : to_drop) {
        auto it = live.find(slot);
        if (it != live.end()) {
            if (it->second.src)
                it->second.src->stop();
            live.erase(it);
            vn::log::info("canvas_loop: dropped slot %s", slot.c_str());
        }
    }
    for (const auto& b : bindings) {
        if (live.find(b.slot) != live.end())
            continue;
        auto s = std::make_unique<scm_rights_source::ScmRightsSource>();
        scm_rights_source::InitParams p;
        p.socket_path = b.scm_path;
        p.dial = true; // composer is the consumer
        if (!s->init(p)) {
            vn::log::error("canvas_loop: init slot %s (%s) failed", b.slot.c_str(),
                           b.scm_path.c_str());
            continue;
        }
        if (!s->start()) {
            vn::log::error("canvas_loop: start slot %s (%s) failed", b.slot.c_str(),
                           b.scm_path.c_str());
            continue;
        }
        LiveSource ls;
        ls.src = std::move(s);
        ls.scm_path = b.scm_path;
        live[b.slot] = std::move(ls);
        vn::log::info("canvas_loop: dialed slot %s -> %s (%s)", b.slot.c_str(), b.source_id.c_str(),
                      b.scm_path.c_str());
    }
}

// Write a solid-black BGRA canvas to stdout; keeps the downstream pipe alive
// before World is ready.
bool write_black_canvas_(int w, int h) {
    if (w <= 0 || h <= 0) {
        std::this_thread::sleep_for(std::chrono::milliseconds(33));
        return true;
    }
    static thread_local std::vector<uint8_t> black;
    size_t need = size_t(w) * size_t(h) * 4;
    if (black.size() != need)
        black.assign(need, 0);
    return write_full_(STDOUT_FILENO, std::span(black.data(), black.size()));
}

// Broadcast the canvas BGRA dma-buf to all connected SCM consumers.
bool broadcast_canvas_(scm_rights_producer::ScmRightsProducer& prod, int canvas_fd, int width,
                       int height, uint32_t stride, uint64_t frame_idx) {
    dmabuf_header::Header h;
    h.slot_index = 0;
    h.width = uint32_t(width);
    h.height = uint32_t(height);
    h.format = "BGRA";
    h.plane_pitches = {stride};
    h.plane_offsets = {0};
    h.color_matrix = dmabuf_header::ColorMatrix::Bt709;
    h.color_range = dmabuf_header::ColorRange::Full;
    h.chroma_siting = dmabuf_header::ChromaSiting::Mpeg2;
    h.frame_idx = frame_idx;
    return prod.broadcast(h, {canvas_fd});
}

// Build SourceSlots for the pl_compose render call from the current snapshot.
std::vector<pl_compose::SourceSlot>
build_render_slots_(const Snapshot& snap, const std::map<std::string, LiveSource>& live_sources) {
    std::vector<pl_compose::SourceSlot> render_slots;
    render_slots.reserve(snap.layout.size());
    for (const auto& rect : snap.layout) {
        auto bit = std::find_if(snap.slots.begin(), snap.slots.end(),
                                [&](const SlotBinding& b) { return b.slot == rect.slot; });
        if (bit == snap.slots.end())
            continue;
        auto lit = live_sources.find(rect.slot);
        if (lit == live_sources.end() || !lit->second.src)
            continue;
        auto fv = lit->second.src->latest_frame();
        if (fv.fd < 0 || fv.frame_idx == 0)
            continue;
        if (fv.plane1_fd < 0 || fv.width <= 0 || fv.height <= 0)
            continue;

        pl_compose::SourceSlot s;
        s.src_y_fd = fv.fd;
        s.src_uv_fd = fv.plane1_fd;
        s.src_w = fv.width;
        s.src_h = fv.height;
        s.src_y_pitch = fv.plane0_pitch ? int(fv.plane0_pitch) : fv.width;
        s.src_uv_pitch = fv.plane1_pitch ? int(fv.plane1_pitch) : fv.width;
        s.x = rect.x;
        s.y = rect.y;
        s.w = rect.w;
        s.h = rect.h;
        s.rotation = rect.rotation;
        auto sit = snap.source_states.find(bit->source_id);
        if (sit != snap.source_states.end()) {
            const auto& ss = sit->second;
            if (ss.state != "placeholder" && ss.has_perspective)
                std::memcpy(s.warp.m, ss.warp.data(), 9 * sizeof(float));
        }
        render_slots.push_back(s);
    }
    return render_slots;
}

// Write canvas rows to stdout (stride-aware). Returns false on write failure.
bool write_canvas_stdout_(void* canvas_map, int compose_w, int compose_h, uint32_t map_stride) {
    std::span<const uint8_t> canvas_span(static_cast<const uint8_t*>(canvas_map),
                                         size_t(compose_h) * map_stride);
    size_t width_bytes = size_t(compose_w) * 4;
    if (map_stride == uint32_t(compose_w) * 4)
        return write_full_(STDOUT_FILENO, canvas_span.subspan(0, width_bytes * size_t(compose_h)));
    for (int y = 0; y < compose_h; ++y) {
        if (!write_full_(STDOUT_FILENO, canvas_span.subspan(size_t(y) * map_stride, width_bytes)))
            return false;
    }
    return true;
}

// Notify the ComposerService of the latest canvas frame (SCM mode only).
void notify_composer_svc_(nativerpc::ComposerService* composer_svc, int canvas_dmabuf_fd,
                          int compose_w, int compose_h, uint32_t stride, uint64_t frame_idx) {
    vn::snapshot::FrameRef r{};
    r.format = vn::snapshot::Format::Bgra;
    r.width = static_cast<uint32_t>(compose_w);
    r.height = static_cast<uint32_t>(compose_h);
    r.pitch_y = stride;
    r.planes[0] = {.fd = canvas_dmabuf_fd,
                   .offset = 0,
                   .pitch = stride,
                   .row_bytes = size_t(compose_w) * 4,
                   .rows = size_t(compose_h)};
    r.frame_idx = frame_idx;
    r.captured_at_ns =
        static_cast<uint64_t>(std::chrono::duration_cast<std::chrono::nanoseconds>(
                                  std::chrono::steady_clock::now().time_since_epoch())
                                  .count());
    composer_svc->UpdateLatestCanvas(r);
}

// Mutable state for the pl_compose instance and its associated dma-buf fd.
struct ComposeState {
    std::unique_ptr<pl_compose::PlCompose> compose;
    int w = 0;
    int h = 0;
    int canvas_dmabuf_fd = -1;
};

// Initialize the lazy pl_compose on the first ready snapshot.
// Returns false on failure (caller should skip the frame and retry).
bool init_compose_(ComposeState& cs, scm_rights_producer::ScmRightsProducer* scm_out,
                   std::atomic<bool>& running, const Snapshot& snap, RenderStats* stats) {
    cs.compose = std::make_unique<pl_compose::PlCompose>();
    const char* drm_candidates[] = {"/dev/dri/renderD128", "/dev/dri/renderD129",
                                    "/dev/dri/renderD130"};
    bool inited = false;
    for (const char* d : drm_candidates) {
        if (cs.compose->init(d, int(snap.canvas_w), int(snap.canvas_h))) {
            inited = true;
            break;
        }
    }
    if (!inited) {
        vn::log::error("canvas_loop: pl_compose init %dx%d failed", int(snap.canvas_w),
                       int(snap.canvas_h));
        cs.compose.reset();
        return false;
    }
    cs.w = int(snap.canvas_w);
    cs.h = int(snap.canvas_h);
    vn::log::info("canvas_loop: ready canvas %dx%d (%u fps)", cs.w, cs.h, snap.canvas_fps);
    if (stats) {
        stats->canvas_w.store(snap.canvas_w, std::memory_order_relaxed);
        stats->canvas_h.store(snap.canvas_h, std::memory_order_relaxed);
        stats->canvas_fps.store(snap.canvas_fps, std::memory_order_relaxed);
    }
    if (scm_out) {
        std::lock_guard<std::mutex> g(gbm_alloc::gbm_device_mu());
        cs.canvas_dmabuf_fd = gbm_bo_get_fd(cs.compose->canvas_bo());
        if (cs.canvas_dmabuf_fd < 0) {
            vn::log::fatal("canvas_loop: gbm_bo_get_fd failed; cannot SCM-broadcast");
            running.store(false);
            return false;
        }
    }
    return true;
}

// Prune dead SCM consumers and log consumer-count transitions.
void prune_consumers_(scm_rights_producer::ScmRightsProducer& scm_out, int& prev_consumer_count,
                      std::chrono::steady_clock::time_point& next_consumer_prune,
                      RenderStats* stats) {
    (void)scm_out.prune_dead_consumers();
    int cur = scm_out.consumer_count();
    if (cur == 0 && prev_consumer_count > 0) {
        vn::log::warn("canvas_loop: all SCM consumers dropped (was %d); "
                      "composer still rendering, waiting for re-dial",
                      prev_consumer_count);
    } else if (cur > 0 && prev_consumer_count == 0) {
        vn::log::info("canvas_loop: SCM consumer connected (count=%d)", cur);
    }
    prev_consumer_count = cur;
    if (stats)
        stats->consumer_count.store(cur, std::memory_order_relaxed);
    next_consumer_prune = std::chrono::steady_clock::now() + std::chrono::seconds(1);
}

// Render one frame in SCM mode: broadcast dma-buf, notify composer service.
bool render_scm_frame_(ComposeState& cs, scm_rights_producer::ScmRightsProducer& scm_out,
                       uint64_t broadcast_frame_idx, nativerpc::ComposerService* composer_svc) {
    uint32_t stride = 0;
    {
        std::lock_guard<std::mutex> g(gbm_alloc::gbm_device_mu());
        stride = gbm_bo_get_stride(cs.compose->canvas_bo());
    }
    (void)broadcast_canvas_(scm_out, cs.canvas_dmabuf_fd, cs.w, cs.h, stride, broadcast_frame_idx);
    if (composer_svc)
        notify_composer_svc_(composer_svc, cs.canvas_dmabuf_fd, cs.w, cs.h, stride,
                             broadcast_frame_idx);
    return true;
}

// Render one frame in stdout mode: mmap, write rows, unmap.
bool render_stdout_frame_(ComposeState& cs) {
    uint32_t map_stride = 0;
    void* map_data = nullptr;
    void* canvas_map = nullptr;
    {
        std::lock_guard<std::mutex> g(gbm_alloc::gbm_device_mu());
        canvas_map = gbm_bo_map(cs.compose->canvas_bo(), 0, 0, cs.w, cs.h, GBM_BO_TRANSFER_READ,
                                &map_stride, &map_data);
    }
    if (!canvas_map) {
        vn::log::error("canvas_loop: gbm_bo_map failed");
        return false;
    }
    bool ok = write_canvas_stdout_(canvas_map, cs.w, cs.h, map_stride);
    {
        std::lock_guard<std::mutex> g(gbm_alloc::gbm_device_mu());
        gbm_bo_unmap(cs.compose->canvas_bo(), map_data);
    }
    return ok;
}

// Update fps_observed_centi once per second.
void update_fps_stats_(RenderStats& stats, int frames_rendered, int& frames_at_last_sample,
                       std::chrono::steady_clock::time_point& next_fps_sample) {
    auto now = std::chrono::steady_clock::now();
    if (now < next_fps_sample)
        return;
    int delta = frames_rendered - frames_at_last_sample;
    frames_at_last_sample = frames_rendered;
    next_fps_sample = now + std::chrono::seconds(1);
    stats.fps_observed_centi.store(uint32_t(delta * 100), std::memory_order_relaxed);
}

// Initialize SCM output producer. Returns false on fatal init failure.
bool init_scm_out_(std::unique_ptr<scm_rights_producer::ScmRightsProducer>& scm_out,
                   const std::string& scm_out_path) {
    scm_out = std::make_unique<scm_rights_producer::ScmRightsProducer>();
    scm_rights_producer::InitParams pp;
    pp.socket_path = scm_out_path;
    if (!scm_out->init(pp) || !scm_out->start()) {
        vn::log::fatal("canvas_loop: SCM output bind failed on %s", scm_out_path.c_str());
        return false;
    }
    vn::log::info("canvas_loop: SCM output listening on %s", scm_out_path.c_str());
    return true;
}

// All mutable per-loop state collected so helper functions take fewer params.
struct LoopState {
    ComposeState cs;
    scm_rights_producer::ScmRightsProducer* scm_out = nullptr; // non-owning view
    int frames_rendered = 0;
    int frames_at_last_sample = 0;
    std::chrono::steady_clock::time_point next_fps_sample;
    uint64_t broadcast_frame_idx = 0;
    std::chrono::steady_clock::time_point next_consumer_prune;
    int prev_consumer_count = 0;
    std::chrono::steady_clock::time_point next_tick;
};

std::chrono::nanoseconds fps_period_(int fps) {
    return std::chrono::nanoseconds(fps > 0 ? 1'000'000'000LL / fps : 1'000'000'000LL / 30);
}

// Advance next_tick by period; rebase if we fell behind.
void advance_tick_(LoopState& ls, std::chrono::nanoseconds period) {
    ls.next_tick += period;
    if (ls.next_tick < std::chrono::steady_clock::now())
        ls.next_tick = std::chrono::steady_clock::now() + period;
}

// Account for one rendered frame: update counter and fps sample.
void count_frame_(LoopState& ls, RenderStats* stats) {
    ++ls.frames_rendered;
    if (stats)
        stats->frames_rendered.store(uint64_t(ls.frames_rendered), std::memory_order_relaxed);
}

// Tick fps sample and consumer prune, then advance the tick clock.
void post_render_tick_(LoopState& ls, RenderStats* stats, int fps) {
    if (ls.scm_out && std::chrono::steady_clock::now() >= ls.next_consumer_prune)
        prune_consumers_(*ls.scm_out, ls.prev_consumer_count, ls.next_consumer_prune, stats);

    if (stats)
        update_fps_stats_(*stats, ls.frames_rendered, ls.frames_at_last_sample, ls.next_fps_sample);

    advance_tick_(ls, fps_period_(fps));
}

// Handle a not-ready tick: write black (stdout mode) or broadcast black
// canvas (SCM mode). Returns false on fatal write/render error.
bool handle_not_ready_(LoopState& ls, const Snapshot& snap, int target_fps, RenderStats* stats,
                       nativerpc::ComposerService* composer_svc) {
    if (!ls.scm_out) {
        int w = ls.cs.compose ? ls.cs.w : (snap.canvas_w ? int(snap.canvas_w) : 1280);
        int h = ls.cs.compose ? ls.cs.h : (snap.canvas_h ? int(snap.canvas_h) : 720);
        if (!write_black_canvas_(w, h))
            return false;
    } else if (ls.cs.compose) {
        if (!ls.cs.compose->render({}))
            return false;
        ls.cs.compose->finish();
        render_scm_frame_(ls.cs, *ls.scm_out, ++ls.broadcast_frame_idx, composer_svc);
    }
    count_frame_(ls, stats);
    int fps = snap.canvas_fps ? int(snap.canvas_fps) : target_fps;
    ls.next_tick += fps_period_(fps);
    return true;
}

// Log a one-time warning when canvas dims change mid-stream (STUB: recreate
// not yet implemented).
void warn_dims_changed_(int old_w, int old_h, uint32_t new_w, uint32_t new_h) {
    static bool warned = false;
    if (warned)
        return;
    vn::log::warn("canvas_loop: canvas dims changed %dx%d -> %ux%u; "
                  "recreate not implemented, ignoring (STUB)",
                  old_w, old_h, new_w, new_h);
    warned = true;
}

// Ensure pl_compose is initialized for the current snapshot.
// Returns false if compose init failed (caller should skip and retry).
// Returns true with cs.compose valid on success.
bool ensure_compose_(LoopState& ls, std::atomic<bool>& running, const Snapshot& snap,
                     RenderStats* stats, int target_fps) {
    if (ls.cs.compose) {
        if (int(snap.canvas_w) != ls.cs.w || int(snap.canvas_h) != ls.cs.h)
            warn_dims_changed_(ls.cs.w, ls.cs.h, snap.canvas_w, snap.canvas_h);
        return true;
    }
    if (init_compose_(ls.cs, ls.scm_out, running, snap, stats))
        return true;
    ls.next_tick += fps_period_(target_fps);
    return false;
}

// Render one frame (GL + output). Returns false on fatal render/write error.
bool render_frame_(LoopState& ls, const Snapshot& snap,
                   std::map<std::string, LiveSource>& live_sources,
                   nativerpc::ComposerService* composer_svc) {
    auto render_slots = build_render_slots_(snap, live_sources);
    if (!ls.cs.compose->render(render_slots)) {
        vn::log::error("canvas_loop: render failed");
        return false;
    }
    ls.cs.compose->finish();
    return ls.scm_out
               ? render_scm_frame_(ls.cs, *ls.scm_out, ++ls.broadcast_frame_idx, composer_svc)
               : render_stdout_frame_(ls.cs);
}

} // namespace

int RunCanvasLoop(CanvasLoopConfig cfg) {
    (void)cfg.ctx; // pl_compose manages its own EGL context
    auto start = std::chrono::steady_clock::now();

    std::unique_ptr<scm_rights_producer::ScmRightsProducer> scm_out_owned;
    if (!cfg.scm_out_path.empty() && !init_scm_out_(scm_out_owned, cfg.scm_out_path))
        return 1;

    std::map<std::string, LiveSource> live_sources;
    LoopState ls;
    ls.scm_out = scm_out_owned.get();
    ls.next_fps_sample = start + std::chrono::seconds(1);
    ls.next_tick = std::chrono::steady_clock::now();
    ls.next_consumer_prune = ls.next_tick;

    while (cfg.running.load()) {
        if (cfg.run_seconds > 0 &&
            std::chrono::steady_clock::now() - start > std::chrono::seconds(cfg.run_seconds))
            break;

        std::this_thread::sleep_until(ls.next_tick);
        Snapshot snap = cfg.world.snapshot();

        // Init compose as soon as canvas dims are known (before ready).
        if (snap.canvas_w > 0 && snap.canvas_h > 0) {
            if (!ensure_compose_(ls, cfg.running, snap, cfg.stats, cfg.target_fps))
                continue;
        }

        if (!snap.ready) {
            if (!handle_not_ready_(ls, snap, cfg.target_fps, cfg.stats, cfg.composer_svc)) {
                cfg.running.store(false);
                break;
            }
            continue;
        }

        reconcile_sources_(live_sources, snap.slots);

        if (!render_frame_(ls, snap, live_sources, cfg.composer_svc)) {
            cfg.running.store(false);
            break;
        }
        count_frame_(ls, cfg.stats);
        int fps = snap.canvas_fps ? int(snap.canvas_fps) : cfg.target_fps;
        post_render_tick_(ls, cfg.stats, fps);
    }

    for (auto& [_, lsrc] : live_sources)
        if (lsrc.src)
            lsrc.src->stop();
    if (scm_out_owned)
        scm_out_owned->stop();
    if (ls.cs.canvas_dmabuf_fd >= 0)
        ::close(ls.cs.canvas_dmabuf_fd);
    return ls.frames_rendered;
}

} // namespace render
