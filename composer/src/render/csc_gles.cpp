#include "src/render/csc_gles.hpp"

#include "src/render/egl_ctx.hpp"

#include <EGL/egl.h>
#include <GLES2/gl2.h>
#include <GLES2/gl2ext.h>
#include <atomic>
#include <cstdio>
#include <cstring>
#include <drm_fourcc.h>

namespace csc_gles {

namespace {

// Persistent backend state — set up once on first init() and reused per
// frame. The EglCtx owns the GBM device, EGLDisplay, EGLContext; the
// programs / VBO / textures are GL-side state created after make_current.
struct State {
    egl_ctx::EglCtx ctx;
    bool ready = false;

    GLuint prog_y = 0;
    GLuint prog_uv = 0;             // NV24 → NV12: 2×2 chroma downsample
    GLuint prog_uv_passthrough = 0; // NV12 → NV12: single-tap UV copy
    GLuint vbo = 0;

    // Sampler textures (rebound per frame to whichever EGLImage we just
    // imported). Created once and reused.
    GLuint tex_src_y = 0;
    GLuint tex_src_uv = 0;

    PFNGLEGLIMAGETARGETTEXTURE2DOESPROC egl_image_to_tex2d = nullptr;
    PFNGLEGLIMAGETARGETRENDERBUFFERSTORAGEOESPROC egl_image_to_rbo = nullptr;
};

State& state() {
    static State s;
    return s;
}

// Shader sources mirror composer/tools/csc-probe.cpp. Keep these two
// strings in sync with the probe — the probe is the validator.
const float kQuadVerts[] = {
    -1.f, -1.f, 1.f, -1.f, -1.f, 1.f, 1.f, 1.f,
};

const char* kVS = R"(
attribute vec2 a_pos;
varying vec2 v_uv;
void main() {
    v_uv = a_pos * 0.5 + 0.5;
    gl_Position = vec4(a_pos, 0.0, 1.0);
}
)";

// Pass 1: pass-through Y plane (NV24's Y = NV12's Y).
const char* kFS_Y = R"(
precision mediump float;
varying vec2 v_uv;
uniform sampler2D u_src_y;
void main() {
    gl_FragColor = vec4(texture2D(u_src_y, v_uv).r, 0.0, 0.0, 1.0);
}
)";

// Pass 2: 4:4:4 → 4:2:0 chroma downsample via 2×2 average.
const char* kFS_UV = R"(
precision mediump float;
varying vec2 v_uv;
uniform sampler2D u_src_uv;
uniform vec2 u_src_uv_texel;
void main() {
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

// Pass 2 alt: 4:2:0 → 4:2:0 single-tap UV copy. Used when the source is
// already NV12, i.e. src UV is sampled at the same half-resolution that
// the destination UV plane uses, so a one-tap fetch is bit-exact.
const char* kFS_UV_PASSTHROUGH = R"(
precision mediump float;
varying vec2 v_uv;
uniform sampler2D u_src_uv;
void main() {
    vec4 s = texture2D(u_src_uv, v_uv);
    gl_FragColor = vec4(s.r, s.g, 0.0, 1.0);
}
)";

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
        std::fprintf(stderr, "csc_gles: shader compile failed:\n%.*s\n", len, log);
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
        std::fprintf(stderr, "csc_gles: program link failed:\n%.*s\n", len, log);
        glDeleteProgram(p);
        return 0;
    }
    return p;
}

void log_once(const char* msg) {
    static std::atomic<bool> warned{false};
    if (!warned.exchange(true))
        std::fprintf(stderr, "csc_gles: %s\n", msg);
}

} // namespace

