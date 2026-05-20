#version 100
// Vertex shader for one composed source quad.
//
// a_pos  — quad corners in canvas pixel coordinates (we shift to NDC here).
// a_uv   — texture coordinates for sampling the source (0..1).
// u_canvas_size — canvas dims in pixels; used to map a_pos -> NDC.
// u_warp — 3x3 homography applied to a_uv. Identity for sources without
//          perspective; non-identity for the perspective-unlock demo.
//
// We keep the warp on UVs (not positions) so the destination region inside
// the canvas stays a clean rectangle; only the SAMPLING is warped. That's
// the same convention FFmpeg's `perspective` filter uses, and it lets us
// keystone or rotate source content while still owning a well-defined
// canvas slot for that source.

attribute vec2 a_pos;
attribute vec2 a_uv;
uniform vec2  u_canvas_size;
uniform mat3  u_warp;

varying vec2 v_uv;

void main() {
    // canvas-px -> NDC
    vec2 ndc = (a_pos / u_canvas_size) * 2.0 - 1.0;
    ndc.y = -ndc.y;  // y-flip so canvas (0,0) is top-left
    gl_Position = vec4(ndc, 0.0, 1.0);

    // Homogeneous-divide UV warp.
    vec3 w = u_warp * vec3(a_uv, 1.0);
    v_uv = w.xy / w.z;
}
