#include "src/render/gl_compose.hpp"

#include "src/common/log_levels.hpp"

#include <EGL/eglext.h>
#include <GLES2/gl2ext.h>
#include <drm_fourcc.h>
#include <gbm.h>
#include <unistd.h>

#include <cstring>
#include <string>

namespace gl_compose {

namespace {

// Embedded shader source — slice 4 is happier as a single binary, no file IO.
// Kept in sync with shaders/quad.vert and shaders/quad.frag for visibility
// and future external loading.
const char* kVS = R"(
attribute vec2 a_pos;
attribute vec2 a_uv;
uniform vec2  u_canvas_size;
uniform mat3  u_warp;
varying vec2 v_uv;
void main() {
    // a_pos is canvas-pixel (origin top-left). LINEAR dma-buf has pixel (0,0)
    // at memory offset 0; Mesa renders such that GL's bottom-left = memory
    // offset 0 = consumer-side image top-left. So canvas y=0 -> NDC y=-1
    // (GL bottom). We do NOT apply an extra y flip here.
    vec2 ndc = (a_pos / u_canvas_size) * 2.0 - 1.0;
    gl_Position = vec4(ndc, 0.0, 1.0);
    vec3 w = u_warp * vec3(a_uv, 1.0);
    v_uv = w.xy / w.z;
}
)";

// Two single-plane sampler2D + manual BT.601 limited YUV→RGB. Each plane
// is its own dma-buf at PLANE0_OFFSET=0 (canonical AMD/minigbm pattern).
// samplerExternalOES isn't used because radeonsi returns zero for NV12
// dma-buf imports through it. csc-probe validated this exact shader math
// (Y byte-exact, UV ±1 LSB) on this GPU.
const char* kFS = R"(
precision mediump float;
uniform sampler2D u_src_y;   // R8: Y in .r
uniform sampler2D u_src_uv;  // GR88, half-res: Cb in .r, Cr in .g
varying vec2 v_uv;
void main() {
    if (v_uv.x < 0.0 || v_uv.x > 1.0 || v_uv.y < 0.0 || v_uv.y > 1.0) {
        gl_FragColor = vec4(0.0, 0.0, 0.0, 1.0);
        return;
    }
    float Y = texture2D(u_src_y,  v_uv).r;
    vec2  C = texture2D(u_src_uv, v_uv).rg;
    float y = 1.164383 * (Y - 0.0627451);
    float u = C.r - 0.501961;
    float v = C.g - 0.501961;
    float r = y                + 1.596027 * v;
    float g = y - 0.391762 * u - 0.812968 * v;
    float b = y + 2.017232 * u;
    gl_FragColor = vec4(clamp(r, 0.0, 1.0),
                        clamp(g, 0.0, 1.0),
                        clamp(b, 0.0, 1.0),
                        1.0);
}
)";

PFNGLEGLIMAGETARGETTEXTURE2DOESPROC pfn_image_to_tex = nullptr;
PFNGLEGLIMAGETARGETRENDERBUFFERSTORAGEOESPROC pfn_image_to_rb = nullptr;

GLuint compile_(GLenum type, std::string_view src) {
    GLuint s = glCreateShader(type);
    const char* p = src.data();
    GLint len = static_cast<GLint>(src.size());
    glShaderSource(s, 1, &p, &len);
    glCompileShader(s);
    GLint ok = 0;
    glGetShaderiv(s, GL_COMPILE_STATUS, &ok);
    if (!ok) {
        char log[1024];
        glGetShaderInfoLog(s, sizeof(log), nullptr, log);
        vn::log::error("gl_compose: shader compile fail: %s", log);
        glDeleteShader(s);
        return 0;
    }
    return s;
}

} // namespace

bool GlCompose::build_program_(std::string_view vs_src, std::string_view fs_src) {
    GLuint vs = compile_(GL_VERTEX_SHADER, vs_src);
    GLuint fs = compile_(GL_FRAGMENT_SHADER, fs_src);
    if (!vs || !fs)
        return false;

    prog_ = glCreateProgram();
    glAttachShader(prog_, vs);
    glAttachShader(prog_, fs);
    glLinkProgram(prog_);
    glDeleteShader(vs);
    glDeleteShader(fs);

    GLint linked = 0;
    glGetProgramiv(prog_, GL_LINK_STATUS, &linked);
    if (!linked) {
        char log[1024];
        glGetProgramInfoLog(prog_, sizeof(log), nullptr, log);
        vn::log::error("gl_compose: program link fail: %s", log);
        return false;
    }

    attr_pos_ = glGetAttribLocation(prog_, "a_pos");
    attr_uv_ = glGetAttribLocation(prog_, "a_uv");
    loc_canvas_size_ = glGetUniformLocation(prog_, "u_canvas_size");
    loc_warp_ = glGetUniformLocation(prog_, "u_warp");
    loc_src_y_ = glGetUniformLocation(prog_, "u_src_y");
    loc_src_uv_ = glGetUniformLocation(prog_, "u_src_uv");
    if (attr_pos_ < 0 || attr_uv_ < 0 || loc_canvas_size_ < 0 || loc_warp_ < 0 || loc_src_y_ < 0 ||
        loc_src_uv_ < 0) {
        vn::log::error("gl_compose: attribute/uniform location missing");
        return false;
    }
    return true;
}

