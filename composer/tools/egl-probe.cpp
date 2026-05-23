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

} // namespace

int main(int argc, char** argv) {
    const char* device = (argc > 1) ? argv[1] : "/dev/dri/renderD130";
    const char* outpath = (argc > 2) ? argv[2] : "/tmp/egl-probe.ppm";
    constexpr int W = 64;
    constexpr int H = 64;

    // 1. EGL/GBM/GLES2 bootstrap via the shared helper.
    egl_ctx::EglCtx ctx;
    VN_CHECK(ctx.init(device), "EglCtx::init(%s)", device);
    std::fprintf(stderr, "ok: EglCtx up on %s\n", device);
    std::fprintf(stderr, "ok: GL renderer=%s\n", glGetString(GL_RENDERER));
    std::fprintf(stderr, "ok: GL version=%s\n", glGetString(GL_VERSION));

    // Driver capability dump — useful when a probe fails on a new rig.
    const char* exts = eglQueryString(ctx.display(), EGL_EXTENSIONS);
    std::fprintf(stderr, "ok: EGL_EXTENSIONS=%s\n", exts ? exts : "(null)");
    dump_modifier_matrix(ctx.display());

    // 2. Allocate a GBM bo we can render to and export as dma-buf.
    gbm_bo* bo = gbm_bo_create(ctx.gbm(), W, H, GBM_FORMAT_ARGB8888,
                               GBM_BO_USE_RENDERING | GBM_BO_USE_LINEAR);
    VN_CHECK(bo, "gbm_bo_create");
    uint32_t stride = gbm_bo_get_stride(bo);
    int dmabuf_fd = gbm_bo_get_fd(bo);
    VN_CHECK(dmabuf_fd >= 0, "gbm_bo_get_fd");
    std::fprintf(stderr, "ok: gbm_bo %dx%d stride=%u dmabuf_fd=%d\n", W, H, stride, dmabuf_fd);

    // 3. Import the dma-buf back as an EGLImage via EglCtx (the round-trip
    // the composer depends on).
    egl_ctx::EglCtx::ImageDesc desc;
    desc.fd = dmabuf_fd;
    desc.fourcc = DRM_FORMAT_ARGB8888;
    desc.modifier = DRM_FORMAT_MOD_LINEAR;
    desc.width = W;
    desc.height = H;
    desc.plane0_offset = 0;
    desc.plane0_pitch = static_cast<int>(stride);
    EGLImage img = ctx.import_dmabuf(desc);
    VN_CHECK(img != EGL_NO_IMAGE, "EglCtx::import_dmabuf");
    std::fprintf(stderr, "ok: imported dma-buf as EGLImage\n");

    // 4. Attach EGLImage to an FBO renderbuffer, clear red, finish.
    GLuint rbo = 0;
    glGenRenderbuffers(1, &rbo);
    glBindRenderbuffer(GL_RENDERBUFFER, rbo);

    auto glEGLImageTargetRenderbufferStorageOES_ =
        (PFNGLEGLIMAGETARGETRENDERBUFFERSTORAGEOESPROC)eglGetProcAddress(
            "glEGLImageTargetRenderbufferStorageOES");
    VN_CHECK(glEGLImageTargetRenderbufferStorageOES_, "no glEGLImageTargetRenderbufferStorageOES");
    glEGLImageTargetRenderbufferStorageOES_(GL_RENDERBUFFER, img);
    GL_CHECK("RBStorageOES");

    GLuint fbo = 0;
    glGenFramebuffers(1, &fbo);
    glBindFramebuffer(GL_FRAMEBUFFER, fbo);
    glFramebufferRenderbuffer(GL_FRAMEBUFFER, GL_COLOR_ATTACHMENT0, GL_RENDERBUFFER, rbo);
    VN_CHECK(glCheckFramebufferStatus(GL_FRAMEBUFFER) == GL_FRAMEBUFFER_COMPLETE,
             "framebuffer incomplete");

    glViewport(0, 0, W, H);
    glClearColor(1.0f, 0.0f, 0.0f, 1.0f);
    glClear(GL_COLOR_BUFFER_BIT);
    glFinish();
    std::fprintf(stderr, "ok: rendered red to dma-buf FBO\n");

    // 5. mmap the bo and write a PPM. Verifies CPU can read what GPU wrote.
    uint32_t map_stride = 0;
    void* map_data = nullptr;
    void* mapped = gbm_bo_map(bo, 0, 0, W, H, GBM_BO_TRANSFER_READ, &map_stride, &map_data);
    VN_CHECK(mapped, "gbm_bo_map");

    FILE* f = std::fopen(outpath, "wb");
    VN_CHECK(f, "fopen(%s): %s", outpath, std::strerror(errno));
    std::fprintf(f, "P6\n%d %d\n255\n", W, H);
    int red_pixels = 0;
    for (int y = 0; y < H; ++y) {
        uint8_t* row = (uint8_t*)mapped + y * map_stride;
        for (int x = 0; x < W; ++x) {
            // ARGB8888 in memory little-endian = BGRA bytes: B=row[0], G=row[1], R=row[2].
            uint8_t b = row[x * 4 + 0];
            uint8_t g = row[x * 4 + 1];
            uint8_t r = row[x * 4 + 2];
            uint8_t out[3] = {r, g, b};
            std::fwrite(out, 1, 3, f);
            if (r >= 250 && g <= 5 && b <= 5)
                red_pixels++;
        }
    }
    std::fclose(f);
    gbm_bo_unmap(bo, map_data);

    int expected = W * H;
    std::fprintf(stderr, "ok: PPM written to %s, %d/%d red pixels\n", outpath, red_pixels,
                 expected);
    VN_CHECK(red_pixels == expected,
             "only %d/%d pixels matched red; GPU did not actually clear the surface (or mapping "
             "is wrong)",
             red_pixels, expected);

    // Teardown. EglCtx's destructor handles display/context/gbm/drm cleanup.
    eglDestroyImage(ctx.display(), img);
    glDeleteFramebuffers(1, &fbo);
    glDeleteRenderbuffers(1, &rbo);
    gbm_bo_destroy(bo);

    std::printf("PASS\n");
    return 0;
}
