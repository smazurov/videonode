// probe_check — shared assertion macros for composer/tools/* probes.
//
// All probes follow the same failure convention: log a single line to stderr
// and exit non-zero. The three macros below capture the three common shapes
// previously copy-pasted into compose-probe, import-probe, live-compose-probe
// and egl-probe.
//
//   VN_CHECK(expr, fmt, ...)   — generic "expression must be truthy" guard.
//   EGL_CHECK(call)            — wraps an EGL call; on zero return, decodes
//                                eglGetError() and exits.
//   GL_CHECK(label)            — checks glGetError() after a GL call; logs
//                                the label + GL error code and exits.
//
// Standalone header — depends only on the EGL/GLES2 headers (for EGL_CHECK /
// GL_CHECK) and <cstdio>/<cstdlib>. No coupling to egl_ctx or any other
// composer library, so it is safe to include from any probe.

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

// Generic guard: if `expr` is falsey, print "FAIL: <fmt>\n" and exit(1).
// The `...` is the trailing format args (may be empty); printf-style.
#define VN_CHECK(expr, ...)                                                                        \
    do {                                                                                           \
        if (!(expr)) {                                                                             \
            std::fprintf(stderr, "FAIL: ");                                                        \
            std::fprintf(stderr, __VA_ARGS__);                                                     \
            std::fputc('\n', stderr);                                                              \
            std::exit(1);                                                                          \
        }                                                                                          \
    } while (0)

// EGL call guard: invokes `call`; on zero return decodes eglGetError() and
// exits with the stringified call + error name.
#define EGL_CHECK(call)                                                                            \
    do {                                                                                           \
        auto _vn_egl_r = (call);                                                                   \
        if (_vn_egl_r == 0) {                                                                      \
            EGLint _vn_egl_e = eglGetError();                                                      \
            std::fprintf(stderr, "FAIL: %s: %s\n", #call, ::vn::probe::egl_err_str(_vn_egl_e));    \
            std::exit(1);                                                                          \
        }                                                                                          \
    } while (0)

// GL error guard: after a GL call, asserts glGetError() == GL_NO_ERROR.
// `label` is a descriptive string included in the failure message.
#define GL_CHECK(label)                                                                            \
    do {                                                                                           \
        GLenum _vn_gl_e = glGetError();                                                            \
        if (_vn_gl_e != GL_NO_ERROR) {                                                             \
            std::fprintf(stderr, "FAIL: %s gl=0x%x\n", (label), _vn_gl_e);                         \
            std::exit(1);                                                                          \
        }                                                                                          \
    } while (0)