bool init() {
    State& s = state();
    if (s.ready)
        return true;
    // TODO: make the EGL render node configurable via env / CLI. For
    // now /dev/dri/renderD128 (typical Fedora) with renderD129/130 as
    // common alternatives — try each.
    const char* candidates[] = {
        "/dev/dri/renderD128",
        "/dev/dri/renderD129",
        "/dev/dri/renderD130",
    };
    bool opened = false;
    for (const char* d : candidates) {
        if (s.ctx.init(d)) {
            opened = true;
            std::fprintf(stderr, "csc_gles: EGL on %s\n", d);
            break;
        }
    }
    if (!opened) {
        std::fprintf(stderr, "csc_gles: no DRM render node found\n");
        return false;
    }

    s.egl_image_to_tex2d =
        (PFNGLEGLIMAGETARGETTEXTURE2DOESPROC)eglGetProcAddress("glEGLImageTargetTexture2DOES");
    s.egl_image_to_rbo = (PFNGLEGLIMAGETARGETRENDERBUFFERSTORAGEOESPROC)eglGetProcAddress(
        "glEGLImageTargetRenderbufferStorageOES");
    if (!s.egl_image_to_tex2d || !s.egl_image_to_rbo) {
        std::fprintf(stderr, "csc_gles: required EGLImage GL ext entrypoints missing\n");
        return false;
    }

    s.prog_y = link_program(kVS, kFS_Y);
    s.prog_uv = link_program(kVS, kFS_UV);
    s.prog_uv_passthrough = link_program(kVS, kFS_UV_PASSTHROUGH);
    if (!s.prog_y || !s.prog_uv || !s.prog_uv_passthrough)
        return false;

    glGenBuffers(1, &s.vbo);
    glBindBuffer(GL_ARRAY_BUFFER, s.vbo);
    glBufferData(GL_ARRAY_BUFFER, sizeof(kQuadVerts), kQuadVerts, GL_STATIC_DRAW);

    glGenTextures(1, &s.tex_src_y);
    glBindTexture(GL_TEXTURE_2D, s.tex_src_y);
    glTexParameteri(GL_TEXTURE_2D, GL_TEXTURE_MIN_FILTER, GL_NEAREST);
    glTexParameteri(GL_TEXTURE_2D, GL_TEXTURE_MAG_FILTER, GL_NEAREST);
    glTexParameteri(GL_TEXTURE_2D, GL_TEXTURE_WRAP_S, GL_CLAMP_TO_EDGE);
    glTexParameteri(GL_TEXTURE_2D, GL_TEXTURE_WRAP_T, GL_CLAMP_TO_EDGE);

    glGenTextures(1, &s.tex_src_uv);
    glBindTexture(GL_TEXTURE_2D, s.tex_src_uv);
    glTexParameteri(GL_TEXTURE_2D, GL_TEXTURE_MIN_FILTER, GL_NEAREST);
    glTexParameteri(GL_TEXTURE_2D, GL_TEXTURE_MAG_FILTER, GL_NEAREST);
    glTexParameteri(GL_TEXTURE_2D, GL_TEXTURE_WRAP_S, GL_CLAMP_TO_EDGE);
    glTexParameteri(GL_TEXTURE_2D, GL_TEXTURE_WRAP_T, GL_CLAMP_TO_EDGE);

    s.ready = true;
    return true;
}

gbm_device* gbm_device_for_io() {
    State& s = state();
    return s.ready ? s.ctx.gbm() : nullptr;
}

void shutdown() {
    State& s = state();
    if (!s.ready)
        return;
    if (s.prog_y)
        glDeleteProgram(s.prog_y);
    if (s.prog_uv)
        glDeleteProgram(s.prog_uv);
    if (s.prog_uv_passthrough)
        glDeleteProgram(s.prog_uv_passthrough);
    if (s.vbo)
        glDeleteBuffers(1, &s.vbo);
    if (s.tex_src_y)
        glDeleteTextures(1, &s.tex_src_y);
    if (s.tex_src_uv)
        glDeleteTextures(1, &s.tex_src_uv);
    s.prog_y = s.prog_uv = s.prog_uv_passthrough = s.vbo = s.tex_src_y = s.tex_src_uv = 0;
    s.ready = false;
    // s.ctx destructor runs on process exit; we leave it owned by the
    // singleton so re-init() picks up the same context.
}

