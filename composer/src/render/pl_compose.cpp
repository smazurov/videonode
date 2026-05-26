#include "src/render/pl_compose.hpp"

#include "src/common/log_levels.hpp"
#include "src/render/egl_ctx.hpp"

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

#include <cstring>
#include <unistd.h>

namespace pl_compose {

namespace {

constexpr uint64_t kModInvalid = (uint64_t{1} << 56) - 1;

// Vulkan requires an explicit modifier; GL accepts INVALID as "driver picks".
// When GBM reports INVALID but we allocated with GBM_BO_USE_LINEAR, pass LINEAR
// for Vulkan. For GL, INVALID is fine.
uint64_t normalize_mod(uint64_t mod, bool vulkan) {
    if (vulkan && mod == kModInvalid)
        return DRM_FORMAT_MOD_LINEAR;
    return mod;
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
    pl_tex canvas_tex = nullptr;
    bool using_vulkan = false;
};

PlCompose::~PlCompose() {
    if (!impl_)
        return;
    if (impl_->canvas_tex)
        pl_tex_destroy(impl_->gpu, &impl_->canvas_tex);
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
    if (canvas_bo_)
        gbm_bo_destroy(canvas_bo_);
    if (canvas_fd_ >= 0)
        ::close(canvas_fd_);
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
        delete impl_;
        impl_ = nullptr;
        return false;
    }

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

    // Fall back to OpenGL if Vulkan didn't work.
    if (!impl_->gpu) {
        struct pl_opengl_params glp = {};
        glp.egl_display = impl_->ctx.display();
        glp.egl_context = impl_->ctx.context();
        glp.get_proc_addr = reinterpret_cast<pl_voidfunc_t (*)(const char*)>(eglGetProcAddress);
        impl_->gl = pl_opengl_create(impl_->logger, &glp);
        if (!impl_->gl) {
            vn::log::error("pl_compose: both Vulkan and OpenGL backends failed");
            delete impl_;
            delete impl_;
            impl_ = nullptr;
            return false;
        }
        impl_->gpu = impl_->gl->gpu;
        vn::log::info("pl_compose: using OpenGL backend");
    }

    impl_->renderer = pl_renderer_create(impl_->logger, impl_->gpu);
    impl_->dispatch = pl_dispatch_create(impl_->logger, impl_->gpu);
    if (!impl_->renderer || !impl_->dispatch) {
        vn::log::error("pl_compose: renderer/dispatch create failed");
        delete impl_;
        impl_ = nullptr;
        return false;
    }

    // Allocate canvas via GBM (ARGB8888)
    canvas_bo_ = gbm_bo_create(impl_->ctx.gbm(), canvas_w, canvas_h, GBM_FORMAT_ARGB8888,
                               GBM_BO_USE_LINEAR | GBM_BO_USE_RENDERING);
    if (!canvas_bo_) {
        vn::log::error("pl_compose: gbm_bo_create canvas %dx%d", canvas_w, canvas_h);
        delete impl_;
        impl_ = nullptr;
        return false;
    }
    canvas_fd_ = gbm_bo_get_fd(canvas_bo_);
    canvas_stride_ = gbm_bo_get_stride(canvas_bo_);
    canvas_w_ = canvas_w;
    canvas_h_ = canvas_h;

    // Import canvas as renderable pl_tex
    pl_fmt fmt_bgra = pl_find_named_fmt(impl_->gpu, "bgra8");
    if (!fmt_bgra)
        fmt_bgra = pl_find_named_fmt(impl_->gpu, "rgba8");
    if (!fmt_bgra) {
        vn::log::error("pl_compose: no rgba8/bgra8 format");
        delete impl_;
        impl_ = nullptr;
        return false;
    }

    struct pl_tex_params tp = {};
    tp.w = canvas_w;
    tp.h = canvas_h;
    tp.format = fmt_bgra;
    tp.renderable = true;
    tp.blit_dst = true;
    tp.import_handle = PL_HANDLE_DMA_BUF;
    tp.shared_mem.handle.fd = dup(canvas_fd_);
    tp.shared_mem.size = static_cast<size_t>(canvas_stride_) * canvas_h;
    tp.shared_mem.drm_format_mod =
        normalize_mod(gbm_bo_get_modifier(canvas_bo_), impl_->using_vulkan);
    tp.shared_mem.stride_w = static_cast<int>(canvas_stride_);
    impl_->canvas_tex = pl_tex_create(impl_->gpu, &tp);
    if (!impl_->canvas_tex) {
        vn::log::error("pl_compose: canvas pl_tex_create failed");
        delete impl_;
        impl_ = nullptr;
        return false;
    }

