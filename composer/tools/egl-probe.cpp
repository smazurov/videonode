// egl-probe — success gate for the GPU composer's EGL/GBM setup.
//
// What it proves:
//  1. We can open a DRM render node, create a GBM device, initialize EGL,
//     and bind a surfaceless GLES2 context on Mali-G610 via Mesa+Panthor.
//  2. We can allocate a GBM buffer object, export it as a dma-buf fd,
//     import that fd back as an EGLImage, attach it to an FBO renderbuffer,
//     render to it, and read the result back via mmap.
//  3. The full dma-buf round-trip the composer depends on works.
//
// Usage:  ./egl-probe [/dev/dri/renderD130] [out.ppm]
//   - Default device: /dev/dri/renderD130 (panthor on this rig)
//   - Default output: /tmp/egl-probe.ppm (should be solid red on success)
//
// Exit status: 0 on full success; non-zero with a diagnostic line on failure.

#include <EGL/egl.h>
#include <EGL/eglext.h>
#include <GLES2/gl2.h>
#include <GLES2/gl2ext.h>
#include <gbm.h>
#include <drm_fourcc.h>

#include <cerrno>
#include <cstdint>
#include <cstdio>
#include <cstdlib>
#include <cstring>
#include <fcntl.h>
#include <sys/mman.h>
#include <unistd.h>

#define DIE(...)                                                                                   \
    do {                                                                                           \
        fprintf(stderr, "FAIL: " __VA_ARGS__);                                                     \
        fprintf(stderr, "\n");                                                                     \
        return 1;                                                                                  \
    } while (0)
#define LOG(...)                                                                                   \
    do {                                                                                           \
        fprintf(stderr, "ok: " __VA_ARGS__);                                                       \
        fprintf(stderr, "\n");                                                                     \
    } while (0)

static const char* egl_err_str(EGLint e) {
    switch (e) {
    case EGL_SUCCESS:
        return "EGL_SUCCESS";
    case EGL_NOT_INITIALIZED:
        return "EGL_NOT_INITIALIZED";
    case EGL_BAD_ACCESS:
        return "EGL_BAD_ACCESS";
    case EGL_BAD_ALLOC:
        return "EGL_BAD_ALLOC";
    case EGL_BAD_ATTRIBUTE:
        return "EGL_BAD_ATTRIBUTE";
    case EGL_BAD_CONFIG:
        return "EGL_BAD_CONFIG";
    case EGL_BAD_CONTEXT:
        return "EGL_BAD_CONTEXT";
    case EGL_BAD_CURRENT_SURFACE:
        return "EGL_BAD_CURRENT_SURFACE";
    case EGL_BAD_DISPLAY:
        return "EGL_BAD_DISPLAY";
    case EGL_BAD_MATCH:
        return "EGL_BAD_MATCH";
    case EGL_BAD_NATIVE_PIXMAP:
        return "EGL_BAD_NATIVE_PIXMAP";
    case EGL_BAD_NATIVE_WINDOW:
        return "EGL_BAD_NATIVE_WINDOW";
    case EGL_BAD_PARAMETER:
        return "EGL_BAD_PARAMETER";
    case EGL_BAD_SURFACE:
        return "EGL_BAD_SURFACE";
    case EGL_CONTEXT_LOST:
        return "EGL_CONTEXT_LOST";
    default:
        return "EGL_?";
    }
}

