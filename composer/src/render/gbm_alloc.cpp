#include "src/render/gbm_alloc.hpp"

#include <cstdio>
#include <drm_fourcc.h>
#include <gbm.h>
#include <unistd.h>

#ifndef DRM_FORMAT_MOD_LINEAR
#define DRM_FORMAT_MOD_LINEAR 0ULL
#endif

namespace gbm_alloc {

std::mutex& gbm_device_mu() {
    static std::mutex m;
    return m;
}

namespace {

struct MapState {
    void* map_data = nullptr;
};

gbm_bo* alloc_linear_(gbm_device* gbm, int w, int h, uint32_t fourcc) {
    uint64_t linear = DRM_FORMAT_MOD_LINEAR;
    gbm_bo* bo = gbm_bo_create_with_modifiers(gbm, w, h, fourcc, &linear, 1);
    if (bo)
        return bo;
    return gbm_bo_create(gbm, w, h, fourcc, GBM_BO_USE_LINEAR | GBM_BO_USE_RENDERING);
}

} // namespace

Nv12Buf alloc(gbm_device* gbm, int width, int height) {
    Nv12Buf out;
    if (!gbm || width <= 0 || height <= 0 || (width & 1) || (height & 1)) {
        std::fprintf(stderr, "gbm_alloc: invalid args (gbm=%p w=%d h=%d)\n", (void*)gbm, width,
                     height);
        return out;
    }

    // Y plane: R8 at full resolution.
    gbm_bo* y_bo = alloc_linear_(gbm, width, height, DRM_FORMAT_R8);
    if (!y_bo) {
        std::fprintf(stderr, "gbm_alloc: gbm_bo_create R8 %dx%d failed (Y plane)\n", width, height);
        return out;
    }
    // UV plane: GR88 at half resolution.
    gbm_bo* uv_bo = alloc_linear_(gbm, width / 2, height / 2, DRM_FORMAT_GR88);
    if (!uv_bo) {
        std::fprintf(stderr, "gbm_alloc: gbm_bo_create GR88 %dx%d failed (UV plane)\n", width / 2,
                     height / 2);
        gbm_bo_destroy(y_bo);
        return out;
    }
    int y_fd = gbm_bo_get_fd(y_bo);
    int uv_fd = gbm_bo_get_fd(uv_bo);
    if (y_fd < 0 || uv_fd < 0) {
        std::fprintf(stderr, "gbm_alloc: gbm_bo_get_fd y=%d uv=%d\n", y_fd, uv_fd);
        if (y_fd >= 0)
            ::close(y_fd);
        if (uv_fd >= 0)
            ::close(uv_fd);
        gbm_bo_destroy(y_bo);
        gbm_bo_destroy(uv_bo);
        return out;
    }

    out.y_bo = y_bo;
    out.y_fd = y_fd;
    out.y_stride = gbm_bo_get_stride(y_bo);
    out.uv_bo = uv_bo;
    out.uv_fd = uv_fd;
    out.uv_stride = gbm_bo_get_stride(uv_bo);
    out.width = width;
    out.height = height;
    out.modifier = gbm_bo_get_modifier(y_bo);

    gbm_bo_set_user_data(y_bo, new MapState(), [](gbm_bo*, void* p) { delete (MapState*)p; });
    gbm_bo_set_user_data(uv_bo, new MapState(), [](gbm_bo*, void* p) { delete (MapState*)p; });
    return out;
}

Mapped map_rw(Nv12Buf& b) {
    Mapped m;
    if (!b.valid())
        return m;
    std::lock_guard<std::mutex> g(gbm_device_mu());
    uint32_t s = 0;
    auto* ys = static_cast<MapState*>(gbm_bo_get_user_data(b.y_bo));
    auto* uvs = static_cast<MapState*>(gbm_bo_get_user_data(b.uv_bo));
    if (ys)
        m.y = gbm_bo_map(b.y_bo, 0, 0, b.width, b.height, GBM_BO_TRANSFER_READ_WRITE, &s,
                         &ys->map_data);
    if (uvs)
        m.uv = gbm_bo_map(b.uv_bo, 0, 0, b.width / 2, b.height / 2, GBM_BO_TRANSFER_READ_WRITE, &s,
                          &uvs->map_data);
    m.height = b.height;
    m.y_stride = b.y_stride;
    m.uv_stride = b.uv_stride;
    return m;
}

std::span<uint8_t> Mapped::y_bytes() const {
    if (!y)
        return {};
    return {static_cast<uint8_t*>(y), size_t(height) * y_stride};
}

std::span<uint8_t> Mapped::uv_bytes() const {
    if (!uv)
        return {};
    return {static_cast<uint8_t*>(uv), size_t(height) / 2 * uv_stride};
}

void unmap(Nv12Buf& b) {
    if (!b.valid())
        return;
    std::lock_guard<std::mutex> g(gbm_device_mu());
    auto* ys = static_cast<MapState*>(gbm_bo_get_user_data(b.y_bo));
    auto* uvs = static_cast<MapState*>(gbm_bo_get_user_data(b.uv_bo));
    if (ys && ys->map_data) {
        gbm_bo_unmap(b.y_bo, ys->map_data);
        ys->map_data = nullptr;
    }
    if (uvs && uvs->map_data) {
        gbm_bo_unmap(b.uv_bo, uvs->map_data);
        uvs->map_data = nullptr;
    }
}

void free(Nv12Buf& b) {
    unmap(b);
    if (b.y_fd >= 0)
        ::close(b.y_fd);
    if (b.uv_fd >= 0)
        ::close(b.uv_fd);
    if (b.y_bo)
        gbm_bo_destroy(b.y_bo);
    if (b.uv_bo)
        gbm_bo_destroy(b.uv_bo);
    b = Nv12Buf{};
}

} // namespace gbm_alloc
