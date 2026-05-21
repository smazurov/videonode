// csc-probe — Phase 0 spike: GLES2 two-pass NV24 → NV12 color-space
// conversion validator. Runs on any Linux box with Mesa + libgbm + a DRM
// render node (no Rockchip / dma_heap dependency). Validates the path the
// production csc_gles backend will use.
//
// Pipeline:
//   1. Open EGL/GBM/GLES2 on /dev/dri/renderD128 (Fedora) or /dev/dri/renderD130 (rig).
//   2. GBM-allocate a "source NV24" buffer of size W * 3H bytes (R8 of
//      width W, height 3H): layout = Y plane [0..H), then UV interleaved
//      at full resolution [H..3H) — 2 bytes per UV sample on each row.
//   3. GBM-allocate a "destination NV12" buffer of size W * 1.5H bytes
//      (R8 of width W, height 3H/2): layout = Y plane [0..H), then UV
//      interleaved at half resolution [H..1.5H).
//   4. mmap the source, fill with a known ramp:
//        Y[x,y]  = (x + y) & 0xFF       // diagonal gradient
//        U[x,y]  = (x ^ y) & 0xFF       // checker pattern
//        V[x,y]  = (x * 7 + y * 11) & 0xFF
//   5. Import source as 3 EGLImages (Y as R8, U as R8 at offset H, V skipped
//      because we read UV interleaved as a single GR88 plane).
//      Actually: import Y as R8 (W×H), UV as GR88 (W×H, full 4:4:4 res).
//   6. Import destination as 2 EGLImages: Y as R8 (W×H, offset 0),
//      UV as GR88 (W/2 × H/2, offset W*H, pitch W).
//   7. Pass 1: bind dst-Y FBO, viewport WxH, shader: gl_FragColor.r = src_Y.r
//   8. Pass 2: bind dst-UV FBO, viewport W/2 x H/2, shader: sample 2x2
//      avg of src_UV at the corresponding source coords, write (U, V) into
//      gl_FragColor.rg.
//   9. glFinish, mmap destination, verify byte-exact pixels.
//
// Expected output: PASS with timings; failure with first mismatched pixel.
//
// Usage:
//   ./csc-probe [device=/dev/dri/renderD128] [W=640] [H=480]

#include "../src/egl_ctx.hpp"

#include <EGL/egl.h>
#include <GLES2/gl2.h>
#include <GLES2/gl2ext.h>
#include <drm_fourcc.h>
#include <gbm.h>

#include <chrono>
#include <cstdint>
#include <cstdio>
#include <cstdlib>
#include <cstring>
#include <unistd.h>

#define DIE(...)                                                                                   \
    do {                                                                                           \
        std::fprintf(stderr, "csc-probe: " __VA_ARGS__);                                           \
        std::fprintf(stderr, "\n");                                                                \
        return 1;                                                                                  \
    } while (0)