bool GlCompose::make_canvas_(int w, int h) {
    canvas_bo_ = gbm_bo_create(ctx_->gbm(), w, h, GBM_FORMAT_ARGB8888,
                               GBM_BO_USE_RENDERING | GBM_BO_USE_LINEAR);
    if (!canvas_bo_) {
        vn::log::error("gl_compose: gbm_bo_create canvas %dx%d", w, h);
        return false;
    }
    canvas_stride_ = gbm_bo_get_stride(canvas_bo_);
    canvas_fd_ = gbm_bo_get_fd(canvas_bo_);

    egl_ctx::EglCtx::ImageDesc d;
    d.fd = canvas_fd_;
    d.fourcc = DRM_FORMAT_ARGB8888;
    d.modifier = DRM_FORMAT_MOD_LINEAR;
    d.width = w;
    d.height = h;
    d.plane0_offset = 0;
    d.plane0_pitch = canvas_stride_;
    canvas_img_ = ctx_->import_dmabuf(d);
    if (canvas_img_ == EGL_NO_IMAGE) {
        vn::log::error("gl_compose: canvas import_dmabuf");
        return false;
    }

    glGenRenderbuffers(1, &rbo_);
    glBindRenderbuffer(GL_RENDERBUFFER, rbo_);
    pfn_image_to_rb(GL_RENDERBUFFER, canvas_img_);
    if (glGetError() != GL_NO_ERROR) {
        vn::log::error("gl_compose: image->RBO");
        return false;
    }

    glGenFramebuffers(1, &fbo_);
    glBindFramebuffer(GL_FRAMEBUFFER, fbo_);
    glFramebufferRenderbuffer(GL_FRAMEBUFFER, GL_COLOR_ATTACHMENT0, GL_RENDERBUFFER, rbo_);
    if (glCheckFramebufferStatus(GL_FRAMEBUFFER) != GL_FRAMEBUFFER_COMPLETE) {
        vn::log::error("gl_compose: canvas FBO incomplete");
        return false;
    }
    return true;
}

bool GlCompose::init(egl_ctx::EglCtx& ctx, int canvas_w, int canvas_h) {
    ctx_ = &ctx;
    canvas_w_ = canvas_w;
    canvas_h_ = canvas_h;

    pfn_image_to_tex =
        (PFNGLEGLIMAGETARGETTEXTURE2DOESPROC)eglGetProcAddress("glEGLImageTargetTexture2DOES");
    pfn_image_to_rb = (PFNGLEGLIMAGETARGETRENDERBUFFERSTORAGEOESPROC)eglGetProcAddress(
        "glEGLImageTargetRenderbufferStorageOES");
    if (!pfn_image_to_tex || !pfn_image_to_rb) {
        vn::log::error("gl_compose: missing GL_OES_EGL_image_external entry points");
        return false;
    }

    if (!build_program_(kVS, kFS))
        return false;
    if (!make_canvas_(canvas_w_, canvas_h_))
        return false;

    // Per-quad geometry is computed per render() call (positions depend on
    // slot rect), so we just allocate the dynamic VBO/IBO once.
    glGenBuffers(1, &vbo_);
    glGenBuffers(1, &ibo_);
    return true;
}

GlCompose::~GlCompose() {
    if (fbo_)
        glDeleteFramebuffers(1, &fbo_);
    if (rbo_)
        glDeleteRenderbuffers(1, &rbo_);
    if (vbo_)
        glDeleteBuffers(1, &vbo_);
    if (ibo_)
        glDeleteBuffers(1, &ibo_);
    if (prog_)
        glDeleteProgram(prog_);
    if (ctx_ && canvas_img_ != EGL_NO_IMAGE)
        eglDestroyImage(ctx_->display(), canvas_img_);
    if (canvas_bo_)
        gbm_bo_destroy(canvas_bo_);
    // canvas_fd_ was owned by gbm_bo (gbm_bo_get_fd dups internally on some
    // drivers, on others it's owned by the bo). Safer to not close it here;
    // gbm_bo_destroy releases it.
}

