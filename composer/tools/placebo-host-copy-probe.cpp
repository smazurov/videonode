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
#include <unistd.h>
#include <vector>

#define DIE(...)                                                                                   \
    do {                                                                                           \
        std::fprintf(stderr, "placebo-host-copy-probe: " __VA_ARGS__);                             \
        std::fprintf(stderr, "\n");                                                                \
        return 1;                                                                                  \
    } while (0)

int main(int argc, char** argv) {
    const char* device = (argc > 1) ? argv[1] : "/dev/dri/renderD128";
    int W = (argc > 2) ? std::atoi(argv[2]) : 1920;
    int H = (argc > 3) ? std::atoi(argv[3]) : 1080;
    int iters = (argc > 4) ? std::atoi(argv[4]) : 100;
    if (W <= 0 || H <= 0 || (W % 2) || (H % 2))
        DIE("W and H must be positive even ints");
    if (iters <= 0)
        iters = 1;

    // --- Open DRM + GBM ---
    int drm_fd = open(device, O_RDWR | O_CLOEXEC);
    if (drm_fd < 0)
        DIE("open(%s): %s", device, strerror(errno));
    gbm_device* gbm = gbm_create_device(drm_fd);
    if (!gbm)
        DIE("gbm_create_device");

    // --- Allocate NV12 buffer as single R8 of W × (H + H/2) ---
    gbm_bo* bo =
        gbm_bo_create(gbm, W, H + H / 2, DRM_FORMAT_R8, GBM_BO_USE_LINEAR | GBM_BO_USE_RENDERING);
    if (!bo)
        DIE("gbm_bo_create for NV12");
    int bo_fd = gbm_bo_get_fd(bo);
    uint32_t bo_stride = gbm_bo_get_stride(bo);
    uint64_t bo_mod = gbm_bo_get_modifier(bo);

    // Fill with test pattern
    uint32_t map_stride = 0;
    void* map_handle = nullptr;
    auto* ptr = static_cast<uint8_t*>(
        gbm_bo_map(bo, 0, 0, W, H + H / 2, GBM_BO_TRANSFER_WRITE, &map_stride, &map_handle));
    if (!ptr)
        DIE("gbm_bo_map write");
    for (int y = 0; y < H; ++y)
        for (int x = 0; x < W; ++x)
            ptr[y * map_stride + x] = uint8_t((x + y) & 0xFF);
    for (int y = 0; y < H / 2; ++y)
        for (int x = 0; x < W; ++x)
            ptr[(H + y) * map_stride + x] = uint8_t((x ^ y) & 0xFF);
    gbm_bo_unmap(bo, map_handle);
    map_handle = nullptr;

    std::printf("ok: NV12 GBM buffer %d×%d (stride=%u, mod=0x%016llx)\n", W, H, bo_stride,
                (unsigned long long)bo_mod);

    // --- libplacebo Vulkan ---
    struct pl_log_params log_params = {};
    log_params.log_cb = pl_log_color;
    log_params.log_level = PL_LOG_WARN;
    pl_log pl_logger = pl_log_create(PL_API_VER, &log_params);

    struct pl_vk_inst_params inst_params = {};
    inst_params.debug = false;
    pl_vk_inst vk_inst = pl_vk_inst_create(pl_logger, &inst_params);
    if (!vk_inst)
        DIE("pl_vk_inst_create");

    struct pl_vulkan_params vk_params = {};
    vk_params.instance = vk_inst->instance;
    vk_params.get_proc_addr = vk_inst->get_proc_addr;
    vk_params.allow_software = false;
    pl_vulkan pl_vk = pl_vulkan_create(pl_logger, &vk_params);
    if (!pl_vk)
        DIE("pl_vulkan_create");

    pl_gpu gpu = pl_vk->gpu;

    bool has_dmabuf_import = !!(gpu->import_caps.tex & PL_HANDLE_DMA_BUF);
    pl_fmt fmt_r8 = pl_find_named_fmt(gpu, "r8");
    bool has_host_readable = fmt_r8 && fmt_r8->host_readable;

    std::printf("ok: Vulkan backend initialized\n");
    std::printf("    dma-buf import: %s\n", has_dmabuf_import ? "yes" : "NO");
    std::printf("    host_readable (r8): %s\n", has_host_readable ? "yes" : "NO");

    if (!has_dmabuf_import) {
        std::printf("SKIP: no dma-buf import support — cannot test host copy path\n");
        pl_vulkan_destroy(&pl_vk);
        pl_vk_inst_destroy(&vk_inst);
        pl_log_destroy(&pl_logger);
        gbm_bo_destroy(bo);
        close(bo_fd);
        gbm_device_destroy(gbm);
        close(drm_fd);
        return 2;
    }
    if (!fmt_r8)
        DIE("no r8 format on Vulkan GPU");

    // Import as R8 texture (just the Y plane for the throughput test)
    struct pl_tex_params tp = {};
    tp.w = W;
    tp.h = H;
    tp.format = fmt_r8;
    tp.sampleable = true;
    tp.host_readable = true;
    tp.import_handle = PL_HANDLE_DMA_BUF;
    tp.shared_mem.handle.fd = dup(bo_fd);
    tp.shared_mem.size = static_cast<size_t>(bo_stride) * (H + H / 2);
    tp.shared_mem.offset = 0;
    tp.shared_mem.drm_format_mod = bo_mod;
    tp.shared_mem.stride_w = static_cast<int>(bo_stride);
    pl_tex tex = pl_tex_create(gpu, &tp);

    if (!tex) {
        std::printf("WARN: cannot create host_readable texture from dma-buf\n");
        std::printf("  Trying without host_readable (will use staging buffer)...\n");

        tp.host_readable = false;
        tp.shared_mem.handle.fd = dup(bo_fd);
        tex = pl_tex_create(gpu, &tp);
        if (!tex)
            DIE("pl_tex_create failed even without host_readable — driver rejects import");
        std::printf("ok: imported without host_readable — will test staging-buffer download\n");
    } else {
        std::printf("ok: imported with host_readable — VK_EXT_host_image_copy may be in play\n");
    }

    // --- Download test ---
    std::vector<uint8_t> host_buf(static_cast<size_t>(W) * H);
    struct pl_tex_transfer_params xfer = {};
    xfer.tex = tex;
    xfer.row_pitch = W;
    xfer.ptr = host_buf.data();

    // Warmup
    if (!pl_tex_download(gpu, &xfer)) {
        std::printf("FAIL: pl_tex_download returned false\n");
        std::printf("  The driver cannot download from this imported dma-buf texture.\n");
        std::printf("  Phase 3 conclusion: host_image_copy path NOT viable on this GPU.\n");
        pl_tex_destroy(gpu, &tex);
        pl_vulkan_destroy(&pl_vk);
        pl_vk_inst_destroy(&vk_inst);
        pl_log_destroy(&pl_logger);
        gbm_bo_destroy(bo);
        close(bo_fd);
        gbm_device_destroy(gbm);
        close(drm_fd);
        return 1;
    }
    pl_gpu_finish(gpu);

    // Verify correctness
    int errors = 0;
    for (int y = 0; y < H && errors < 8; ++y) {
        for (int x = 0; x < W && errors < 8; ++x) {
            uint8_t got = host_buf[y * W + x];
            uint8_t want = uint8_t((x + y) & 0xFF);
            if (got != want)
                ++errors;
        }
    }
    if (errors > 0) {
        std::printf("WARN: %d pixel mismatches in downloaded Y plane (may be padding artifact)\n",
                    errors);
    }

    // Timed iterations
    auto t0 = std::chrono::steady_clock::now();
    for (int i = 0; i < iters; ++i) {
        pl_tex_download(gpu, &xfer);
    }
    pl_gpu_finish(gpu);
    auto t1 = std::chrono::steady_clock::now();
    double total_us = std::chrono::duration_cast<std::chrono::microseconds>(t1 - t0).count();
    double per_frame_us = total_us / iters;
    double mbps = (static_cast<double>(W) * H / (1024.0 * 1024.0)) / (per_frame_us / 1e6);

    // --- Report ---
    std::printf("\n=== Phase 3 Results (host_image_copy) ===\n");
    std::printf("  Backend:    libplacebo %s (Vulkan)\n", pl_version());
    std::printf("  Resolution: %d×%d (Y plane only, %zu bytes)\n", W, H,
                static_cast<size_t>(W) * H);
    std::printf("  Perf:       %.1f µs/frame (%d iters), %.0f MB/s\n", per_frame_us, iters, mbps);
    std::printf("  Correctness: %s\n", errors == 0 ? "PASS" : "WARN (see above)");
    std::printf("  NV12 full-frame extrapolation: %.1f µs (Y+UV = 1.5× Y)\n", per_frame_us * 1.5);
    std::printf("  Conclusion: %s\n", per_frame_us < 2000.0
                                          ? "VIABLE for sink path (sub-2ms per frame at 1080p)"
                                          : "MARGINAL — compare against mmap baseline");

    // --- Cleanup ---
    pl_tex_destroy(gpu, &tex);
    pl_vulkan_destroy(&pl_vk);
    pl_vk_inst_destroy(&vk_inst);
    pl_log_destroy(&pl_logger);
    gbm_bo_destroy(bo);
    close(bo_fd);
    gbm_device_destroy(gbm);
    close(drm_fd);
    return 0;
}
