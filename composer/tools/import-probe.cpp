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
#include <memory>
#include <span>
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

static GLuint build_program() {
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
        return 0;
    }
    return prog;
}

struct FboSetup {
    gbm_bo* bo{nullptr};
    EGLImage img{EGL_NO_IMAGE};
    GLuint rbo{0};
    GLuint fbo{0};
    uint32_t stride{0};
};

static FboSetup create_fbo(egl_ctx::EglCtx& ctx, int W, int H) {
    FboSetup result;
    result.bo = gbm_bo_create(ctx.gbm(), W, H, GBM_FORMAT_ARGB8888,
                              GBM_BO_USE_RENDERING | GBM_BO_USE_LINEAR);
    VN_CHECK(result.bo, "gbm_bo_create FBO");
    result.stride = gbm_bo_get_stride(result.bo);
    int fbo_fd = gbm_bo_get_fd(result.bo);

    egl_ctx::EglCtx::ImageDesc fbo_desc;
    fbo_desc.fd = fbo_fd;
    fbo_desc.fourcc = DRM_FORMAT_ARGB8888;
    fbo_desc.modifier = DRM_FORMAT_MOD_LINEAR;
    fbo_desc.width = W;
    fbo_desc.height = H;
    fbo_desc.plane0_offset = 0;
    fbo_desc.plane0_pitch = static_cast<int>(result.stride);
    result.img = ctx.import_dmabuf(fbo_desc);
    VN_CHECK(result.img != EGL_NO_IMAGE, "import RGBA FBO dmabuf");

    glGenRenderbuffers(1, &result.rbo);
    glBindRenderbuffer(GL_RENDERBUFFER, result.rbo);
    auto glEGLImageTargetRenderbufferStorageOES_ =
        (PFNGLEGLIMAGETARGETRENDERBUFFERSTORAGEOESPROC)eglGetProcAddress(
            "glEGLImageTargetRenderbufferStorageOES");
    VN_CHECK(glEGLImageTargetRenderbufferStorageOES_, "no glEGLImageTargetRenderbufferStorageOES");
    glEGLImageTargetRenderbufferStorageOES_(GL_RENDERBUFFER, result.img);
    GL_CHECK("bind FBO RBO storage");

    glGenFramebuffers(1, &result.fbo);
    glBindFramebuffer(GL_FRAMEBUFFER, result.fbo);
    glFramebufferRenderbuffer(GL_FRAMEBUFFER, GL_COLOR_ATTACHMENT0, GL_RENDERBUFFER, result.rbo);
    VN_CHECK(glCheckFramebufferStatus(GL_FRAMEBUFFER) == GL_FRAMEBUFFER_COMPLETE,
             "framebuffer incomplete");
    return result;
}

static void draw_fullscreen_triangle(int W, int H) {
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
}

static bool dump_ppm(const void* mapped, uint32_t map_stride, int W, int H) {
    std::unique_ptr<FILE, decltype(&std::fclose)> f(std::fopen("/tmp/import-probe.ppm", "wb"),
                                                    &std::fclose);
    if (!f) {
        fprintf(stderr, "warn: fopen /tmp/import-probe.ppm failed (errno=%d)\n", errno);
        return false;
    }
    std::fprintf(f.get(), "P6\n%d %d\n255\n", W, H);
    const std::span<const uint8_t> pixels(static_cast<const uint8_t*>(mapped),
                                          static_cast<size_t>(map_stride) * H);
    for (int y = 0; y < H; ++y) {
        const std::span<const uint8_t> prow =
            pixels.subspan(static_cast<size_t>(y) * map_stride, static_cast<size_t>(W) * 4);
        for (int x = 0; x < W; ++x) {
            const size_t off = static_cast<size_t>(x) * 4;
            uint8_t out[3] = {prow[off + 2], prow[off + 1], prow[off + 0]};
            std::fwrite(out, 1, 3, f.get());
        }
    }
    return true;
}

static EGLImage bind_nv12_source(egl_ctx::EglCtx& ctx, fake_source::FakeSource& src, int W, int H) {
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
    return src_img;
}

int main(int argc, char** argv) {
    setvbuf(stdout, nullptr, _IONBF, 0);
    setvbuf(stderr, nullptr, _IONBF, 0);
    fprintf(stderr, "[trace] main entry argc=%d\n", argc);

    const std::span<char*> args(argv, static_cast<size_t>(argc));
    const char* dev = (args.size() > 1) ? args[1] : "/dev/dri/renderD130";
    int W = (args.size() > 2) ? std::atoi(args[2]) : 640;
    int H = (args.size() > 3) ? std::atoi(args[3]) : 480;
    fprintf(stderr, "[trace] dev=%s W=%d H=%d\n", dev, W, H);

    egl_ctx::EglCtx ctx;
    VN_CHECK(ctx.init(dev), "EglCtx::init");
    const GLubyte* renderer = glGetString(GL_RENDERER);
    printf("ok: EGL renderer=%s\n", renderer ? (const char*)renderer : "<null>");

    fake_source::FakeSource src;
    VN_CHECK(src.init(W, H, fake_source::kRed), "FakeSource::init");
    src.tick(10);
    printf("ok: synth src %dx%d fd=%d\n", W, H, src.dmabuf_fd());

    EGLImage src_img = bind_nv12_source(ctx, src, W, H);
    printf("ok: bound NV12 EGLImage to GL_TEXTURE_EXTERNAL_OES\n");

    FboSetup fbo = create_fbo(ctx, W, H);

    GLuint prog = build_program();
    VN_CHECK(prog != 0, "build program");
    glUseProgram(prog);
    glActiveTexture(GL_TEXTURE0);
    glUniform1i(glGetUniformLocation(prog, "u_src"), 0);

    draw_fullscreen_triangle(W, H);

    uint32_t map_stride = 0;
    void* map_data = nullptr;
    void* mapped = gbm_bo_map(fbo.bo, 0, 0, W, H, GBM_BO_TRANSFER_READ, &map_stride, &map_data);
    VN_CHECK(mapped, "gbm_bo_map");

    // Source had a 200x200 red square at column sweep=10*4=40, row (H-200)/2.
    // Sample one pixel inside that square at (~140, H/2) to confirm it's reddish.
    const std::span<const uint8_t> pixels(static_cast<const uint8_t*>(mapped),
                                          static_cast<size_t>(map_stride) * H);
    const size_t sample_off =
        static_cast<size_t>(H / 2) * map_stride + static_cast<size_t>(140) * 4;
    const std::span<const uint8_t> spx = pixels.subspan(sample_off, 4);
    uint8_t b = spx[0], g = spx[1], r = spx[2], a = spx[3];
    printf("ok: sampled pixel(140,%d) BGRA=(%u,%u,%u,%u)\n", H / 2, b, g, r, a);

    bool ppm_ok = dump_ppm(mapped, map_stride, W, H);
    gbm_bo_unmap(fbo.bo, map_data);

    if (!(r >= 150 && g <= 100 && b <= 100)) {
        fprintf(stderr, "FAIL: pixel did not look red\n");
        return 1;
    }

    eglDestroyImage(ctx.display(), src_img);
    eglDestroyImage(ctx.display(), fbo.img);
    gbm_bo_destroy(fbo.bo);

    printf("PASS: NV12 dmabuf -> samplerExternalOES -> RGBA dmabuf round-trip works%s\n",
           ppm_ok ? " (PPM dumped to /tmp/import-probe.ppm)" : "");
    return 0;
}