namespace {

// One-plane GBM buffer used as the underlying storage for either NV24
// source or NV12 destination. We allocate it as R8 with custom height
// so each "row" is W bytes, and stack planes vertically.
struct R8Bo {
    gbm_bo* bo = nullptr;
    int fd = -1;
    uint32_t stride = 0;
    int w = 0;
    int h = 0;
    void* map_handle = nullptr;
    void* mapped = nullptr;
};

bool r8_alloc(gbm_device* gbm, R8Bo& out, int w, int h) {
    out.bo = gbm_bo_create(gbm, w, h, DRM_FORMAT_R8,
                           GBM_BO_USE_LINEAR | GBM_BO_USE_RENDERING);
    if (!out.bo)
        return false;
    out.fd = gbm_bo_get_fd(out.bo);
    out.stride = gbm_bo_get_stride(out.bo);
    out.w = w;
    out.h = h;
    return true;
}

void r8_free(R8Bo& b) {
    if (b.mapped)
        gbm_bo_unmap(b.bo, b.map_handle);
    if (b.fd >= 0)
        ::close(b.fd);
    if (b.bo)
        gbm_bo_destroy(b.bo);
    b = R8Bo{};
}

uint8_t* r8_map_write(R8Bo& b) {
    uint32_t s = 0;
    b.mapped = gbm_bo_map(b.bo, 0, 0, b.w, b.h,
                          GBM_BO_TRANSFER_READ_WRITE, &s, &b.map_handle);
    b.stride = s;
    return static_cast<uint8_t*>(b.mapped);
}

uint8_t* r8_map_read(R8Bo& b) {
    uint32_t s = 0;
    b.mapped = gbm_bo_map(b.bo, 0, 0, b.w, b.h,
                          GBM_BO_TRANSFER_READ, &s, &b.map_handle);
    b.stride = s;
    return static_cast<uint8_t*>(b.mapped);
}

void r8_unmap(R8Bo& b) {
    if (b.mapped) {
        gbm_bo_unmap(b.bo, b.map_handle);
        b.mapped = nullptr;
        b.map_handle = nullptr;
    }
}

GLuint compile_shader(GLenum type, const char* src) {
    GLuint s = glCreateShader(type);
    glShaderSource(s, 1, &src, nullptr);
    glCompileShader(s);
    GLint ok = 0;
    glGetShaderiv(s, GL_COMPILE_STATUS, &ok);
    if (!ok) {
        char log[2048];
        GLsizei len = 0;
        glGetShaderInfoLog(s, sizeof(log), &len, log);
        std::fprintf(stderr, "csc-probe: shader compile failed:\n%.*s\n", len, log);
        glDeleteShader(s);
        return 0;
    }
    return s;
}

GLuint link_program(const char* vs_src, const char* fs_src) {
    GLuint vs = compile_shader(GL_VERTEX_SHADER, vs_src);
    GLuint fs = compile_shader(GL_FRAGMENT_SHADER, fs_src);
    if (!vs || !fs)
        return 0;
    GLuint p = glCreateProgram();
    glAttachShader(p, vs);
    glAttachShader(p, fs);
    glBindAttribLocation(p, 0, "a_pos");
    glLinkProgram(p);
    glDeleteShader(vs);
    glDeleteShader(fs);
    GLint ok = 0;
    glGetProgramiv(p, GL_LINK_STATUS, &ok);
    if (!ok) {
        char log[2048];
        GLsizei len = 0;
        glGetProgramInfoLog(p, sizeof(log), &len, log);
        std::fprintf(stderr, "csc-probe: program link failed:\n%.*s\n", len, log);
        glDeleteProgram(p);
        return 0;
    }
    return p;
}

// Full-screen quad covering the whole viewport in clip space.
const float kQuadVerts[] = {
    -1.f, -1.f, 1.f, -1.f, -1.f, 1.f, 1.f, 1.f,
};

// Vertex shader: pass-through clip coords + derive 0..1 UV.
const char* kVS = R"(
attribute vec2 a_pos;
varying vec2 v_uv;
void main() {
    v_uv = a_pos * 0.5 + 0.5;
    gl_Position = vec4(a_pos, 0.0, 1.0);
}
)";

// Pass 1: copy Y plane verbatim. Sample R8 source at v_uv, write to R8 dst.
const char* kFS_Y = R"(
precision mediump float;
varying vec2 v_uv;
uniform sampler2D u_src_y;
void main() {
    gl_FragColor = vec4(texture2D(u_src_y, v_uv).r, 0.0, 0.0, 1.0);
}
)";

// Pass 2: 4:4:4 → 4:2:0 chroma downsample. Source UV is at full resolution
// (NV24); destination UV is at half resolution. At each dst UV pixel we
// average a 2×2 block of src UV samples.
const char* kFS_UV = R"(
precision mediump float;
varying vec2 v_uv;
uniform sampler2D u_src_uv; // GR88: R=U, G=V; sampled in [0,1] x [0,1]
uniform vec2 u_src_uv_texel; // (1/W, 1/H) of source UV plane
void main() {
    // Sample 2×2 block centered around v_uv. Source UV is 2x wider/taller
    // than the dst UV viewport, so we offset by ±0.5 texel from the v_uv
    // center (which lands on a source-pixel boundary).
    vec2 off = u_src_uv_texel * 0.5;
    vec4 a = texture2D(u_src_uv, v_uv + vec2(-off.x, -off.y));
    vec4 b = texture2D(u_src_uv, v_uv + vec2( off.x, -off.y));
    vec4 c = texture2D(u_src_uv, v_uv + vec2(-off.x,  off.y));
    vec4 d = texture2D(u_src_uv, v_uv + vec2( off.x,  off.y));
    gl_FragColor = vec4((a.r + b.r + c.r + d.r) * 0.25,
                        (a.g + b.g + c.g + d.g) * 0.25,
                        0.0, 1.0);
}
)";

} // namespace

