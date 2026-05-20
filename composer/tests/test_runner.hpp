// test_runner — minimal header-only assertion framework for composer-spike
// host-side tests. No external test-framework dependency (we don't want to
// drag GoogleTest just for ~50 assertions).
//
// Each test_*.cpp main() runs a sequence of CHECK_*() macros and prints a
// pass/fail summary. Meson's `test()` target invokes them and treats a
// non-zero exit as a failure.

#pragma once

#include <cstdio>
#include <cstdlib>
#include <cstring>
#include <string>

namespace test_runner {

inline int g_passes = 0;
inline int g_fails = 0;
inline const char* g_current = "";

inline void start_case(const char* name) {
    g_current = name;
}

inline int report_and_exit_code() {
    fprintf(stderr, "%s: %d passed, %d failed\n", g_fails == 0 ? "OK" : "FAIL", g_passes, g_fails);
    return g_fails == 0 ? 0 : 1;
}

} // namespace test_runner

#define CHECK_TRUE(expr)                                                                           \
    do {                                                                                           \
        if (expr) {                                                                                \
            ++test_runner::g_passes;                                                               \
        } else {                                                                                   \
            ++test_runner::g_fails;                                                                \
            fprintf(stderr, "  [%s] FAIL %s:%d: %s\n", test_runner::g_current, __FILE__, __LINE__, \
                    #expr);                                                                        \
        }                                                                                          \
    } while (0)

#define CHECK_EQ(a, b)                                                                             \
    do {                                                                                           \
        auto _a = (a);                                                                             \
        auto _b = (b);                                                                             \
        if (_a == _b) {                                                                            \
            ++test_runner::g_passes;                                                               \
        } else {                                                                                   \
            ++test_runner::g_fails;                                                                \
            fprintf(stderr, "  [%s] FAIL %s:%d: %s != %s\n", test_runner::g_current, __FILE__,     \
                    __LINE__, #a, #b);                                                             \
        }                                                                                          \
    } while (0)

#define CHECK_STR_EQ(a, b)                                                                         \
    do {                                                                                           \
        std::string _a(a), _b(b);                                                                  \
        if (_a == _b) {                                                                            \
            ++test_runner::g_passes;                                                               \
        } else {                                                                                   \
            ++test_runner::g_fails;                                                                \
            fprintf(stderr, "  [%s] FAIL %s:%d: \"%s\" != \"%s\"\n", test_runner::g_current,       \
                    __FILE__, __LINE__, _a.c_str(), _b.c_str());                                   \
        }                                                                                          \
    } while (0)

#define CHECK_CONTAINS(haystack_vec, needle)                                                       \
    do {                                                                                           \
        bool _found = false;                                                                       \
        for (const auto& _x : (haystack_vec)) {                                                    \
            if (_x == (needle)) {                                                                  \
                _found = true;                                                                     \
                break;                                                                             \
            }                                                                                      \
        }                                                                                          \
        if (_found) {                                                                              \
            ++test_runner::g_passes;                                                               \
        } else {                                                                                   \
            ++test_runner::g_fails;                                                                \
            fprintf(stderr, "  [%s] FAIL %s:%d: vector missing \"%s\"\n", test_runner::g_current,  \
                    __FILE__, __LINE__, std::string(needle).c_str());                              \
        }                                                                                          \
    } while (0)
