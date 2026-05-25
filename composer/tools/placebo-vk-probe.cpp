// placebo-vk-probe — libplacebo Vulkan backend: dma-buf import + NV24→NV12 CSC.
//
// Phase 2 of the #9 evaluation. Same test pattern as placebo-csc-probe but
// uses pl_vulkan instead of pl_opengl. Validates:
//   1. VkInstance/VkDevice creation targeting the selected GPU
//   2. dma-buf import via PL_HANDLE_DMA_BUF on the Vulkan backend
//   3. NV24→NV12 color-space conversion via pl_renderer
//   4. Correctness + performance comparison
//
// On the rig: targets Mali-G610 via PanVK (VK_EXT_external_memory_dma_buf +
// VK_EXT_image_drm_format_modifier). On the dev box: targets radeonsi/ANV.
//
// Usage:
//   ./placebo-vk-probe [device=/dev/dri/renderD128] [W=1920] [H=1080] [iters=100]

#include <drm_fourcc.h>
#include <gbm.h>
#include <libplacebo/config.h>
#include <libplacebo/gpu.h>
#include <libplacebo/log.h>
#include <libplacebo/renderer.h>
#include <libplacebo/vulkan.h>

#include <chrono>
#include <cstdint>
#include <cstdio>
#include <cstdlib>
#include <cstring>
#include <fcntl.h>
#include <span>
#include <unistd.h>

