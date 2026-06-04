#include "src/render/csc_placebo.hpp"

#include "src/common/log_levels.hpp"
#include "src/render/egl_ctx.hpp"

#include <EGL/egl.h>
#include <drm_fourcc.h>
#include <libplacebo/gpu.h>
#include <libplacebo/log.h>
#include <libplacebo/opengl.h>
#include <libplacebo/renderer.h>
#include <unistd.h>

#include <unistd.h>

#include <atomic>
#include <cstring>
#include <vector>

#include <unistd.h>

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

constexpr uint64_t kModInvalid = (uint64_t{1} << 56) - 1;

struct PlaneImport {
    pl_gpu gpu;
    pl_fmt fmt;
    int fd;
    int w;
    int h;
    int pitch;
    int offset;
    bool renderable;
};

struct ImportedTex {
    pl_tex tex = nullptr;
    int fd = -1;
};

ImportedTex import_plane(const PlaneImport& p) {
    int fd_dup = dup(p.fd);
    struct pl_tex_params tp = {};
    tp.w = p.w;
    tp.h = p.h;
    tp.format = p.fmt;
    tp.sampleable = !p.renderable;
    tp.renderable = p.renderable;
    tp.import_handle = PL_HANDLE_DMA_BUF;
    tp.shared_mem.handle.fd = fd_dup;
    tp.shared_mem.size = static_cast<size_t>(p.pitch) * p.h + p.offset;
    tp.shared_mem.offset = static_cast<size_t>(p.offset);
    tp.shared_mem.drm_format_mod = kModInvalid;
    tp.shared_mem.stride_w = p.pitch;
    pl_tex tex = pl_tex_create(p.gpu, &tp);
    if (!tex) {
        ::close(fd_dup);
        return {};
    }
    return {.tex = tex, .fd = fd_dup};
}

// Owns imported textures; destroys each tex and closes its dup'd fd on
// scope exit, so convert() needs no per-error-path cleanup.
class TexBag {
  public:
    explicit TexBag(pl_gpu gpu) : gpu_(gpu) {}
    ~TexBag() {
        for (auto& t : items_) {
            pl_tex_destroy(gpu_, &t.tex);
            ::close(t.fd);
        }
    }
    TexBag(const TexBag&) = delete;
    TexBag& operator=(const TexBag&) = delete;

    pl_tex add(const PlaneImport& p) {
        ImportedTex t = import_plane(p);
        if (t.tex)
            items_.push_back(t);
        return t.tex;
    }

  private:
    pl_gpu gpu_;
    std::vector<ImportedTex> items_;
};

struct CscTextures {
    pl_tex src_y = nullptr;
    pl_tex src_uv = nullptr;
    pl_tex src_rgb = nullptr;           // non-null => single-plane RGB source
    pl_tex src_packed_luma = nullptr;   // packed 4:2:2: full-res luma view
    pl_tex src_packed_chroma = nullptr; // packed 4:2:2: half-width Y0/U/Y1/V view
    pl_tex dst_y = nullptr;
    pl_tex dst_uv = nullptr;
    bool src_is_nv12 = false;
    int src_w = 0, src_h = 0;
    int dst_w = 0, dst_h = 0;
    bool dst_bt709 = false;
};

using SetFrameFn = void (*)(pl_frame&, const CscTextures&);

void set_crop(pl_frame& f, const CscTextures& t) {
    f.crop = {
        .x0 = 0, .y0 = 0, .x1 = static_cast<float>(t.src_w), .y1 = static_cast<float>(t.src_h)};
}

void set_frame_rgb(pl_frame& f, const CscTextures& t) {
    f.num_planes = 1;
    f.planes[0].texture = t.src_rgb;
    f.planes[0].components = 4;
    for (int i = 0; i < 4; ++i)
        f.planes[0].component_mapping[i] = i;
    f.repr.sys = PL_COLOR_SYSTEM_RGB;
    f.repr.levels = PL_COLOR_LEVELS_FULL;
    set_crop(f, t);
}

void set_frame_nv(pl_frame& f, const CscTextures& t) {
    f.num_planes = 2;
    f.planes[0].texture = t.src_y;
    f.planes[0].components = 1;
    f.planes[0].component_mapping[0] = 0;
    f.planes[1].texture = t.src_uv;
    f.planes[1].components = 2;
    f.planes[1].component_mapping[0] = 1;
    f.planes[1].component_mapping[1] = 2;
    f.repr.sys = PL_COLOR_SYSTEM_BT_601;
    f.repr.levels = PL_COLOR_LEVELS_LIMITED;
    f.planes[1].shift_x = t.src_is_nv12 ? -1 : 0;
    f.planes[1].shift_y = t.src_is_nv12 ? -1 : 0;
    pl_frame_set_chroma_location(&f, PL_CHROMA_LEFT);
    set_crop(f, t);
}

