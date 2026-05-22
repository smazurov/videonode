#include "src/render/canvas_loop.hpp"

#include "src/ipc/scm_rights_source.hpp"
#include "src/render/egl_ctx.hpp"
#include "src/render/gbm_alloc.hpp"
#include "src/render/gl_compose.hpp"
#include "src/render/world.hpp"
#include "src/rpc/control_channel.hpp"

#include <EGL/egl.h>
#include <GLES2/gl2.h>
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

constexpr uint64_t kModInvalid = (uint64_t{1} << 56) - 1;

// Two single-plane EGLImages per NV12 frame. See the old canvas_loop's
// comment chain (preserved here in spirit): radeonsi/amdgpu wants
// single-plane R8 + GR88 imports with MOD_INVALID, not a multi-plane
// import. Same path Panthor accepts.
struct SourceImagePair {
    EGLImage y = EGL_NO_IMAGE;
    EGLImage uv = EGL_NO_IMAGE;
};

SourceImagePair import_frame_(const egl_ctx::EglCtx& ctx,
                              const scm_rights_source::FrameView& v) {
    SourceImagePair p;
    if (v.fd < 0 || v.plane1_fd < 0 || v.width <= 0 || v.height <= 0)
        return p;
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

bool write_full_(int fd, std::span<const uint8_t> buf) {
    while (!buf.empty()) {
        ssize_t w = ::write(fd, buf.data(), buf.size());
        if (w < 0) {
            if (errno == EINTR)
                continue;
            if (errno == EPIPE)
                return false; // consumer hung up; clean exit
            fprintf(stderr, "canvas_loop: write stdout: %s\n", strerror(errno));
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
    // Phase 1: figure out which existing slots need to be torn down (no
    // longer present, or path changed).
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
            fprintf(stderr, "canvas_loop: dropped slot %s\n", slot.c_str());
        }
    }
    // Phase 2: add any new slot bindings.
    for (const auto& b : bindings) {
        if (live.find(b.slot) != live.end())
            continue;
        auto s = std::make_unique<scm_rights_source::ScmRightsSource>();
        scm_rights_source::InitParams p;
        p.socket_path = b.scm_path;
        p.dial = true; // composer is the consumer
        if (!s->init(p)) {
            fprintf(stderr, "canvas_loop: init slot %s (%s) failed\n",
                    b.slot.c_str(), b.scm_path.c_str());
            continue;
        }
        if (!s->start()) {
            fprintf(stderr, "canvas_loop: start slot %s (%s) failed\n",
                    b.slot.c_str(), b.scm_path.c_str());
            continue;
        }
        LiveSource ls;
        ls.src = std::move(s);
        ls.scm_path = b.scm_path;
        live[b.slot] = std::move(ls);
        fprintf(stderr, "canvas_loop: dialed slot %s -> %s (%s)\n",
                b.slot.c_str(), b.source_id.c_str(), b.scm_path.c_str());
    }
}

// Write a solid-black BGRA canvas of the requested size to stdout. Used
// before World is ready, so the downstream encoder pipe stays alive.
bool write_black_canvas_(int w, int h) {
    if (w <= 0 || h <= 0) {
        // Nothing to write — sleep briefly so the caller doesn't spin.
        std::this_thread::sleep_for(std::chrono::milliseconds(33));
        return true;
    }
    static thread_local std::vector<uint8_t> black;
    size_t need = size_t(w) * size_t(h) * 4;
    if (black.size() != need)
        black.assign(need, 0);
    return write_full_(STDOUT_FILENO, std::span(black.data(), black.size()));
}

} // namespace

