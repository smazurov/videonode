// placebo-host-copy-probe — VK_EXT_host_image_copy dma-buf→host experiment.
//
// Phase 3 of the #9 evaluation. Tests whether we can:
//   1. Import a dma-buf as a VkImage via libplacebo's Vulkan backend
//   2. Copy image data to host memory via pl_tex_download (which uses
//      VK_EXT_host_image_copy when available, or falls back to staging)
//   3. Measure throughput for the sink use case (dma-buf → stdout pipe)
//
// This is the candidate workaround for #7: if VK_EXT_host_image_copy works
// on Intel ANV, the sink path can bypass mmap and go VkImage→host directly.
//
// Usage:
//   ./placebo-host-copy-probe [device=/dev/dri/renderD128] [W=1920] [H=1080] [iters=100]

#include <drm_fourcc.h>
#include <gbm.h>
#include <libplacebo/config.h>
#include <libplacebo/gpu.h>
#include <libplacebo/log.h>
#include <libplacebo/vulkan.h>

#include <chrono>
#include <cstdint>
#include <cstdio>
#include <cstdlib>
#include <cstring>
#include <fcntl.h>
#include <span>
#include <unistd.h>
#include <vector>

#define DIE(...)                                                                                   \
    do {                                                                                           \
        std::fprintf(stderr, "placebo-host-copy-probe: " __VA_ARGS__);                             \
        std::fprintf(stderr, "\n");                                                                \
        return 1;                                                                                  \
    } while (0)

