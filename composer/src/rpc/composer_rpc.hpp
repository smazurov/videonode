// composer_rpc — params parsers for the daemon→composer JSON-RPC methods.
//
// videonode-composer is daemon-driven: argv is just `--drm-device
// --ctl-connect --composer-id`. Everything else (canvas dims, slot
// bindings, layout, effects, per-source state) arrives over the existing
// control channel (src/rpc/control_channel) as JSON-RPC requests. This
// module owns the params decoders so the wire format has a small,
// unit-testable surface (untrusted bytes from the daemon).
//
// Methods (caller dispatches by name, then calls the matching parse_*):
//
//   set_canvas        {w, h, fps}
//   set_source        {slot, source_id, scm_path, width, height, fps}
//   clear_source      {slot}
//   set_layout        {slots:[{slot, x, y, w, h}, ...]}
//   set_effects       {source_id, effects:[{type, ...params}, ...]}
//   set_source_state  {source_id, state}
//
// Effect types currently understood:
//   {"type":"perspective","corners":[[x,y],[x,y],[x,y],[x,y]],"snapshot_w":W,"snapshot_h":H}
//
// Unknown effect types are kept in the request as `Effect{type:"…", recognized:false}` so the
// composer can log + skip — adding crop/bbox later won't require composer rebuilds on
// the rig until the rig wants to actually apply them.

#pragma once

#include <cstdint>
#include <optional>
#include <string>
#include <string_view>
#include <vector>

namespace composer_rpc {

// JSON-RPC error codes; -32602 invalid params is the only one we emit
// from these parsers. -32000 is reserved for "method understood but
// semantically rejected" (e.g. unknown effect type when we eventually
// add strict mode). Today the parsers only ever emit -32602.
struct ParseError {
    int code = 0;
    std::string message;
};

struct SetCanvasRequest {
    uint32_t w = 0;
    uint32_t h = 0;
    uint32_t fps = 0;
};
[[nodiscard]] bool parse_set_canvas(std::string_view params_json,
                                    SetCanvasRequest& out, ParseError& err);

struct SetSourceRequest {
    std::string slot;       // "a" or "b" (free-form: composer's slot map keys on the string)
    std::string source_id;  // stable identifier from the daemon
    std::string scm_path;   // Unix-socket path the source is listening on
    uint32_t width = 0;
    uint32_t height = 0;
    uint32_t fps = 0;
};
[[nodiscard]] bool parse_set_source(std::string_view params_json,
                                    SetSourceRequest& out, ParseError& err);

struct ClearSourceRequest {
    std::string slot;
};
[[nodiscard]] bool parse_clear_source(std::string_view params_json,
                                      ClearSourceRequest& out, ParseError& err);

struct LayoutSlot {
    std::string slot;
    int32_t x = 0;
    int32_t y = 0;
    int32_t w = 0;
    int32_t h = 0;
};
struct SetLayoutRequest {
    std::vector<LayoutSlot> slots;
};
[[nodiscard]] bool parse_set_layout(std::string_view params_json,
                                    SetLayoutRequest& out, ParseError& err);

// PerspectiveEffectParams is the only fully-typed effect today. The
// snapshot dims are required so we can normalize integer pixel corners
// into UV [0,1] without ambiguity if the live source resolution drifts.
struct PerspectiveEffectParams {
    int32_t corners[8] = {};       // TLx,TLy,TRx,TRy,BRx,BRy,BLx,BLy
    int32_t snapshot_w = 0;
    int32_t snapshot_h = 0;
};

struct Effect {
    std::string type;                                   // "perspective", "crop", "bbox", ...
    bool recognized = false;                             // false → composer should log + skip
    std::optional<PerspectiveEffectParams> perspective;  // populated iff type == "perspective"
};

struct SetEffectsRequest {
    std::string source_id;
    std::vector<Effect> effects;  // order-significant; composer applies in order
};
[[nodiscard]] bool parse_set_effects(std::string_view params_json,
                                     SetEffectsRequest& out, ParseError& err);

struct SetSourceStateRequest {
    std::string source_id;
    std::string state;  // "live" | "transitioning" | "placeholder"
};
[[nodiscard]] bool parse_set_source_state(std::string_view params_json,
                                          SetSourceStateRequest& out, ParseError& err);

} // namespace composer_rpc
