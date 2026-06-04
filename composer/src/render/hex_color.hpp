// hex_color — parse a CSS-style hex color string into a packed 0xRRGGBBAA
// value for the composer canvas background. Header-only so it stays unit
// testable without pulling in the gRPC service translation unit.

#pragma once

#include <cstdint>
#include <string_view>

namespace hex_color {

// Parse "#RRGGBB" / "#RRGGBBAA" (leading '#' optional) into packed
// 0xRRGGBBAA via `out`. Empty input leaves `out` untouched (caller keeps its
// default). Returns false on malformed input (bad length or non-hex digit),
// leaving `out` untouched.
[[nodiscard]] inline bool parse(std::string_view s, uint32_t& out) {
    if (!s.empty() && s.front() == '#')
        s.remove_prefix(1);
    if (s.empty())
        return true;
    if (s.size() != 6 && s.size() != 8)
        return false;
    uint32_t value = 0;
    for (char c : s) {
        uint32_t nibble = 0;
        if (c >= '0' && c <= '9')
            nibble = uint32_t(c - '0');
        else if (c >= 'a' && c <= 'f')
            nibble = uint32_t(c - 'a' + 10);
        else if (c >= 'A' && c <= 'F')
            nibble = uint32_t(c - 'A' + 10);
        else
            return false;
        value = (value << 4) | nibble;
    }
    out = s.size() == 6 ? (value << 8) | 0xFFU : value;
    return true;
}

} // namespace hex_color
