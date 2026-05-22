#include "src/rpc/composer_rpc.hpp"

#include "src/rpc/jsonrpc_msg.hpp"

namespace composer_rpc {

namespace {

constexpr int kInvalidParams = -32602;

// Sane upper bounds for canvas / source dims. Anything bigger is a typo
// or an attack; the GPU side rejects oversized FBOs anyway.
constexpr uint64_t kMaxDim = 16384;
constexpr uint64_t kMaxFps = 240;

bool fail(ParseError& err, const char* msg) {
    err.code = kInvalidParams;
    err.message = msg;
    return false;
}

// Open the outer object, returning the position just past the opening '{'.
// Returns std::string::npos on error and populates `err`.
size_t open_object(std::string_view s, ParseError& err) {
    using namespace jsonrpc_msg::parse;
    size_t p = skip_ws(s, 0);
    if (p >= s.size() || s[p] != '{') {
        fail(err, "params must be object");
        return std::string::npos;
    }
    return p + 1;
}

// Advance past ',' or '}' after a key:value pair. Returns the new
// position; if we hit '}', sets `*done = true`. std::string::npos on error.
size_t after_pair(std::string_view s, size_t p, ParseError& err, bool* done) {
    using namespace jsonrpc_msg::parse;
    p = skip_ws(s, p);
    if (p < s.size() && s[p] == ',') {
        *done = false;
        return p + 1;
    }
    if (p < s.size() && s[p] == '}') {
        *done = true;
        return p + 1;
    }
    fail(err, "expected ',' or '}'");
    return std::string::npos;
}

// Read the next key. Returns the position right after the ':' (start of
// the value), or std::string::npos on error / closing brace. If the
// object closed, sets `*closed = true` and returns position just past '}'.
size_t next_key(std::string_view s, size_t p, std::string& key, ParseError& err, bool* closed) {
    using namespace jsonrpc_msg::parse;
    p = skip_ws(s, p);
    if (p >= s.size()) {
        fail(err, "truncated params");
        return std::string::npos;
    }
    if (s[p] == '}') {
        *closed = true;
        return p + 1;
    }
    *closed = false;
    size_t np = parse_string(s, p, key);
    if (np == std::string::npos) {
        fail(err, "bad key");
        return std::string::npos;
    }
    p = np;
    p = skip_ws(s, p);
    if (p >= s.size() || s[p] != ':') {
        fail(err, "expected ':'");
        return std::string::npos;
    }
    p = skip_ws(s, p + 1);
    return p;
}

// Parse a uint into `out`; returns position past value or std::string::npos.
size_t take_uint(std::string_view s, size_t p, uint64_t& out, ParseError& err) {
    using namespace jsonrpc_msg::parse;
    size_t np = parse_uint(s, p, out);
    if (np == std::string::npos) {
        fail(err, "expected non-negative integer");
        return std::string::npos;
    }
    return np;
}

// Parse a signed integer.
size_t take_int(std::string_view s, size_t p, int64_t& out, ParseError& err) {
    using namespace jsonrpc_msg::parse;
    size_t np = parse_int(s, p, out);
    if (np == std::string::npos) {
        fail(err, "expected integer");
        return std::string::npos;
    }
    return np;
}

// Parse a JSON string into `out`.
size_t take_string(std::string_view s, size_t p, std::string& out, ParseError& err) {
    using namespace jsonrpc_msg::parse;
    size_t np = parse_string(s, p, out);
    if (np == std::string::npos) {
        fail(err, "expected string");
        return std::string::npos;
    }
    return np;
}

// Skip an unknown JSON value (forward-compat for new fields).
size_t skip_unknown(std::string_view s, size_t p, ParseError& err) {
    using namespace jsonrpc_msg::parse;
    size_t np = skip_value(s, p);
    if (np == std::string::npos) {
        fail(err, "bad value for unknown key");
        return std::string::npos;
    }
    return np;
}

} // namespace

// ---- set_canvas -------------------------------------------------------

bool parse_set_canvas(std::string_view s, SetCanvasRequest& out, ParseError& err) {
    size_t p = open_object(s, err);
    if (p == std::string::npos)
        return false;

    uint64_t w = 0, h = 0, fps = 0;
    bool got_w = false, got_h = false, got_fps = false;

    bool closed = false;
    while (!closed) {
        std::string key;
        p = next_key(s, p, key, err, &closed);
        if (p == std::string::npos)
            return false;
        if (closed)
            break;

        if (key == "w") {
            p = take_uint(s, p, w, err);
            if (p == std::string::npos)
                return false;
            got_w = true;
        } else if (key == "h") {
            p = take_uint(s, p, h, err);
            if (p == std::string::npos)
                return false;
            got_h = true;
        } else if (key == "fps") {
            p = take_uint(s, p, fps, err);
            if (p == std::string::npos)
                return false;
            got_fps = true;
        } else {
            p = skip_unknown(s, p, err);
            if (p == std::string::npos)
                return false;
        }

        bool done = false;
        p = after_pair(s, p, err, &done);
        if (p == std::string::npos)
            return false;
        if (done)
            break;
    }

    if (!got_w || !got_h || !got_fps)
        return fail(err, "missing required field (w, h, fps)");
    if (w == 0 || h == 0 || fps == 0)
        return fail(err, "w, h, fps must be > 0");
    if (w > kMaxDim || h > kMaxDim)
        return fail(err, "w / h exceed sane upper bound (16384)");
    if (fps > kMaxFps)
        return fail(err, "fps exceeds sane upper bound (240)");

    out.w = uint32_t(w);
    out.h = uint32_t(h);
    out.fps = uint32_t(fps);
    return true;
}

// ---- set_source -------------------------------------------------------

bool parse_set_source(std::string_view s, SetSourceRequest& out, ParseError& err) {
    size_t p = open_object(s, err);
    if (p == std::string::npos)
        return false;

    std::string slot, source_id, scm_path;
    uint64_t width = 0, height = 0, fps = 0;
    bool got_slot = false, got_sid = false, got_path = false;
    bool got_w = false, got_h = false, got_fps = false;

    bool closed = false;
    while (!closed) {
        std::string key;
        p = next_key(s, p, key, err, &closed);
        if (p == std::string::npos)
            return false;
        if (closed)
            break;

        if (key == "slot") {
            p = take_string(s, p, slot, err);
            if (p == std::string::npos)
                return false;
            got_slot = true;
        } else if (key == "source_id") {
            p = take_string(s, p, source_id, err);
            if (p == std::string::npos)
                return false;
            got_sid = true;
        } else if (key == "scm_path") {
            p = take_string(s, p, scm_path, err);
            if (p == std::string::npos)
                return false;
            got_path = true;
        } else if (key == "width") {
            p = take_uint(s, p, width, err);
            if (p == std::string::npos)
                return false;
            got_w = true;
        } else if (key == "height") {
            p = take_uint(s, p, height, err);
            if (p == std::string::npos)
                return false;
            got_h = true;
        } else if (key == "fps") {
            p = take_uint(s, p, fps, err);
            if (p == std::string::npos)
                return false;
            got_fps = true;
        } else {
            p = skip_unknown(s, p, err);
            if (p == std::string::npos)
                return false;
        }

        bool done = false;
        p = after_pair(s, p, err, &done);
        if (p == std::string::npos)
            return false;
        if (done)
            break;
    }

    if (!got_slot || !got_sid || !got_path || !got_w || !got_h || !got_fps)
        return fail(err, "missing required field (slot, source_id, scm_path, width, height, fps)");
    if (slot.empty() || source_id.empty() || scm_path.empty())
        return fail(err, "slot, source_id, scm_path must be non-empty");
    if (width == 0 || height == 0 || fps == 0)
        return fail(err, "width, height, fps must be > 0");
    if (width > kMaxDim || height > kMaxDim)
        return fail(err, "width / height exceed sane upper bound (16384)");
    if (fps > kMaxFps)
        return fail(err, "fps exceeds sane upper bound (240)");

    out.slot = std::move(slot);
    out.source_id = std::move(source_id);
    out.scm_path = std::move(scm_path);
    out.width = uint32_t(width);
    out.height = uint32_t(height);
    out.fps = uint32_t(fps);
    return true;
}

// ---- clear_source -----------------------------------------------------

bool parse_clear_source(std::string_view s, ClearSourceRequest& out, ParseError& err) {
    size_t p = open_object(s, err);
    if (p == std::string::npos)
        return false;

    std::string slot;
    bool got_slot = false;

    bool closed = false;
    while (!closed) {
        std::string key;
        p = next_key(s, p, key, err, &closed);
        if (p == std::string::npos)
            return false;
        if (closed)
            break;

        if (key == "slot") {
            p = take_string(s, p, slot, err);
            if (p == std::string::npos)
                return false;
            got_slot = true;
        } else {
            p = skip_unknown(s, p, err);
            if (p == std::string::npos)
                return false;
        }

        bool done = false;
        p = after_pair(s, p, err, &done);
        if (p == std::string::npos)
            return false;
        if (done)
            break;
    }

    if (!got_slot || slot.empty())
        return fail(err, "missing or empty 'slot'");
    out.slot = std::move(slot);
    return true;
}

// ---- set_layout -------------------------------------------------------

namespace {

// Parse one {"slot":"a","x":...,"y":...,"w":...,"h":...} object starting at '{'.
bool parse_layout_slot_obj(std::string_view s, size_t& p, LayoutSlot& out, ParseError& err) {
    using namespace jsonrpc_msg::parse;
    p = skip_ws(s, p);
    if (p >= s.size() || s[p] != '{')
        return fail(err, "layout slot must be object");
    ++p;

    std::string slot;
    int64_t x = 0, y = 0, w = 0, h = 0;
    bool got_slot = false, got_x = false, got_y = false, got_w = false, got_h = false;

    bool closed = false;
    while (!closed) {
        std::string key;
        p = next_key(s, p, key, err, &closed);
        if (p == std::string::npos)
            return false;
        if (closed)
            break;

        if (key == "slot") {
            p = take_string(s, p, slot, err);
            if (p == std::string::npos)
                return false;
            got_slot = true;
        } else if (key == "x") {
            p = take_int(s, p, x, err);
            if (p == std::string::npos)
                return false;
            got_x = true;
        } else if (key == "y") {
            p = take_int(s, p, y, err);
            if (p == std::string::npos)
                return false;
            got_y = true;
        } else if (key == "w") {
            p = take_int(s, p, w, err);
            if (p == std::string::npos)
                return false;
            got_w = true;
        } else if (key == "h") {
            p = take_int(s, p, h, err);
            if (p == std::string::npos)
                return false;
            got_h = true;
        } else {
            p = skip_unknown(s, p, err);
            if (p == std::string::npos)
                return false;
        }

        bool done = false;
        p = after_pair(s, p, err, &done);
        if (p == std::string::npos)
            return false;
        if (done)
            break;
    }

    if (!got_slot || !got_x || !got_y || !got_w || !got_h)
        return fail(err, "layout slot missing required field");
    if (slot.empty())
        return fail(err, "layout slot 'slot' must be non-empty");
    if (w <= 0 || h <= 0)
        return fail(err, "layout slot w/h must be positive");
    out.slot = std::move(slot);
    out.x = int32_t(x);
    out.y = int32_t(y);
    out.w = int32_t(w);
    out.h = int32_t(h);
    return true;
}

} // namespace

bool parse_set_layout(std::string_view s, SetLayoutRequest& out, ParseError& err) {
    using namespace jsonrpc_msg::parse;
    size_t p = open_object(s, err);
    if (p == std::string::npos)
        return false;

    bool got_slots = false;

    bool closed = false;
    while (!closed) {
        std::string key;
        p = next_key(s, p, key, err, &closed);
        if (p == std::string::npos)
            return false;
        if (closed)
            break;

        if (key == "slots") {
            p = skip_ws(s, p);
            if (p >= s.size() || s[p] != '[')
                return fail(err, "'slots' must be array");
            ++p;
            // Parse array elements.
            while (true) {
                p = skip_ws(s, p);
                if (p >= s.size())
                    return fail(err, "truncated slots array");
                if (s[p] == ']') {
                    ++p;
                    break;
                }
                LayoutSlot ls{};
                if (!parse_layout_slot_obj(s, p, ls, err))
                    return false;
                out.slots.push_back(std::move(ls));
                p = skip_ws(s, p);
                if (p < s.size() && s[p] == ',') {
                    ++p;
                    continue;
                }
                if (p < s.size() && s[p] == ']') {
                    ++p;
                    break;
                }
                return fail(err, "expected ',' or ']' in slots");
            }
            got_slots = true;
        } else {
            p = skip_unknown(s, p, err);
            if (p == std::string::npos)
                return false;
        }

        bool done = false;
        p = after_pair(s, p, err, &done);
        if (p == std::string::npos)
            return false;
        if (done)
            break;
    }

    if (!got_slots)
        return fail(err, "missing 'slots'");
    return true;
}

// ---- set_effects ------------------------------------------------------

namespace {

// Parse {"corners":[[x,y]x4]} into corners[8]. Accepts both flat
// [x,y,x,y,...] (8 ints) and nested [[x,y],...] (4 pairs); tests use
// the nested form to match the API model.
bool parse_corners_array(std::string_view s, size_t& p, int32_t corners[8], ParseError& err) {
    using namespace jsonrpc_msg::parse;
    p = skip_ws(s, p);
    if (p >= s.size() || s[p] != '[')
        return fail(err, "corners must be array");
    ++p;

    int count = 0;
    // Look at the first non-ws char to detect nested vs flat.
    size_t peek = skip_ws(s, p);
    if (peek >= s.size())
        return fail(err, "truncated corners");
    bool nested = (s[peek] == '[');

    while (true) {
        p = skip_ws(s, p);
        if (p >= s.size())
            return fail(err, "truncated corners");
        if (s[p] == ']') {
            ++p;
            break;
        }
        if (nested) {
            if (s[p] != '[')
                return fail(err, "expected '[' in nested corners");
            ++p;
            int64_t x = 0, y = 0;
            p = take_int(s, p, x, err);
            if (p == std::string::npos)
                return false;
            p = skip_ws(s, p);
            if (p >= s.size() || s[p] != ',')
                return fail(err, "expected ',' between x,y");
            ++p;
            p = skip_ws(s, p);
            p = take_int(s, p, y, err);
            if (p == std::string::npos)
                return false;
            p = skip_ws(s, p);
            if (p >= s.size() || s[p] != ']')
                return fail(err, "expected ']' closing corner");
            ++p;
            if (count + 1 >= 8)
                return fail(err, "too many corners (want 4)");
            corners[count++] = int32_t(x);
            corners[count++] = int32_t(y);
        } else {
            int64_t v = 0;
            p = take_int(s, p, v, err);
            if (p == std::string::npos)
                return false;
            if (count >= 8)
                return fail(err, "too many corner values (want 8)");
            corners[count++] = int32_t(v);
        }
        p = skip_ws(s, p);
        if (p < s.size() && s[p] == ',') {
            ++p;
            continue;
        }
        if (p < s.size() && s[p] == ']') {
            ++p;
            break;
        }
        return fail(err, "expected ',' or ']' in corners");
    }
    if (count != 8)
        return fail(err, "corners must have 4 (x,y) pairs");
    return true;
}

// Parse one effect object starting at '{'.
bool parse_effect_obj(std::string_view s, size_t& p, Effect& out, ParseError& err) {
    using namespace jsonrpc_msg::parse;
    p = skip_ws(s, p);
    if (p >= s.size() || s[p] != '{')
        return fail(err, "effect must be object");
    ++p;

    std::string type;
    PerspectiveEffectParams persp{};
    bool got_corners = false, got_snap_w = false, got_snap_h = false;

    bool closed = false;
    while (!closed) {
        std::string key;
        p = next_key(s, p, key, err, &closed);
        if (p == std::string::npos)
            return false;
        if (closed)
            break;

        if (key == "type") {
            p = take_string(s, p, type, err);
            if (p == std::string::npos)
                return false;
        } else if (key == "corners") {
            if (!parse_corners_array(s, p, persp.corners, err))
                return false;
            got_corners = true;
        } else if (key == "snapshot_w") {
            int64_t v = 0;
            p = take_int(s, p, v, err);
            if (p == std::string::npos)
                return false;
            persp.snapshot_w = int32_t(v);
            got_snap_w = true;
        } else if (key == "snapshot_h") {
            int64_t v = 0;
            p = take_int(s, p, v, err);
            if (p == std::string::npos)
                return false;
            persp.snapshot_h = int32_t(v);
            got_snap_h = true;
        } else {
            // Unknown field — keep for forward-compat with new effect kinds.
            p = skip_unknown(s, p, err);
            if (p == std::string::npos)
                return false;
        }

        bool done = false;
        p = after_pair(s, p, err, &done);
        if (p == std::string::npos)
            return false;
        if (done)
            break;
    }

    if (type.empty())
        return fail(err, "effect missing 'type'");
    out.type = type;
    if (type == "perspective") {
        if (!got_corners || !got_snap_w || !got_snap_h)
            return fail(err, "perspective effect requires corners, snapshot_w, snapshot_h");
        if (persp.snapshot_w <= 0 || persp.snapshot_h <= 0)
            return fail(err, "perspective snapshot_w/h must be positive");
        out.perspective = persp;
        out.recognized = true;
    } else {
        // Unknown / future effect (crop, bbox, ...). Composer will log + skip.
        out.recognized = false;
    }
    return true;
}

} // namespace

bool parse_set_effects(std::string_view s, SetEffectsRequest& out, ParseError& err) {
    using namespace jsonrpc_msg::parse;
    size_t p = open_object(s, err);
    if (p == std::string::npos)
        return false;

    bool got_sid = false, got_effects = false;

    bool closed = false;
    while (!closed) {
        std::string key;
        p = next_key(s, p, key, err, &closed);
        if (p == std::string::npos)
            return false;
        if (closed)
            break;

        if (key == "source_id") {
            p = take_string(s, p, out.source_id, err);
            if (p == std::string::npos)
                return false;
            got_sid = true;
        } else if (key == "effects") {
            p = skip_ws(s, p);
            if (p >= s.size() || s[p] != '[')
                return fail(err, "'effects' must be array");
            ++p;
            while (true) {
                p = skip_ws(s, p);
                if (p >= s.size())
                    return fail(err, "truncated effects array");
                if (s[p] == ']') {
                    ++p;
                    break;
                }
                Effect e{};
                if (!parse_effect_obj(s, p, e, err))
                    return false;
                out.effects.push_back(std::move(e));
                p = skip_ws(s, p);
                if (p < s.size() && s[p] == ',') {
                    ++p;
                    continue;
                }
                if (p < s.size() && s[p] == ']') {
                    ++p;
                    break;
                }
                return fail(err, "expected ',' or ']' in effects");
            }
            got_effects = true;
        } else {
            p = skip_unknown(s, p, err);
            if (p == std::string::npos)
                return false;
        }

        bool done = false;
        p = after_pair(s, p, err, &done);
        if (p == std::string::npos)
            return false;
        if (done)
            break;
    }

    if (!got_sid || out.source_id.empty())
        return fail(err, "missing or empty 'source_id'");
    if (!got_effects)
        return fail(err, "missing 'effects'");
    return true;
}

// ---- set_source_state -------------------------------------------------

bool parse_set_source_state(std::string_view s, SetSourceStateRequest& out, ParseError& err) {
    size_t p = open_object(s, err);
    if (p == std::string::npos)
        return false;

    bool got_sid = false, got_state = false;

    bool closed = false;
    while (!closed) {
        std::string key;
        p = next_key(s, p, key, err, &closed);
        if (p == std::string::npos)
            return false;
        if (closed)
            break;

        if (key == "source_id") {
            p = take_string(s, p, out.source_id, err);
            if (p == std::string::npos)
                return false;
            got_sid = true;
        } else if (key == "state") {
            p = take_string(s, p, out.state, err);
            if (p == std::string::npos)
                return false;
            got_state = true;
        } else {
            p = skip_unknown(s, p, err);
            if (p == std::string::npos)
                return false;
        }

        bool done = false;
        p = after_pair(s, p, err, &done);
        if (p == std::string::npos)
            return false;
        if (done)
            break;
    }

    if (!got_sid || out.source_id.empty())
        return fail(err, "missing or empty 'source_id'");
    if (!got_state)
        return fail(err, "missing 'state'");
    if (out.state != "live" && out.state != "transitioning" && out.state != "placeholder")
        return fail(err, "'state' must be one of: live, transitioning, placeholder");
    return true;
}

} // namespace composer_rpc