int main(int argc, char** argv) {
    const char* device = (argc > 1) ? argv[1] : "/dev/dri/renderD128";
    int W = (argc > 2) ? std::atoi(argv[2]) : 640;
    int H = (argc > 3) ? std::atoi(argv[3]) : 480;
    if (W <= 0 || H <= 0 || (W % 2) || (H % 2))
        DIE("W and H must be positive even ints");

    egl_ctx::EglCtx ctx;
    if (!ctx.init(device))
        DIE("EglCtx::init(%s)", device);
    std::printf("ok: EGL on %s (%s)\n", device, glGetString(GL_RENDERER));

    // Source NV24: Y plane (R8, W×H), UV plane (GR88 at full resolution
    // logically; allocated as R8 of width 2W to carry 2 bytes per sample).
    R8Bo src_y, src_uv;
    if (!r8_alloc(ctx.gbm(), src_y, W, H))
        DIE("alloc src_y");
    if (!r8_alloc(ctx.gbm(), src_uv, W * 2, H))
        DIE("alloc src_uv");

    // Destination NV12: Y plane (R8, W×H), UV plane (GR88 at half
    // resolution; allocated as R8 of width W).
    R8Bo dst_y, dst_uv;
    if (!r8_alloc(ctx.gbm(), dst_y, W, H))
        DIE("alloc dst_y");
    if (!r8_alloc(ctx.gbm(), dst_uv, W, H / 2))
        DIE("alloc dst_uv");

    std::printf("ok: allocated 4 R8 GBM buffers (src_y=%d×%d, src_uv=%d×%d, dst_y=%d×%d, "
                "dst_uv=%d×%d)\n",
                src_y.w, src_y.h, src_uv.w, src_uv.h, dst_y.w, dst_y.h, dst_uv.w, dst_uv.h);

    // Fill source with known ramp.
    uint8_t* sy = r8_map_write(src_y);
    if (!sy)
        DIE("map src_y");
    for (int y = 0; y < H; ++y)
        for (int x = 0; x < W; ++x)
            sy[y * src_y.stride + x] = uint8_t((x + y) & 0xFF);
    r8_unmap(src_y);

    uint8_t* suv = r8_map_write(src_uv);
    if (!suv)
        DIE("map src_uv");
    for (int y = 0; y < H; ++y) {
        for (int x = 0; x < W; ++x) {
            suv[y * src_uv.stride + 2 * x + 0] = uint8_t((x ^ y) & 0xFF);            // U
            suv[y * src_uv.stride + 2 * x + 1] = uint8_t((x * 7 + y * 11) & 0xFF); // V
        }
    }
    r8_unmap(src_uv);

    // Import all four as EGLImages.
    auto import_r8 = [&](const R8Bo& b, uint32_t fourcc) -> EGLImage {
        egl_ctx::EglCtx::ImageDesc d;
        d.fd = b.fd;
        d.fourcc = fourcc;
        d.modifier = gbm_bo_get_modifier(b.bo);
        d.width = b.w;
        d.height = b.h;
        d.plane0_offset = 0;
        d.plane0_pitch = b.stride;
        return ctx.import_dmabuf(d);
    };

    EGLImage img_src_y = import_r8(src_y, DRM_FORMAT_R8);
    if (img_src_y == EGL_NO_IMAGE)
        DIE("import src_y as R8");
    // src_uv is allocated as R8 width=2W; reinterpret as GR88 width=W.
    egl_ctx::EglCtx::ImageDesc d_src_uv;
    d_src_uv.fd = src_uv.fd;
    d_src_uv.fourcc = DRM_FORMAT_GR88;
    d_src_uv.modifier = gbm_bo_get_modifier(src_uv.bo);
    d_src_uv.width = W;
    d_src_uv.height = H;
    d_src_uv.plane0_offset = 0;
    d_src_uv.plane0_pitch = src_uv.stride;
    EGLImage img_src_uv = ctx.import_dmabuf(d_src_uv);
    if (img_src_uv == EGL_NO_IMAGE)
        DIE("import src_uv as GR88");

    EGLImage img_dst_y = import_r8(dst_y, DRM_FORMAT_R8);
    if (img_dst_y == EGL_NO_IMAGE)
        DIE("import dst_y as R8");
    // dst_uv allocated as R8 width=W; reinterpret as GR88 width=W/2.
    egl_ctx::EglCtx::ImageDesc d_dst_uv;
    d_dst_uv.fd = dst_uv.fd;
    d_dst_uv.fourcc = DRM_FORMAT_GR88;
    d_dst_uv.modifier = gbm_bo_get_modifier(dst_uv.bo);
    d_dst_uv.width = W / 2;
    d_dst_uv.height = H / 2;
    d_dst_uv.plane0_offset = 0;
    d_dst_uv.plane0_pitch = dst_uv.stride;
    EGLImage img_dst_uv = ctx.import_dmabuf(d_dst_uv);
    if (img_dst_uv == EGL_NO_IMAGE)
        DIE("import dst_uv as GR88");

    std::printf("ok: 4 EGLImages imported  [modifiers: src_y=0x%016llx src_uv=0x%016llx "
                "dst_y=0x%016llx dst_uv=0x%016llx]\n",
                (unsigned long long)gbm_bo_get_modifier(src_y.bo),
                (unsigned long long)gbm_bo_get_modifier(src_uv.bo),
                (unsigned long long)gbm_bo_get_modifier(dst_y.bo),
                (unsigned long long)gbm_bo_get_modifier(dst_uv.bo));

    // Textures for source planes.
    GLuint tex_src_y = 0, tex_src_uv = 0;
    glGenTextures(1, &tex_src_y);
    glBindTexture(GL_TEXTURE_2D, tex_src_y);
    glTexParameteri(GL_TEXTURE_2D, GL_TEXTURE_MIN_FILTER, GL_NEAREST);
    glTexParameteri(GL_TEXTURE_2D, GL_TEXTURE_MAG_FILTER, GL_NEAREST);
    glTexParameteri(GL_TEXTURE_2D, GL_TEXTURE_WRAP_S, GL_CLAMP_TO_EDGE);
    glTexParameteri(GL_TEXTURE_2D, GL_TEXTURE_WRAP_T, GL_CLAMP_TO_EDGE);
    auto eglImageTargetTexture2DOES =
        (PFNGLEGLIMAGETARGETTEXTURE2DOESPROC)eglGetProcAddress("glEGLImageTargetTexture2DOES");
    if (!eglImageTargetTexture2DOES)
        DIE("glEGLImageTargetTexture2DOES not found");
    eglImageTargetTexture2DOES(GL_TEXTURE_2D, img_src_y);

    glGenTextures(1, &tex_src_uv);
    glBindTexture(GL_TEXTURE_2D, tex_src_uv);
    glTexParameteri(GL_TEXTURE_2D, GL_TEXTURE_MIN_FILTER, GL_NEAREST);
    glTexParameteri(GL_TEXTURE_2D, GL_TEXTURE_MAG_FILTER, GL_NEAREST);
    glTexParameteri(GL_TEXTURE_2D, GL_TEXTURE_WRAP_S, GL_CLAMP_TO_EDGE);
    glTexParameteri(GL_TEXTURE_2D, GL_TEXTURE_WRAP_T, GL_CLAMP_TO_EDGE);
    eglImageTargetTexture2DOES(GL_TEXTURE_2D, img_src_uv);

    // Renderbuffers for dst planes (FBO color attachments).
    auto eglImageTargetRenderbufferStorageOES =
        (PFNGLEGLIMAGETARGETRENDERBUFFERSTORAGEOESPROC)eglGetProcAddress(
            "glEGLImageTargetRenderbufferStorageOES");
    if (!eglImageTargetRenderbufferStorageOES)
        DIE("glEGLImageTargetRenderbufferStorageOES not found");

    GLuint rb_dst_y = 0, rb_dst_uv = 0;
    GLuint fbo_y = 0, fbo_uv = 0;
    glGenRenderbuffers(1, &rb_dst_y);
    glBindRenderbuffer(GL_RENDERBUFFER, rb_dst_y);
    eglImageTargetRenderbufferStorageOES(GL_RENDERBUFFER, img_dst_y);
    glGenFramebuffers(1, &fbo_y);
    glBindFramebuffer(GL_FRAMEBUFFER, fbo_y);
    glFramebufferRenderbuffer(GL_FRAMEBUFFER, GL_COLOR_ATTACHMENT0, GL_RENDERBUFFER, rb_dst_y);
    GLenum st = glCheckFramebufferStatus(GL_FRAMEBUFFER);
    if (st != GL_FRAMEBUFFER_COMPLETE)
        DIE("fbo_y status=0x%x", st);

    glGenRenderbuffers(1, &rb_dst_uv);
    glBindRenderbuffer(GL_RENDERBUFFER, rb_dst_uv);
    eglImageTargetRenderbufferStorageOES(GL_RENDERBUFFER, img_dst_uv);
    glGenFramebuffers(1, &fbo_uv);
    glBindFramebuffer(GL_FRAMEBUFFER, fbo_uv);
    glFramebufferRenderbuffer(GL_FRAMEBUFFER, GL_COLOR_ATTACHMENT0, GL_RENDERBUFFER, rb_dst_uv);
    st = glCheckFramebufferStatus(GL_FRAMEBUFFER);
    if (st != GL_FRAMEBUFFER_COMPLETE)
        DIE("fbo_uv status=0x%x", st);

    GLuint prog_y = link_program(kVS, kFS_Y);
    GLuint prog_uv = link_program(kVS, kFS_UV);
    if (!prog_y || !prog_uv)
        DIE("program link");

    GLuint vbo = 0;
    glGenBuffers(1, &vbo);
    glBindBuffer(GL_ARRAY_BUFFER, vbo);
    glBufferData(GL_ARRAY_BUFFER, sizeof(kQuadVerts), kQuadVerts, GL_STATIC_DRAW);
    glEnableVertexAttribArray(0);
    glVertexAttribPointer(0, 2, GL_FLOAT, GL_FALSE, 2 * sizeof(float), nullptr);

    auto t0 = std::chrono::steady_clock::now();

    // Pass 1: Y plane.
    glBindFramebuffer(GL_FRAMEBUFFER, fbo_y);
    glViewport(0, 0, W, H);
    glUseProgram(prog_y);
    glActiveTexture(GL_TEXTURE0);
    glBindTexture(GL_TEXTURE_2D, tex_src_y);
    glUniform1i(glGetUniformLocation(prog_y, "u_src_y"), 0);
    glDrawArrays(GL_TRIANGLE_STRIP, 0, 4);

    // Pass 2: UV plane, half-res, 2x2 downsample.
    glBindFramebuffer(GL_FRAMEBUFFER, fbo_uv);
    glViewport(0, 0, W / 2, H / 2);
    glUseProgram(prog_uv);
    glActiveTexture(GL_TEXTURE0);
    glBindTexture(GL_TEXTURE_2D, tex_src_uv);
    glUniform1i(glGetUniformLocation(prog_uv, "u_src_uv"), 0);
    glUniform2f(glGetUniformLocation(prog_uv, "u_src_uv_texel"), 1.0f / float(W), 1.0f / float(H));
    glDrawArrays(GL_TRIANGLE_STRIP, 0, 4);

    glFinish();
    auto t1 = std::chrono::steady_clock::now();
    double us = std::chrono::duration_cast<std::chrono::microseconds>(t1 - t0).count();
    std::printf("ok: GPU passes done in %.0f µs\n", us);

    // Verify.
    int errors_y = 0, errors_uv = 0;
    int first_err_y_x = -1, first_err_y_y = -1, first_err_y_got = -1, first_err_y_want = -1;
    int first_err_uv_x = -1, first_err_uv_y = -1, first_err_uv_g = -1, first_err_uv_w = -1;

    uint8_t* dy = r8_map_read(dst_y);
    if (!dy)
        DIE("map dst_y for read");
    for (int y = 0; y < H && errors_y < 4; ++y) {
        for (int x = 0; x < W && errors_y < 4; ++x) {
            uint8_t got = dy[y * dst_y.stride + x];
            uint8_t want = uint8_t((x + y) & 0xFF);
            if (got != want) {
                if (errors_y == 0) {
                    first_err_y_x = x;
                    first_err_y_y = y;
                    first_err_y_got = got;
                    first_err_y_want = want;
                }
                ++errors_y;
            }
        }
    }
    r8_unmap(dst_y);

    // For UV verification we need to re-read the source UV (mmap doesn't
    // survive across passes if we unmapped; just rebuild the expected
    // pattern from the same formula).
    uint8_t* duv = r8_map_read(dst_uv);
    if (!duv)
        DIE("map dst_uv for read");
    // Expected: at dst (x, y) we average the 2×2 block from source UV at
    // (2x, 2y), (2x+1, 2y), (2x, 2y+1), (2x+1, 2y+1).
    for (int y = 0; y < H / 2 && errors_uv < 4; ++y) {
        for (int x = 0; x < W / 2 && errors_uv < 4; ++x) {
            int sx = 2 * x, sy = 2 * y;
            int eu = (((sx ^ sy) & 0xFF) + (((sx + 1) ^ sy) & 0xFF) +
                      ((sx ^ (sy + 1)) & 0xFF) + (((sx + 1) ^ (sy + 1)) & 0xFF) + 2) /
                     4;
            int ev = ((sx * 7 + sy * 11) & 0xFF) + (((sx + 1) * 7 + sy * 11) & 0xFF) +
                     ((sx * 7 + (sy + 1) * 11) & 0xFF) +
                     (((sx + 1) * 7 + (sy + 1) * 11) & 0xFF);
            ev = (ev + 2) / 4;
            uint8_t got_u = duv[y * dst_uv.stride + 2 * x + 0];
            uint8_t got_v = duv[y * dst_uv.stride + 2 * x + 1];
            // Allow ±1 tolerance: shader uses mediump float, 1/255
            // quantization can give off-by-one.
            int du = int(got_u) - eu;
            int dv = int(got_v) - ev;
            if (du < -1 || du > 1 || dv < -1 || dv > 1) {
                if (errors_uv == 0) {
                    first_err_uv_x = x;
                    first_err_uv_y = y;
                    first_err_uv_g = got_u;
                    first_err_uv_w = eu;
                }
                ++errors_uv;
            }
        }
    }
    r8_unmap(dst_uv);

    if (errors_y == 0 && errors_uv == 0) {
        std::printf("PASS: Y plane byte-exact, UV plane within ±1 LSB tolerance (%dx%d in %.0f "
                    "µs)\n",
                    W, H, us);
    } else {
        if (errors_y > 0)
            std::fprintf(stderr,
                         "FAIL: Y plane: %d mismatches; first at (%d,%d) got=%d want=%d\n",
                         errors_y, first_err_y_x, first_err_y_y, first_err_y_got, first_err_y_want);
        if (errors_uv > 0)
            std::fprintf(stderr,
                         "FAIL: UV plane: %d mismatches; first at (%d,%d) got_u=%d want_u=%d\n",
                         errors_uv, first_err_uv_x, first_err_uv_y, first_err_uv_g, first_err_uv_w);
    }

    // Cleanup.
    eglDestroyImage(ctx.display(), img_src_y);
    eglDestroyImage(ctx.display(), img_src_uv);
    eglDestroyImage(ctx.display(), img_dst_y);
    eglDestroyImage(ctx.display(), img_dst_uv);
    r8_free(src_y);
    r8_free(src_uv);
    r8_free(dst_y);
    r8_free(dst_uv);
    return (errors_y || errors_uv) ? 1 : 0;
}
