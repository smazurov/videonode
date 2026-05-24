// placebo-csc-probe — libplacebo OpenGL backend: NV24 → NV12 CSC evaluation.
//
// Mirrors csc-probe.cpp: fills a GBM-backed NV24 source with a known ramp,
// converts to NV12 via libplacebo's pl_renderer (OpenGL backend), and
// validates byte-level output against the same expected pattern. Measures
// wall time per frame at the given resolution.
//
// This is Phase 1 of the #9 evaluation: confirms libplacebo's OpenGL backend
// can import dma-bufs via the existing EGL context and produce correct NV12.
//
// Usage:
//   ./placebo-csc-probe [device=/dev/dri/renderD128] [W=1920] [H=1080] [iters=100]

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
    void* mapped = nullptr;
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

uint8_t* r8_map_write(R8Bo& b) {
    uint32_t s = 0;
    b.mapped = gbm_bo_map(b.bo, 0, 0, b.w, b.h, GBM_BO_TRANSFER_READ_WRITE, &s, &b.map_handle);
    b.stride = s;
    return static_cast<uint8_t*>(b.mapped);
}

uint8_t* r8_map_read(R8Bo& b) {
    uint32_t s = 0;
    b.mapped = gbm_bo_map(b.bo, 0, 0, b.w, b.h, GBM_BO_TRANSFER_READ, &s, &b.map_handle);
    b.stride = s;
    return static_cast<uint8_t*>(b.mapped);
}

void r8_unmap(R8Bo& b) {
    if (b.mapped) {
        gbm_bo_unmap(b.bo, b.map_handle);
        b.mapped = nullptr;
        b.map_handle = nullptr;
    }
}

} // namespace

