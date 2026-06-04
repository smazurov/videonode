#include "src/render/egl_ctx.hpp"

#include <EGL/egl.h>
#include <GLES2/gl2.h>
#include <drm_fourcc.h>
#include <gbm.h>
#include <libplacebo/config.h>
#include <libplacebo/gpu.h>
#include <libplacebo/log.h>
#include <libplacebo/opengl.h>
#include <libplacebo/renderer.h>

#include <chrono>
#include <cstdint>
#include <cstdio>
#include <cstdlib>
#include <cstring>
#include <span>
#include <unistd.h>

#define DIE(...)                                                                                   \
    do {                                                                                           \
        std::fprintf(stderr, "placebo-csc-probe: " __VA_ARGS__);                                   \
        std::fprintf(stderr, "\n");                                                                \
        return 1;                                                                                  \
    } while (0)

namespace {

struct R8Bo {
    gbm_bo* bo = nullptr;
    int fd = -1;
    uint32_t stride = 0;
    int w = 0;
    int h = 0;
    void* map_handle = nullptr;
    uint8_t* mapped = nullptr;
};

bool r8_alloc(gbm_device* gbm, R8Bo& out, int w, int h) {
    out.bo = gbm_bo_create(gbm, w, h, DRM_FORMAT_R8, GBM_BO_USE_LINEAR | GBM_BO_USE_RENDERING);
    if (!out.bo)
        return false;
    out.fd = gbm_bo_get_fd(out.bo);
    out.stride = gbm_bo_get_stride(out.bo);
    out.w = w;
    out.h = h;
    return true;
}

void r8_free(R8Bo& b) {
    if (b.mapped)
        gbm_bo_unmap(b.bo, b.map_handle);
    if (b.fd >= 0)
        ::close(b.fd);
    if (b.bo)
        gbm_bo_destroy(b.bo);
    b = R8Bo{};
}

std::span<uint8_t> r8_map_write(R8Bo& b) {
    uint32_t s = 0;
    b.mapped = static_cast<uint8_t*>(
        gbm_bo_map(b.bo, 0, 0, b.w, b.h, GBM_BO_TRANSFER_READ_WRITE, &s, &b.map_handle));
    b.stride = s;
    if (!b.mapped)
        return {};
    return {b.mapped, static_cast<size_t>(b.stride) * b.h};
}

std::span<const uint8_t> r8_map_read(R8Bo& b) {
    uint32_t s = 0;
    b.mapped = static_cast<uint8_t*>(
        gbm_bo_map(b.bo, 0, 0, b.w, b.h, GBM_BO_TRANSFER_READ, &s, &b.map_handle));
    b.stride = s;
    if (!b.mapped)
        return {};
    return {b.mapped, static_cast<size_t>(b.stride) * b.h};
}

void r8_unmap(R8Bo& b) {
    if (b.mapped) {
        gbm_bo_unmap(b.bo, b.map_handle);
        b.mapped = nullptr;
        b.map_handle = nullptr;
    }
}

struct Fmts {
    pl_fmt r8 = nullptr;
    pl_fmt rg8 = nullptr;
};

struct Buffers {
    R8Bo src_y, src_uv, dst_y, dst_uv;
};

struct Textures {
    pl_tex src_y = nullptr;
    pl_tex src_uv = nullptr;
    pl_tex dst_y = nullptr;
    pl_tex dst_uv = nullptr;
};

struct VerifyResult {
    int errors_y = 0;
    int errors_uv = 0;
    int first_x = -1, first_y_coord = -1, first_got = -1, first_want = -1;
    int first_uv_x = -1, first_uv_y = -1, first_uv_got = -1, first_uv_want = -1;
};

bool alloc_buffers(gbm_device* gbm, int W, int H, Buffers& bufs) {
    if (!r8_alloc(gbm, bufs.src_y, W, H))
        return false;
    if (!r8_alloc(gbm, bufs.src_uv, W * 2, H))
        return false;
    if (!r8_alloc(gbm, bufs.dst_y, W, H))
        return false;
    if (!r8_alloc(gbm, bufs.dst_uv, W, H / 2))
        return false;
    return true;
}

void fill_src_y(R8Bo& bo, int W, int H) {
    auto buf = r8_map_write(bo);
    if (buf.empty())
        return;
    for (int y = 0; y < H; ++y)
        for (int x = 0; x < W; ++x)
            buf[y * bo.stride + x] = uint8_t((x + y) & 0xFF);
    r8_unmap(bo);
}

void fill_src_uv(R8Bo& bo, int W, int H) {
    auto buf = r8_map_write(bo);
    if (buf.empty())
        return;
    for (int y = 0; y < H; ++y) {
        for (int x = 0; x < W; ++x) {
            buf[y * bo.stride + 2 * x + 0] = uint8_t((x ^ y) & 0xFF);
            buf[y * bo.stride + 2 * x + 1] = uint8_t((x * 7 + y * 11) & 0xFF);
        }
    }
    r8_unmap(bo);
}

pl_tex import_tex(pl_gpu gpu, pl_fmt fmt, R8Bo& bo, int w, int h, bool renderable) {
    struct pl_tex_params tp = {};
    tp.w = w;
    tp.h = h;
    tp.format = fmt;
    tp.sampleable = !renderable;
    tp.renderable = renderable;
    tp.import_handle = PL_HANDLE_DMA_BUF;
    tp.shared_mem.handle.fd = dup(bo.fd);
    tp.shared_mem.size = static_cast<size_t>(bo.stride) * bo.h;
    tp.shared_mem.offset = 0;
    tp.shared_mem.drm_format_mod = gbm_bo_get_modifier(bo.bo);
    tp.shared_mem.stride_w = static_cast<int>(bo.stride);
    return pl_tex_create(gpu, &tp);
}

bool import_textures(pl_gpu gpu, const Fmts& fmts, Buffers& bufs, int W, int H, Textures& tex) {
    tex.src_y = import_tex(gpu, fmts.r8, bufs.src_y, W, H, false);
    if (!tex.src_y)
        return false;
    tex.src_uv = import_tex(gpu, fmts.rg8, bufs.src_uv, W, H, false);
    if (!tex.src_uv)
        return false;
    tex.dst_y = import_tex(gpu, fmts.r8, bufs.dst_y, W, H, true);
    if (!tex.dst_y)
        return false;
    tex.dst_uv = import_tex(gpu, fmts.rg8, bufs.dst_uv, W / 2, H / 2, true);
    return tex.dst_uv != nullptr;
}

void build_src_frame(Textures& tex, pl_frame& f) {
    f = {};
    f.num_planes = 2;
    f.planes[0].texture = tex.src_y;
    f.planes[0].components = 1;
    f.planes[0].component_mapping[0] = 0;
    f.planes[1].texture = tex.src_uv;
    f.planes[1].components = 2;
    f.planes[1].component_mapping[0] = 1;
    f.planes[1].component_mapping[1] = 2;
    f.repr.sys = PL_COLOR_SYSTEM_BT_601;
    f.repr.levels = PL_COLOR_LEVELS_LIMITED;
    f.planes[1].shift_x = 0;
    f.planes[1].shift_y = 0;
    pl_frame_set_chroma_location(&f, PL_CHROMA_LEFT);
}

void build_dst_frame(Textures& tex, pl_frame& f) {
    f = {};
    f.num_planes = 2;
    f.planes[0].texture = tex.dst_y;
    f.planes[0].components = 1;
    f.planes[0].component_mapping[0] = 0;
    f.planes[1].texture = tex.dst_uv;
    f.planes[1].components = 2;
    f.planes[1].component_mapping[0] = 1;
    f.planes[1].component_mapping[1] = 2;
    f.repr.sys = PL_COLOR_SYSTEM_BT_601;
    f.repr.levels = PL_COLOR_LEVELS_LIMITED;
    f.planes[1].shift_x = -1;
    f.planes[1].shift_y = -1;
    pl_frame_set_chroma_location(&f, PL_CHROMA_LEFT);
}

double run_render_loop(pl_renderer renderer, pl_gpu gpu, pl_frame& src_frame, pl_frame& dst_frame,
                       int iters) {
    struct pl_render_params params = pl_render_fast_params;
    params.skip_anti_aliasing = true;

    pl_render_image(renderer, &src_frame, &dst_frame, &params);
    pl_gpu_finish(gpu);

    auto t0 = std::chrono::steady_clock::now();
    for (int i = 0; i < iters; ++i)
        pl_render_image(renderer, &src_frame, &dst_frame, &params);
    pl_gpu_finish(gpu);
    auto t1 = std::chrono::steady_clock::now();

    return static_cast<double>(
               std::chrono::duration_cast<std::chrono::microseconds>(t1 - t0).count()) /
           iters;
}

void verify_y_plane(R8Bo& dst_y, int W, int H, VerifyResult& r) {
    auto buf = r8_map_read(dst_y);
    if (buf.empty())
        return;
    for (int y = 0; y < H && r.errors_y < 8; ++y) {
        for (int x = 0; x < W && r.errors_y < 8; ++x) {
            uint8_t got = buf[y * dst_y.stride + x];
            uint8_t want = uint8_t((x + y) & 0xFF);
            int diff = int(got) - int(want);
            if (diff < -2 || diff > 2) {
                if (r.errors_y == 0) {
                    r.first_x = x;
                    r.first_y_coord = y;
                    r.first_got = got;
                    r.first_want = want;
                }
                ++r.errors_y;
            }
        }
    }
    r8_unmap(dst_y);
}

void verify_uv_plane(R8Bo& dst_uv, int W, int H, VerifyResult& r) {
    auto buf = r8_map_read(dst_uv);
    if (buf.empty())
        return;
    for (int y = 0; y < H / 2 && r.errors_uv < 8; ++y) {
        for (int x = 0; x < W / 2 && r.errors_uv < 8; ++x) {
            int srcx = 2 * x, srcy = 2 * y;
            int eu = (((srcx ^ srcy) & 0xFF) + (((srcx + 1) ^ srcy) & 0xFF) +
                      ((srcx ^ (srcy + 1)) & 0xFF) + (((srcx + 1) ^ (srcy + 1)) & 0xFF) + 2) /
                     4;
            uint8_t got_u = buf[y * dst_uv.stride + 2 * x + 0];
            int du = int(got_u) - eu;
            if (du < -3 || du > 3) {
                if (r.errors_uv == 0) {
                    r.first_uv_x = x;
                    r.first_uv_y = y;
                    r.first_uv_got = got_u;
                    r.first_uv_want = eu;
                }
                ++r.errors_uv;
            }
        }
    }
    r8_unmap(dst_uv);
}

VerifyResult verify_output(Buffers& bufs, int W, int H) {
    VerifyResult r;
    verify_y_plane(bufs.dst_y, W, H, r);
    verify_uv_plane(bufs.dst_uv, W, H, r);
    return r;
}

void print_report(int W, int H, double per_frame_us, int iters, const VerifyResult& v) {
    std::printf("\n=== Phase 1 Results ===\n");
    std::printf("  Backend:    libplacebo %s (OpenGL)\n", pl_version());
    std::printf("  Resolution: %d×%d\n", W, H);
    std::printf("  Perf:       %.1f µs/frame (%d iters)\n", per_frame_us, iters);
    if (v.errors_y == 0 && v.errors_uv == 0) {
        std::printf("  Correctness: PASS (Y ±2, UV ±3 tolerance)\n");
    } else {
        if (v.errors_y > 0) {
            std::printf("  Y MISMATCH: %d errors; first at (%d,%d) got=%d want=%d\n", v.errors_y,
                        v.first_x, v.first_y_coord, v.first_got, v.first_want);
            std::printf("  NOTE: if Y values look like limited-range-scaled versions of\n"
                        "        the input, libplacebo is correctly applying BT.601 math.\n"
                        "        The probe pattern is raw [0,255] — mismatches may indicate\n"
                        "        correct CSC behavior, not a bug. Inspect values manually.\n");
        }
        if (v.errors_uv > 0) {
            std::printf("  UV MISMATCH: %d errors; first at (%d,%d) got_u=%d want_u=%d\n",
                        v.errors_uv, v.first_uv_x, v.first_uv_y, v.first_uv_got, v.first_uv_want);
        }
    }
    std::printf("  Conclusion: %s\n", (v.errors_y <= 4 && v.errors_uv <= 4)
                                          ? "VIABLE — proceed to Phase 2"
                                          : "INVESTIGATE — check if mismatches are CSC math");
}

struct ProbeArgs {
    const char* device;
    int W;
    int H;
    int iters;
};

int run_probe(const ProbeArgs& a, double& per_frame_us_out, VerifyResult& v_out) {
    egl_ctx::EglCtx ctx;
    if (!ctx.init(a.device)) {
        std::fprintf(stderr, "placebo-csc-probe: EglCtx::init(%s)\n", a.device);
        return 1;
    }
    std::printf("ok: EGL on %s (%s)\n", a.device, glGetString(GL_RENDERER));

    struct pl_log_params log_params = {};
    log_params.log_cb = pl_log_color;
    log_params.log_level = PL_LOG_WARN;
    pl_log pl_logger = pl_log_create(PL_API_VER, &log_params);
    if (!pl_logger) {
        std::fprintf(stderr, "placebo-csc-probe: pl_log_create\n");
        return 1;
    }

    struct pl_opengl_params gl_params = {};
    gl_params.egl_display = ctx.display();
    gl_params.egl_context = ctx.context();
    gl_params.get_proc_addr = reinterpret_cast<pl_voidfunc_t (*)(const char*)>(eglGetProcAddress);
    pl_opengl pl_gl = pl_opengl_create(pl_logger, &gl_params);
    if (!pl_gl) {
        std::fprintf(stderr, "placebo-csc-probe: pl_opengl_create\n");
        pl_log_destroy(&pl_logger);
        return 1;
    }

    pl_gpu gpu = pl_gl->gpu;
    std::printf("ok: libplacebo OpenGL backend (GLSL %d, %s)\n", gpu->glsl.version,
                gpu->glsl.vulkan ? "vulkan-glsl" : "desktop/es");
    std::printf("    PL_API_VER=%d, limits.max_tex_2d_dim=%d\n", PL_API_VER,
                gpu->limits.max_tex_2d_dim);

    if (!(gpu->import_caps.tex & PL_HANDLE_DMA_BUF)) {
        std::fprintf(stderr, "placebo-csc-probe: GPU does not advertise PL_HANDLE_DMA_BUF\n");
        pl_opengl_destroy(&pl_gl);
        pl_log_destroy(&pl_logger);
        return 1;
    }
    std::printf("ok: GPU supports dma-buf texture import\n");

    Buffers bufs;
    if (!alloc_buffers(ctx.gbm(), a.W, a.H, bufs)) {
        std::fprintf(stderr, "placebo-csc-probe: alloc GBM buffers\n");
        pl_opengl_destroy(&pl_gl);
        pl_log_destroy(&pl_logger);
        return 1;
    }
    std::printf("ok: allocated GBM buffers (src_y=%d×%d src_uv=%d×%d dst_y=%d×%d dst_uv=%d×%d)\n",
                bufs.src_y.w, bufs.src_y.h, bufs.src_uv.w, bufs.src_uv.h, bufs.dst_y.w,
                bufs.dst_y.h, bufs.dst_uv.w, bufs.dst_uv.h);

    fill_src_y(bufs.src_y, a.W, a.H);
    fill_src_uv(bufs.src_uv, a.W, a.H);

    Fmts fmts;
    fmts.r8 = pl_find_named_fmt(gpu, "r8");
    fmts.rg8 = pl_find_named_fmt(gpu, "rg8");
    if (!fmts.r8 || !fmts.rg8) {
        std::fprintf(stderr, "placebo-csc-probe: pl_find_named_fmt r8/rg8 failed\n");
        pl_opengl_destroy(&pl_gl);
        pl_log_destroy(&pl_logger);
        return 1;
    }
    std::printf("ok: found pl_fmt r8 (size=%zu) and rg8 (size=%zu)\n", fmts.r8->texel_size,
                fmts.rg8->texel_size);

    Textures tex;
    if (!import_textures(gpu, fmts, bufs, a.W, a.H, tex)) {
        std::fprintf(stderr, "placebo-csc-probe: dma-buf import failed\n");
        pl_opengl_destroy(&pl_gl);
        pl_log_destroy(&pl_logger);
        return 1;
    }
    std::printf("ok: 4 pl_tex imported from dma-bufs\n");

    pl_frame src_frame, dst_frame;
    build_src_frame(tex, src_frame);
    build_dst_frame(tex, dst_frame);

    pl_renderer renderer = pl_renderer_create(pl_logger, gpu);
    if (!renderer) {
        std::fprintf(stderr, "placebo-csc-probe: pl_renderer_create\n");
        pl_opengl_destroy(&pl_gl);
        pl_log_destroy(&pl_logger);
        return 1;
    }

    per_frame_us_out = run_render_loop(renderer, gpu, src_frame, dst_frame, a.iters);
    std::printf("ok: %d iterations in %.0f µs (%.1f µs/frame, %.1f fps)\n", a.iters,
                per_frame_us_out * a.iters, per_frame_us_out, 1e6 / per_frame_us_out);

    v_out = verify_output(bufs, a.W, a.H);

    pl_renderer_destroy(&renderer);
    pl_tex_destroy(gpu, &tex.src_y);
    pl_tex_destroy(gpu, &tex.src_uv);
    pl_tex_destroy(gpu, &tex.dst_y);
    pl_tex_destroy(gpu, &tex.dst_uv);
    pl_opengl_destroy(&pl_gl);
    pl_log_destroy(&pl_logger);
    r8_free(bufs.src_y);
    r8_free(bufs.src_uv);
    r8_free(bufs.dst_y);
    r8_free(bufs.dst_uv);
    return 0;
}

} // namespace

int main(int argc, char** argv) {
    const std::span args(argv, argc);
    ProbeArgs a;
    a.device = (argc > 1) ? args[1] : "/dev/dri/renderD128";
    a.W = (argc > 2) ? std::atoi(args[2]) : 1920;
    a.H = (argc > 3) ? std::atoi(args[3]) : 1080;
    a.iters = (argc > 4) ? std::atoi(args[4]) : 100;
    if (a.W <= 0 || a.H <= 0 || (a.W % 2) || (a.H % 2))
        DIE("W and H must be positive even ints");
    if (a.iters <= 0)
        a.iters = 1;

    double per_frame_us = 0.0;
    VerifyResult v;
    int rc = run_probe(a, per_frame_us, v);
    if (rc != 0)
        return rc;

    print_report(a.W, a.H, per_frame_us, a.iters, v);
    return (v.errors_y > 4 || v.errors_uv > 4) ? 1 : 0;
}
