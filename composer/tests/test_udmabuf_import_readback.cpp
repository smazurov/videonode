#include "src/render/nv12_buf.hpp"

#include <drm_fourcc.h>
#include <gbm.h>
#include <gtest/gtest.h>
#include <libplacebo/gpu.h>
#include <libplacebo/log.h>
#include <libplacebo/renderer.h>
#include <libplacebo/vulkan.h>

#include <algorithm>
#include <cstdint>
#include <cstdio>
#include <fcntl.h>
#include <span>
#include <sys/mman.h>
#include <unistd.h>

namespace {

struct PlGpu {
    pl_log log = nullptr;
    pl_vk_inst inst = nullptr;
    pl_vulkan vk = nullptr;
    pl_gpu gpu = nullptr;

    bool init() {
        struct pl_log_params lp = {};
        lp.log_cb = pl_log_simple;
        lp.log_level = PL_LOG_WARN;
        log = pl_log_create(PL_API_VER, &lp);
        struct pl_vk_inst_params ip = {};
        inst = pl_vk_inst_create(log, &ip);
        if (!inst)
            return false;
        struct pl_vulkan_params vkp = {};
        vkp.instance = inst->instance;
        vkp.get_proc_addr = inst->get_proc_addr;
        vkp.allow_software = false;
        vk = pl_vulkan_create(log, &vkp);
        if (!vk || !(vk->gpu->import_caps.tex & PL_HANDLE_DMA_BUF))
            return false;
        gpu = vk->gpu;
        return true;
    }

    ~PlGpu() {
        if (vk)
            pl_vulkan_destroy(&vk);
        if (inst)
            pl_vk_inst_destroy(&inst);
        if (log)
            pl_log_destroy(&log);
    }
};

pl_tex import_plane(pl_gpu gpu, pl_fmt fmt, int fd, int w, int h, uint32_t pitch) {
    struct pl_tex_params tp = {};
    tp.w = w;
    tp.h = h;
    tp.format = fmt;
    tp.sampleable = true;
    tp.import_handle = PL_HANDLE_DMA_BUF;
    tp.shared_mem.handle.fd = ::dup(fd);
    tp.shared_mem.size = size_t(pitch) * h;
    tp.shared_mem.drm_format_mod = DRM_FORMAT_MOD_LINEAR;
    tp.shared_mem.stride_w = pitch;
    pl_tex tex = pl_tex_create(gpu, &tp);
    if (!tex)
        ::close(tp.shared_mem.handle.fd);
    return tex;
}

} // namespace

