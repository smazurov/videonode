// import-probe — slice 3 verifier for EglCtx.
//
// Allocates a fake NV12 source, ticks it once, then asks EglCtx to import
// the dma-buf as an EGLImage. If that succeeds, samples it via a
// samplerExternalOES into an RGBA FBO (also a GBM-backed dma-buf), reads
// pixels back, and counts how many look like our source's square color.
//
// Why this is its own probe: NV12 dma-buf import + samplerExternalOES is
// the single most likely place for Mali/panthor quirks to bite. Validating
// it here in isolation means slice 4's multi-quad compose can assume the
// NV12 sampling path works.
//
// Usage: ./import-probe [device] [src_w] [src_h]

#include "src/common/probe_check.hpp"
#include "src/render/egl_ctx.hpp"
#include "src/render/fake_source.hpp"
#include "src/ipc/dma_heap.hpp"

#include <EGL/egl.h>
#include <EGL/eglext.h>
#include <GLES2/gl2.h>
#include <GLES2/gl2ext.h>
#include <drm_fourcc.h>
#include <gbm.h>

#include <cstdint>
#include <cstdio>
#include <cstdlib>
#include <cstring>
#include <sys/mman.h>
#include <unistd.h>

// Vertex shader: a full-screen triangle (one big triangle is faster than two).
static const char* kVS = R"(
attribute vec2 a_pos;
varying vec2 v_uv;
void main() {
  v_uv = a_pos * 0.5 + 0.5;
  v_uv.y = 1.0 - v_uv.y;     // Y-flip so source's top-left appears top-left in FBO.
  gl_Position = vec4(a_pos, 0.0, 1.0);
}
)";

// Fragment shader: sample external NV12 texture directly.
// samplerExternalOES handles YUV->RGB internally per the EGLImage hint.
static const char* kFS = R"(
#extension GL_OES_EGL_image_external : require
precision mediump float;
uniform samplerExternalOES u_src;
varying vec2 v_uv;
void main() {
  gl_FragColor = texture2D(u_src, v_uv);
}
)";

static GLuint compile_shader(GLenum type, const char* src) {
    GLuint s = glCreateShader(type);
    glShaderSource(s, 1, &src, nullptr);
    glCompileShader(s);
    GLint ok = 0;
    glGetShaderiv(s, GL_COMPILE_STATUS, &ok);
    if (!ok) {
        char log[1024];
        glGetShaderInfoLog(s, sizeof(log), nullptr, log);
        fprintf(stderr, "shader: %s\n", log);
        return 0;
    }
    return s;
}

