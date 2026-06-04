// composer_rpc — strong-typed request structs for the daemon→composer
// control surface, consumed by render::World::apply_*.
//
// Historically these were JSON-RPC params with hand-rolled parsers; the
// gRPC migration replaced the wire format with proto messages, and the
// service handler (render/composer_service.cpp) now marshals proto →
// composer_rpc::Request structs → World. Keeping these structs as the
// World API boundary avoids retyping the apply_* methods to proto and
// keeps the unit-test surface for World pure C++.

#pragma once

#include <cstdint>
#include <optional>
#include <string>
#include <vector>

namespace composer_rpc {

// Semantic-reject diagnostic. JSON-RPC error codes are retained
// (-32602 invalid params, -32000 semantic reject) so World's existing
// error reporting stays uniform; the service handler maps them to
// grpc::StatusCode::INVALID_ARGUMENT on the wire.
struct ParseError {
    int code = 0;
    std::string message;
};

struct SetCanvasRequest {
    uint32_t w = 0;
    uint32_t h = 0;
    uint32_t fps = 0;
    uint32_t background_rgba = 0x000000FFU; // packed 0xRRGGBBAA, opaque black default
};

struct SetSourceRequest {
    std::string slot; // "a" or "b" (free-form: composer's slot map keys on the string)
    std::string source_id;
    std::string scm_path;
    uint32_t width = 0;
    uint32_t height = 0;
    uint32_t fps = 0;
};

struct ClearSourceRequest {
    std::string slot;
};

struct LayoutSlot {
    std::string slot;
    int32_t x = 0;
    int32_t y = 0;
    int32_t w = 0;
    int32_t h = 0;
    int32_t rotation = 0;          // 0, 90, 180, 270 clockwise degrees
    int32_t aspect_ratio_mode = 0; // 0=stretch, 1=fit, 2=crop
    float crop_x = 0.0f;           // 0-1 normalized horizontal crop offset (0.5 = centered)
    float crop_y = 0.0f;           // 0-1 normalized vertical crop offset (0.5 = centered)
    float crop_scale = 1.0f;       // >= 1.0, source overfill factor
};
struct SetLayoutRequest {
    std::vector<LayoutSlot> slots;
};

// PerspectiveEffectParams is the only fully-typed effect today. The
// snapshot dims are required so we can normalize integer pixel corners
// into UV [0,1] without ambiguity if the live source resolution drifts.
struct PerspectiveEffectParams {
    int32_t corners[8] = {}; // TLx,TLy,TRx,TRy,BRx,BRy,BLx,BLy
    int32_t snapshot_w = 0;
    int32_t snapshot_h = 0;
};

struct Effect {
    std::string type;                                   // "perspective", "crop", "bbox", ...
    bool recognized = false;                            // false → composer should log + skip
    std::optional<PerspectiveEffectParams> perspective; // populated iff type == "perspective"
};

struct SetEffectsRequest {
    std::string source_id;
    std::vector<Effect> effects; // order-significant; composer applies in order
};

struct SetSourceStateRequest {
    std::string source_id;
    std::string state; // "live" | "transitioning" | "placeholder"
};

} // namespace composer_rpc
