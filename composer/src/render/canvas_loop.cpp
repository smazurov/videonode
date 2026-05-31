#include "src/render/canvas_loop.hpp"

#include "src/common/log_levels.hpp"
#include "src/ipc/dmabuf_header.hpp"
#include "src/ipc/scm_rights_producer.hpp"
#include "src/ipc/scm_rights_source.hpp"
#include "src/render/build_source_slot.hpp"
#include "src/render/composer_service.hpp"
#include "src/render/egl_ctx.hpp"
#include "src/render/gbm_alloc.hpp"
#include "src/render/pl_compose.hpp"
#include "src/render/world.hpp"
#include "src/snapshot/snapshot.hpp"

#include <drm_fourcc.h>
#include <gbm.h>
#include <linux/memfd.h>
#include <poll.h>
#include <sys/mman.h>
#include <sys/syscall.h>

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

struct RenderBatch {
    std::vector<pl_compose::SourceSlot> slots;
    std::vector<scm_rights_source::OwnedFrameView> owned_frames;
};

RenderBatch build_render_slots_(const Snapshot& snap,
                                const std::map<std::string, LiveSource>& live_sources) {
    RenderBatch batch;
    batch.slots.reserve(snap.layout.size());
    batch.owned_frames.reserve(snap.layout.size());
    for (const auto& rect : snap.layout) {
        auto bit = std::find_if(snap.slots.begin(), snap.slots.end(),
                                [&](const SlotBinding& b) { return b.slot == rect.slot; });
        if (bit == snap.slots.end())
            continue;
        auto lit = live_sources.find(rect.slot);
        if (lit == live_sources.end() || !lit->second.src)
            continue;
        auto fv = lit->second.src->latest_frame();
        if (fv.fd.get() < 0 || fv.frame_idx == 0)
            continue;
        if (fv.plane1_fd.get() < 0 || fv.width <= 0 || fv.height <= 0)
            continue;

        FrameGeom geom;
        geom.y_fd = fv.fd.get();
        geom.uv_fd = fv.plane1_fd.get();
        geom.width = fv.width;
        geom.height = fv.height;
        geom.y_pitch = int(fv.plane0_pitch);
        geom.uv_pitch = int(fv.plane1_pitch);
        geom.y_offset = int(fv.plane0_offset);
        geom.uv_offset = int(fv.plane1_offset);

        auto sit = snap.source_states.find(bit->source_id);
        const SourceState* state = sit != snap.source_states.end() ? &sit->second : nullptr;
        pl_compose::SourceSlot s = build_source_slot(geom, rect, state);
        batch.slots.push_back(s);
        batch.owned_frames.push_back(std::move(fv));
    }
    return batch;
}

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

struct ComposeState {
    std::unique_ptr<pl_compose::PlCompose> compose;
    int w = 0;
    int h = 0;
};

