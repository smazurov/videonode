// egl-probe — success gate for the GPU composer's EGL/GBM setup.
//
// What it proves:
//  1. egl_ctx::EglCtx::init brings up DRM render node → GBM device → EGL
//     display → surfaceless GLES2 context on the target driver
//     (Mali-G610/Panthor on the rig, radeonsi on the dev box).
//  2. We can allocate a GBM buffer object, export it as a dma-buf fd,
//     import that fd back as an EGLImage via EglCtx::import_dmabuf, attach
//     it to an FBO renderbuffer, render to it, and read the result via mmap.
//  3. The full dma-buf round-trip the composer depends on works.
//
// Also dumps:
//   - EGL_EXTENSIONS string (driver capability bitmap)
//   - Modifier matrix from EGL_EXT_image_dma_buf_import_modifiers
//     (eglQueryDmaBufFormatsEXT + eglQueryDmaBufModifiersEXT). For
//     end-to-end format viability use dmabuf-format-probe; this list is
//     just what the driver claims.
//
// Usage:  ./egl-probe [/dev/dri/renderD130] [out.ppm]
//   - Default device: /dev/dri/renderD130 (panthor on this rig)
//   - Default output: /tmp/egl-probe.ppm (should be solid red on success)
//
// Exit status: 0 on full success; non-zero with a diagnostic line on failure.

#include "src/common/probe_check.hpp"
#include "src/render/egl_ctx.hpp"

#include <EGL/egl.h>
#include <EGL/eglext.h>
#include <GLES2/gl2.h>
#include <GLES2/gl2ext.h>
#include <drm_fourcc.h>
#include <gbm.h>

#include <cerrno>
#include <cstdint>
#include <cstdio>
#include <cstdlib>
#include <cstring>
#include <memory>
#include <span>
#include <vector>