// Packed 4:2:2 as two views of one dma-buf: a full-res rg8 luma view (.r=Y for
// YUYV, .g=Y for UYVY) and a half-width rgba8 chroma view whose texel is the
// {Y0,U,Y1,V} (YUYV) or {U,Y0,V,Y1} (UYVY) quad. The half-vs-full width lets
// libplacebo infer 2:1 horizontal subsampling; sparse component_mapping routes
// the chroma channels and drops the luma ones.
void set_frame_packed(pl_frame& f, const CscTextures& t, bool uyvy) {
    f.num_planes = 2;
    f.planes[0].texture = t.src_packed_luma;
    f.planes[0].components = uyvy ? 2 : 1;
    f.planes[0].component_mapping[0] = uyvy ? PL_CHANNEL_NONE : 0;
    if (uyvy)
        f.planes[0].component_mapping[1] = 0;
    f.planes[1].texture = t.src_packed_chroma;
    f.planes[1].components = 4;
    f.planes[1].component_mapping[0] = uyvy ? 1 : PL_CHANNEL_NONE;
    f.planes[1].component_mapping[1] = uyvy ? PL_CHANNEL_NONE : 1;
    f.planes[1].component_mapping[2] = uyvy ? 2 : PL_CHANNEL_NONE;
    f.planes[1].component_mapping[3] = uyvy ? PL_CHANNEL_NONE : 2;
    f.repr.sys = PL_COLOR_SYSTEM_BT_601;
    f.repr.levels = PL_COLOR_LEVELS_LIMITED;
    pl_frame_set_chroma_location(&f, PL_CHROMA_LEFT);
    set_crop(f, t);
}

void set_frame_yuyv(pl_frame& f, const CscTextures& t) {
    set_frame_packed(f, t, false);
}
void set_frame_uyvy(pl_frame& f, const CscTextures& t) {
    set_frame_packed(f, t, true);
}

bool import_bgra(pl_gpu gpu, TexBag& bag, const csc::ConvertParams& src, CscTextures& t) {
    pl_fmt fmt = pl_find_named_fmt(gpu, "bgra8");
    if (!fmt)
        fmt = pl_find_named_fmt(gpu, "rgba8");
    if (!fmt)
        return false;
    const int pitch = src.wstride > 0 ? src.wstride : src.width * 4;
    t.src_rgb = bag.add({.gpu = gpu,
                         .fmt = fmt,
                         .fd = src.fd,
                         .w = src.width,
                         .h = src.height,
                         .pitch = pitch,
                         .offset = 0,
                         .renderable = false});
    return t.src_rgb != nullptr;
}

bool import_nv(pl_gpu gpu, TexBag& bag, const csc::ConvertParams& src, CscTextures& t) {
    pl_fmt fmt_r8 = pl_find_named_fmt(gpu, "r8");
    pl_fmt fmt_rg8 = pl_find_named_fmt(gpu, "rg8");
    if (!fmt_r8 || !fmt_rg8)
        return false;
    const int W = src.width;
    const int H = src.height;
    t.src_is_nv12 = (src.fmt == csc::PixelFormat::Nv12);
    const int y_pitch = src.wstride > 0 ? src.wstride : W;
    const int uv_pitch =
        src.uv_wstride > 0 ? src.uv_wstride : (t.src_is_nv12 ? y_pitch : y_pitch * 2);
    const int uv_w = t.src_is_nv12 ? W / 2 : W;
    const int uv_h = t.src_is_nv12 ? H / 2 : H;
    const bool split = src.uv_fd >= 0;
    const int uv_fd = split ? src.uv_fd : src.fd;
    const int uv_off = split ? 0 : y_pitch * H;
    t.src_y = bag.add({.gpu = gpu,
                       .fmt = fmt_r8,
                       .fd = src.fd,
                       .w = W,
                       .h = H,
                       .pitch = y_pitch,
                       .offset = 0,
                       .renderable = false});
    if (!t.src_y)
        return false;
    t.src_uv = bag.add({.gpu = gpu,
                        .fmt = fmt_rg8,
                        .fd = uv_fd,
                        .w = uv_w,
                        .h = uv_h,
                        .pitch = uv_pitch,
                        .offset = uv_off,
                        .renderable = false});
    return t.src_uv != nullptr;
}

bool import_packed_422(pl_gpu gpu, TexBag& bag, const csc::ConvertParams& src, CscTextures& t) {
    pl_fmt fmt_rg8 = pl_find_named_fmt(gpu, "rg8");
    pl_fmt fmt_rgba8 = pl_find_named_fmt(gpu, "rgba8");
    if (!fmt_rg8 || !fmt_rgba8)
        return false;
    const int W = src.width;
    const int H = src.height;
    const int pitch = src.wstride > 0 ? src.wstride : W * 2;
    t.src_packed_luma = bag.add({.gpu = gpu,
                                 .fmt = fmt_rg8,
                                 .fd = src.fd,
                                 .w = W,
                                 .h = H,
                                 .pitch = pitch,
                                 .offset = 0,
                                 .renderable = false});
    if (!t.src_packed_luma)
        return false;
    t.src_packed_chroma = bag.add({.gpu = gpu,
                                   .fmt = fmt_rgba8,
                                   .fd = src.fd,
                                   .w = W / 2,
                                   .h = H,
                                   .pitch = pitch,
                                   .offset = 0,
                                   .renderable = false});
    return t.src_packed_chroma != nullptr;
}

