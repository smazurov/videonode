#include "src/render/egl_ctx.hpp"

#include <cerrno>
#include <cstdio>
#include <cstring>
#include <fcntl.h>
#include <gbm.h>
#include <string>
#include <unistd.h>

namespace egl_ctx {

namespace {

#define DIE_F(...)                                                                                 \
    do {                                                                                           \
        fprintf(stderr, "egl_ctx: " __VA_ARGS__);                                                  \
        fprintf(stderr, "\n");                                                                     \
        return false;                                                                              \
    } while (0)

} // namespace

bool EglCtx::init(std::string_view device_path) {
    std::string dev(device_path);
    drm_fd_ = ::open(dev.c_str(), O_RDWR | O_CLOEXEC);
    if (drm_fd_ < 0)
        DIE_F("open(%s): %s", dev.c_str(), strerror(errno));

    gbm_ = gbm_create_device(drm_fd_);
    if (!gbm_)
        DIE_F("gbm_create_device");

    dpy_ = eglGetPlatformDisplay(EGL_PLATFORM_GBM_KHR, gbm_, nullptr);
    if (dpy_ == EGL_NO_DISPLAY)
        DIE_F("eglGetPlatformDisplay");

    EGLint major = 0, minor = 0;
    if (!eglInitialize(dpy_, &major, &minor))
        DIE_F("eglInitialize");

    if (!eglBindAPI(EGL_OPENGL_ES_API))
        DIE_F("eglBindAPI");

    const char* exts = eglQueryString(dpy_, EGL_EXTENSIONS);
    if (!exts || !std::strstr(exts, "EGL_KHR_no_config_context") ||
        !std::strstr(exts, "EGL_KHR_surfaceless_context") ||
        !std::strstr(exts, "EGL_EXT_image_dma_buf_import")) {
        DIE_F("required EGL extensions missing");
    }

    const EGLint ctx_attribs[] = {EGL_CONTEXT_CLIENT_VERSION, 2, EGL_NONE};
    ctx_ = eglCreateContext(dpy_, EGL_NO_CONFIG_KHR, EGL_NO_CONTEXT, ctx_attribs);
    if (ctx_ == EGL_NO_CONTEXT)
        DIE_F("eglCreateContext");

    if (!eglMakeCurrent(dpy_, EGL_NO_SURFACE, EGL_NO_SURFACE, ctx_))
        DIE_F("eglMakeCurrent");
    return true;
}

EglCtx::~EglCtx() {
    if (dpy_ != EGL_NO_DISPLAY) {
        eglMakeCurrent(dpy_, EGL_NO_SURFACE, EGL_NO_SURFACE, EGL_NO_CONTEXT);
        if (ctx_ != EGL_NO_CONTEXT)
            eglDestroyContext(dpy_, ctx_);
        eglTerminate(dpy_);
    }
    if (gbm_)
        gbm_device_destroy(gbm_);
    if (drm_fd_ >= 0)
        ::close(drm_fd_);
}

bool EglCtx::make_current() const {
    return eglMakeCurrent(dpy_, EGL_NO_SURFACE, EGL_NO_SURFACE, ctx_);
}

EGLImage EglCtx::import_dmabuf(const ImageDesc& d) const {
    // Build the attribute list dynamically: NV12 needs two plane descriptors,
    // single-plane formats only need one. Modifier is optional but Mali
    // tends to prefer it being explicit even for LINEAR. radeonsi rejects
    // explicit LINEAR for some YUV formats (NV12) and only accepts implicit
    // modifier handling, so when the caller signals INVALID (or just leaves
    // modifier at the default 0/LINEAR) we may need to skip the MODIFIER
    // attrs. Sentinel for "let the driver pick": DRM_FORMAT_MOD_INVALID.
    constexpr uint64_t kModInvalid = (uint64_t{1} << 56) - 1; // 0x00ffffffffffffff
    const bool send_modifier = (d.modifier != kModInvalid);
    EGLAttrib attrs[40];
    int i = 0;
    attrs[i++] = EGL_WIDTH;
    attrs[i++] = d.width;
    attrs[i++] = EGL_HEIGHT;
    attrs[i++] = d.height;
    attrs[i++] = EGL_LINUX_DRM_FOURCC_EXT;
    attrs[i++] = (EGLAttrib)d.fourcc;
    attrs[i++] = EGL_DMA_BUF_PLANE0_FD_EXT;
    attrs[i++] = (EGLAttrib)d.fd;
    attrs[i++] = EGL_DMA_BUF_PLANE0_OFFSET_EXT;
    attrs[i++] = d.plane0_offset;
    attrs[i++] = EGL_DMA_BUF_PLANE0_PITCH_EXT;
    attrs[i++] = d.plane0_pitch;
    if (send_modifier) {
        attrs[i++] = EGL_DMA_BUF_PLANE0_MODIFIER_LO_EXT;
        attrs[i++] = (EGLAttrib)(d.modifier & 0xFFFFFFFFu);
        attrs[i++] = EGL_DMA_BUF_PLANE0_MODIFIER_HI_EXT;
        attrs[i++] = (EGLAttrib)(d.modifier >> 32);
    }
    if (d.plane1_pitch > 0) {
        attrs[i++] = EGL_DMA_BUF_PLANE1_FD_EXT;
        attrs[i++] = (EGLAttrib)(d.plane1_fd >= 0 ? d.plane1_fd : d.fd);
        attrs[i++] = EGL_DMA_BUF_PLANE1_OFFSET_EXT;
        attrs[i++] = d.plane1_offset;
        attrs[i++] = EGL_DMA_BUF_PLANE1_PITCH_EXT;
        attrs[i++] = d.plane1_pitch;
        if (send_modifier) {
            attrs[i++] = EGL_DMA_BUF_PLANE1_MODIFIER_LO_EXT;
            attrs[i++] = (EGLAttrib)(d.modifier & 0xFFFFFFFFu);
            attrs[i++] = EGL_DMA_BUF_PLANE1_MODIFIER_HI_EXT;
            attrs[i++] = (EGLAttrib)(d.modifier >> 32);
        }
        // YUV colorspace + range hints. Required by Mesa for some YUV
        // dma-buf imports to actually sample as YUV (rather than zeroed).
        // BT.601 limited range matches dmabuf_header::Header's
        // Bt601/Limited contract; if a producer ever needs a different
        // matrix it'd propagate via FrameView and feed in here.
        attrs[i++] = EGL_YUV_COLOR_SPACE_HINT_EXT;
        attrs[i++] = EGL_ITU_REC601_EXT;
        attrs[i++] = EGL_SAMPLE_RANGE_HINT_EXT;
        attrs[i++] = EGL_YUV_NARROW_RANGE_EXT;
        attrs[i++] = EGL_YUV_CHROMA_HORIZONTAL_SITING_HINT_EXT;
        attrs[i++] = EGL_YUV_CHROMA_SITING_0_EXT;
        attrs[i++] = EGL_YUV_CHROMA_VERTICAL_SITING_HINT_EXT;
        attrs[i++] = EGL_YUV_CHROMA_SITING_0_EXT;
    }
    attrs[i++] = EGL_NONE;

    EGLImage img = eglCreateImage(dpy_, EGL_NO_CONTEXT, EGL_LINUX_DMA_BUF_EXT,
                                  (EGLClientBuffer) nullptr, attrs);
    if (img == EGL_NO_IMAGE) {
        fprintf(stderr, "egl_ctx: eglCreateImage failed (fourcc=0x%08x w=%d h=%d)\n", d.fourcc,
                d.width, d.height);
    }
    return img;
}

} // namespace egl_ctx
