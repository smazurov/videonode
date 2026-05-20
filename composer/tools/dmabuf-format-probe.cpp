// dmabuf-format-probe — enumerate which (DRM fourcc, modifier) pairs the
// running EGL/GBM stack will accept for EGL_LINUX_DMA_BUF_EXT import.
//
// Uses EGL_EXT_image_dma_buf_import_modifiers:
//   - eglQueryDmaBufFormatsEXT  → set of fourccs
//   - eglQueryDmaBufModifiersEXT → per-fourcc modifier list
//
// Then for each (fourcc, LINEAR) it allocates a dma_heap NV12-sized
// buffer and attempts an actual eglCreateImage to confirm the format
// works end-to-end, not just on the support-claim list.
//
// Useful on the rig to tell us *exactly* which formats panthor accepts,
// which determines whether HDMI-IN native formats (NV24/NV16/BGR3) can be
// fed to the composer without an RGA CSC step.

#include "../src/egl_ctx.hpp"

#include <EGL/egl.h>
#include <EGL/eglext.h>
#include <drm_fourcc.h>
#include <fcntl.h>
#include <linux/dma-heap.h>
#include <sys/ioctl.h>

#include <cstdio>
#include <cstdint>
#include <cstring>
#include <string>
#include <unistd.h>
#include <vector>

namespace {

// Format names we want to report on by name. The probe also lists every
// fourcc the driver claims; this map just adds readable labels.
struct NamedFmt {
    uint32_t fourcc;
    const char* name;
};
const NamedFmt kNamedFormats[] = {
    {DRM_FORMAT_NV12, "NV12"},     {DRM_FORMAT_NV16, "NV16"},     {DRM_FORMAT_NV24, "NV24"},
    {DRM_FORMAT_NV21, "NV21"},     {DRM_FORMAT_YUYV, "YUYV"},     {DRM_FORMAT_UYVY, "UYVY"},
    {DRM_FORMAT_BGR888, "BG24"},   {DRM_FORMAT_RGB888, "RG24"},   {DRM_FORMAT_ARGB8888, "AR24"},
    {DRM_FORMAT_XRGB8888, "XR24"}, {DRM_FORMAT_ABGR8888, "AB24"}, {DRM_FORMAT_XBGR8888, "XB24"},
};

std::string fourcc_str(uint32_t f) {
    char b[5] = {char(f & 0xFF), char((f >> 8) & 0xFF), char((f >> 16) & 0xFF),
                 char((f >> 24) & 0xFF), 0};
    for (int i = 0; i < 4; ++i)
        if (b[i] < 32 || b[i] > 126)
            b[i] = '?';
    return std::string(b);
}

const char* known_name(uint32_t f) {
    for (auto& n : kNamedFormats)
        if (n.fourcc == f)
            return n.name;
    return "(unknown)";
}

// alloc_dmaheap allocates `size` bytes from /dev/dma_heap/system; returns
// fd or -1.
int alloc_dmaheap(size_t size) {
    int hfd = ::open("/dev/dma_heap/system", O_RDWR | O_CLOEXEC);
    if (hfd < 0)
        return -1;
    dma_heap_allocation_data req{};
    req.len = size;
    req.fd_flags = O_RDWR | O_CLOEXEC;
    int ret = ::ioctl(hfd, DMA_HEAP_IOCTL_ALLOC, &req);
    ::close(hfd);
    return ret < 0 ? -1 : static_cast<int>(req.fd);
}

// per_format_byte_size returns a reasonable dma-buf size for a 320x240
// frame in the given format. Conservative — over-allocate is fine for a
// probe, under-allocate would fail import.
size_t probe_size(uint32_t fourcc, int w, int h) {
    switch (fourcc) {
    case DRM_FORMAT_NV12:
    case DRM_FORMAT_NV21:
        return size_t(w * h * 3 / 2);
    case DRM_FORMAT_NV16:
        return size_t(w * h * 2);
    case DRM_FORMAT_NV24:
        return size_t(w * h * 3);
    case DRM_FORMAT_YUYV:
    case DRM_FORMAT_UYVY:
        return size_t(w * h * 2);
    case DRM_FORMAT_BGR888:
    case DRM_FORMAT_RGB888:
        return size_t(w * h * 3);
    case DRM_FORMAT_ARGB8888:
    case DRM_FORMAT_XRGB8888:
    case DRM_FORMAT_ABGR8888:
    case DRM_FORMAT_XBGR8888:
        return size_t(w * h * 4);
    }
    return size_t(w * h * 4); // safe fallback
}

bool try_import(const egl_ctx::EglCtx& ctx, uint32_t fourcc, int w, int h) {
    size_t size = probe_size(fourcc, w, h);
    int fd = alloc_dmaheap(size);
    if (fd < 0)
        return false;

    egl_ctx::EglCtx::ImageDesc d;
    d.fd = fd;
    d.fourcc = fourcc;
    d.modifier = DRM_FORMAT_MOD_LINEAR;
    d.width = w;
    d.height = h;
    d.plane0_offset = 0;

    // Per-format plane setup — keep in lockstep with format_dispatch.cpp.
    switch (fourcc) {
    case DRM_FORMAT_NV12:
        d.plane0_pitch = w;
        d.plane1_pitch = w;
        d.plane1_offset = w * h;
        break;
    case DRM_FORMAT_NV21:
        d.plane0_pitch = w;
        d.plane1_pitch = w;
        d.plane1_offset = w * h;
        break;
    case DRM_FORMAT_NV16:
        d.plane0_pitch = w;
        d.plane1_pitch = w;
        d.plane1_offset = w * h;
        break;
    case DRM_FORMAT_NV24:
        d.plane0_pitch = w;
        d.plane1_pitch = w * 2;
        d.plane1_offset = w * h;
        break;
    case DRM_FORMAT_YUYV:
    case DRM_FORMAT_UYVY:
        d.plane0_pitch = w * 2;
        break;
    case DRM_FORMAT_BGR888:
    case DRM_FORMAT_RGB888:
        d.plane0_pitch = w * 3;
        break;
    case DRM_FORMAT_ARGB8888:
    case DRM_FORMAT_XRGB8888:
    case DRM_FORMAT_ABGR8888:
    case DRM_FORMAT_XBGR8888:
        d.plane0_pitch = w * 4;
        break;
    default:
        d.plane0_pitch = w; // best effort
    }

    EGLImage img = ctx.import_dmabuf(d);
    ::close(fd);
    if (img != EGL_NO_IMAGE) {
        eglDestroyImage(ctx.display(), img);
        return true;
    }
    return false;
}

} // namespace

