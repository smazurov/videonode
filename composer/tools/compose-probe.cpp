// compose-probe — slice 4 verifier for GlCompose.
//
// 1. Init EglCtx on /dev/dri/renderD130 (panthor).
// 2. Allocate 4 FakeSources (640x480), color-coded.
// 3. Import each as an NV12 EGLImage.
// 4. Build GlCompose at 1280x720 canvas size.
// 5. Lay sources out in a 2x2 grid (640x360 cells, half each axis).
// 6. Source 1 (top-right) gets a non-identity 3x3 warp (UV homography that
//    creates a slight keystone — the perspective-unlock demonstration).
// 7. tick() all 4 sources, render frame, dump RGBA canvas to PPM.

#include "../src/egl_ctx.hpp"
#include "../src/fake_source.hpp"
#include "../src/gl_compose.hpp"
#include "../src/dma_heap.hpp"

#include <EGL/egl.h>
#include <GLES2/gl2.h>
#include <drm_fourcc.h>
#include <gbm.h>

#include <cstdint>
#include <cstdio>
#include <cstdlib>
#include <vector>

#define CHECK(expr, msg)                                                                           \
    do {                                                                                           \
        if (!(expr)) {                                                                             \
            fprintf(stderr, "FAIL: %s\n", msg);                                                    \
            return 1;                                                                              \
        }                                                                                          \
    } while (0)

int main(int argc, char** argv) {
    int frame_idx = (argc > 1) ? std::atoi(argv[1]) : 60;
    int Cw = (argc > 2) ? std::atoi(argv[2]) : 1280;
    int Ch = (argc > 3) ? std::atoi(argv[3]) : 720;
    int Sw = 640, Sh = 480;
    const char* out = (argc > 4) ? argv[4] : "/tmp/compose-probe.ppm";

    egl_ctx::EglCtx ctx;
    const char* dev = std::getenv("VN_DRM_DEVICE");
    if (!dev) dev = "/dev/dri/renderD128";
    CHECK(ctx.init(dev), "EglCtx::init");
    printf("ok: renderer=%s\n", glGetString(GL_RENDERER));

    fake_source::FakeSource src[4];
    fake_source::Color colors[4] = {
        fake_source::kRed,
        fake_source::kGreen,
        fake_source::kBlue,
        fake_source::kYellow,
    };
    // Single multi-plane NV12 EGLImage per source — matches the production
    // gl_compose API. samplerExternalOES on the shader side does YUV→RGB.
    EGLImage img[4] = {EGL_NO_IMAGE, EGL_NO_IMAGE, EGL_NO_IMAGE, EGL_NO_IMAGE};
    for (int i = 0; i < 4; ++i) {
        CHECK(src[i].init(Sw, Sh, colors[i]), "FakeSource::init");
        egl_ctx::EglCtx::ImageDesc d;
        d.fd = src[i].dmabuf_fd();
        d.fourcc = DRM_FORMAT_NV12;
        d.modifier = DRM_FORMAT_MOD_LINEAR;
        d.width = Sw;
        d.height = Sh;
        d.plane0_offset = 0;
        d.plane0_pitch = Sw;
        d.plane1_offset = Sw * Sh;
        d.plane1_pitch = Sw;
        img[i] = ctx.import_dmabuf(d);
        CHECK(img[i] != EGL_NO_IMAGE, "import NV12 dmabuf");
    }
    printf("ok: 4 NV12 EGLImages imported\n");

    gl_compose::GlCompose compose;
    CHECK(compose.init(ctx, Cw, Ch), "GlCompose::init");
    printf("ok: GlCompose canvas %dx%d stride=%u fd=%d\n", Cw, Ch, compose.canvas_stride(),
           compose.canvas_dmabuf_fd());

    // 2x2 grid: each cell Cw/2 x Ch/2.
    int cell_w = Cw / 2;
    int cell_h = Ch / 2;
    std::vector<gl_compose::SourceSlot> slots(4);
    for (int i = 0; i < 4; ++i) {
        int row = i / 2, col = i % 2;
        slots[i].src_image = img[i];
        slots[i].x = col * cell_w;
        slots[i].y = row * cell_h;
        slots[i].w = cell_w;
        slots[i].h = cell_h;
    }
    // Source 1 (top-right) gets a keystone warp: pull bottom-right of UV
    // inward to make the source look "tilted back". 3x3 row-major homography
    // mapping (u, v) -> (u', v') / w'.
    //
    // For a clean keystone: scale UVs at v=1 (bottom edge) toward center.
    //   u' = (u - 0.5) * scale_at_v(v) + 0.5
    //   v' = v
    // where scale_at_v(v) = 1.0 - 0.3 * v (top edge full width, bottom 70% width).
    //
    // Expressed as a homogeneous matrix: this is a horizontal scaling that
    // depends on v. We approximate it with a perspective by putting the
    // v-dependence in the w component.
    //
    // Specifically: a projective transform mapping (u, v, 1) to
    //   ((u - 0.5)*1 + 0.5, v, 1 - 0.3*v)
    // gives, after homogeneous divide, the keystone effect at the bottom.
    // Matrix (row-major):
    slots[1].warp = {{1.0f, 0.0f, 0.0f, 0.0f, 1.0f, 0.0f, 0.0f, -0.3f, 1.0f}};

    for (int i = 0; i < 4; ++i)
        src[i].tick(frame_idx);
    CHECK(compose.render(slots), "render");
    compose.finish();
    printf("ok: rendered frame %d\n", frame_idx);

    // Dump canvas to PPM.
    uint32_t stride = 0;
    void* map_data = nullptr;
    void* mapped =
        gbm_bo_map(compose.canvas_bo(), 0, 0, Cw, Ch, GBM_BO_TRANSFER_READ, &stride, &map_data);
    CHECK(mapped, "gbm_bo_map canvas");

    FILE* f = std::fopen(out, "wb");
    CHECK(f, "fopen PPM");
    std::fprintf(f, "P6\n%d %d\n255\n", Cw, Ch);
    for (int y = 0; y < Ch; ++y) {
        uint8_t* row = (uint8_t*)mapped + y * stride;
        for (int x = 0; x < Cw; ++x) {
            uint8_t bgra[3] = {row[x * 4 + 2], row[x * 4 + 1], row[x * 4 + 0]};
            std::fwrite(bgra, 1, 3, f);
        }
    }
    std::fclose(f);
    gbm_bo_unmap(compose.canvas_bo(), map_data);

    // Cleanup.
    for (int i = 0; i < 4; ++i)
        eglDestroyImage(ctx.display(), img[i]);

    printf("PASS: 4-quad GPU compose at %dx%d, PPM at %s\n", Cw, Ch, out);
    return 0;
}
