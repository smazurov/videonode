// Severity-preserving stderr helpers for native C++ binaries.
//
// Each helper writes a single line to stderr in the form `[level] message\n`.
// The Go daemon's `ffmpeg.ParseLogLevel` (internal/ffmpeg/log_parser.go) maps
// the bracketed prefix back to a slog level, so journald sees the real
// severity instead of treating every C++ stderr line as info.
//
// Usage:
//   vn::log::fatal("gRPC server failed to start on %s", path);
//   vn::log::error("mmap fd=%d failed: %s", fd, strerror(errno));
//   vn::log::warn("dimensions changed %dx%d -> %dx%d", ow, oh, w, h);
//   vn::log::info("placeholder %dx%d ready", w, h);
//
// Header-only by design; no link dependency.

#pragma once

#include <cstdarg>
#include <cstdio>

namespace vn::log {

inline void vlog_prefixed(const char* level_prefix, const char* fmt, std::va_list ap) {
    std::fprintf(stderr, "%s", level_prefix);
    std::vfprintf(stderr, fmt, ap);
    std::fputc('\n', stderr);
}

inline void fatal(const char* fmt, ...) {
    std::va_list ap;
    va_start(ap, fmt);
    vlog_prefixed("[fatal] ", fmt, ap);
    va_end(ap);
}

inline void error(const char* fmt, ...) {
    std::va_list ap;
    va_start(ap, fmt);
    vlog_prefixed("[error] ", fmt, ap);
    va_end(ap);
}

inline void warn(const char* fmt, ...) {
    std::va_list ap;
    va_start(ap, fmt);
    vlog_prefixed("[warning] ", fmt, ap);
    va_end(ap);
}

inline void info(const char* fmt, ...) {
    std::va_list ap;
    va_start(ap, fmt);
    vlog_prefixed("[info] ", fmt, ap);
    va_end(ap);
}

} // namespace vn::log
