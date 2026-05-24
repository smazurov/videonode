#include "src/render/csc_placebo.hpp"

#include "src/common/log_levels.hpp"
#include "src/render/egl_ctx.hpp"

#include <EGL/egl.h>
#include <drm_fourcc.h>
#include <libplacebo/gpu.h>
#include <libplacebo/log.h>
#include <libplacebo/opengl.h>
#include <libplacebo/renderer.h>

#include <atomic>
#include <cstring>

namespace csc_placebo {

namespace {

struct State {
    egl_ctx::EglCtx ctx;
    pl_log logger = nullptr;
    pl_opengl gl = nullptr;
    pl_gpu gpu = nullptr;
    pl_renderer renderer = nullptr;
    bool ready = false;
};

State& state() {
    static State s;
    return s;
}

void log_once(const char* msg) {
    static std::atomic<bool> warned{false};
    if (!warned.exchange(true))
        vn::log::warn("csc_placebo: %s", msg);
}

} // namespace

bool init() {
    State& s = state();
    if (s.ready)
        return true;

    const char* candidates[] = {
        "/dev/dri/renderD128",
        "/dev/dri/renderD129",
        "/dev/dri/renderD130",
    };
    bool opened = false;
    for (const char* d : candidates) {
        if (s.ctx.init(d)) {
            opened = true;
            vn::log::info("csc_placebo: EGL on %s", d);
            break;
        }
    }
    if (!opened) {
        vn::log::error("csc_placebo: no DRM render node found");
        return false;
    }

    struct pl_log_params lp = {};
    lp.log_cb = nullptr;
    lp.log_level = PL_LOG_ERR;
    s.logger = pl_log_create(PL_API_VER, &lp);
    if (!s.logger) {
        vn::log::error("csc_placebo: pl_log_create failed");
        return false;
    }

    struct pl_opengl_params glp = {};
    glp.egl_display = s.ctx.display();
    glp.egl_context = s.ctx.context();
    glp.get_proc_addr = reinterpret_cast<pl_voidfunc_t (*)(const char*)>(eglGetProcAddress);
    s.gl = pl_opengl_create(s.logger, &glp);
    if (!s.gl) {
        vn::log::error("csc_placebo: pl_opengl_create failed");
        return false;
    }
    s.gpu = s.gl->gpu;

    if (!(s.gpu->import_caps.tex & PL_HANDLE_DMA_BUF)) {
        vn::log::error("csc_placebo: GPU does not support dma-buf import");
        return false;
    }

    s.renderer = pl_renderer_create(s.logger, s.gpu);
    if (!s.renderer) {
        vn::log::error("csc_placebo: pl_renderer_create failed");
        return false;
    }

    s.ready = true;
    vn::log::info("csc_placebo: ready (GLSL %d)", s.gpu->glsl.version);
    return true;
}

gbm_device* gbm_device_for_io() {
    State& s = state();
    return s.ready ? s.ctx.gbm() : nullptr;
}

void shutdown() {
    State& s = state();
    if (!s.ready)
        return;
    if (s.renderer)
        pl_renderer_destroy(&s.renderer);
    if (s.gl)
        pl_opengl_destroy(&s.gl);
    if (s.logger)
        pl_log_destroy(&s.logger);
    s.gpu = nullptr;
    s.ready = false;
}

bool convert(const csc::ConvertParams& src, const csc::ConvertParams& dst) {
    State& s = state();
    if (!s.ready && !init())
        return false;

    if (dst.fmt != csc::PixelFormat::Nv12) {
        log_once("dst.fmt != Nv12 — only NV12 output is supported");
        return false;
    }
    if (src.fmt != csc::PixelFormat::Nv24 && src.fmt != csc::PixelFormat::Nv12) {
        log_once("only NV12/NV24 input is implemented; other formats are TODO");
        return false;
    }
    if (src.width <= 0 || src.height <= 0 || (src.width & 1) || (src.height & 1))
        return false;
    if (dst.width != src.width || dst.height != src.height)
        return false;

    const int W = src.width;
    const int H = src.height;
    const bool src_is_nv12 = (src.fmt == csc::PixelFormat::Nv12);
    const int src_y_pitch = (src.wstride > 0 ? src.wstride : W);
    const int src_uv_pitch = src_is_nv12 ? src_y_pitch : src_y_pitch * 2;
    const int src_uv_w = src_is_nv12 ? (W / 2) : W;
    const int src_uv_h = src_is_nv12 ? (H / 2) : H;
    const int dst_y_pitch = (dst.wstride > 0 ? dst.wstride : W);
    const int dst_uv_pitch = (dst.uv_wstride > 0 ? dst.uv_wstride : dst_y_pitch);
    const int dst_y_size = dst_y_pitch * H;

    const bool src_split = (src.uv_fd >= 0);
    const bool dst_split = (dst.uv_fd >= 0);
    const int src_uv_actual_fd = src_split ? src.uv_fd : src.fd;
    const int src_uv_actual_offset = src_split ? 0 : (src_y_pitch * H);
    const int src_uv_actual_pitch = (src.uv_wstride > 0 ? src.uv_wstride : src_uv_pitch);
    const int dst_uv_actual_fd = dst_split ? dst.uv_fd : dst.fd;
    const int dst_uv_actual_offset = dst_split ? 0 : dst_y_size;

    // DRM_FORMAT_MOD_INVALID tells the GL backend "let the driver decide".
    constexpr uint64_t kModInvalid = (uint64_t{1} << 56) - 1;

    pl_fmt fmt_r8 = pl_find_named_fmt(s.gpu, "r8");
    pl_fmt fmt_rg8 = pl_find_named_fmt(s.gpu, "rg8");
    if (!fmt_r8 || !fmt_rg8)
        return false;

    // Import source Y (R8)
    struct pl_tex_params tp_src_y = {};
    tp_src_y.w = W;
    tp_src_y.h = H;
    tp_src_y.format = fmt_r8;
    tp_src_y.sampleable = true;
    tp_src_y.import_handle = PL_HANDLE_DMA_BUF;
    tp_src_y.shared_mem.handle.fd = dup(src.fd);
    tp_src_y.shared_mem.size = static_cast<size_t>(src_y_pitch) * H;
    tp_src_y.shared_mem.offset = 0;
    tp_src_y.shared_mem.drm_format_mod = kModInvalid;
    tp_src_y.shared_mem.stride_w = src_y_pitch;
    pl_tex tex_src_y = pl_tex_create(s.gpu, &tp_src_y);
    if (!tex_src_y)
        return false;

    // Import source UV (RG8)
    struct pl_tex_params tp_src_uv = {};
    tp_src_uv.w = src_uv_w;
    tp_src_uv.h = src_uv_h;
    tp_src_uv.format = fmt_rg8;
    tp_src_uv.sampleable = true;
    tp_src_uv.import_handle = PL_HANDLE_DMA_BUF;
    tp_src_uv.shared_mem.handle.fd = dup(src_uv_actual_fd);
    tp_src_uv.shared_mem.size =
        static_cast<size_t>(src_uv_actual_pitch) * src_uv_h + src_uv_actual_offset;
    tp_src_uv.shared_mem.offset = static_cast<size_t>(src_uv_actual_offset);
    tp_src_uv.shared_mem.drm_format_mod = kModInvalid;
    tp_src_uv.shared_mem.stride_w = src_uv_actual_pitch;
    pl_tex tex_src_uv = pl_tex_create(s.gpu, &tp_src_uv);
    if (!tex_src_uv) {
        pl_tex_destroy(s.gpu, &tex_src_y);
        return false;
    }

    // Import destination Y (R8, renderable)
    struct pl_tex_params tp_dst_y = {};
    tp_dst_y.w = W;
    tp_dst_y.h = H;
    tp_dst_y.format = fmt_r8;
    tp_dst_y.renderable = true;
    tp_dst_y.import_handle = PL_HANDLE_DMA_BUF;
    tp_dst_y.shared_mem.handle.fd = dup(dst.fd);
    tp_dst_y.shared_mem.size = static_cast<size_t>(dst_y_pitch) * H;
    tp_dst_y.shared_mem.offset = 0;
    tp_dst_y.shared_mem.drm_format_mod = kModInvalid;
    tp_dst_y.shared_mem.stride_w = dst_y_pitch;
    pl_tex tex_dst_y = pl_tex_create(s.gpu, &tp_dst_y);
    if (!tex_dst_y) {
        pl_tex_destroy(s.gpu, &tex_src_y);
        pl_tex_destroy(s.gpu, &tex_src_uv);
        return false;
    }

    // Import destination UV (RG8, renderable)
    struct pl_tex_params tp_dst_uv = {};
    tp_dst_uv.w = W / 2;
    tp_dst_uv.h = H / 2;
    tp_dst_uv.format = fmt_rg8;
    tp_dst_uv.renderable = true;
    tp_dst_uv.import_handle = PL_HANDLE_DMA_BUF;
    tp_dst_uv.shared_mem.handle.fd = dup(dst_uv_actual_fd);
    tp_dst_uv.shared_mem.size = static_cast<size_t>(dst_uv_pitch) * (H / 2) + dst_uv_actual_offset;
    tp_dst_uv.shared_mem.offset = static_cast<size_t>(dst_uv_actual_offset);
    tp_dst_uv.shared_mem.drm_format_mod = kModInvalid;
    tp_dst_uv.shared_mem.stride_w = dst_uv_pitch;
    pl_tex tex_dst_uv = pl_tex_create(s.gpu, &tp_dst_uv);
    if (!tex_dst_uv) {
        pl_tex_destroy(s.gpu, &tex_src_y);
        pl_tex_destroy(s.gpu, &tex_src_uv);
        pl_tex_destroy(s.gpu, &tex_dst_y);
        return false;
    }

    // Build frames
    struct pl_frame src_frame = {};
    src_frame.num_planes = 2;
    src_frame.planes[0].texture = tex_src_y;
    src_frame.planes[0].components = 1;
    src_frame.planes[0].component_mapping[0] = 0;
    src_frame.planes[1].texture = tex_src_uv;
    src_frame.planes[1].components = 2;
    src_frame.planes[1].component_mapping[0] = 1;
    src_frame.planes[1].component_mapping[1] = 2;
    src_frame.repr.sys = PL_COLOR_SYSTEM_BT_601;
    src_frame.repr.levels = PL_COLOR_LEVELS_LIMITED;
    src_frame.planes[1].shift_x = src_is_nv12 ? -1 : 0;
    src_frame.planes[1].shift_y = src_is_nv12 ? -1 : 0;
    pl_frame_set_chroma_location(&src_frame, PL_CHROMA_LEFT);

    struct pl_frame dst_frame = {};
    dst_frame.num_planes = 2;
    dst_frame.planes[0].texture = tex_dst_y;
    dst_frame.planes[0].components = 1;
    dst_frame.planes[0].component_mapping[0] = 0;
    dst_frame.planes[1].texture = tex_dst_uv;
    dst_frame.planes[1].components = 2;
    dst_frame.planes[1].component_mapping[0] = 1;
    dst_frame.planes[1].component_mapping[1] = 2;
    dst_frame.repr.sys = PL_COLOR_SYSTEM_BT_601;
    dst_frame.repr.levels = PL_COLOR_LEVELS_LIMITED;
    dst_frame.planes[1].shift_x = -1;
    dst_frame.planes[1].shift_y = -1;
    pl_frame_set_chroma_location(&dst_frame, PL_CHROMA_LEFT);

    struct pl_render_params params = pl_render_fast_params;
    params.skip_anti_aliasing = true;
    bool ok = pl_render_image(s.renderer, &src_frame, &dst_frame, &params);
    pl_gpu_finish(s.gpu);

    pl_tex_destroy(s.gpu, &tex_src_y);
    pl_tex_destroy(s.gpu, &tex_src_uv);
    pl_tex_destroy(s.gpu, &tex_dst_y);
    pl_tex_destroy(s.gpu, &tex_dst_uv);
    return ok;
}

} // namespace csc_placebo