namespace {

void dump_modifier_matrix(EGLDisplay dpy) {
    const char* exts = eglQueryString(dpy, EGL_EXTENSIONS);
    if (!exts || !std::strstr(exts, "EGL_EXT_image_dma_buf_import_modifiers")) {
        std::fprintf(stderr, "ok: EGL_EXT_image_dma_buf_import_modifiers not advertised\n");
        return;
    }
    auto eglQueryDmaBufFormatsEXT_ =
        (PFNEGLQUERYDMABUFFORMATSEXTPROC)eglGetProcAddress("eglQueryDmaBufFormatsEXT");
    auto eglQueryDmaBufModifiersEXT_ =
        (PFNEGLQUERYDMABUFMODIFIERSEXTPROC)eglGetProcAddress("eglQueryDmaBufModifiersEXT");
    if (!eglQueryDmaBufFormatsEXT_ || !eglQueryDmaBufModifiersEXT_) {
        std::fprintf(stderr, "ok: modifier query entry points missing\n");
        return;
    }
    EGLint n_fmt = 0;
    if (!eglQueryDmaBufFormatsEXT_(dpy, 0, nullptr, &n_fmt) || n_fmt <= 0) {
        std::fprintf(stderr, "ok: 0 dma-buf formats reported\n");
        return;
    }
    std::vector<EGLint> formats(n_fmt);
    if (!eglQueryDmaBufFormatsEXT_(dpy, n_fmt, formats.data(), &n_fmt)) {
        std::fprintf(stderr, "ok: eglQueryDmaBufFormatsEXT enumerate failed\n");
        return;
    }
    std::fprintf(stderr, "ok: %d dma-buf formats advertised\n", n_fmt);
    for (EGLint f : formats) {
        char fc[5] = {char(f & 0xFF), char((f >> 8) & 0xFF), char((f >> 16) & 0xFF),
                      char((f >> 24) & 0xFF), 0};
        EGLint n_mod = 0;
        eglQueryDmaBufModifiersEXT_(dpy, f, 0, nullptr, nullptr, &n_mod);
        std::fprintf(stderr, "  fourcc=0x%08x '%s' modifiers=%d\n", f, fc, n_mod);
    }
}

int write_ppm(const char* outpath, const void* mapped, uint32_t map_stride, int W, int H) {
    std::unique_ptr<FILE, decltype(&std::fclose)> f(std::fopen(outpath, "wb"), &std::fclose);
    VN_CHECK(f, "fopen(%s): %s", outpath, std::strerror(errno));
    std::fprintf(f.get(), "P6\n%d %d\n255\n", W, H);
    int red_pixels = 0;
    const std::span<const uint8_t> pixels(static_cast<const uint8_t*>(mapped),
                                          static_cast<size_t>(map_stride) * H);
    for (int y = 0; y < H; ++y) {
        const std::span<const uint8_t> row =
            pixels.subspan(static_cast<size_t>(y) * map_stride, static_cast<size_t>(W) * 4);
        for (int x = 0; x < W; ++x) {
            const size_t off = static_cast<size_t>(x) * 4;
            uint8_t b = row[off + 0];
            uint8_t g = row[off + 1];
            uint8_t r = row[off + 2];
            uint8_t out[3] = {r, g, b};
            std::fwrite(out, 1, 3, f.get());
            if (r >= 250 && g <= 5 && b <= 5)
                red_pixels++;
        }
    }
    return red_pixels;
}

struct FboState {
    gbm_bo* bo{nullptr};
    EGLImage img{EGL_NO_IMAGE};
    GLuint rbo{0};
    GLuint fbo{0};
    uint32_t stride{0};
};

FboState setup_fbo(egl_ctx::EglCtx& ctx, int W, int H) {
    FboState s;
    s.bo = gbm_bo_create(ctx.gbm(), W, H, GBM_FORMAT_ARGB8888,
                         GBM_BO_USE_RENDERING | GBM_BO_USE_LINEAR);
    VN_CHECK(s.bo, "gbm_bo_create");
    s.stride = gbm_bo_get_stride(s.bo);
    int dmabuf_fd = gbm_bo_get_fd(s.bo);
    VN_CHECK(dmabuf_fd >= 0, "gbm_bo_get_fd");
    std::fprintf(stderr, "ok: gbm_bo %dx%d stride=%u dmabuf_fd=%d\n", W, H, s.stride, dmabuf_fd);

    egl_ctx::EglCtx::ImageDesc desc;
    desc.fd = dmabuf_fd;
    desc.fourcc = DRM_FORMAT_ARGB8888;
    desc.modifier = DRM_FORMAT_MOD_LINEAR;
    desc.width = W;
    desc.height = H;
    desc.plane0_offset = 0;
    desc.plane0_pitch = static_cast<int>(s.stride);
    s.img = ctx.import_dmabuf(desc);
    VN_CHECK(s.img != EGL_NO_IMAGE, "EglCtx::import_dmabuf");
    std::fprintf(stderr, "ok: imported dma-buf as EGLImage\n");

    glGenRenderbuffers(1, &s.rbo);
    glBindRenderbuffer(GL_RENDERBUFFER, s.rbo);
    auto glEGLImageTargetRenderbufferStorageOES_ =
        (PFNGLEGLIMAGETARGETRENDERBUFFERSTORAGEOESPROC)eglGetProcAddress(
            "glEGLImageTargetRenderbufferStorageOES");
    VN_CHECK(glEGLImageTargetRenderbufferStorageOES_, "no glEGLImageTargetRenderbufferStorageOES");
    glEGLImageTargetRenderbufferStorageOES_(GL_RENDERBUFFER, s.img);
    GL_CHECK("RBStorageOES");

    glGenFramebuffers(1, &s.fbo);
    glBindFramebuffer(GL_FRAMEBUFFER, s.fbo);
    glFramebufferRenderbuffer(GL_FRAMEBUFFER, GL_COLOR_ATTACHMENT0, GL_RENDERBUFFER, s.rbo);
    VN_CHECK(glCheckFramebufferStatus(GL_FRAMEBUFFER) == GL_FRAMEBUFFER_COMPLETE,
             "framebuffer incomplete");
    return s;
}

int render_and_verify(egl_ctx::EglCtx& ctx, const FboState& s, const char* outpath, int W, int H) {
    glViewport(0, 0, W, H);
    glClearColor(1.0f, 0.0f, 0.0f, 1.0f);
    glClear(GL_COLOR_BUFFER_BIT);
    glFinish();
    std::fprintf(stderr, "ok: rendered red to dma-buf FBO\n");

    uint32_t map_stride = 0;
    void* map_data = nullptr;
    void* mapped = gbm_bo_map(s.bo, 0, 0, W, H, GBM_BO_TRANSFER_READ, &map_stride, &map_data);
    VN_CHECK(mapped, "gbm_bo_map");

    int red_pixels = write_ppm(outpath, mapped, map_stride, W, H);
    gbm_bo_unmap(s.bo, map_data);

    int expected = W * H;
    std::fprintf(stderr, "ok: PPM written to %s, %d/%d red pixels\n", outpath, red_pixels,
                 expected);
    VN_CHECK(red_pixels == expected,
             "only %d/%d pixels matched red; GPU did not actually clear the surface (or mapping "
             "is wrong)",
             red_pixels, expected);

    eglDestroyImage(ctx.display(), s.img);
    glDeleteFramebuffers(1, &s.fbo);
    glDeleteRenderbuffers(1, &s.rbo);
    gbm_bo_destroy(s.bo);
    return red_pixels;
}

} // namespace

int main(int argc, char** argv) {
    const std::span<char*> args(argv, static_cast<size_t>(argc));
    const char* device = (args.size() > 1) ? args[1] : "/dev/dri/renderD130";
    const char* outpath = (args.size() > 2) ? args[2] : "/tmp/egl-probe.ppm";
    constexpr int W = 64;
    constexpr int H = 64;

    egl_ctx::EglCtx ctx;
    VN_CHECK(ctx.init(device), "EglCtx::init(%s)", device);
    std::fprintf(stderr, "ok: EglCtx up on %s\n", device);
    std::fprintf(stderr, "ok: GL renderer=%s\n", glGetString(GL_RENDERER));
    std::fprintf(stderr, "ok: GL version=%s\n", glGetString(GL_VERSION));

    const char* exts = eglQueryString(ctx.display(), EGL_EXTENSIONS);
    std::fprintf(stderr, "ok: EGL_EXTENSIONS=%s\n", exts ? exts : "(null)");
    dump_modifier_matrix(ctx.display());

    FboState s = setup_fbo(ctx, W, H);
    render_and_verify(ctx, s, outpath, W, H);

    std::printf("PASS\n");
    return 0;
}
