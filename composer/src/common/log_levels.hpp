#pragma once

#include <cstdarg>
#include <cstddef>
#include <cstdio>
#include <initializer_list>
#include <span>

namespace vn::log {

inline void vlog_prefixed(const char* level_prefix, const char* fmt, std::va_list ap) {
    std::fprintf(stderr, "%s", level_prefix);
    std::vfprintf(stderr, fmt, ap);
    std::fputc('\n', stderr);
}

inline void vlog_prefixed_kv(const char* prefix, const char* msg,
                             std::initializer_list<const char*> kvs) {
    std::fprintf(stderr, "%s%s", prefix, msg);
    const std::span<const char* const> kv(kvs.begin(), kvs.size());
    if (!kv.empty())
        std::fputc('\t', stderr);
    for (size_t i = 0; i + 1 < kv.size(); i += 2) {
        const char* k = kv[i];
        const char* v = kv[i + 1];
        if (i + 2 < kv.size())
            std::fprintf(stderr, "%s=%s ", k, v);
        else
            std::fprintf(stderr, "%s=%s", k, v);
    }
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

inline void debug(const char* fmt, ...) {
    std::va_list ap;
    va_start(ap, fmt);
    vlog_prefixed("[debug] ", fmt, ap);
    va_end(ap);
}

inline void info_s(const char* msg, std::initializer_list<const char*> kvs) {
    vlog_prefixed_kv("[info] ", msg, kvs);
}

inline void debug_s(const char* msg, std::initializer_list<const char*> kvs) {
    vlog_prefixed_kv("[debug] ", msg, kvs);
}

inline void warn_s(const char* msg, std::initializer_list<const char*> kvs) {
    vlog_prefixed_kv("[warning] ", msg, kvs);
}

} // namespace vn::log