#define DIE(...)                                                                                   \
    do {                                                                                           \
        std::fprintf(stderr, "placebo-vk-probe: " __VA_ARGS__);                                    \
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

uint64_t bo_modifier(gbm_bo* bo) {
    uint64_t mod = gbm_bo_get_modifier(bo);
    if (mod == DRM_FORMAT_MOD_INVALID)
        mod = DRM_FORMAT_MOD_LINEAR;
    return mod;
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

struct GbmState {
    int drm_fd = -1;
    gbm_device* gbm = nullptr;
};

void gbm_cleanup(GbmState& g) {
    if (g.gbm)
        gbm_device_destroy(g.gbm);
    if (g.drm_fd >= 0)
        close(g.drm_fd);
}

struct VkState {
    pl_log logger = nullptr;
    pl_vk_inst vk_inst = nullptr;
    pl_vulkan pl_vk = nullptr;
};

void vk_cleanup(VkState& v) {
    if (v.pl_vk)
        pl_vulkan_destroy(&v.pl_vk);
    if (v.vk_inst)
        pl_vk_inst_destroy(&v.vk_inst);
    if (v.logger)
        pl_log_destroy(&v.logger);
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

bool setup_gbm(const char* device, GbmState& g) {
    g.drm_fd = open(device, O_RDWR | O_CLOEXEC);
    if (g.drm_fd < 0)
        return false;
    g.gbm = gbm_create_device(g.drm_fd);
    return g.gbm != nullptr;
}

bool setup_vulkan(VkState& v) {
    struct pl_log_params log_params = {};
    log_params.log_cb = pl_log_color;
    log_params.log_level = PL_LOG_WARN;
    v.logger = pl_log_create(PL_API_VER, &log_params);
    if (!v.logger)
        return false;

    struct pl_vk_inst_params inst_params = {};
    inst_params.debug = false;
    v.vk_inst = pl_vk_inst_create(v.logger, &inst_params);
    if (!v.vk_inst)
        return false;

    std::printf("ok: Vulkan instance created (api_version=%u.%u.%u)\n",
                VK_API_VERSION_MAJOR(v.vk_inst->api_version),
                VK_API_VERSION_MINOR(v.vk_inst->api_version),
                VK_API_VERSION_PATCH(v.vk_inst->api_version));

    struct pl_vulkan_params vk_params = {};
    vk_params.instance = v.vk_inst->instance;
    vk_params.get_proc_addr = v.vk_inst->get_proc_addr;
    vk_params.allow_software = false;
    v.pl_vk = pl_vulkan_create(v.logger, &vk_params);
    return v.pl_vk != nullptr;
}

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
    tp.shared_mem.drm_format_mod = bo_modifier(bo.bo);
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

void print_report(int W, int H, double per_frame_us, int iters, const VerifyResult& v,
                  const Buffers& bufs) {
    std::printf("\n=== Phase 2 Results (Vulkan) ===\n");
    std::printf("  Backend:    libplacebo %s (Vulkan)\n", pl_version());
    std::printf("  Resolution: %d×%d\n", W, H);
    std::printf("  Perf:       %.1f µs/frame (%d iters)\n", per_frame_us, iters);
    if (v.errors_y == 0 && v.errors_uv == 0) {
        std::printf("  Correctness: PASS (Y ±2, UV ±3 tolerance)\n");
    } else {
        if (v.errors_y > 0)
            std::printf("  Y MISMATCH: %d errors; first at (%d,%d) got=%d want=%d\n", v.errors_y,
                        v.first_x, v.first_y_coord, v.first_got, v.first_want);
        if (v.errors_uv > 0)
            std::printf("  UV MISMATCH: %d errors; first at (%d,%d) got_u=%d want_u=%d\n",
                        v.errors_uv, v.first_uv_x, v.first_uv_y, v.first_uv_got, v.first_uv_want);
    }
    std::printf("  DRM modifiers: src_y=0x%016llx src_uv=0x%016llx\n",
                (unsigned long long)gbm_bo_get_modifier(bufs.src_y.bo),
                (unsigned long long)gbm_bo_get_modifier(bufs.src_uv.bo));
    std::printf("  Conclusion: %s\n",
                (v.errors_y <= 4 && v.errors_uv <= 4)
                    ? "VIABLE — Vulkan dma-buf import works end-to-end"
                    : "INVESTIGATE — check modifier compatibility or driver bugs");
}

struct ProbeArgs {
    const char* device;
    int W;
    int H;
    int iters;
};

// Runs render + verify once Vulkan GPU is available. Populates outputs.
int render_and_verify(pl_gpu gpu, gbm_device* gbm, const ProbeArgs& a, double& per_frame_us_out,
                      VerifyResult& v_out, Buffers& bufs_out) {
    if (!alloc_buffers(gbm, a.W, a.H, bufs_out)) {
        std::fprintf(stderr, "placebo-vk-probe: alloc GBM buffers\n");
        return 1;
    }
    std::printf("ok: allocated GBM buffers\n");

    fill_src_y(bufs_out.src_y, a.W, a.H);
    fill_src_uv(bufs_out.src_uv, a.W, a.H);

    Fmts fmts;
    fmts.r8 = pl_find_named_fmt(gpu, "r8");
    fmts.rg8 = pl_find_named_fmt(gpu, "rg8");
    if (!fmts.r8 || !fmts.rg8) {
        std::fprintf(stderr, "placebo-vk-probe: r8/rg8 format not available\n");
        return 1;
    }

    Textures tex;
    if (!import_textures(gpu, fmts, bufs_out, a.W, a.H, tex)) {
        std::fprintf(stderr, "placebo-vk-probe: Vulkan dma-buf import failed\n");
        return 1;
    }
    std::printf("ok: 4 pl_tex imported via Vulkan dma-buf\n");

    pl_frame src_frame, dst_frame;
    build_src_frame(tex, src_frame);
    build_dst_frame(tex, dst_frame);

    pl_renderer renderer = pl_renderer_create(nullptr, gpu);
    if (!renderer) {
        std::fprintf(stderr, "placebo-vk-probe: pl_renderer_create\n");
        pl_tex_destroy(gpu, &tex.src_y);
        pl_tex_destroy(gpu, &tex.src_uv);
        pl_tex_destroy(gpu, &tex.dst_y);
        pl_tex_destroy(gpu, &tex.dst_uv);
        return 1;
    }

    per_frame_us_out = run_render_loop(renderer, gpu, src_frame, dst_frame, a.iters);
    std::printf("ok: %d iterations in %.0f µs (%.1f µs/frame, %.1f fps)\n", a.iters,
                per_frame_us_out * a.iters, per_frame_us_out, 1e6 / per_frame_us_out);

    v_out = verify_output(bufs_out, a.W, a.H);

    pl_renderer_destroy(&renderer);
    pl_tex_destroy(gpu, &tex.src_y);
    pl_tex_destroy(gpu, &tex.src_uv);
    pl_tex_destroy(gpu, &tex.dst_y);
    pl_tex_destroy(gpu, &tex.dst_uv);
    return 0;
}

int run_probe(const ProbeArgs& a, double& per_frame_us_out, VerifyResult& v_out,
              Buffers& bufs_out) {
    GbmState g;
    if (!setup_gbm(a.device, g)) {
        if (g.drm_fd < 0)
            std::fprintf(stderr, "placebo-vk-probe: open(%s): %s\n", a.device, strerror(errno));
        else
            std::fprintf(stderr, "placebo-vk-probe: gbm_create_device\n");
        gbm_cleanup(g);
        return 1;
    }
    std::printf("ok: GBM device on %s\n", a.device);

    VkState vk;
    if (!setup_vulkan(vk)) {
        if (!vk.logger)
            std::fprintf(stderr, "placebo-vk-probe: pl_log_create\n");
        else if (!vk.vk_inst)
            std::fprintf(stderr, "placebo-vk-probe: pl_vk_inst_create\n");
        else
            std::fprintf(stderr, "placebo-vk-probe: pl_vulkan_create\n");
        vk_cleanup(vk);
        gbm_cleanup(g);
        return 1;
    }

    pl_gpu gpu = vk.pl_vk->gpu;
    std::printf("ok: libplacebo Vulkan backend\n");
    std::printf("    GLSL: %s (version %d)\n", gpu->glsl.vulkan ? "vulkan" : "non-vulkan",
                gpu->glsl.version);
    std::printf("    max_tex_2d_dim: %d\n", gpu->limits.max_tex_2d_dim);

    if (!(gpu->import_caps.tex & PL_HANDLE_DMA_BUF)) {
        std::printf("SKIP: Vulkan GPU does not advertise PL_HANDLE_DMA_BUF import\n");
        std::printf("  This means VK_EXT_external_memory_dma_buf is missing or the\n");
        std::printf("  driver does not expose it to libplacebo. Phase 2 cannot proceed.\n");
        vk_cleanup(vk);
        gbm_cleanup(g);
        return 2;
    }
    std::printf("ok: Vulkan GPU supports dma-buf texture import\n");

    int rc = render_and_verify(gpu, g.gbm, a, per_frame_us_out, v_out, bufs_out);
    vk_cleanup(vk);
    gbm_cleanup(g);
    return rc;
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
    Buffers bufs;
    int rc = run_probe(a, per_frame_us, v, bufs);
    if (rc != 0)
        return rc;

    print_report(a.W, a.H, per_frame_us, a.iters, v, bufs);
    r8_free(bufs.src_y);
    r8_free(bufs.src_uv);
    r8_free(bufs.dst_y);
    r8_free(bufs.dst_uv);
    return (v.errors_y > 4 || v.errors_uv > 4) ? 1 : 0;
}