bool init_compose_(ComposeState& cs, const Snapshot& snap, RenderStats* stats) {
    cs.compose = std::make_unique<pl_compose::PlCompose>();
    bool inited = false;
    for (const char* d : kDrmRenderCandidates) {
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
    if (cs.compose->canvas_dmabuf_fd() < 0) {
        vn::log::error("canvas_loop: canvas dma-buf fd invalid after init");
        return false;
    }
    vn::log::info("canvas_loop: ready canvas %dx%d (%u fps)", cs.w, cs.h, snap.canvas_fps);
    if (stats) {
        stats->canvas_w.store(snap.canvas_w, std::memory_order_relaxed);
        stats->canvas_h.store(snap.canvas_h, std::memory_order_relaxed);
        stats->canvas_fps.store(snap.canvas_fps, std::memory_order_relaxed);
    }
    return true;
}

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

// Rotating pool of snapshot memfds broadcast via SCM_RIGHTS. Must be deep
// enough that a consumer finishes reading slot N before the pool wraps and
// overwrites N's pages.
struct SnapPool {
    static constexpr int kPoolSize = 3;
    struct Slot {
        int fd = -1;
        void* map = nullptr;
        size_t size = 0;
    };
    Slot slots[kPoolSize] = {};
    int next = 0;

    Slot& advance() {
        Slot& s = slots[next];
        next = (next + 1) % kPoolSize;
        return s;
    }

    bool ensure_slot(Slot& s, size_t needed) {
        if (s.map && s.size == needed)
            return true;
        if (s.map)
            ::munmap(s.map, s.size);
        if (s.fd >= 0)
            ::close(s.fd);
        s.fd = static_cast<int>(
            ::syscall(SYS_memfd_create, "canvas-snap", static_cast<unsigned int>(MFD_CLOEXEC)));
        if (s.fd < 0)
            return false;
        if (::ftruncate(s.fd, static_cast<off_t>(needed)) < 0) {
            ::close(s.fd);
            s.fd = -1;
            return false;
        }
        s.map = ::mmap(nullptr, needed, PROT_READ | PROT_WRITE, MAP_SHARED, s.fd, 0);
        if (s.map == MAP_FAILED) {
            s.map = nullptr;
            ::close(s.fd);
            s.fd = -1;
            return false;
        }
        s.size = needed;
        return true;
    }

    ~SnapPool() {
        for (auto& s : slots) {
            if (s.map)
                ::munmap(s.map, s.size);
            if (s.fd >= 0)
                ::close(s.fd);
        }
    }
};

void copy_canvas_to_(void* dst, const void* src, uint32_t dst_stride, uint32_t src_stride, int w,
                     int h) {
    if (src_stride == dst_stride) {
        std::memcpy(dst, src, size_t(dst_stride) * h);
        return;
    }
    auto* d = static_cast<uint8_t*>(dst);
    const auto* s = static_cast<const uint8_t*>(src);
    size_t row_bytes = size_t(w) * 4;
    for (int row = 0; row < h; ++row)
        std::memcpy(d + size_t(row) * dst_stride, s + size_t(row) * src_stride, row_bytes);
}

// On-demand: copy the just-rendered canvas into a stable memfd for the
// Snapshot RPC. Never runs on the steady-state path.
void fill_snapshot_(ComposeState& cs, SnapPool& pool, uint32_t stride, uint64_t frame_idx,
                    nativerpc::ComposerService* composer_svc) {
    auto& snap = pool.advance();
    if (!pool.ensure_slot(snap, size_t(stride) * cs.h))
        return;
    bool ok = false;
    {
        std::lock_guard<std::mutex> g(gbm_alloc::gbm_device_mu());
        uint32_t map_stride = 0;
        void* map_handle = nullptr;
        void* src = gbm_bo_map(cs.compose->canvas_bo(), 0, 0, cs.w, cs.h, GBM_BO_TRANSFER_READ,
                               &map_stride, &map_handle);
        if (src) {
            copy_canvas_to_(snap.map, src, stride, map_stride, cs.w, cs.h);
            gbm_bo_unmap(cs.compose->canvas_bo(), map_handle);
            ok = true;
        }
    }
    if (ok)
        notify_composer_svc_(composer_svc, snap.fd, cs.w, cs.h, stride, frame_idx);
}

bool render_scm_frame_(ComposeState& cs, SnapPool& pool,
                       scm_rights_producer::ScmRightsProducer& scm_out,
                       uint64_t broadcast_frame_idx, nativerpc::ComposerService* composer_svc) {
    uint32_t stride = cs.compose->canvas_stride();
    // Zero-copy: broadcast the canvas BO's own dma-buf fd; the swap() ring
    // keeps it stable until the ring laps.
    (void)broadcast_canvas_(scm_out, cs.compose->canvas_dmabuf_fd(), cs.w, cs.h, stride,
                            broadcast_frame_idx);
    if (composer_svc && composer_svc->snapshot_pending())
        fill_snapshot_(cs, pool, stride, broadcast_frame_idx, composer_svc);
    return true;
}

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
    bool gpu_pending = false;
    RenderBatch pending_batch;
    SnapPool snap_pool;
    int init_attempts = 0;   // consecutive init_compose_ failures
    bool loop_error = false; // true => terminate with kCanvasRuntimeError
};

std::chrono::nanoseconds fps_period_(int fps) {
    return std::chrono::nanoseconds(fps > 0 ? 1'000'000'000LL / fps : 1'000'000'000LL / 30);
}

void advance_tick_(LoopState& ls, std::chrono::nanoseconds period) {
    ls.next_tick += period;
    if (ls.next_tick < std::chrono::steady_clock::now())
        ls.next_tick = std::chrono::steady_clock::now() + period;
}

void count_frame_(LoopState& ls, RenderStats* stats) {
    ++ls.frames_rendered;
    if (stats)
        stats->frames_rendered.store(uint64_t(ls.frames_rendered), std::memory_order_relaxed);
}

void post_render_tick_(LoopState& ls, RenderStats* stats, int fps) {
    if (ls.scm_out && std::chrono::steady_clock::now() >= ls.next_consumer_prune)
        prune_consumers_(*ls.scm_out, ls.prev_consumer_count, ls.next_consumer_prune, stats);

    if (stats)
        update_fps_stats_(*stats, ls.frames_rendered, ls.frames_at_last_sample, ls.next_fps_sample);

    advance_tick_(ls, fps_period_(fps));
}

// Consumer gating: render only when someone is consuming or a snapshot is
// pending. stdout mode is never gated.
bool should_render_(LoopState& ls, nativerpc::ComposerService* composer_svc) {
    if (!ls.scm_out)
        return true;
    if (ls.scm_out->consumer_count() > 0)
        return true;
    return composer_svc != nullptr && composer_svc->snapshot_pending();
}