struct SrcFormatDesc {
    bool (*import)(pl_gpu, TexBag&, const csc::ConvertParams&, CscTextures&);
    SetFrameFn set_frame;
};

const SrcFormatDesc* find_src_desc(csc::PixelFormat f) {
    switch (f) {
    case csc::PixelFormat::Bgra: {
        static const SrcFormatDesc d{.import = import_bgra, .set_frame = set_frame_rgb};
        return &d;
    }
    case csc::PixelFormat::Nv12:
    case csc::PixelFormat::Nv24: {
        static const SrcFormatDesc d{.import = import_nv, .set_frame = set_frame_nv};
        return &d;
    }
    case csc::PixelFormat::Yuyv: {
        static const SrcFormatDesc d{.import = import_packed_422, .set_frame = set_frame_yuyv};
        return &d;
    }
    case csc::PixelFormat::Uyvy: {
        static const SrcFormatDesc d{.import = import_packed_422, .set_frame = set_frame_uyvy};
        return &d;
    }
    case csc::PixelFormat::Nv16:
    case csc::PixelFormat::Bgr3:
        break;
    }
    return nullptr;
}

bool render_csc(pl_renderer renderer, pl_gpu gpu, const CscTextures& t, SetFrameFn set_frame) {
    struct pl_frame src_frame = {};
    set_frame(src_frame, t);

    struct pl_frame dst_frame = {};
    dst_frame.num_planes = 2;
    dst_frame.planes[0].texture = t.dst_y;
    dst_frame.planes[0].components = 1;
    dst_frame.planes[0].component_mapping[0] = 0;
    dst_frame.planes[1].texture = t.dst_uv;
    dst_frame.planes[1].components = 2;
    dst_frame.planes[1].component_mapping[0] = 1;
    dst_frame.planes[1].component_mapping[1] = 2;
    dst_frame.repr.sys = t.dst_bt709 ? PL_COLOR_SYSTEM_BT_709 : PL_COLOR_SYSTEM_BT_601;
    dst_frame.repr.levels = PL_COLOR_LEVELS_LIMITED;
    dst_frame.planes[1].shift_x = -1;
    dst_frame.planes[1].shift_y = -1;
    pl_frame_set_chroma_location(&dst_frame, PL_CHROMA_LEFT);
    dst_frame.crop = {
        .x0 = 0, .y0 = 0, .x1 = static_cast<float>(t.dst_w), .y1 = static_cast<float>(t.dst_h)};

    struct pl_render_params params = pl_render_fast_params;
    params.skip_anti_aliasing = true;
    bool ok = pl_render_image(renderer, &src_frame, &dst_frame, &params);
    pl_gpu_finish(gpu);
    return ok;
}

bool import_dst(pl_gpu gpu, TexBag& bag, const csc::ConvertParams& dst, CscTextures& t) {
    pl_fmt fmt_r8 = pl_find_named_fmt(gpu, "r8");
    pl_fmt fmt_rg8 = pl_find_named_fmt(gpu, "rg8");
    if (!fmt_r8 || !fmt_rg8)
        return false;
    const int W = dst.width;
    const int H = dst.height;
    const int y_pitch = dst.wstride > 0 ? dst.wstride : W;
    const int uv_pitch = dst.uv_wstride > 0 ? dst.uv_wstride : y_pitch;
    const bool split = dst.uv_fd >= 0;
    const int uv_fd = split ? dst.uv_fd : dst.fd;
    const int uv_off = split ? 0 : y_pitch * H;
    t.dst_y = bag.add({.gpu = gpu,
                       .fmt = fmt_r8,
                       .fd = dst.fd,
                       .w = W,
                       .h = H,
                       .pitch = y_pitch,
                       .offset = 0,
                       .renderable = true});
    if (!t.dst_y)
        return false;
    t.dst_uv = bag.add({.gpu = gpu,
                        .fmt = fmt_rg8,
                        .fd = uv_fd,
                        .w = W / 2,
                        .h = H / 2,
                        .pitch = uv_pitch,
                        .offset = uv_off,
                        .renderable = true});
    return t.dst_uv != nullptr;
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
    const SrcFormatDesc* desc = find_src_desc(src.fmt);
    if (!desc) {
        log_once("unsupported source format");
        return false;
    }
    if (src.width <= 0 || src.height <= 0 || (src.width & 1) || (src.height & 1))
        return false;
    if (dst.width <= 0 || dst.height <= 0 || (dst.width & 1) || (dst.height & 1))
        return false;

    CscTextures t;
    t.src_w = src.width;
    t.src_h = src.height;
    t.dst_w = dst.width;
    t.dst_h = dst.height;
    t.dst_bt709 = (dst.color_space == csc::ColorSpace::Bt709Limited);

    TexBag bag(s.gpu);
    if (!desc->import(s.gpu, bag, src, t) || !import_dst(s.gpu, bag, dst, t))
        return false;
    return render_csc(s.renderer, s.gpu, t, desc->set_frame);
}

} // namespace csc_placebo