int main(int argc, char** argv) {
    const char* drm_path = (argc > 1) ? argv[1] : "/dev/dri/renderD130";

    egl_ctx::EglCtx ctx;
    if (!ctx.init(drm_path)) {
        fprintf(stderr, "init EGL via %s failed\n", drm_path);
        return 1;
    }

    auto eglQueryDmaBufFormats =
        reinterpret_cast<EGLBoolean(EGLAPIENTRY*)(EGLDisplay, EGLint, EGLint*, EGLint*)>(
            eglGetProcAddress("eglQueryDmaBufFormatsEXT"));
    auto eglQueryDmaBufModifiers = reinterpret_cast<EGLBoolean(EGLAPIENTRY*)(
        EGLDisplay, EGLint, EGLint, EGLuint64KHR*, EGLBoolean*, EGLint*)>(
        eglGetProcAddress("eglQueryDmaBufModifiersEXT"));

    if (!eglQueryDmaBufFormats || !eglQueryDmaBufModifiers) {
        fprintf(stderr, "EGL_EXT_image_dma_buf_import_modifiers missing\n");
        return 2;
    }

    EGLDisplay dpy = ctx.display();
    EGLint n_formats = 0;
    eglQueryDmaBufFormats(dpy, 0, nullptr, &n_formats);
    std::vector<EGLint> formats(n_formats);
    eglQueryDmaBufFormats(dpy, n_formats, formats.data(), &n_formats);

    printf("EGL claims %d supported dma-buf fourccs via %s\n", n_formats, drm_path);
    printf("\n%-6s %-10s %-12s %-8s\n", "fourcc", "name", "modifiers", "real");
    printf("------ ---------- ------------ --------\n");

    for (EGLint f : formats) {
        EGLint n_mods = 0;
        eglQueryDmaBufModifiers(dpy, f, 0, nullptr, nullptr, &n_mods);
        bool import_ok = try_import(ctx, static_cast<uint32_t>(f), 320, 240);
        printf("%-6s %-10s %-12d %-8s\n", fourcc_str(static_cast<uint32_t>(f)).c_str(),
               known_name(static_cast<uint32_t>(f)), n_mods,
               import_ok ? "import-OK" : "import-FAIL");
    }

    printf("\n'real' column: actual eglCreateImage attempt with LINEAR modifier on 320x240\n");
    return 0;
}