namespace {

struct GbmState {
    int drm_fd = -1;
    gbm_device* gbm = nullptr;
    gbm_bo* bo = nullptr;
    int bo_fd = -1;
    uint32_t bo_stride = 0;
    uint64_t bo_mod = 0;
};

void gbm_cleanup(GbmState& g) {
    if (g.bo_fd >= 0)
        close(g.bo_fd);
    if (g.bo)
        gbm_bo_destroy(g.bo);
    if (g.gbm)
        gbm_device_destroy(g.gbm);
    if (g.drm_fd >= 0)
        close(g.drm_fd);
}

struct PlState {
    pl_log logger = nullptr;
    pl_vk_inst vk_inst = nullptr;
    pl_vulkan pl_vk = nullptr;
};

void pl_cleanup(PlState& p) {
    if (p.pl_vk)
        pl_vulkan_destroy(&p.pl_vk);
    if (p.vk_inst)
        pl_vk_inst_destroy(&p.vk_inst);
    if (p.logger)
        pl_log_destroy(&p.logger);
}

bool setup_gbm(const char* device, int W, int H, GbmState& g) {
    g.drm_fd = open(device, O_RDWR | O_CLOEXEC);
    if (g.drm_fd < 0)
        return false;
    g.gbm = gbm_create_device(g.drm_fd);
    if (!g.gbm)
        return false;
    g.bo =
        gbm_bo_create(g.gbm, W, H + H / 2, DRM_FORMAT_R8, GBM_BO_USE_LINEAR | GBM_BO_USE_RENDERING);
    if (!g.bo)
        return false;
    g.bo_fd = gbm_bo_get_fd(g.bo);
    g.bo_stride = gbm_bo_get_stride(g.bo);
    g.bo_mod = gbm_bo_get_modifier(g.bo);
    if (g.bo_mod == DRM_FORMAT_MOD_INVALID)
        g.bo_mod = DRM_FORMAT_MOD_LINEAR;
    return true;
}

void fill_nv12_pattern(GbmState& g, int W, int H) {
    uint32_t map_stride = 0;
    void* map_handle = nullptr;
    void* raw =
        gbm_bo_map(g.bo, 0, 0, W, H + H / 2, GBM_BO_TRANSFER_WRITE, &map_stride, &map_handle);
    if (!raw)
        return;
    std::span<uint8_t> buf(static_cast<uint8_t*>(raw),
                           static_cast<size_t>(map_stride) * (H + H / 2));
    for (int y = 0; y < H; ++y)
        for (int x = 0; x < W; ++x)
            buf[y * map_stride + x] = uint8_t((x + y) & 0xFF);
    for (int y = 0; y < H / 2; ++y)
        for (int x = 0; x < W; ++x)
            buf[(H + y) * map_stride + x] = uint8_t((x ^ y) & 0xFF);
    gbm_bo_unmap(g.bo, map_handle);
}

bool setup_vulkan(PlState& p) {
    struct pl_log_params log_params = {};
    log_params.log_cb = pl_log_color;
    log_params.log_level = PL_LOG_WARN;
    p.logger = pl_log_create(PL_API_VER, &log_params);
    if (!p.logger)
        return false;

    struct pl_vk_inst_params inst_params = {};
    inst_params.debug = false;
    p.vk_inst = pl_vk_inst_create(p.logger, &inst_params);
    if (!p.vk_inst)
        return false;

    struct pl_vulkan_params vk_params = {};
    vk_params.instance = p.vk_inst->instance;
    vk_params.get_proc_addr = p.vk_inst->get_proc_addr;
    vk_params.allow_software = false;
    p.pl_vk = pl_vulkan_create(p.logger, &vk_params);
    return p.pl_vk != nullptr;
}

pl_tex import_nv12_tex(pl_gpu gpu, pl_fmt fmt_r8, GbmState& g, int W, int H, bool host_readable) {
    struct pl_tex_params tp = {};
    tp.w = W;
    tp.h = H;
    tp.format = fmt_r8;
    tp.sampleable = true;
    tp.host_readable = host_readable;
    tp.import_handle = PL_HANDLE_DMA_BUF;
    tp.shared_mem.handle.fd = dup(g.bo_fd);
    tp.shared_mem.size = static_cast<size_t>(g.bo_stride) * (H + H / 2);
    tp.shared_mem.offset = 0;
    tp.shared_mem.drm_format_mod = g.bo_mod;
    tp.shared_mem.stride_w = static_cast<int>(g.bo_stride);
    return pl_tex_create(gpu, &tp);
}

int verify_y_plane(pl_gpu gpu, pl_tex tex, std::vector<uint8_t>& host_buf, int W, int H) {
    struct pl_tex_transfer_params xfer = {};
    xfer.tex = tex;
    xfer.row_pitch = W;
    xfer.ptr = host_buf.data();

    if (!pl_tex_download(gpu, &xfer))
        return -1;
    pl_gpu_finish(gpu);

    int errors = 0;
    for (int y = 0; y < H && errors < 8; ++y) {
        for (int x = 0; x < W && errors < 8; ++x) {
            uint8_t got = host_buf[y * W + x];
            uint8_t want = uint8_t((x + y) & 0xFF);
            if (got != want)
                ++errors;
        }
    }
    return errors;
}

double run_download_loop(pl_gpu gpu, pl_tex tex, std::vector<uint8_t>& host_buf, int W, int iters) {
    struct pl_tex_transfer_params xfer = {};
    xfer.tex = tex;
    xfer.row_pitch = W;
    xfer.ptr = host_buf.data();

    auto t0 = std::chrono::steady_clock::now();
    for (int i = 0; i < iters; ++i)
        pl_tex_download(gpu, &xfer);
    pl_gpu_finish(gpu);
    auto t1 = std::chrono::steady_clock::now();

    return static_cast<double>(
               std::chrono::duration_cast<std::chrono::microseconds>(t1 - t0).count()) /
           iters;
}

struct ProbeArgs {
    const char* device;
    int W;
    int H;
    int iters;
};

struct ProbeResult {
    double per_frame_us = 0.0;
    double mbps = 0.0;
    int errors = 0;
};

int run_download_probe(pl_gpu gpu, pl_fmt fmt_r8, GbmState& gbm, const ProbeArgs& a,
                       ProbeResult& res);

int run_probe(const ProbeArgs& a, ProbeResult& res) {
    GbmState gbm;
    if (!setup_gbm(a.device, a.W, a.H, gbm)) {
        if (gbm.drm_fd < 0)
            std::fprintf(stderr, "placebo-host-copy-probe: open(%s): %s\n", a.device,
                         strerror(errno));
        else if (!gbm.gbm)
            std::fprintf(stderr, "placebo-host-copy-probe: gbm_create_device\n");
        else
            std::fprintf(stderr, "placebo-host-copy-probe: gbm_bo_create for NV12\n");
        gbm_cleanup(gbm);
        return 1;
    }
    fill_nv12_pattern(gbm, a.W, a.H);
    std::printf("ok: NV12 GBM buffer %d×%d (stride=%u, mod=0x%016llx)\n", a.W, a.H, gbm.bo_stride,
                (unsigned long long)gbm.bo_mod);

    PlState pl;
    if (!setup_vulkan(pl)) {
        if (!pl.logger)
            std::fprintf(stderr, "placebo-host-copy-probe: pl_log_create\n");
        else if (!pl.vk_inst)
            std::fprintf(stderr, "placebo-host-copy-probe: pl_vk_inst_create\n");
        else
            std::fprintf(stderr, "placebo-host-copy-probe: pl_vulkan_create\n");
        pl_cleanup(pl);
        gbm_cleanup(gbm);
        return 1;
    }

    pl_gpu gpu = pl.pl_vk->gpu;
    bool has_dmabuf_import = !!(gpu->import_caps.tex & PL_HANDLE_DMA_BUF);
    pl_fmt fmt_r8 = pl_find_named_fmt(gpu, "r8");
    bool has_host_readable = fmt_r8 && (fmt_r8->caps & PL_FMT_CAP_HOST_READABLE);

    std::printf("ok: Vulkan backend initialized\n");
    std::printf("    dma-buf import: %s\n", has_dmabuf_import ? "yes" : "NO");
    std::printf("    host_readable (r8): %s\n", has_host_readable ? "yes" : "NO");

    if (!has_dmabuf_import) {
        std::printf("SKIP: no dma-buf import support — cannot test host copy path\n");
        pl_cleanup(pl);
        gbm_cleanup(gbm);
        return 2;
    }

    int rc = run_download_probe(gpu, fmt_r8, gbm, a, res);
    pl_cleanup(pl);
    gbm_cleanup(gbm);
    return rc;
}

int run_download_probe(pl_gpu gpu, pl_fmt fmt_r8, GbmState& gbm, const ProbeArgs& a,
                       ProbeResult& res) {
    if (!fmt_r8) {
        std::fprintf(stderr, "placebo-host-copy-probe: no r8 format on Vulkan GPU\n");
        return 1;
    }

    pl_tex tex = import_nv12_tex(gpu, fmt_r8, gbm, a.W, a.H, true);
    if (!tex) {
        std::printf("WARN: cannot create host_readable texture from dma-buf\n");
        std::printf("  Trying without host_readable (will use staging buffer)...\n");
        tex = import_nv12_tex(gpu, fmt_r8, gbm, a.W, a.H, false);
        if (!tex) {
            std::fprintf(stderr,
                         "placebo-host-copy-probe: pl_tex_create failed — driver rejects import\n");
            return 1;
        }
        std::printf("ok: imported without host_readable — will test staging-buffer download\n");
    } else {
        std::printf("ok: imported with host_readable — VK_EXT_host_image_copy may be in play\n");
    }

    std::vector<uint8_t> host_buf(static_cast<size_t>(a.W) * a.H);
    res.errors = verify_y_plane(gpu, tex, host_buf, a.W, a.H);
    if (res.errors < 0) {
        std::printf("FAIL: pl_tex_download returned false\n");
        std::printf("  The driver cannot download from this imported dma-buf texture.\n");
        std::printf("  Phase 3 conclusion: host_image_copy path NOT viable on this GPU.\n");
        pl_tex_destroy(gpu, &tex);
        return 1;
    }
    if (res.errors > 0) {
        std::printf("WARN: %d pixel mismatches in downloaded Y plane (may be padding artifact)\n",
                    res.errors);
    }

    res.per_frame_us = run_download_loop(gpu, tex, host_buf, a.W, a.iters);
    res.mbps = (static_cast<double>(a.W) * a.H / (1024.0 * 1024.0)) / (res.per_frame_us / 1e6);

    pl_tex_destroy(gpu, &tex);
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

    ProbeResult res;
    int rc = run_probe(a, res);
    if (rc != 0)
        return rc;

    std::printf("\n=== Phase 3 Results (host_image_copy) ===\n");
    std::printf("  Backend:    libplacebo %s (Vulkan)\n", pl_version());
    std::printf("  Resolution: %d×%d (Y plane only, %zu bytes)\n", a.W, a.H,
                static_cast<size_t>(a.W) * a.H);
    std::printf("  Perf:       %.1f µs/frame (%d iters), %.0f MB/s\n", res.per_frame_us, a.iters,
                res.mbps);
    std::printf("  Correctness: %s\n", res.errors == 0 ? "PASS" : "WARN (see above)");
    std::printf("  NV12 full-frame extrapolation: %.1f µs (Y+UV = 1.5× Y)\n",
                res.per_frame_us * 1.5);
    std::printf("  Conclusion: %s\n", res.per_frame_us < 2000.0
                                          ? "VIABLE for sink path (sub-2ms per frame at 1080p)"
                                          : "MARGINAL — compare against mmap baseline");
    return 0;
}
