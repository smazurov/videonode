// vn::flags::normalize_argv — rewrite hyphenated absl-flag arguments to use
// underscores in-place before absl::ParseCommandLine sees them.
//
// Abseil's flag parser does NOT treat `--foo-bar` and `--foo_bar` as
// equivalent: ABSL_FLAG requires a C++ identifier (so `foo_bar`), and the
// parser only accepts the exact spelling. The videonode Go daemon spawns
// these binaries with the historical hyphenated spelling (`--out-socket`,
// `--device-id`, `--in-format`, …). Rather than break that ABI, we mutate
// `argv` in place: for any token that starts with `--` (long flag), every
// `-` in the name segment (before `=`, if any) becomes `_`.
//
// The function mutates the C strings argv points at directly. argv is
// guaranteed writable per POSIX, and absl::ParseCommandLine documents that
// it may rewrite argv too — so we are not introducing new aliasing issues.
//
// Short flags (`-v`) and positional tokens are untouched. The leading `--`
// terminator stops processing for everything after it.

#pragma once

#include <absl/flags/usage_config.h>
#include <absl/strings/string_view.h>

#include <cstring>
#include <span>

namespace vn::flags {

// configure_help_filter teaches absl's `--help` filter to recognize our
// project's source files. Absl's `--help` matches the binary basename as a
// substring against each flag's defining source file path; our binaries are
// hyphenated (`videonode-composer`) while the TUs are underscored
// (`videonode_composer_main.cpp`), so the default match always fails and
// `--help` prints "No flags matched". Returning true for any path containing
// `composer/src/` makes `--help` list every flag we define, while still
// suppressing absl-internal flags vendored under `external/` or similar.
inline void configure_help_filter() {
    absl::FlagsUsageConfig cfg;
    cfg.contains_helpshort_flags = [](absl::string_view path) {
        return path.find("composer/src/") != absl::string_view::npos;
    };
    cfg.contains_help_flags = cfg.contains_helpshort_flags;
    absl::SetFlagsUsageConfig(cfg);
}

inline void normalize_argv(int argc, char** argv) {
    const std::span<char*> args(argv, static_cast<size_t>(argc));
    for (size_t i = 1; i < args.size(); ++i) {
        char* a = args[i];
        if (a == nullptr)
            continue;
        const size_t alen = std::strlen(a);
        if (alen < 2)
            continue;
        const std::span<char> tok(a, alen);
        // POSIX `--` terminator: everything after this is positional.
        if (tok.size() == 2 && tok[0] == '-' && tok[1] == '-')
            break;
        if (tok[0] != '-' || tok[1] != '-')
            continue;
        for (size_t j = 2; j < tok.size(); ++j) {
            if (tok[j] == '=')
                break;
            if (tok[j] == '-')
                tok[j] = '_';
        }
    }
}

} // namespace vn::flags
