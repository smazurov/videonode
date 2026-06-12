#include "src/render/pl_compose.hpp"

#include "src/common/log_levels.hpp"
#include "src/render/egl_ctx.hpp"
#include "src/render/gbm_alloc.hpp"

#include <EGL/egl.h>
#include <drm_fourcc.h>
#include <fcntl.h>
#include <gbm.h>
#include <libplacebo/dispatch.h>
#include <libplacebo/gpu.h>
#include <libplacebo/log.h>
#include <libplacebo/opengl.h>
#include <libplacebo/renderer.h>
#include <libplacebo/shaders/custom.h>
#include <libplacebo/vulkan.h>

#include <array>
#include <atomic>
#include <cstdio>
#include <cstdlib>
#include <cstring>
#include <span>
#include <unistd.h>

namespace pl_compose {

namespace {

constexpr uint64_t kModInvalid = (uint64_t{1} << 56) - 1;

struct ClearColor {
    float rgba[4];     // {R,G,B,A} in [0,1] for pl_tex_clear
    uint32_t argb8888; // little-endian packed pixel for the ARGB8888 canvas BO
};

ClearColor unpack_background(uint32_t background_rgba) {
    const auto r = uint8_t((background_rgba >> 24) & 0xFFU);
    const auto g = uint8_t((background_rgba >> 16) & 0xFFU);
    const auto b = uint8_t((background_rgba >> 8) & 0xFFU);
    const auto a = uint8_t(background_rgba & 0xFFU);
    return ClearColor{
        .rgba = {float(r) / 255.0F, float(g) / 255.0F, float(b) / 255.0F, float(a) / 255.0F},
        .argb8888 = (uint32_t(a) << 24) | (uint32_t(r) << 16) | (uint32_t(g) << 8) | uint32_t(b),
    };
}

// Fill the whole canvas with a solid color via the render path. pl_tex_clear
// is a blit op and silently no-ops on imported linear dma-bufs that lack
// blit_dst (the Panthor/PanVK case), so we draw a solid-color fragment shader
// to the renderable canvas instead — the same path the slot composites use.
void fill_canvas_gpu(pl_dispatch dispatch, pl_tex target, std::span<const float, 4> rgba) {
    pl_shader sh = pl_dispatch_begin(dispatch);
    std::array<char, 96> body{};
    std::snprintf(body.data(), body.size(), "color = vec4(%f, %f, %f, %f);", double(rgba[0]),
                  double(rgba[1]), double(rgba[2]), double(rgba[3]));
    struct pl_custom_shader cs = {};
    cs.body = body.data();
    cs.input = PL_SHADER_SIG_NONE;
    cs.output = PL_SHADER_SIG_COLOR;
    if (!pl_shader_custom(sh, &cs)) {
        pl_dispatch_abort(dispatch, &sh);
        return;
    }
    struct pl_dispatch_params dp = {};
    dp.shader = &sh;
    dp.target = target;
    pl_dispatch_finish(dispatch, &dp);
}

void fill_canvas_cpu(std::span<std::byte> buf, uint32_t stride, int w, int h, uint32_t argb8888) {
    if (argb8888 == 0) {
        std::memset(buf.data(), 0, buf.size());
        return;
    }
    for (int row = 0; row < h; ++row) {
        auto line = buf.subspan(size_t(row) * stride, size_t(w) * 4);
        for (size_t px = 0; px < size_t(w); ++px)
            std::memcpy(line.subspan(px * 4, 4).data(), &argb8888, sizeof(argb8888));
    }
}

// Vulkan requires an explicit modifier; GL accepts INVALID as "driver picks".
// When GBM reports INVALID but we allocated with GBM_BO_USE_LINEAR, pass LINEAR
// for Vulkan. For GL, INVALID is fine.
uint64_t normalize_mod(uint64_t mod, bool vulkan) {
    if (vulkan && mod == kModInvalid)
        return DRM_FORMAT_MOD_LINEAR;
    return mod;
}

struct SlotRenderCtx {
    pl_gpu gpu;
    pl_renderer renderer;
    pl_tex canvas_tex;
    pl_fmt fmt_r8;
    pl_fmt fmt_rg8;
    uint64_t src_mod;
};

void log_import_fail(const char* plane, int w, int h) {
    static std::atomic<int> count{0};
    if (count.fetch_add(1) < 3)
        vn::log::error("pl_compose: %s plane dma-buf import failed (%dx%d); "
                       "slot renders black — source fd not an importable dma-buf?",
                       plane, w, h);
}

void render_slot(const SlotRenderCtx& ctx, const SourceSlot& slot) {
    if (slot.src_y_fd < 0 || slot.src_uv_fd < 0)
        return;
    if (slot.src_w <= 0 || slot.src_h <= 0 || slot.w <= 0 || slot.h <= 0)
        return;

    int src_y_pitch = slot.src_y_pitch > 0 ? slot.src_y_pitch : slot.src_w;
    int src_uv_pitch = slot.src_uv_pitch > 0 ? slot.src_uv_pitch : slot.src_w;

    // Import source Y — caller owns the dup'd fd; libplacebo does not
    // close it on pl_tex_destroy.
    int fd_y = dup(slot.src_y_fd);
    struct pl_tex_params tp_y = {};
    tp_y.w = slot.src_w;
    tp_y.h = slot.src_h;
    tp_y.format = ctx.fmt_r8;
    tp_y.sampleable = true;
    tp_y.import_handle = PL_HANDLE_DMA_BUF;
    tp_y.shared_mem.handle.fd = fd_y;
    tp_y.shared_mem.offset = static_cast<size_t>(slot.src_y_offset);
    tp_y.shared_mem.size = static_cast<size_t>(src_y_pitch) * slot.src_h + slot.src_y_offset;
    tp_y.shared_mem.drm_format_mod = ctx.src_mod;
    tp_y.shared_mem.stride_w = src_y_pitch;
    pl_tex tex_y = pl_tex_create(ctx.gpu, &tp_y);
    if (!tex_y) {
        log_import_fail("y", slot.src_w, slot.src_h);
        ::close(fd_y);
        return;
    }

    int fd_uv = dup(slot.src_uv_fd);
    struct pl_tex_params tp_uv = {};
    tp_uv.w = slot.src_w / 2;
    tp_uv.h = slot.src_h / 2;
    tp_uv.format = ctx.fmt_rg8;
    tp_uv.sampleable = true;
    tp_uv.import_handle = PL_HANDLE_DMA_BUF;
    tp_uv.shared_mem.handle.fd = fd_uv;
    tp_uv.shared_mem.offset = static_cast<size_t>(slot.src_uv_offset);
    tp_uv.shared_mem.size =
        static_cast<size_t>(src_uv_pitch) * (slot.src_h / 2) + slot.src_uv_offset;
    tp_uv.shared_mem.drm_format_mod = ctx.src_mod;
    tp_uv.shared_mem.stride_w = src_uv_pitch;
    pl_tex tex_uv = pl_tex_create(ctx.gpu, &tp_uv);
    if (!tex_uv) {
        log_import_fail("uv", slot.src_w, slot.src_h);
        pl_tex_destroy(ctx.gpu, &tex_y);
        ::close(fd_y);
        ::close(fd_uv);
        return;
    }

    struct pl_frame src_frame = {};
    src_frame.num_planes = 2;
    src_frame.planes[0].texture = tex_y;
    src_frame.planes[0].components = 1;
    src_frame.planes[0].component_mapping[0] = 0;
    src_frame.planes[1].texture = tex_uv;
    src_frame.planes[1].components = 2;
    src_frame.planes[1].component_mapping[0] = 1;
    src_frame.planes[1].component_mapping[1] = 2;
    src_frame.repr.sys = slot.src_bt709 ? PL_COLOR_SYSTEM_BT_709 : PL_COLOR_SYSTEM_BT_601;
    src_frame.repr.levels = PL_COLOR_LEVELS_LIMITED;
    src_frame.planes[1].shift_x = -1;
    src_frame.planes[1].shift_y = -1;
    pl_frame_set_chroma_location(&src_frame, PL_CHROMA_LEFT);

    src_frame.crop.x0 = slot.src_crop_x0 * static_cast<float>(slot.src_w);
    src_frame.crop.y0 = slot.src_crop_y0 * static_cast<float>(slot.src_h);
    src_frame.crop.x1 = slot.src_crop_x1 * static_cast<float>(slot.src_w);
    src_frame.crop.y1 = slot.src_crop_y1 * static_cast<float>(slot.src_h);

    switch (slot.rotation) {
    case 90:
        src_frame.rotation = PL_ROTATION_90;
        break;
    case 180:
        src_frame.rotation = PL_ROTATION_180;
        break;
    case 270:
        src_frame.rotation = PL_ROTATION_270;
        break;
    default:
        break;
    }

    struct pl_frame dst_frame = {};
    dst_frame.num_planes = 1;
    dst_frame.planes[0].texture = ctx.canvas_tex;
    dst_frame.planes[0].components = 4;
    dst_frame.planes[0].component_mapping[0] = 0;
    dst_frame.planes[0].component_mapping[1] = 1;
    dst_frame.planes[0].component_mapping[2] = 2;
    dst_frame.planes[0].component_mapping[3] = 3;
    dst_frame.repr.sys = PL_COLOR_SYSTEM_RGB;
    dst_frame.repr.levels = PL_COLOR_LEVELS_FULL;
    dst_frame.crop.x0 = static_cast<float>(slot.x);
    dst_frame.crop.y0 = static_cast<float>(slot.y);
    dst_frame.crop.x1 = static_cast<float>(slot.x + slot.w);
    dst_frame.crop.y1 = static_cast<float>(slot.y + slot.h);

    struct pl_render_params params = pl_render_fast_params;
    params.skip_anti_aliasing = true;
    params.border = PL_CLEAR_SKIP;
    // TODO: per-source warp via pl_shader_custom when warp != identity
    pl_render_image(ctx.renderer, &src_frame, &dst_frame, &params);

    pl_tex_destroy(ctx.gpu, &tex_y);
    pl_tex_destroy(ctx.gpu, &tex_uv);
    ::close(fd_y);
    ::close(fd_uv);
}

} // namespace

struct PlCompose::Impl {
    egl_ctx::EglCtx ctx;
    pl_log logger = nullptr;
    // Exactly one of these is non-null after init.
    pl_vulkan vk = nullptr;
    pl_vk_inst vk_inst = nullptr;
    pl_opengl gl = nullptr;
    pl_gpu gpu = nullptr;
    pl_renderer renderer = nullptr;
    pl_dispatch dispatch = nullptr;
    pl_tex canvas_tex[kBufCount] = {};
    bool using_vulkan = false;
};

PlCompose::~PlCompose() {
    if (!impl_)
        return;
    for (auto& tex : impl_->canvas_tex) {
        if (tex)
            pl_tex_destroy(impl_->gpu, &tex);
    }
    if (impl_->dispatch)
        pl_dispatch_destroy(&impl_->dispatch);
    if (impl_->renderer)
        pl_renderer_destroy(&impl_->renderer);
    if (impl_->vk)
        pl_vulkan_destroy(&impl_->vk);
    if (impl_->vk_inst)
        pl_vk_inst_destroy(&impl_->vk_inst);
    if (impl_->gl)
        pl_opengl_destroy(&impl_->gl);
    if (impl_->logger)
        pl_log_destroy(&impl_->logger);
    for (int i = 0; i < kBufCount; ++i) {
        if (canvas_bo_[i])
            gbm_bo_destroy(canvas_bo_[i]);
        if (canvas_fd_[i] >= 0)
            ::close(canvas_fd_[i]);
    }
    delete impl_;
}

bool PlCompose::init(std::string_view device_path, int canvas_w, int canvas_h) {
    if (canvas_w <= 0 || canvas_h <= 0)
        return false;

    impl_ = new Impl;
    // EGL context is needed for GBM device regardless of backend.
    if (!impl_->ctx.init(device_path)) {
        vn::log::error("pl_compose: EglCtx::init(%.*s)", int(device_path.size()),
                       device_path.data());
    } else if (!init_backend()) {
        // init_backend logs the specific failure.
    } else if (!alloc_canvas(canvas_w, canvas_h)) {
        // alloc_canvas logs the specific failure.
    } else {
        vn::log::info("pl_compose: ready %dx%d (stride=%u, ring=%d, clear=%s)", canvas_w, canvas_h,
                      canvas_stride_, kBufCount, cpu_clear_ ? "cpu" : "gpu");
        return true;
    }

    delete impl_;
    impl_ = nullptr;
    return false;
}

bool PlCompose::init_backend() {
    struct pl_log_params lp = {};
    lp.log_cb = nullptr;
    lp.log_level = PL_LOG_ERR;
    impl_->logger = pl_log_create(PL_API_VER, &lp);

    // Try Vulkan first — 20% faster on Mali-G610 (PanVK).
    struct pl_vk_inst_params ip = {};
    impl_->vk_inst = pl_vk_inst_create(impl_->logger, &ip);
    if (impl_->vk_inst) {
        struct pl_vulkan_params vkp = {};
        vkp.instance = impl_->vk_inst->instance;
        vkp.get_proc_addr = impl_->vk_inst->get_proc_addr;
        vkp.allow_software = false;
        impl_->vk = pl_vulkan_create(impl_->logger, &vkp);
        if (impl_->vk && (impl_->vk->gpu->import_caps.tex & PL_HANDLE_DMA_BUF)) {
            impl_->gpu = impl_->vk->gpu;
            impl_->using_vulkan = true;
            vn::log::info("pl_compose: using Vulkan backend");
        } else {
            if (impl_->vk)
                pl_vulkan_destroy(&impl_->vk);
            pl_vk_inst_destroy(&impl_->vk_inst);
        }
    }

    if (!impl_->gpu) {
        struct pl_opengl_params glp = {};
        glp.egl_display = impl_->ctx.display();
        glp.egl_context = impl_->ctx.context();
        glp.get_proc_addr = reinterpret_cast<pl_voidfunc_t (*)(const char*)>(eglGetProcAddress);
        impl_->gl = pl_opengl_create(impl_->logger, &glp);
        if (!impl_->gl) {
            vn::log::error("pl_compose: both Vulkan and OpenGL backends failed");
            return false;
        }
        impl_->gpu = impl_->gl->gpu;
        vn::log::info("pl_compose: using OpenGL backend");
    }

    impl_->renderer = pl_renderer_create(impl_->logger, impl_->gpu);
    impl_->dispatch = pl_dispatch_create(impl_->logger, impl_->gpu);
    if (!impl_->renderer || !impl_->dispatch) {
        vn::log::error("pl_compose: renderer/dispatch create failed");
        return false;
    }
    return true;
}

bool PlCompose::alloc_canvas(int canvas_w, int canvas_h) {
    pl_fmt fmt_bgra = pl_find_named_fmt(impl_->gpu, "bgra8");
    if (!fmt_bgra)
        fmt_bgra = pl_find_named_fmt(impl_->gpu, "rgba8");
    if (!fmt_bgra) {
        vn::log::error("pl_compose: no rgba8/bgra8 format");
        return false;
    }

    canvas_w_ = canvas_w;
    canvas_h_ = canvas_h;
    cpu_clear_ = std::getenv("VIDEONODE_COMPOSER_CPU_CLEAR") != nullptr;
    for (int i = 0; i < kBufCount; ++i) {
        canvas_bo_[i] = gbm_bo_create(impl_->ctx.gbm(), canvas_w, canvas_h, GBM_FORMAT_ARGB8888,
                                      GBM_BO_USE_LINEAR | GBM_BO_USE_RENDERING);
        if (!canvas_bo_[i]) {
            vn::log::error("pl_compose: gbm_bo_create canvas[%d] %dx%d", i, canvas_w, canvas_h);
            return false;
        }
        canvas_fd_[i] = gbm_bo_get_fd(canvas_bo_[i]);
        if (i == 0) {
            canvas_stride_ = gbm_bo_get_stride(canvas_bo_[i]);
        }

        struct pl_tex_params tp = {};
        tp.w = canvas_w;
        tp.h = canvas_h;
        tp.format = fmt_bgra;
        tp.renderable = true;
        tp.import_handle = PL_HANDLE_DMA_BUF;
        tp.shared_mem.handle.fd = dup(canvas_fd_[i]);
        tp.shared_mem.size = static_cast<size_t>(canvas_stride_) * canvas_h;
        tp.shared_mem.drm_format_mod =
            normalize_mod(gbm_bo_get_modifier(canvas_bo_[i]), impl_->using_vulkan);
        tp.shared_mem.stride_w = static_cast<int>(canvas_stride_);
        impl_->canvas_tex[i] = pl_tex_create(impl_->gpu, &tp);
        if (!impl_->canvas_tex[i]) {
            vn::log::error("pl_compose: canvas pl_tex_create[%d] failed", i);
            return false;
        }
    }
    return true;
}

gbm_device* PlCompose::gbm() const {
    return impl_ ? impl_->ctx.gbm() : nullptr;
}

bool PlCompose::render(const std::vector<SourceSlot>& slots, uint32_t background_rgba) {
    if (!impl_)
        return false;

    const ClearColor bg = unpack_background(background_rgba);

    // GPU clear keeps the CPU off the canvas BO; memset fallback for drivers
    // where GPU clear is unreliable on imported linear dma-bufs.
    if (cpu_clear_) {
        std::lock_guard<std::mutex> g(gbm_alloc::gbm_device_mu());
        uint32_t stride = 0;
        void* map_handle = nullptr;
        void* ptr = gbm_bo_map(canvas_bo_[back_], 0, 0, canvas_w_, canvas_h_, GBM_BO_TRANSFER_WRITE,
                               &stride, &map_handle);
        if (ptr) {
            auto buf = std::span(static_cast<std::byte*>(ptr), size_t(stride) * canvas_h_);
            fill_canvas_cpu(buf, stride, canvas_w_, canvas_h_, bg.argb8888);
            gbm_bo_unmap(canvas_bo_[back_], map_handle);
        }
    } else {
        fill_canvas_gpu(impl_->dispatch, impl_->canvas_tex[back_], bg.rgba);
    }

    pl_fmt fmt_r8 = pl_find_named_fmt(impl_->gpu, "r8");
    pl_fmt fmt_rg8 = pl_find_named_fmt(impl_->gpu, "rg8");
    if (!fmt_r8 || !fmt_rg8)
        return false;

    const SlotRenderCtx ctx{.gpu = impl_->gpu,
                            .renderer = impl_->renderer,
                            .canvas_tex = impl_->canvas_tex[back_],
                            .fmt_r8 = fmt_r8,
                            .fmt_rg8 = fmt_rg8,
                            .src_mod = normalize_mod(kModInvalid, impl_->using_vulkan)};
    for (const auto& slot : slots)
        render_slot(ctx, slot);

    return true;
}

void PlCompose::finish() {
    if (impl_)
        pl_gpu_finish(impl_->gpu);
}

void PlCompose::swap() {
    back_ = (back_ + 1) % kBufCount;
}

} // namespace pl_compose