bool GlCompose::render(const std::vector<SourceSlot>& slots) {
    glBindFramebuffer(GL_FRAMEBUFFER, fbo_);
    glViewport(0, 0, canvas_w_, canvas_h_);

    // Black background. (Slots may not cover the whole canvas, e.g. with
    // a 2x2 grid plus padding, so the clear color is what shows through.)
    glClearColor(0.0f, 0.0f, 0.0f, 1.0f);
    glClear(GL_COLOR_BUFFER_BIT);

    glUseProgram(prog_);
    glUniform2f(loc_canvas_size_, (float)canvas_w_, (float)canvas_h_);
    glUniform1i(loc_src_y_, 0);
    glUniform1i(loc_src_uv_, 1);

    for (const auto& s : slots) {
        if (s.src_y_image == EGL_NO_IMAGE || s.src_uv_image == EGL_NO_IMAGE)
            continue;

        GLuint tex_y = 0, tex_uv = 0;
        glGenTextures(1, &tex_y);
        glActiveTexture(GL_TEXTURE0);
        glBindTexture(GL_TEXTURE_2D, tex_y);
        glTexParameteri(GL_TEXTURE_2D, GL_TEXTURE_MIN_FILTER, GL_LINEAR);
        glTexParameteri(GL_TEXTURE_2D, GL_TEXTURE_MAG_FILTER, GL_LINEAR);
        glTexParameteri(GL_TEXTURE_2D, GL_TEXTURE_WRAP_S, GL_CLAMP_TO_EDGE);
        glTexParameteri(GL_TEXTURE_2D, GL_TEXTURE_WRAP_T, GL_CLAMP_TO_EDGE);
        pfn_image_to_tex(GL_TEXTURE_2D, s.src_y_image);

        glGenTextures(1, &tex_uv);
        glActiveTexture(GL_TEXTURE1);
        glBindTexture(GL_TEXTURE_2D, tex_uv);
        glTexParameteri(GL_TEXTURE_2D, GL_TEXTURE_MIN_FILTER, GL_LINEAR);
        glTexParameteri(GL_TEXTURE_2D, GL_TEXTURE_MAG_FILTER, GL_LINEAR);
        glTexParameteri(GL_TEXTURE_2D, GL_TEXTURE_WRAP_S, GL_CLAMP_TO_EDGE);
        glTexParameteri(GL_TEXTURE_2D, GL_TEXTURE_WRAP_T, GL_CLAMP_TO_EDGE);
        pfn_image_to_tex(GL_TEXTURE_2D, s.src_uv_image);

        // Slot quad in canvas-px (top-left origin), with UV (0..1).
        float verts[16] = {
            // x,         y,        u, v
            (float)s.x,         (float)s.y,         0.f, 0.f,
            (float)(s.x + s.w), (float)s.y,         1.f, 0.f,
            (float)(s.x + s.w), (float)(s.y + s.h), 1.f, 1.f,
            (float)s.x,         (float)(s.y + s.h), 0.f, 1.f,
        };
        uint16_t idx[6] = {0, 1, 2, 0, 2, 3};

        glBindBuffer(GL_ARRAY_BUFFER, vbo_);
        glBufferData(GL_ARRAY_BUFFER, sizeof(verts), verts, GL_DYNAMIC_DRAW);
        glBindBuffer(GL_ELEMENT_ARRAY_BUFFER, ibo_);
        glBufferData(GL_ELEMENT_ARRAY_BUFFER, sizeof(idx), idx, GL_DYNAMIC_DRAW);

        glEnableVertexAttribArray(attr_pos_);
        glVertexAttribPointer(attr_pos_, 2, GL_FLOAT, GL_FALSE, 4 * sizeof(float), (void*)(0));
        glEnableVertexAttribArray(attr_uv_);
        glVertexAttribPointer(attr_uv_, 2, GL_FLOAT, GL_FALSE, 4 * sizeof(float),
                              (void*)(2 * sizeof(float)));

        glUniformMatrix3fv(loc_warp_, 1, GL_FALSE, s.warp.m);

        glDrawElements(GL_TRIANGLES, 6, GL_UNSIGNED_SHORT, nullptr);
        glDeleteTextures(1, &tex_y);
        glDeleteTextures(1, &tex_uv);
    }

    GLenum e = glGetError();
    if (e != GL_NO_ERROR) {
        vn::log::error("gl_compose: glError 0x%x", e);
        return false;
    }
    return true;
}

void GlCompose::finish() {
    glFinish();
}

} // namespace gl_compose
