#pragma once

#include <cstdio>
#include <cstdlib>

#include <EGL/egl.h>
#include <GLES2/gl2.h>

namespace vn::probe {

[[nodiscard]] inline const char* egl_err_str(EGLint e) {
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

} // namespace vn::probe

#define VN_CHECK(expr, ...)                                                                        \
    do {                                                                                           \
        if (!(expr)) {                                                                             \
            std::fprintf(stderr, "FAIL: ");                                                        \
            std::fprintf(stderr, __VA_ARGS__);                                                     \
            std::fputc('\n', stderr);                                                              \
            std::exit(1);                                                                          \
        }                                                                                          \
    } while (0)

#define EGL_CHECK(call)                                                                            \
    do {                                                                                           \
        auto _vn_egl_r = (call);                                                                   \
        if (_vn_egl_r == 0) {                                                                      \
            EGLint _vn_egl_e = eglGetError();                                                      \
            std::fprintf(stderr, "FAIL: %s: %s\n", #call, ::vn::probe::egl_err_str(_vn_egl_e));    \
            std::exit(1);                                                                          \
        }                                                                                          \
    } while (0)

#define GL_CHECK(label)                                                                            \
    do {                                                                                           \
        GLenum _vn_gl_e = glGetError();                                                            \
        if (_vn_gl_e != GL_NO_ERROR) {                                                             \
            std::fprintf(stderr, "FAIL: %s gl=0x%x\n", (label), _vn_gl_e);                         \
            std::exit(1);                                                                          \
        }                                                                                          \
    } while (0)