bool convert(const csc::ConvertParams& src, const csc::ConvertParams& dst) {
    State& s = state();
    if (!s.ready && !init())
        return false;

    if (dst.fmt != csc::PixelFormat::Nv12) {
        log_once("dst.fmt != Nv12 — only NV12 output is supported");
        return false;
    }
    if (src.fmt != csc::PixelFormat::Nv24 && src.fmt != csc::PixelFormat::Nv12) {
        // TODO: add NV16 / YUYV / UYVY / BGR3 shaders. See GitHub issue #6.
        log_once("only NV12/NV24 input is implemented; other formats are TODO");
        return false;
    }
    if (src.width <= 0 || src.height <= 0 || (src.width & 1) || (src.height & 1))
        return false;
    if (dst.width != src.width || dst.height != src.height)
        return false;

    const int W = src.width;
    const int H = src.height;
    const bool src_is_nv12 = (src.fmt == csc::PixelFormat::Nv12);
    const int src_y_pitch = (src.wstride > 0 ? src.wstride : W);
    // NV24 UV row = W × 2 bytes (full-res interleaved). NV12 UV row = W
    // bytes (W/2 interleaved UV pairs at 2 bytes each = W); both reduce
    // to "Y stride × bpp/2", which for the formats we care about is the
    // Y stride itself for NV12 and 2× for NV24.
    const int src_uv_pitch = src_is_nv12 ? src_y_pitch : src_y_pitch * 2;
    const int src_uv_w = src_is_nv12 ? (W / 2) : W;
    const int src_uv_h = src_is_nv12 ? (H / 2) : H;
    const int dst_y_pitch = (dst.wstride > 0 ? dst.wstride : W);
    // NV12 UV: W bytes per row (W/2 samples × 2 bytes); honour caller-
    // supplied uv_wstride when set (host GBM split allocator returns a
    // different stride for the half-res UV BO).
    const int dst_uv_pitch = (dst.uv_wstride > 0 ? dst.uv_wstride : dst_y_pitch);
    const int dst_y_size = dst_y_pitch * H;

    // Split-buffer routing: when uv_fd is set, the UV plane lives in its
    // own dma-buf at offset 0; otherwise UV trails Y in the same fd.
    const bool src_split = (src.uv_fd >= 0);
    const bool dst_split = (dst.uv_fd >= 0);
    const int src_uv_actual_fd = src_split ? src.uv_fd : src.fd;
    const int src_uv_actual_offset = src_split ? 0 : (src_y_pitch * H);
    const int src_uv_actual_pitch = (src.uv_wstride > 0 ? src.uv_wstride : src_uv_pitch);
    const int dst_uv_actual_fd = dst_split ? dst.uv_fd : dst.fd;
    const int dst_uv_actual_offset = dst_split ? 0 : dst_y_size;

    // Use the "let the driver pick" modifier sentinel for every plane.
    // Hardcoding DRM_FORMAT_MOD_LINEAR was rejected by Mesa/radeonsi for
    // GBM-allocated R8/GR88 BOs: gbm_bo_get_modifier() reports INVALID,
    // and explicit LINEAR fails the renderbuffer-storage import even
    // when the underlying layout is linear. egl_ctx::import_dmabuf
    // omits the modifier attrs when it sees this sentinel.
    constexpr uint64_t kModInvalid = (uint64_t{1} << 56) - 1;

    // Import source as two planes: Y (R8, W×H), UV (GR88).
    //   NV24: UV at full resolution W×H (4:4:4).
    //   NV12: UV at half resolution (W/2)×(H/2) (4:2:0).
    egl_ctx::EglCtx::ImageDesc sd_y;
    sd_y.fd = src.fd;
    sd_y.fourcc = DRM_FORMAT_R8;
    sd_y.modifier = kModInvalid;
    sd_y.width = W;
    sd_y.height = H;
    sd_y.plane0_offset = 0;
    sd_y.plane0_pitch = src_y_pitch;
    EGLImage img_src_y = s.ctx.import_dmabuf(sd_y);
    if (img_src_y == EGL_NO_IMAGE)
        return false;

    egl_ctx::EglCtx::ImageDesc sd_uv;
    sd_uv.fd = src_uv_actual_fd;
    sd_uv.fourcc = DRM_FORMAT_GR88;
    sd_uv.modifier = kModInvalid;
    sd_uv.width = src_uv_w;
    sd_uv.height = src_uv_h;
    sd_uv.plane0_offset = src_uv_actual_offset;
    sd_uv.plane0_pitch = src_uv_actual_pitch;
    EGLImage img_src_uv = s.ctx.import_dmabuf(sd_uv);
    if (img_src_uv == EGL_NO_IMAGE) {
        eglDestroyImage(s.ctx.display(), img_src_y);
        return false;
    }

    // Import destination as two planes from the same fd: Y (R8 at offset
    // 0), UV (GR88 at offset W*H, half-resolution).
    egl_ctx::EglCtx::ImageDesc dd_y;
    dd_y.fd = dst.fd;
    dd_y.fourcc = DRM_FORMAT_R8;
    dd_y.modifier = kModInvalid;
    dd_y.width = W;
    dd_y.height = H;
    dd_y.plane0_offset = 0;
    dd_y.plane0_pitch = dst_y_pitch;
    EGLImage img_dst_y = s.ctx.import_dmabuf(dd_y);
    if (img_dst_y == EGL_NO_IMAGE) {
        eglDestroyImage(s.ctx.display(), img_src_y);
        eglDestroyImage(s.ctx.display(), img_src_uv);
        return false;
    }

    egl_ctx::EglCtx::ImageDesc dd_uv;
    dd_uv.fd = dst_uv_actual_fd;
    dd_uv.fourcc = DRM_FORMAT_GR88;
    dd_uv.modifier = kModInvalid;
    dd_uv.width = W / 2;
    dd_uv.height = H / 2;
    dd_uv.plane0_offset = dst_uv_actual_offset;
    dd_uv.plane0_pitch = dst_uv_pitch;
    EGLImage img_dst_uv = s.ctx.import_dmabuf(dd_uv);
    if (img_dst_uv == EGL_NO_IMAGE) {
        eglDestroyImage(s.ctx.display(), img_src_y);
        eglDestroyImage(s.ctx.display(), img_src_uv);
        eglDestroyImage(s.ctx.display(), img_dst_y);
        return false;
    }

    // Bind sources to persistent texture handles.
    glActiveTexture(GL_TEXTURE0);
    glBindTexture(GL_TEXTURE_2D, s.tex_src_y);
    s.egl_image_to_tex2d(GL_TEXTURE_2D, img_src_y);
    glBindTexture(GL_TEXTURE_2D, s.tex_src_uv);
    s.egl_image_to_tex2d(GL_TEXTURE_2D, img_src_uv);

    // Renderbuffers per-frame (cheap; EGLImages are different each call).
    GLuint rb_y = 0, rb_uv = 0;
    GLuint fbo_y = 0, fbo_uv = 0;
    glGenRenderbuffers(1, &rb_y);
    glBindRenderbuffer(GL_RENDERBUFFER, rb_y);
    s.egl_image_to_rbo(GL_RENDERBUFFER, img_dst_y);
    glGenFramebuffers(1, &fbo_y);
    glBindFramebuffer(GL_FRAMEBUFFER, fbo_y);
    glFramebufferRenderbuffer(GL_FRAMEBUFFER, GL_COLOR_ATTACHMENT0, GL_RENDERBUFFER, rb_y);
    if (glCheckFramebufferStatus(GL_FRAMEBUFFER) != GL_FRAMEBUFFER_COMPLETE) {
        log_once("fbo_y incomplete");
        goto cleanup;
    }

    glGenRenderbuffers(1, &rb_uv);
    glBindRenderbuffer(GL_RENDERBUFFER, rb_uv);
    s.egl_image_to_rbo(GL_RENDERBUFFER, img_dst_uv);
    glGenFramebuffers(1, &fbo_uv);
    glBindFramebuffer(GL_FRAMEBUFFER, fbo_uv);
    glFramebufferRenderbuffer(GL_FRAMEBUFFER, GL_COLOR_ATTACHMENT0, GL_RENDERBUFFER, rb_uv);
    if (glCheckFramebufferStatus(GL_FRAMEBUFFER) != GL_FRAMEBUFFER_COMPLETE) {
        log_once("fbo_uv incomplete");
        goto cleanup;
    }

    // Bind quad VBO + attrib pointer.
    glBindBuffer(GL_ARRAY_BUFFER, s.vbo);
    glEnableVertexAttribArray(0);
    glVertexAttribPointer(0, 2, GL_FLOAT, GL_FALSE, 2 * sizeof(float), nullptr);

    // Pass 1: Y plane, full resolution.
    glBindFramebuffer(GL_FRAMEBUFFER, fbo_y);
    glViewport(0, 0, W, H);
    glUseProgram(s.prog_y);
    glActiveTexture(GL_TEXTURE0);
    glBindTexture(GL_TEXTURE_2D, s.tex_src_y);
    glUniform1i(glGetUniformLocation(s.prog_y, "u_src_y"), 0);
    glDrawArrays(GL_TRIANGLE_STRIP, 0, 4);

    // Pass 2: UV plane, half resolution.
    //   NV24 src → 2×2 average downsample (prog_uv).
    //   NV12 src → single-tap copy (prog_uv_passthrough); src UV is
    //   already at the same W/2 × H/2 the dst UV viewport draws into.
    {
        glBindFramebuffer(GL_FRAMEBUFFER, fbo_uv);
        glViewport(0, 0, W / 2, H / 2);
        const GLuint prog_uv = src_is_nv12 ? s.prog_uv_passthrough : s.prog_uv;
        glUseProgram(prog_uv);
        glActiveTexture(GL_TEXTURE0);
        glBindTexture(GL_TEXTURE_2D, s.tex_src_uv);
        glUniform1i(glGetUniformLocation(prog_uv, "u_src_uv"), 0);
        if (!src_is_nv12) {
            glUniform2f(glGetUniformLocation(prog_uv, "u_src_uv_texel"), 1.0f / float(W),
                        1.0f / float(H));
        }
        glDrawArrays(GL_TRIANGLE_STRIP, 0, 4);
    }

    glFinish();

cleanup:
    if (fbo_y)
        glDeleteFramebuffers(1, &fbo_y);
    if (fbo_uv)
        glDeleteFramebuffers(1, &fbo_uv);
    if (rb_y)
        glDeleteRenderbuffers(1, &rb_y);
    if (rb_uv)
        glDeleteRenderbuffers(1, &rb_uv);
    eglDestroyImage(s.ctx.display(), img_src_y);
    eglDestroyImage(s.ctx.display(), img_src_uv);
    eglDestroyImage(s.ctx.display(), img_dst_y);
    eglDestroyImage(s.ctx.display(), img_dst_uv);
    return true;
}

} // namespace csc_gles
