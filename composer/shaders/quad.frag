#version 100
#extension GL_OES_EGL_image_external : require
precision mediump float;

uniform samplerExternalOES u_src;
varying vec2 v_uv;

void main() {
    // Clamp UV to (0..1) so warped UVs going outside the source don't
    // read garbage; they show black instead (the default sampler border
    // behaviour with CLAMP_TO_EDGE samples the edge pixel, which is what
    // we want for our black-background sources).
    if (v_uv.x < 0.0 || v_uv.x > 1.0 || v_uv.y < 0.0 || v_uv.y > 1.0) {
        gl_FragColor = vec4(0.0, 0.0, 0.0, 1.0);
        return;
    }
    gl_FragColor = texture2D(u_src, v_uv);
}