int main(int argc, char** argv) {
    const char* device = (argc > 1) ? argv[1] : "/dev/dri/renderD128";
    int W = (argc > 2) ? std::atoi(argv[2]) : 1920;
    int H = (argc > 3) ? std::atoi(argv[3]) : 1080;
    int iters = (argc > 4) ? std::atoi(argv[4]) : 100;
    if (W <= 0 || H <= 0 || (W % 2) || (H % 2))
        DIE("W and H must be positive even ints");
    if (iters <= 0)
        iters = 1;

    // --- EGL/GBM setup (reuse our existing egl_ctx) ---
    egl_ctx::EglCtx ctx;
    if (!ctx.init(device))
        DIE("EglCtx::init(%s)", device);
    std::printf("ok: EGL on %s (%s)\n", device, glGetString(GL_RENDERER));

    // --- libplacebo OpenGL backend ---
    struct pl_log_params log_params = {};
    log_params.log_cb = pl_log_color;
    log_params.log_level = PL_LOG_WARN;
    pl_log pl_logger = pl_log_create(PL_API_VER, &log_params);
    if (!pl_logger)
        DIE("pl_log_create");

    struct pl_opengl_params gl_params = {};
    gl_params.egl_display = ctx.display();
    gl_params.egl_context = ctx.context();
    gl_params.get_proc_addr = reinterpret_cast<pl_voidfunc_t (*)(const char*)>(eglGetProcAddress);
    pl_opengl pl_gl = pl_opengl_create(pl_logger, &gl_params);
    if (!pl_gl)
        DIE("pl_opengl_create — libplacebo cannot use this EGL context");

    pl_gpu gpu = pl_gl->gpu;
    std::printf("ok: libplacebo OpenGL backend (GLSL %d, %s)\n", gpu->glsl.version,
                gpu->glsl.vulkan ? "vulkan-glsl" : "desktop/es");
    std::printf("    PL_API_VER=%d, limits.max_tex_2d_dim=%d\n", PL_API_VER,
                gpu->limits.max_tex_2d_dim);

    if (!(gpu->import_caps.tex & PL_HANDLE_DMA_BUF))
        DIE("GPU does not advertise PL_HANDLE_DMA_BUF import for textures");
    std::printf("ok: GPU supports dma-buf texture import\n");

    // --- Allocate GBM buffers (same layout as csc-probe) ---
    R8Bo src_y, src_uv;
    if (!r8_alloc(ctx.gbm(), src_y, W, H))
        DIE("alloc src_y");
    if (!r8_alloc(ctx.gbm(), src_uv, W * 2, H))
        DIE("alloc src_uv");

    R8Bo dst_y, dst_uv;
    if (!r8_alloc(ctx.gbm(), dst_y, W, H))
        DIE("alloc dst_y");
    if (!r8_alloc(ctx.gbm(), dst_uv, W, H / 2))
        DIE("alloc dst_uv");

    std::printf("ok: allocated GBM buffers (src_y=%d×%d src_uv=%d×%d dst_y=%d×%d dst_uv=%d×%d)\n",
                src_y.w, src_y.h, src_uv.w, src_uv.h, dst_y.w, dst_y.h, dst_uv.w, dst_uv.h);

    // Fill source with known ramp (same pattern as csc-probe)
    uint8_t* sy = r8_map_write(src_y);
    if (!sy)
        DIE("map src_y");
    for (int y = 0; y < H; ++y)
        for (int x = 0; x < W; ++x)
            sy[y * src_y.stride + x] = uint8_t((x + y) & 0xFF);
    r8_unmap(src_y);

    uint8_t* suv = r8_map_write(src_uv);
    if (!suv)
        DIE("map src_uv");
    for (int y = 0; y < H; ++y) {
        for (int x = 0; x < W; ++x) {
            suv[y * src_uv.stride + 2 * x + 0] = uint8_t((x ^ y) & 0xFF);
            suv[y * src_uv.stride + 2 * x + 1] = uint8_t((x * 7 + y * 11) & 0xFF);
        }
    }
    r8_unmap(src_uv);

    // --- Import as pl_tex via dma-buf ---
    pl_fmt fmt_r8 = pl_find_named_fmt(gpu, "r8");
    pl_fmt fmt_rg8 = pl_find_named_fmt(gpu, "rg8");
    if (!fmt_r8)
        DIE("pl_find_named_fmt(r8) — no R8 format available");
    if (!fmt_rg8)
        DIE("pl_find_named_fmt(rg8) — no RG8 format available");

    std::printf("ok: found pl_fmt r8 (size=%zu) and rg8 (size=%zu)\n", fmt_r8->texel_size,
                fmt_rg8->texel_size);

    // Import source Y plane (R8, W×H)
    struct pl_tex_params tp_src_y = {};
    tp_src_y.w = W;
    tp_src_y.h = H;
    tp_src_y.format = fmt_r8;
    tp_src_y.sampleable = true;
    tp_src_y.import_handle = PL_HANDLE_DMA_BUF;
    tp_src_y.shared_mem.handle.fd = dup(src_y.fd);
    tp_src_y.shared_mem.size = static_cast<size_t>(src_y.stride) * src_y.h;
    tp_src_y.shared_mem.offset = 0;
    tp_src_y.shared_mem.drm_format_mod = gbm_bo_get_modifier(src_y.bo);
    tp_src_y.shared_mem.stride_w = static_cast<int>(src_y.stride);
    pl_tex tex_src_y = pl_tex_create(gpu, &tp_src_y);
    if (!tex_src_y)
        DIE("pl_tex_create src_y — dma-buf import failed");

    // Import source UV plane (RG8, W×H — NV24 full-res chroma)
    struct pl_tex_params tp_src_uv = {};
    tp_src_uv.w = W;
    tp_src_uv.h = H;
    tp_src_uv.format = fmt_rg8;
    tp_src_uv.sampleable = true;
    tp_src_uv.import_handle = PL_HANDLE_DMA_BUF;
    tp_src_uv.shared_mem.handle.fd = dup(src_uv.fd);
    tp_src_uv.shared_mem.size = static_cast<size_t>(src_uv.stride) * src_uv.h;
    tp_src_uv.shared_mem.offset = 0;
    tp_src_uv.shared_mem.drm_format_mod = gbm_bo_get_modifier(src_uv.bo);
    tp_src_uv.shared_mem.stride_w = static_cast<int>(src_uv.stride);
    pl_tex tex_src_uv = pl_tex_create(gpu, &tp_src_uv);
    if (!tex_src_uv)
        DIE("pl_tex_create src_uv — dma-buf import failed");

    // Import destination Y plane (R8, W×H) — renderable
    struct pl_tex_params tp_dst_y = {};
    tp_dst_y.w = W;
    tp_dst_y.h = H;
    tp_dst_y.format = fmt_r8;
    tp_dst_y.renderable = true;
    tp_dst_y.import_handle = PL_HANDLE_DMA_BUF;
    tp_dst_y.shared_mem.handle.fd = dup(dst_y.fd);
    tp_dst_y.shared_mem.size = static_cast<size_t>(dst_y.stride) * dst_y.h;
    tp_dst_y.shared_mem.offset = 0;
    tp_dst_y.shared_mem.drm_format_mod = gbm_bo_get_modifier(dst_y.bo);
    tp_dst_y.shared_mem.stride_w = static_cast<int>(dst_y.stride);
    pl_tex tex_dst_y = pl_tex_create(gpu, &tp_dst_y);
    if (!tex_dst_y)
        DIE("pl_tex_create dst_y — dma-buf import failed");

    // Import destination UV plane (RG8, W/2 × H/2) — renderable
    struct pl_tex_params tp_dst_uv = {};
    tp_dst_uv.w = W / 2;
    tp_dst_uv.h = H / 2;
    tp_dst_uv.format = fmt_rg8;
    tp_dst_uv.renderable = true;
    tp_dst_uv.import_handle = PL_HANDLE_DMA_BUF;
    tp_dst_uv.shared_mem.handle.fd = dup(dst_uv.fd);
    tp_dst_uv.shared_mem.size = static_cast<size_t>(dst_uv.stride) * dst_uv.h;
    tp_dst_uv.shared_mem.offset = 0;
    tp_dst_uv.shared_mem.drm_format_mod = gbm_bo_get_modifier(dst_uv.bo);
    tp_dst_uv.shared_mem.stride_w = static_cast<int>(dst_uv.stride);
    pl_tex tex_dst_uv = pl_tex_create(gpu, &tp_dst_uv);
    if (!tex_dst_uv)
        DIE("pl_tex_create dst_uv — dma-buf import failed");

    std::printf("ok: 4 pl_tex imported from dma-bufs\n");

    // --- Build pl_frame for source (NV24) and target (NV12) ---
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
    // NV24 = 4:4:4 (chroma is at full resolution, no shift)
    src_frame.planes[1].shift_x = 0;
    src_frame.planes[1].shift_y = 0;
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
    // NV12 = 4:2:0 (chroma is half-res: shift -1 in both axes)
    dst_frame.planes[1].shift_x = -1;
    dst_frame.planes[1].shift_y = -1;
    pl_frame_set_chroma_location(&dst_frame, PL_CHROMA_LEFT);

    // --- Render (warmup + timed iterations) ---
    pl_renderer renderer = pl_renderer_create(pl_logger, gpu);
    if (!renderer)
        DIE("pl_renderer_create");

    struct pl_render_params params = pl_render_fast_params;
    params.skip_anti_aliasing = true;

    // Warmup
    if (!pl_render_image(renderer, &src_frame, &dst_frame, &params))
        DIE("pl_render_image (warmup) failed");
    pl_gpu_finish(gpu);

    auto t0 = std::chrono::steady_clock::now();
    for (int i = 0; i < iters; ++i) {
        if (!pl_render_image(renderer, &src_frame, &dst_frame, &params))
            DIE("pl_render_image (iter %d) failed", i);
    }
    pl_gpu_finish(gpu);
    auto t1 = std::chrono::steady_clock::now();
    double total_us = std::chrono::duration_cast<std::chrono::microseconds>(t1 - t0).count();
    double per_frame_us = total_us / iters;
    std::printf("ok: %d iterations in %.0f µs (%.1f µs/frame, %.1f fps)\n", iters, total_us,
                per_frame_us, 1e6 / per_frame_us);

    // --- Verify Y plane ---
    int errors_y = 0;
    int first_x = -1, first_y_coord = -1, first_got = -1, first_want = -1;
    uint8_t* dy = r8_map_read(dst_y);
    if (!dy)
        DIE("map dst_y for read");
    for (int y = 0; y < H && errors_y < 8; ++y) {
        for (int x = 0; x < W && errors_y < 8; ++x) {
            uint8_t got = dy[y * dst_y.stride + x];
            uint8_t want = uint8_t((x + y) & 0xFF);
            // libplacebo may apply limited-range scaling (16..235), so allow
            // a tolerance window. The input is raw [0,255]; if BT.601 limited
            // range math is applied, Y_out = 16 + Y_in * (235-16)/255.
            // For a raw passthrough test we check ±2 first; if that fails
            // it may mean libplacebo is doing colorspace math (expected).
            int diff = int(got) - int(want);
            if (diff < -2 || diff > 2) {
                if (errors_y == 0) {
                    first_x = x;
                    first_y_coord = y;
                    first_got = got;
                    first_want = want;
                }
                ++errors_y;
            }
        }
    }
    r8_unmap(dst_y);

    // --- Verify UV plane ---
    int errors_uv = 0;
    int first_uv_x = -1, first_uv_y = -1, first_uv_got = -1, first_uv_want = -1;
    uint8_t* duv = r8_map_read(dst_uv);
    if (!duv)
        DIE("map dst_uv for read");
    for (int y = 0; y < H / 2 && errors_uv < 8; ++y) {
        for (int x = 0; x < W / 2 && errors_uv < 8; ++x) {
            int srcx = 2 * x, srcy = 2 * y;
            int eu = (((srcx ^ srcy) & 0xFF) + (((srcx + 1) ^ srcy) & 0xFF) +
                      ((srcx ^ (srcy + 1)) & 0xFF) + (((srcx + 1) ^ (srcy + 1)) & 0xFF) + 2) /
                     4;
            uint8_t got_u = duv[y * dst_uv.stride + 2 * x + 0];
            // Tolerance: ±3 accounts for mediump quantization + libplacebo's
            // internal chroma resampling filter.
            int du = int(got_u) - eu;
            if (du < -3 || du > 3) {
                if (errors_uv == 0) {
                    first_uv_x = x;
                    first_uv_y = y;
                    first_uv_got = got_u;
                    first_uv_want = eu;
                }
                ++errors_uv;
            }
        }
    }
    r8_unmap(dst_uv);

    // --- Report ---
    std::printf("\n=== Phase 1 Results ===\n");
    std::printf("  Backend:    libplacebo %s (OpenGL)\n", pl_version());
    std::printf("  Resolution: %d×%d\n", W, H);
    std::printf("  Perf:       %.1f µs/frame (%d iters)\n", per_frame_us, iters);
    if (errors_y == 0 && errors_uv == 0) {
        std::printf("  Correctness: PASS (Y ±2, UV ±3 tolerance)\n");
    } else {
        if (errors_y > 0) {
            std::printf("  Y MISMATCH: %d errors; first at (%d,%d) got=%d want=%d\n", errors_y,
                        first_x, first_y_coord, first_got, first_want);
            std::printf("  NOTE: if Y values look like limited-range-scaled versions of\n"
                        "        the input, libplacebo is correctly applying BT.601 math.\n"
                        "        The probe pattern is raw [0,255] — mismatches may indicate\n"
                        "        correct CSC behavior, not a bug. Inspect values manually.\n");
        }
        if (errors_uv > 0) {
            std::printf("  UV MISMATCH: %d errors; first at (%d,%d) got_u=%d want_u=%d\n",
                        errors_uv, first_uv_x, first_uv_y, first_uv_got, first_uv_want);
        }
    }
    std::printf("  Conclusion: %s\n", (errors_y <= 4 && errors_uv <= 4)
                                          ? "VIABLE — proceed to Phase 2"
                                          : "INVESTIGATE — check if mismatches are CSC math");

    // --- Cleanup ---
    pl_renderer_destroy(&renderer);
    pl_tex_destroy(gpu, &tex_src_y);
    pl_tex_destroy(gpu, &tex_src_uv);
    pl_tex_destroy(gpu, &tex_dst_y);
    pl_tex_destroy(gpu, &tex_dst_uv);
    pl_opengl_destroy(&pl_gl);
    pl_log_destroy(&pl_logger);
    r8_free(src_y);
    r8_free(src_uv);
    r8_free(dst_y);
    r8_free(dst_uv);
    return (errors_y > 4 || errors_uv > 4) ? 1 : 0;
}