    vn::log::info("pl_compose: ready %dx%d (stride=%u)", canvas_w, canvas_h, canvas_stride_);
    return true;
}

gbm_device* PlCompose::gbm() const {
    return impl_ ? impl_->ctx.gbm() : nullptr;
}

bool PlCompose::render(const std::vector<SourceSlot>& slots) {
    if (!impl_)
        return false;

    // Clear canvas to black
    pl_tex_clear(impl_->gpu, impl_->canvas_tex, (float[4]){0, 0, 0, 1});

    const uint64_t src_mod = normalize_mod(kModInvalid, impl_->using_vulkan);
    pl_fmt fmt_r8 = pl_find_named_fmt(impl_->gpu, "r8");
    pl_fmt fmt_rg8 = pl_find_named_fmt(impl_->gpu, "rg8");
    if (!fmt_r8 || !fmt_rg8)
        return false;

    for (const auto& slot : slots) {
        if (slot.src_y_fd < 0 || slot.src_uv_fd < 0)
            continue;
        if (slot.src_w <= 0 || slot.src_h <= 0 || slot.w <= 0 || slot.h <= 0)
            continue;

        int src_y_pitch = slot.src_y_pitch > 0 ? slot.src_y_pitch : slot.src_w;
        int src_uv_pitch = slot.src_uv_pitch > 0 ? slot.src_uv_pitch : slot.src_w;

        // Import source Y — caller owns the dup'd fd; libplacebo does not
        // close it on pl_tex_destroy.
        int fd_y = dup(slot.src_y_fd);
        struct pl_tex_params tp_y = {};
        tp_y.w = slot.src_w;
        tp_y.h = slot.src_h;
        tp_y.format = fmt_r8;
        tp_y.sampleable = true;
        tp_y.import_handle = PL_HANDLE_DMA_BUF;
        tp_y.shared_mem.handle.fd = fd_y;
        tp_y.shared_mem.size = static_cast<size_t>(src_y_pitch) * slot.src_h;
        tp_y.shared_mem.drm_format_mod = src_mod;
        tp_y.shared_mem.stride_w = src_y_pitch;
        pl_tex tex_y = pl_tex_create(impl_->gpu, &tp_y);
        if (!tex_y) {
            ::close(fd_y);
            continue;
        }

        // Import source UV
        int fd_uv = dup(slot.src_uv_fd);
        struct pl_tex_params tp_uv = {};
        tp_uv.w = slot.src_w / 2;
        tp_uv.h = slot.src_h / 2;
        tp_uv.format = fmt_rg8;
        tp_uv.sampleable = true;
        tp_uv.import_handle = PL_HANDLE_DMA_BUF;
        tp_uv.shared_mem.handle.fd = fd_uv;
        tp_uv.shared_mem.size = static_cast<size_t>(src_uv_pitch) * (slot.src_h / 2);
        tp_uv.shared_mem.drm_format_mod = src_mod;
        tp_uv.shared_mem.stride_w = src_uv_pitch;
        pl_tex tex_uv = pl_tex_create(impl_->gpu, &tp_uv);
        if (!tex_uv) {
            pl_tex_destroy(impl_->gpu, &tex_y);
            ::close(fd_y);
            ::close(fd_uv);
            continue;
        }

        // Use pl_renderer to convert NV12 source to the BGRA canvas region.
        // pl_renderer handles YCbCr→RGB + scaling in one pass.
        struct pl_frame src_frame = {};
        src_frame.num_planes = 2;
        src_frame.planes[0].texture = tex_y;
        src_frame.planes[0].components = 1;
        src_frame.planes[0].component_mapping[0] = 0;
        src_frame.planes[1].texture = tex_uv;
        src_frame.planes[1].components = 2;
        src_frame.planes[1].component_mapping[0] = 1;
        src_frame.planes[1].component_mapping[1] = 2;
        src_frame.repr.sys = PL_COLOR_SYSTEM_BT_601;
        src_frame.repr.levels = PL_COLOR_LEVELS_LIMITED;
        src_frame.planes[1].shift_x = -1;
        src_frame.planes[1].shift_y = -1;
        pl_frame_set_chroma_location(&src_frame, PL_CHROMA_LEFT);

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
        dst_frame.planes[0].texture = impl_->canvas_tex;
        dst_frame.planes[0].components = 4;
        dst_frame.planes[0].component_mapping[0] = 0;
        dst_frame.planes[0].component_mapping[1] = 1;
        dst_frame.planes[0].component_mapping[2] = 2;
        dst_frame.planes[0].component_mapping[3] = 3;
        dst_frame.repr.sys = PL_COLOR_SYSTEM_RGB;
        dst_frame.repr.levels = PL_COLOR_LEVELS_FULL;
        // Crop the destination to the slot's canvas region
        dst_frame.crop.x0 = static_cast<float>(slot.x);
        dst_frame.crop.y0 = static_cast<float>(slot.y);
        dst_frame.crop.x1 = static_cast<float>(slot.x + slot.w);
        dst_frame.crop.y1 = static_cast<float>(slot.y + slot.h);

        struct pl_render_params params = pl_render_fast_params;
        params.skip_anti_aliasing = true;
        // TODO: per-source warp via pl_shader_custom when warp != identity
        pl_render_image(impl_->renderer, &src_frame, &dst_frame, &params);

        pl_tex_destroy(impl_->gpu, &tex_y);
        pl_tex_destroy(impl_->gpu, &tex_uv);
        ::close(fd_y);
        ::close(fd_uv);
    }

    return true;
}

void PlCompose::finish() {
    if (impl_)
        pl_gpu_finish(impl_->gpu);
}

} // namespace pl_compose