// Idle: advance the monotonic tick and keep pruning so a new consumer is
// picked up within ~one tick. No GPU work, no frame counted.
void idle_tick_(LoopState& ls, RenderStats* stats, int fps) {
    if (ls.scm_out && std::chrono::steady_clock::now() >= ls.next_consumer_prune)
        prune_consumers_(*ls.scm_out, ls.prev_consumer_count, ls.next_consumer_prune, stats);
    advance_tick_(ls, fps_period_(fps));
}

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
        render_scm_frame_(ls.cs, ls.snap_pool, *ls.scm_out, ++ls.broadcast_frame_idx, composer_svc);
        ls.cs.compose->swap();
    }
    count_frame_(ls, stats);
    int fps = snap.canvas_fps ? int(snap.canvas_fps) : target_fps;
    ls.next_tick += fps_period_(fps);
    return true;
}

// STUB: mid-stream canvas-dim recreate not implemented; warn once and ignore.
void warn_dims_changed_(int old_w, int old_h, uint32_t new_w, uint32_t new_h) {
    static bool warned = false;
    if (warned)
        return;
    vn::log::warn("canvas_loop: canvas dims changed %dx%d -> %ux%u; "
                  "recreate not implemented, ignoring (STUB)",
                  old_w, old_h, new_w, new_h);
    warned = true;
}

// Bounded so transient GPU/CMA pressure rides out, but a genuinely-absent
// render node (fails every attempt) exits non-retryable after the budget.
constexpr int kCanvasInitMaxAttempts = 5;
constexpr auto kCanvasInitRetryDelay = std::chrono::milliseconds(1000);

bool ensure_compose_(LoopState& ls, std::atomic<bool>& running, const Snapshot& snap,
                     RenderStats* stats) {
    if (ls.cs.compose) {
        if (int(snap.canvas_w) != ls.cs.w || int(snap.canvas_h) != ls.cs.h)
            warn_dims_changed_(ls.cs.w, ls.cs.h, snap.canvas_w, snap.canvas_h);
        return true;
    }
    if (init_compose_(ls.cs, snap, stats)) {
        ls.init_attempts = 0;
        return true;
    }
    if (++ls.init_attempts < kCanvasInitMaxAttempts) {
        ls.next_tick += kCanvasInitRetryDelay; // back off, retry next tick
        return false;
    }
    vn::log::error("canvas_loop: pl_compose init exhausted retries; giving up");
    ls.loop_error = true;
    running.store(false, std::memory_order_release);
    return false;
}

bool submit_frame_(LoopState& ls, const Snapshot& snap,
                   std::map<std::string, LiveSource>& live_sources) {
    ls.pending_batch = build_render_slots_(snap, live_sources);
    if (!ls.cs.compose->render(ls.pending_batch.slots)) {
        vn::log::error("canvas_loop: render failed");
        return false;
    }
    ls.gpu_pending = true;
    return true;
}

bool complete_frame_(LoopState& ls, nativerpc::ComposerService* composer_svc) {
    if (!ls.gpu_pending)
        return true;
    ls.cs.compose->finish();
    bool ok = ls.scm_out ? render_scm_frame_(ls.cs, ls.snap_pool, *ls.scm_out,
                                             ++ls.broadcast_frame_idx, composer_svc)
                         : render_stdout_frame_(ls.cs);
    ls.cs.compose->swap();
    ls.gpu_pending = false;
    ls.pending_batch = {};
    return ok;
}

} // namespace

int RunCanvasLoop(CanvasLoopConfig cfg) {
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

        // Pipelined: previous tick's GPU work ran during sleep_until, so this
        // finish() is near-zero wait for workloads under one tick period.
        if (!complete_frame_(ls, cfg.composer_svc)) {
            if (ls.scm_out)
                ls.loop_error = true;
            cfg.running.store(false);
            break;
        }

        std::this_thread::sleep_until(ls.next_tick);
        Snapshot snap = cfg.world.snapshot();

        if (snap.canvas_w > 0 && snap.canvas_h > 0) {
            if (!ensure_compose_(ls, cfg.running, snap, cfg.stats))
                continue;
        }

        int fps = snap.canvas_fps ? int(snap.canvas_fps) : cfg.target_fps;
        if (!should_render_(ls, cfg.composer_svc)) {
            idle_tick_(ls, cfg.stats, fps);
            continue;
        }

        if (!snap.ready) {
            if (!handle_not_ready_(ls, snap, cfg.target_fps, cfg.stats, cfg.composer_svc)) {
                if (ls.scm_out)
                    ls.loop_error = true;
                cfg.running.store(false);
                break;
            }
            continue;
        }

        reconcile_sources_(live_sources, snap.slots);

        if (!submit_frame_(ls, snap, live_sources)) {
            ls.loop_error = true;
            cfg.running.store(false);
            break;
        }
        count_frame_(ls, cfg.stats);
        post_render_tick_(ls, cfg.stats, fps);
    }

    // Flush any in-flight GPU work before shutdown.
    (void)complete_frame_(ls, cfg.composer_svc);

    for (auto& [_, lsrc] : live_sources)
        if (lsrc.src)
            lsrc.src->stop();
    if (scm_out_owned)
        scm_out_owned->stop();
    return ls.loop_error ? kCanvasRuntimeError : ls.frames_rendered;
}

} // namespace render