int main(int argc, char** argv) {
    setvbuf(stdout, nullptr, _IONBF, 0);
    setvbuf(stderr, nullptr, _IONBF, 0);
    fprintf(stderr, "[trace] main entry argc=%d\n", argc);

    const char* dev = (argc > 1) ? argv[1] : "/dev/dri/renderD130";
    int W = (argc > 2) ? std::atoi(argv[2]) : 640;
    int H = (argc > 3) ? std::atoi(argv[3]) : 480;
    fprintf(stderr, "[trace] dev=%s W=%d H=%d\n", dev, W, H);

    egl_ctx::EglCtx ctx;
    fprintf(stderr, "[trace] before init\n");
    VN_CHECK(ctx.init(dev), "EglCtx::init");
    fprintf(stderr, "[trace] after init, before glGetString\n");
    const GLubyte* renderer = glGetString(GL_RENDERER);
    printf("ok: EGL renderer=%s\n", renderer ? (const char*)renderer : "<null>");

    // 1. Synthetic NV12 source.
    fake_source::FakeSource src;
    VN_CHECK(src.init(W, H, fake_source::kRed), "FakeSource::init");
    src.tick(10); // square at sweep position 10*4=40 px from left
    printf("ok: synth src %dx%d fd=%d\n", W, H, src.dmabuf_fd());

    // 2. Import NV12 as EGLImage.
    egl_ctx::EglCtx::ImageDesc desc;
    desc.fd = src.dmabuf_fd();
    desc.fourcc = DRM_FORMAT_NV12;
    desc.modifier = DRM_FORMAT_MOD_LINEAR;
    desc.width = W;
    desc.height = H;
    desc.plane0_offset = 0;
    desc.plane0_pitch = W;
    desc.plane1_offset = W * H;
    desc.plane1_pitch = W;
    EGLImage src_img = ctx.import_dmabuf(desc);
    VN_CHECK(src_img != EGL_NO_IMAGE, "import NV12 dmabuf");
    printf("ok: imported NV12 EGLImage\n");

    // 3. Bind to GL_TEXTURE_EXTERNAL_OES.
    GLuint tex;
    glGenTextures(1, &tex);
    glBindTexture(GL_TEXTURE_EXTERNAL_OES, tex);
    glTexParameteri(GL_TEXTURE_EXTERNAL_OES, GL_TEXTURE_MIN_FILTER, GL_LINEAR);
    glTexParameteri(GL_TEXTURE_EXTERNAL_OES, GL_TEXTURE_MAG_FILTER, GL_LINEAR);
    glTexParameteri(GL_TEXTURE_EXTERNAL_OES, GL_TEXTURE_WRAP_S, GL_CLAMP_TO_EDGE);
    glTexParameteri(GL_TEXTURE_EXTERNAL_OES, GL_TEXTURE_WRAP_T, GL_CLAMP_TO_EDGE);
    auto glEGLImageTargetTexture2DOES_ =
        (PFNGLEGLIMAGETARGETTEXTURE2DOESPROC)eglGetProcAddress("glEGLImageTargetTexture2DOES");
    VN_CHECK(glEGLImageTargetTexture2DOES_, "no glEGLImageTargetTexture2DOES");
    glEGLImageTargetTexture2DOES_(GL_TEXTURE_EXTERNAL_OES, src_img);
    GL_CHECK("bind NV12 to external texture");
    printf("ok: bound NV12 EGLImage to GL_TEXTURE_EXTERNAL_OES\n");

    // 4. Destination FBO backed by an RGBA GBM dma-buf so we can read pixels.
    gbm_bo* fbo_bo = gbm_bo_create(ctx.gbm(), W, H, GBM_FORMAT_ARGB8888,
                                   GBM_BO_USE_RENDERING | GBM_BO_USE_LINEAR);
    VN_CHECK(fbo_bo, "gbm_bo_create FBO");
    uint32_t fbo_stride = gbm_bo_get_stride(fbo_bo);
    int fbo_fd = gbm_bo_get_fd(fbo_bo);

    egl_ctx::EglCtx::ImageDesc fbo_desc;
    fbo_desc.fd = fbo_fd;
    fbo_desc.fourcc = DRM_FORMAT_ARGB8888;
    fbo_desc.modifier = DRM_FORMAT_MOD_LINEAR;
    fbo_desc.width = W;
    fbo_desc.height = H;
    fbo_desc.plane0_offset = 0;
    fbo_desc.plane0_pitch = fbo_stride;
    EGLImage fbo_img = ctx.import_dmabuf(fbo_desc);
    VN_CHECK(fbo_img != EGL_NO_IMAGE, "import RGBA FBO dmabuf");

    GLuint rbo;
    glGenRenderbuffers(1, &rbo);
    glBindRenderbuffer(GL_RENDERBUFFER, rbo);
    auto glEGLImageTargetRenderbufferStorageOES_ =
        (PFNGLEGLIMAGETARGETRENDERBUFFERSTORAGEOESPROC)eglGetProcAddress(
            "glEGLImageTargetRenderbufferStorageOES");
    VN_CHECK(glEGLImageTargetRenderbufferStorageOES_, "no glEGLImageTargetRenderbufferStorageOES");
    glEGLImageTargetRenderbufferStorageOES_(GL_RENDERBUFFER, fbo_img);
    GL_CHECK("bind FBO RBO storage");

    GLuint fbo;
    glGenFramebuffers(1, &fbo);
    glBindFramebuffer(GL_FRAMEBUFFER, fbo);
    glFramebufferRenderbuffer(GL_FRAMEBUFFER, GL_COLOR_ATTACHMENT0, GL_RENDERBUFFER, rbo);
    VN_CHECK(glCheckFramebufferStatus(GL_FRAMEBUFFER) == GL_FRAMEBUFFER_COMPLETE,
             "framebuffer incomplete");

    // 5. Program + draw.
    GLuint vs = compile_shader(GL_VERTEX_SHADER, kVS);
    GLuint fs = compile_shader(GL_FRAGMENT_SHADER, kFS);
    VN_CHECK(vs && fs, "shader compile");
    GLuint prog = glCreateProgram();
    glAttachShader(prog, vs);
    glAttachShader(prog, fs);
    glBindAttribLocation(prog, 0, "a_pos");
    glLinkProgram(prog);
    GLint linked = 0;
    glGetProgramiv(prog, GL_LINK_STATUS, &linked);
    if (!linked) {
        char log[1024];
        glGetProgramInfoLog(prog, sizeof(log), nullptr, log);
        fprintf(stderr, "FAIL link: %s\n", log);
        return 1;
    }
    glUseProgram(prog);
    glActiveTexture(GL_TEXTURE0);
    glUniform1i(glGetUniformLocation(prog, "u_src"), 0);

    // Single big triangle covering NDC [-1..1] -> samples whole source.
    float verts[6] = {-1.f, -1.f, 3.f, -1.f, -1.f, 3.f};
    GLuint vbo;
    glGenBuffers(1, &vbo);
    glBindBuffer(GL_ARRAY_BUFFER, vbo);
    glBufferData(GL_ARRAY_BUFFER, sizeof(verts), verts, GL_STATIC_DRAW);
    glEnableVertexAttribArray(0);
    glVertexAttribPointer(0, 2, GL_FLOAT, GL_FALSE, 0, nullptr);

    glViewport(0, 0, W, H);
    glClearColor(0, 0, 0, 1);
    glClear(GL_COLOR_BUFFER_BIT);
    glDrawArrays(GL_TRIANGLES, 0, 3);
    glFinish();
    GL_CHECK("draw");

    // 6. Map FBO once, scan for square-color pixels AND dump PPM in the same pass.
    uint32_t map_stride = 0;
    void* map_data = nullptr;
    void* mapped = gbm_bo_map(fbo_bo, 0, 0, W, H, GBM_BO_TRANSFER_READ, &map_stride, &map_data);
    VN_CHECK(mapped, "gbm_bo_map");

    // Source had a 200x200 red square at column sweep=10*4=40, row (H-200)/2.
    // Sample one pixel inside that square at (~140, H/2) to confirm it's reddish.
    int sample_x = 140;
    int sample_y = H / 2;
    uint8_t* sample_row = (uint8_t*)mapped + sample_y * map_stride + sample_x * 4;
    uint8_t b = sample_row[0], g = sample_row[1], r = sample_row[2], a = sample_row[3];
    printf("ok: sampled pixel(%d,%d) BGRA=(%u,%u,%u,%u)\n", sample_x, sample_y, b, g, r, a);

    // Dump a PPM for visual diff while we still have the mapping.
    FILE* f = std::fopen("/tmp/import-probe.ppm", "wb");
    bool ppm_ok = false;
    if (f) {
        std::fprintf(f, "P6\n%d %d\n255\n", W, H);
        for (int y = 0; y < H; ++y) {
            uint8_t* prow = (uint8_t*)mapped + y * map_stride;
            for (int x = 0; x < W; ++x) {
                uint8_t out[3] = {prow[x * 4 + 2], prow[x * 4 + 1], prow[x * 4 + 0]};
                std::fwrite(out, 1, 3, f);
            }
        }
        std::fclose(f);
        ppm_ok = true;
    } else {
        fprintf(stderr, "warn: fopen /tmp/import-probe.ppm failed (errno=%d)\n", errno);
    }

    gbm_bo_unmap(fbo_bo, map_data);

    // Red square in NV12 (BT.601) decoded as RGB should be roughly R>=180, G<=80, B<=80.
    // Allow generous tolerance because Mali's CSC and our YUV constants may
    // disagree by a few digits.
    if (!(r >= 150 && g <= 100 && b <= 100)) {
        fprintf(stderr, "FAIL: pixel did not look red\n");
        return 1;
    }

    eglDestroyImage(ctx.display(), src_img);
    eglDestroyImage(ctx.display(), fbo_img);
    gbm_bo_destroy(fbo_bo);

    printf("PASS: NV12 dmabuf -> samplerExternalOES -> RGBA dmabuf round-trip works%s\n",
           ppm_ok ? " (PPM dumped to /tmp/import-probe.ppm)" : "");
    return 0;
}