#define EGL_CHECK(call)                                                                            \
    do {                                                                                           \
        auto _r = (call);                                                                          \
        if (_r == 0) {                                                                             \
            EGLint _e = eglGetError();                                                             \
            DIE(#call ": %s", egl_err_str(_e));                                                    \
        }                                                                                          \
    } while (0)

int main(int argc, char** argv) {
    const char* device = (argc > 1) ? argv[1] : "/dev/dri/renderD130";
    const char* outpath = (argc > 2) ? argv[2] : "/tmp/egl-probe.ppm";
    constexpr int W = 64, H = 64;

    // 1. DRM render node + GBM device.
    int drm_fd = open(device, O_RDWR | O_CLOEXEC);
    if (drm_fd < 0)
        DIE("open(%s): %s", device, strerror(errno));
    LOG("opened %s fd=%d", device, drm_fd);

    gbm_device* gbm = gbm_create_device(drm_fd);
    if (!gbm)
        DIE("gbm_create_device");
    LOG("gbm_device backend=%s", gbm_device_get_backend_name(gbm));

    // 2. EGL display via GBM platform.
    EGLDisplay dpy = eglGetPlatformDisplay(EGL_PLATFORM_GBM_KHR, gbm, nullptr);
    if (dpy == EGL_NO_DISPLAY)
        DIE("eglGetPlatformDisplay");

    EGLint major, minor;
    EGL_CHECK(eglInitialize(dpy, &major, &minor));
    LOG("EGL %d.%d vendor=%s", major, minor, eglQueryString(dpy, EGL_VENDOR));
    LOG("EGL driver=%s", eglQueryString(dpy, EGL_VENDOR));

    // 3. Bind GLES2 + a surfaceless context.
    // We render only into FBOs backed by dma-buf EGLImages, so we want no window
    // surface at all. EGL_KHR_no_config_context (advertised by panthor's Mesa
    // driver) lets us skip eglChooseConfig entirely. This is also more portable
    // across Mesa drivers than requesting EGL_SURFACE_TYPE = EGL_PBUFFER_BIT.
    EGL_CHECK(eglBindAPI(EGL_OPENGL_ES_API));

    const char* exts = eglQueryString(dpy, EGL_EXTENSIONS);
    if (!exts || !strstr(exts, "EGL_KHR_no_config_context"))
        DIE("driver lacks EGL_KHR_no_config_context");
    if (!strstr(exts, "EGL_KHR_surfaceless_context"))
        DIE("driver lacks EGL_KHR_surfaceless_context");

    const EGLint ctx_attribs[] = {EGL_CONTEXT_CLIENT_VERSION, 2, EGL_NONE};
    EGLContext ctx = eglCreateContext(dpy, EGL_NO_CONFIG_KHR, EGL_NO_CONTEXT, ctx_attribs);
    if (ctx == EGL_NO_CONTEXT)
        DIE("eglCreateContext: %s", egl_err_str(eglGetError()));
    LOG("created EGL_NO_CONFIG_KHR GLES2 context");

    EGL_CHECK(eglMakeCurrent(dpy, EGL_NO_SURFACE, EGL_NO_SURFACE, ctx));
    LOG("GL renderer=%s", glGetString(GL_RENDERER));
    LOG("GL version=%s", glGetString(GL_VERSION));

    // 4. Allocate a GBM bo we can render to and export as dma-buf.
    gbm_bo* bo =
        gbm_bo_create(gbm, W, H, GBM_FORMAT_ARGB8888, GBM_BO_USE_RENDERING | GBM_BO_USE_LINEAR);
    if (!bo)
        DIE("gbm_bo_create");
    uint32_t stride = gbm_bo_get_stride(bo);
    int dmabuf_fd = gbm_bo_get_fd(bo);
    if (dmabuf_fd < 0)
        DIE("gbm_bo_get_fd");
    LOG("gbm_bo %dx%d stride=%u dmabuf_fd=%d", W, H, stride, dmabuf_fd);

    // 5. Import the dma-buf back as an EGLImage (the round-trip the composer depends on).
    EGLAttrib img_attribs[] = {
        EGL_WIDTH,
        W,
        EGL_HEIGHT,
        H,
        EGL_LINUX_DRM_FOURCC_EXT,
        (EGLAttrib)DRM_FORMAT_ARGB8888,
        EGL_DMA_BUF_PLANE0_FD_EXT,
        (EGLAttrib)dmabuf_fd,
        EGL_DMA_BUF_PLANE0_OFFSET_EXT,
        0,
        EGL_DMA_BUF_PLANE0_PITCH_EXT,
        (EGLAttrib)stride,
        EGL_NONE,
    };
    EGLImage img = eglCreateImage(dpy, EGL_NO_CONTEXT, EGL_LINUX_DMA_BUF_EXT,
                                  (EGLClientBuffer) nullptr, img_attribs);
    if (img == EGL_NO_IMAGE)
        DIE("eglCreateImage: %s", egl_err_str(eglGetError()));
    LOG("eglCreateImage(dmabuf) OK");

    // 6. Attach EGLImage to an FBO renderbuffer, clear red, finish.
    GLuint rbo;
    glGenRenderbuffers(1, &rbo);
    glBindRenderbuffer(GL_RENDERBUFFER, rbo);

    auto glEGLImageTargetRenderbufferStorageOES =
        (PFNGLEGLIMAGETARGETRENDERBUFFERSTORAGEOESPROC)eglGetProcAddress(
            "glEGLImageTargetRenderbufferStorageOES");
    if (!glEGLImageTargetRenderbufferStorageOES)
        DIE("no glEGLImageTargetRenderbufferStorageOES");
    glEGLImageTargetRenderbufferStorageOES(GL_RENDERBUFFER, img);
    if (GLenum e = glGetError(); e != GL_NO_ERROR)
        DIE("RBStorageOES: 0x%x", e);

    GLuint fbo;
    glGenFramebuffers(1, &fbo);
    glBindFramebuffer(GL_FRAMEBUFFER, fbo);
    glFramebufferRenderbuffer(GL_FRAMEBUFFER, GL_COLOR_ATTACHMENT0, GL_RENDERBUFFER, rbo);
    if (glCheckFramebufferStatus(GL_FRAMEBUFFER) != GL_FRAMEBUFFER_COMPLETE)
        DIE("framebuffer incomplete");

    glViewport(0, 0, W, H);
    glClearColor(1.0f, 0.0f, 0.0f, 1.0f);
    glClear(GL_COLOR_BUFFER_BIT);
    glFinish();
    LOG("rendered red to dma-buf FBO");

    // 7. mmap the bo and write a PPM. Verifies CPU can read what GPU wrote.
    uint32_t map_stride = 0;
    void* map_data = nullptr;
    void* mapped = gbm_bo_map(bo, 0, 0, W, H, GBM_BO_TRANSFER_READ, &map_stride, &map_data);
    if (!mapped)
        DIE("gbm_bo_map");

    FILE* f = fopen(outpath, "wb");
    if (!f)
        DIE("fopen(%s): %s", outpath, strerror(errno));
    fprintf(f, "P6\n%d %d\n255\n", W, H);
    int red_pixels = 0;
    for (int y = 0; y < H; ++y) {
        uint8_t* row = (uint8_t*)mapped + y * map_stride;
        for (int x = 0; x < W; ++x) {
            // ARGB8888 in memory little-endian = BGRA bytes; B=row[0], G=row[1], R=row[2],
            // A=row[3].
            uint8_t b = row[x * 4 + 0];
            uint8_t g = row[x * 4 + 1];
            uint8_t r = row[x * 4 + 2];
            uint8_t out[3] = {r, g, b};
            fwrite(out, 1, 3, f);
            if (r >= 250 && g <= 5 && b <= 5)
                red_pixels++;
        }
    }
    fclose(f);
    gbm_bo_unmap(bo, map_data);

    int expected = W * H;
    LOG("PPM written to %s, %d/%d red pixels", outpath, red_pixels, expected);
    if (red_pixels != expected)
        DIE("only %d/%d pixels matched red; GPU did not actually clear the surface (or mapping is "
            "wrong)",
            red_pixels, expected);

    // Teardown.
    eglDestroyImage(dpy, img);
    glDeleteFramebuffers(1, &fbo);
    glDeleteRenderbuffers(1, &rbo);
    eglMakeCurrent(dpy, EGL_NO_SURFACE, EGL_NO_SURFACE, EGL_NO_CONTEXT);
    eglDestroyContext(dpy, ctx);
    eglTerminate(dpy);
    gbm_bo_destroy(bo);
    gbm_device_destroy(gbm);
    close(drm_fd);

    printf("PASS\n");
    return 0;
}