// End-to-end guard for the staged-broadcast path on GBM hosts: a red NV12
// frame staged by stage_for_read (memfd + udmabuf) must import into Vulkan
// and render red — this fails if staging stops producing importable dma-bufs
// (the b547659 black-slot regression) or swaps bytes.
TEST(UdmabufImport, StagedRedFrameRendersRed) {
    PlGpu pl;
    if (!pl.init())
        GTEST_SKIP() << "no Vulkan dma-buf import";

    int drm_fd = ::open("/dev/dri/renderD128", O_RDWR | O_CLOEXEC);
    if (drm_fd < 0)
        GTEST_SKIP() << "no DRM render node";
    gbm_device* gbm = gbm_create_device(drm_fd);
    ASSERT_NE(gbm, nullptr);

    {
        nv12_buf::Allocator alloc;
        ASSERT_TRUE(alloc.init(gbm));
        nv12_buf::Buffer b = alloc.alloc(1920, 1080);
        ASSERT_TRUE(b.valid());

        // Solid red in BT.709 limited: Y=63, Cb=102, Cr=240.
        auto m = nv12_buf::map_rw(b);
        ASSERT_NE(m.y, nullptr);
        auto y = m.y_bytes();
        std::fill(y.begin(), y.end(), uint8_t{63});
        auto uv = m.uv_bytes();
        for (size_t i = 0; i + 1 < uv.size(); i += 2) {
            uv[i] = 102;
            uv[i + 1] = 240;
        }
        nv12_buf::unmap(b);
        nv12_buf::stage_for_read(b);
        ASSERT_GE(b.staged_y_fd, 0);

        pl_fmt r8 = pl_find_named_fmt(pl.gpu, "r8");
        pl_fmt rg8 = pl_find_named_fmt(pl.gpu, "rg8");
        ASSERT_NE(r8, nullptr);
        ASSERT_NE(rg8, nullptr);

        pl_tex tex_y = import_plane(pl.gpu, r8, b.staged_y_fd, 1920, 1080, b.y_pitch);
        ASSERT_NE(tex_y, nullptr) << "Y import failed";
        pl_tex tex_uv = import_plane(pl.gpu, rg8, b.staged_uv_fd, 960, 540, b.uv_pitch);
        ASSERT_NE(tex_uv, nullptr) << "UV import failed";

        struct pl_frame src = {};
        src.num_planes = 2;
        src.planes[0].texture = tex_y;
        src.planes[0].components = 1;
        src.planes[0].component_mapping[0] = 0;
        src.planes[1].texture = tex_uv;
        src.planes[1].components = 2;
        src.planes[1].component_mapping[0] = 1;
        src.planes[1].component_mapping[1] = 2;
        src.repr.sys = PL_COLOR_SYSTEM_BT_709;
        src.repr.levels = PL_COLOR_LEVELS_LIMITED;
        src.planes[1].shift_x = -1;
        src.planes[1].shift_y = -1;
        pl_frame_set_chroma_location(&src, PL_CHROMA_LEFT);

        pl_fmt fmt_bgra = pl_find_named_fmt(pl.gpu, "bgra8");
        if (!fmt_bgra)
            fmt_bgra = pl_find_named_fmt(pl.gpu, "rgba8");
        ASSERT_NE(fmt_bgra, nullptr);
        struct pl_tex_params dp = {};
        dp.w = 64;
        dp.h = 64;
        dp.format = fmt_bgra;
        dp.renderable = true;
        dp.host_readable = true;
        pl_tex dst_tex = pl_tex_create(pl.gpu, &dp);
        ASSERT_NE(dst_tex, nullptr);

        struct pl_frame dst = {};
        dst.num_planes = 1;
        dst.planes[0].texture = dst_tex;
        dst.planes[0].components = 4;
        dst.planes[0].component_mapping[0] = 0;
        dst.planes[0].component_mapping[1] = 1;
        dst.planes[0].component_mapping[2] = 2;
        dst.planes[0].component_mapping[3] = 3;
        dst.repr.sys = PL_COLOR_SYSTEM_RGB;
        dst.repr.levels = PL_COLOR_LEVELS_FULL;

        pl_renderer rr = pl_renderer_create(pl.log, pl.gpu);
        ASSERT_NE(rr, nullptr);
        ASSERT_TRUE(pl_render_image(rr, &src, &dst, &pl_render_fast_params));

        std::vector<uint8_t> out(size_t(64) * 64 * 4);
        struct pl_tex_transfer_params xp = {};
        xp.tex = dst_tex;
        xp.ptr = out.data();
        ASSERT_TRUE(pl_tex_download(pl.gpu, &xp));
        pl_gpu_finish(pl.gpu);

        const bool bgra = fmt_bgra->name == std::string_view{"bgra8"};
        const uint8_t red = bgra ? out[2] : out[0];
        const uint8_t blue = bgra ? out[0] : out[2];
        EXPECT_GT(red, 200) << "red channel lost";
        EXPECT_LT(blue, 60) << "blue channel present: chroma swapped";

        pl_renderer_destroy(&rr);
        pl_tex_destroy(pl.gpu, &dst_tex);
        pl_tex_destroy(pl.gpu, &tex_uv);
        pl_tex_destroy(pl.gpu, &tex_y);
    }

    gbm_device_destroy(gbm);
    ::close(drm_fd);
}