int RunCanvasLoop(egl_ctx::EglCtx& ctx,
                  World& world,
                  control_channel::ControlChannel* ctl,
                  int target_fps,
                  int run_seconds,
                  std::atomic<bool>& running) {
    auto start = std::chrono::steady_clock::now();
    int frames_rendered = 0;

    // gl_compose is constructed lazily on the first ready snapshot, since
    // we don't know the canvas dims until the daemon's set_canvas lands.
    std::unique_ptr<gl_compose::GlCompose> compose;
    int compose_w = 0, compose_h = 0;

    std::map<std::string, LiveSource> live_sources;
    std::map<int, SourceImagePair> img_cache;
    auto get_img = [&](const scm_rights_source::FrameView& v) -> SourceImagePair {
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

    auto period = [&](int fps) {
        return std::chrono::nanoseconds(fps > 0 ? 1'000'000'000LL / fps
                                                : 1'000'000'000LL / 30);
    };

    auto next_tick = std::chrono::steady_clock::now();

    while (running.load()) {
        if (run_seconds > 0 &&
            std::chrono::steady_clock::now() - start > std::chrono::seconds(run_seconds))
            break;

        if (ctl) {
            // Drain any pending control messages with a poll bounded by
            // time-to-next-frame. The handlers update World; we re-read it
            // below. The composer's poll set is just the ctl fd today;
            // ScmRightsSource has its own reader thread so its fds don't
            // belong here.
            ctl->maintain();
            std::vector<pollfd> pset;
            ctl->add_to_poll(pset);
            if (!pset.empty()) {
                auto until = next_tick - std::chrono::steady_clock::now();
                int timeout_ms = int(std::chrono::duration_cast<std::chrono::milliseconds>(until).count());
                if (timeout_ms < 0) timeout_ms = 0;
                if (timeout_ms > 100) timeout_ms = 100;
                int pr = ::poll(pset.data(), pset.size(), timeout_ms);
                if (pr > 0)
                    ctl->handle_events(pset[0].revents);
            } else {
                std::this_thread::sleep_until(next_tick);
            }
        } else {
            std::this_thread::sleep_until(next_tick);
        }

        Snapshot snap = world.snapshot();

        if (!snap.ready) {
            // Render solid black until the daemon has wired us up.
            int w = compose ? compose_w : (snap.canvas_w ? int(snap.canvas_w) : 1280);
            int h = compose ? compose_h : (snap.canvas_h ? int(snap.canvas_h) : 720);
            if (!write_black_canvas_(w, h)) {
                running.store(false);
                break;
            }
            ++frames_rendered;
            next_tick += period(int(snap.canvas_fps ? snap.canvas_fps : uint32_t(target_fps)));
            continue;
        }

        // Lazy gl_compose init on first ready snapshot.
        if (!compose) {
            compose = std::make_unique<gl_compose::GlCompose>();
            if (!compose->init(ctx, int(snap.canvas_w), int(snap.canvas_h))) {
                fprintf(stderr, "canvas_loop: gl_compose init %dx%d failed\n",
                        int(snap.canvas_w), int(snap.canvas_h));
                compose.reset();
                next_tick += period(target_fps);
                continue;
            }
            compose_w = int(snap.canvas_w);
            compose_h = int(snap.canvas_h);
            fprintf(stderr, "canvas_loop: ready canvas %dx%d (%u fps)\n",
                    compose_w, compose_h, snap.canvas_fps);
        } else if (int(snap.canvas_w) != compose_w || int(snap.canvas_h) != compose_h) {
            // MVP STUB: canvas dim change mid-stream not supported. Log once.
            static bool warned = false;
            if (!warned) {
                fprintf(stderr,
                        "canvas_loop: WARN canvas dims changed %dx%d -> %ux%u; "
                        "recreate not implemented, ignoring (STUB)\n",
                        compose_w, compose_h, snap.canvas_w, snap.canvas_h);
                warned = true;
            }
        }

        reconcile_sources_(live_sources, snap.slots);

        // Build SourceSlots for the GLES compose call. One per layout
        // entry whose slot has a live source AND a frame in hand.
        std::vector<gl_compose::SourceSlot> render_slots;
        render_slots.reserve(snap.layout.size());
        for (const auto& rect : snap.layout) {
            // Find binding for this layout slot.
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
            SourceImagePair imgs = get_img(fv);
            if (imgs.y == EGL_NO_IMAGE || imgs.uv == EGL_NO_IMAGE)
                continue;

            gl_compose::SourceSlot s;
            s.src_y_image = imgs.y;
            s.src_uv_image = imgs.uv;
            s.x = rect.x;
            s.y = rect.y;
            s.w = rect.w;
            s.h = rect.h;
            // Pick the warp by source_id. Identity if the source is in
            // placeholder state OR no perspective has been set.
            auto sit = snap.source_states.find(bit->source_id);
            if (sit != snap.source_states.end()) {
                const auto& ss = sit->second;
                if (ss.state != "placeholder" && ss.has_perspective) {
                    s.warp.m[0] = ss.warp[0]; s.warp.m[1] = ss.warp[1]; s.warp.m[2] = ss.warp[2];
                    s.warp.m[3] = ss.warp[3]; s.warp.m[4] = ss.warp[4]; s.warp.m[5] = ss.warp[5];
                    s.warp.m[6] = ss.warp[6]; s.warp.m[7] = ss.warp[7]; s.warp.m[8] = ss.warp[8];
                }
                // else: identity (struct default)
            }
            render_slots.push_back(s);
        }

        if (!compose->render(render_slots)) {
            fprintf(stderr, "canvas_loop: render failed\n");
            break;
        }
        compose->finish();

        // gbm_bo_map under the process-wide lock — concurrent calls from
        // any other gbm_alloc::* user must not race Mesa's threaded
        // context. See gbm_alloc::gbm_device_mu().
        uint32_t map_stride = 0;
        void* map_data = nullptr;
        void* canvas_map = nullptr;
        {
            std::lock_guard<std::mutex> g(gbm_alloc::gbm_device_mu());
            canvas_map = gbm_bo_map(compose->canvas_bo(), 0, 0, compose_w, compose_h,
                                    GBM_BO_TRANSFER_READ, &map_stride, &map_data);
        }
        if (!canvas_map) {
            fprintf(stderr, "canvas_loop: gbm_bo_map failed\n");
            break;
        }
        bool write_ok = true;
        size_t bytes_per_frame = size_t(compose_w) * compose_h * 4;
        if (map_stride == uint32_t(compose_w) * 4) {
            write_ok = write_full_(
                STDOUT_FILENO,
                std::span(static_cast<const uint8_t*>(canvas_map), bytes_per_frame));
        } else {
            for (int y = 0; y < compose_h && write_ok; ++y) {
                const uint8_t* row = static_cast<const uint8_t*>(canvas_map) + y * map_stride;
                if (!write_full_(STDOUT_FILENO,
                                 std::span(row, size_t(compose_w) * 4)))
                    write_ok = false;
            }
        }
        {
            std::lock_guard<std::mutex> g(gbm_alloc::gbm_device_mu());
            gbm_bo_unmap(compose->canvas_bo(), map_data);
        }
        if (!write_ok) {
            running.store(false);
            break;
        }
        ++frames_rendered;

        int fps = snap.canvas_fps ? int(snap.canvas_fps) : target_fps;
        if (fps > 0 && (frames_rendered % fps) == 0) {
            auto now = std::chrono::steady_clock::now();
            double elapsed = std::chrono::duration<double>(now - start).count();
            fprintf(stderr, "[%6.1fs] rendered=%d (%.1f fps)\n", elapsed, frames_rendered,
                    elapsed > 0.0 ? frames_rendered / elapsed : 0.0);
        }

        next_tick += period(fps);
        if (next_tick < std::chrono::steady_clock::now()) {
            // Fell behind — rebase rather than burn loop iterations.
            next_tick = std::chrono::steady_clock::now() + period(fps);
        }
    }

    for (auto& [_, ls] : live_sources)
        if (ls.src)
            ls.src->stop();
    for (auto& [_, p] : img_cache) {
        if (p.y != EGL_NO_IMAGE)
            eglDestroyImage(ctx.display(), p.y);
        if (p.uv != EGL_NO_IMAGE)
            eglDestroyImage(ctx.display(), p.uv);
    }
    return frames_rendered;
}

} // namespace render
