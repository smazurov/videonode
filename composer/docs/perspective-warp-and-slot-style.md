# Perspective warp and per-slot render properties

## Goal

A camera films the backbox of a pinball machine (scores, ball, DMD
display) at an angle. The user marks four corners on a snapshot of the
live feed — the physical edges of the backglass — and the compositor
corrects the perspective in real time so the content within those
corners fills the layout slot rectangularly, as if viewed head-on.

This is keystoning correction, not artistic distortion. The transform
is a projective warp (3x3 homography), not affine — parallel lines in
the source converge toward a vanishing point, and the correction must
undo that convergence. An affine (2x2) matrix cannot represent this.

Beyond the warp itself, the compositor needs a general mechanism for
per-slot render properties — source crop, color adjustment, corner
rounding, background color — threaded through the full stack (REST API,
gRPC, C++ renderer) so the control surface stays consistent even when
the UI only exposes a subset.

## Constraints

These are derived from the codebase and the intended deployment:

| Constraint | Value | Source |
|---|---|---|
| Canvas frame rate | up to 60 fps (16.6 ms budget for all sources) | `DefaultCanvasFPS = 60` in `pipeline/composer.go` |
| Canvas resolution | typically 1920x1080, max 16384x16384 | `kMaxDim` in `world.cpp` |
| Source resolution | HDMI capture, typically 1080p | V4L2 negotiation |
| GPU (rig) | Mali-G610 via Panthor (GLES 3.2, Vulkan) | RK3588 SBC |
| GPU (dev) | radeonsi / RADV (GL, Vulkan) | Fedora workstation |
| Warped sources per frame | 1 typical, 2 max | Use case: one pinball camera |
| Warp magnitude | moderate keystone, not extreme fisheye | Backbox at ~30-45 deg |
| Matrix update frequency | static until user re-picks corners | Not animated |
| Color pipeline | BT.601 SDR, no HDR/ICC | Current pipeline |
| libplacebo version | 7.351.0 | `pkg-config --modversion libplacebo` |
| Backend abstraction | libplacebo's `pl_shader_custom` / `pl_dispatch` abstracts GL vs Vulkan | Shader code is backend-agnostic |

## What already exists

The infrastructure is 80% wired. The gap is the last mile — applying the
matrix in the GPU shader.

**Working today:**
- `homography.cpp` — CPU-side solver: 4 corner points + snapshot dims →
  3x3 row-major float matrix. Maps destination UV → source UV (the
  correct convention for a backward-mapping fragment shader). Validated
  with unit tests including identity, known keystone, round-trip, and
  degenerate rejection.
- `SourceState.warp[9]` — the solved matrix is cached per source in
  `world.cpp` via `recompute_warp()`. Updated atomically when
  `SetEffects` RPC arrives.
- `SourceSlot.warp` — `canvas_loop.cpp:build_render_slots_()` copies the
  matrix from the world snapshot to the per-slot render input struct.
  Guarded by source state (no warp when placeholder/NO-SIGNAL).
- REST API + gRPC — `PATCH /api/composers/{id}/inputs/{ref}/effect` with
  `type: "perspective"` and 4 corner coordinates flows end-to-end from
  the browser UI through the Go service layer, TOML persistence, and
  gRPC `SetEffects` RPC to the C++ binary.
- Frontend — interactive `PerspectiveCanvas` component for picking
  corners on a source snapshot.

**Not working today:**
- `pl_compose.cpp:283` — `// TODO: per-source warp via pl_shader_custom
  when warp != identity`. The matrix arrives at `render()` but is never
  applied. `pl_render_image()` runs unconditionally with no warp.

## Part 1: Perspective warp

### Why `pl_distort_params` is not enough

`pl_render_params.distort_params` carries a `pl_transform2x2` — a 2x2
linear matrix plus a 2D translation offset. This is strictly affine: it
can express rotation, scale, shear, and translate, but not the
projective terms (the third row of a 3x3 homography) that make parallel
lines converge. Keystoning requires those terms. Confirmed by inspecting
`libplacebo/shaders/sampling.h` — no homogeneous divide in the distort
path.

### Approach: inline `pl_dispatch` for warped slots

The recommended approach, validated against libplacebo's API surface and
confirmed by how other production compositors (kwin, GStreamer
`gltransformation`, OBS Corner Pin) solve this:

For each source slot in `pl_compose::render()`:

1. **Identity warp** — use the existing `pl_render_image()` path
   unchanged. No overhead.

2. **Non-identity warp** — bypass `pl_render_image()` for this slot.
   Instead, use `pl_dispatch` (already created in `PlCompose::Impl`) to
   run a custom fragment shader that does the homography warp + NV12→RGB
   conversion in a single pass, writing directly to the canvas region.

This avoids intermediate textures and keeps the per-slot cost to one GPU
dispatch — the same as the current non-warped path.

### The shader

The fragment shader performs inverse homography mapping: for each output
pixel, compute where it came from in the source, sample there.

```glsl
// Uniforms
uniform mat3 u_warp;        // 3x3 row-major: dest UV → source UV
uniform sampler2D u_tex_y;  // NV12 Y plane
uniform sampler2D u_tex_uv; // NV12 UV plane (half-res, RG8)

// BT.601 limited-range decode matrix (same as current pipeline)
const mat3 yuv2rgb = mat3(
    1.164,  1.164, 1.164,
    0.0,   -0.392, 2.017,
    1.596, -0.813, 0.0
);
const vec3 yuv_offset = vec3(16.0/255.0, 128.0/255.0, 128.0/255.0);

void main() {
    // dest_uv is in [0,1] within the slot rectangle
    vec2 dest_uv = ...; // derived from gl_FragCoord relative to slot rect

    // Projective warp: dest UV → source UV with homogeneous divide
    vec3 p = u_warp * vec3(dest_uv, 1.0);
    vec2 src_uv = p.xy / p.z;

    // Analytical Jacobian for textureGrad (see derivation below)
    float inv_w2 = 1.0 / (p.z * p.z);
    vec2 duv_ds = vec2(
        (u_warp[0][0] * p.z - u_warp[2][0] * p.x) * inv_w2,
        (u_warp[1][0] * p.z - u_warp[2][0] * p.y) * inv_w2
    );
    vec2 duv_dt = vec2(
        (u_warp[0][1] * p.z - u_warp[2][1] * p.x) * inv_w2,
        (u_warp[1][1] * p.z - u_warp[2][1] * p.y) * inv_w2
    );

    // Sample NV12 with anisotropic filtering via explicit gradients
    float y  = textureGrad(u_tex_y,  src_uv, duv_ds, duv_dt).r;
    vec2  uv = textureGrad(u_tex_uv, src_uv, duv_ds * 0.5, duv_dt * 0.5).rg;
    // (UV plane is half-res, so gradients scale by 0.5)

    vec3 yuv = vec3(y, uv) - yuv_offset;
    vec4 rgba = vec4(yuv2rgb * yuv, 1.0);
    // clamp to [0,1] for limited-range overshoot
    color = clamp(rgba, 0.0, 1.0);
}
```

### Why analytical Jacobian, not finite differences

The naive approach computes gradients by evaluating the warp at
neighboring texcoords:

```glsl
vec2 uv_dx = warp(texcoord + vec2(epsilon, 0)) - warp(texcoord);
vec2 uv_dy = warp(texcoord + vec2(0, epsilon)) - warp(texcoord);
```

This works for moderate warps but degrades near the vanishing point
(where the homography's nonlinearity is strongest) because it's a
first-order finite difference. The analytical form uses the quotient
rule on `u = p.x/p.z, v = p.y/p.z` to get exact partial derivatives at
every fragment:

```
du/ds = (H[0][0] * w - H[2][0] * x) / w^2
dv/ds = (H[1][0] * w - H[2][0] * y) / w^2
du/dt = (H[0][1] * w - H[2][1] * x) / w^2
dv/dt = (H[1][1] * w - H[2][1] * y) / w^2
```

where `[x, y, w] = H * [s, t, 1]`. This is exact, cheaper (no second
warp evaluation), and works correctly in divergent control flow.

### Mipmaps are required for quality

`textureGrad` computes an LOD from the supplied gradients and uses it to
select between mipmap levels. Without mipmaps, the GPU ignores the LOD
and falls through to plain bilinear — the analytical Jacobian math is
computed then silently discarded.

For a live video compositor the source texture changes every frame, so
mipmaps must be regenerated per frame. In libplacebo this means:

- Create the source `pl_tex` with mipmap support (if the API allows
  specifying mip levels on import — needs verification against
  `pl_tex_params` for dma-buf imported textures).
- If dma-buf-imported textures cannot carry mipmaps (likely, since
  dma-bufs are typically single-level), use a two-step approach: import
  the NV12 planes as before, then blit each plane into a mipmapped
  scratch texture using `pl_tex_blit` or `pl_dispatch`, and sample from
  the scratch texture in the warp shader.

The mipmap generation cost for 1080p is typically <1ms on both Mali-G610
and radeonsi. Since only 1-2 slots need warping, this is well within
the 16.6ms frame budget.

**Fallback if mipmaps prove impractical**: For the moderate keystone
angles in the pinball use case (not extreme foreshortening), bilinear
sampling without mipmaps may be visually acceptable. The analytical
Jacobian still helps `textureGrad` position its bilinear samples
correctly even without mip selection. Ship bilinear first, add mipmaps
if aliasing is visible on real content.

### Anisotropic filtering

When the warp compresses the source along one axis (the far edge of the
keystone), the footprint of an output pixel in source space is elongated.
Hardware anisotropic filtering (`EXT_texture_filter_anisotropic`,
universally available) uses the gradient vectors from `textureGrad` to
take multiple samples along the major axis.

- Query `GL_MAX_TEXTURE_MAX_ANISOTROPY_EXT` at init (typically 16x on
  desktop, verify on Mali-G610 Panthor).
- Set max anisotropy on the source texture sampler.
- For moderate keystone (the pinball case), the anisotropy ratio is
  well within 16x. No special handling needed for extreme ratios.

### Edge handling

When source UVs land outside [0, 1] (output pixels that map beyond the
source frame), the sampler's address mode controls behavior:

- `GL_CLAMP_TO_EDGE` — stretches the edge pixel, safe default
- Transparent — requires alpha and blending setup, better UX

Start with clamp-to-edge. If the UI later supports masking the warp
boundary (dimming or hiding pixels outside the quad), switch to
transparent + blend.

### Implementation in `pl_compose.cpp`

The change is localized to the `render()` loop. Pseudocode:

```cpp
for (const auto& slot : slots) {
    // ... existing validation, texture import ...

    if (is_identity(slot.warp)) {
        // Existing path: pl_render_image
        pl_render_image(renderer, &src_frame, &dst_frame, &params);
    } else {
        // Warp path: pl_dispatch + pl_shader_custom
        pl_shader sh = pl_dispatch_begin(dispatch);
        // Bind Y and UV textures as shader descriptors
        // Upload mat3 warp as a shader variable
        // Set body to the homography + NV12→RGB GLSL
        pl_dispatch_finish(dispatch, pl_dispatch_params(
            .shader = &sh,
            .target = canvas_tex,
            .rect = {slot.x, slot.y, slot.x+slot.w, slot.y+slot.h},
        ));
    }

    // ... existing texture cleanup ...
}
```

The `pl_dispatch` object is already created in `PlCompose::Impl`.
The warp matrix is already in `slot.warp.m[9]`. Estimated delta:
~100-120 lines in `pl_compose.cpp`, no new files, no new dependencies.

### What stays unchanged

- `homography.cpp` — solver is correct, no changes
- `world.cpp` — `recompute_warp()` caching is correct
- `canvas_loop.cpp` — warp propagation to `SourceSlot` is correct
- `composer_service.cpp` — gRPC handler is correct
- The entire Go + proto + frontend stack for perspective effects

## Part 2: Per-slot render properties

### Data model

Two new optional sub-objects on `LayoutSlot`, one for source framing and
one for render style:

```
LayoutSlot {
  // Spatial (flat, already exists)
  input, x, y, w, h, rotation

  // Source framing — maps to pl_frame.crop
  crop: {
    x0: float  // normalized 0.0-1.0
    y0: float
    x1: float
    y1: float
  }

  // Render style — maps to pl_render_params fields
  style: {
    corner_rounding: float     // 0.0 (square) to 1.0 (max round)
    background_color: [r,g,b]  // letterbox fill, 0.0-1.0
    brightness: float          // -1.0 to 1.0, 0.0 neutral
    contrast: float            // 0.0+, 1.0 neutral
    saturation: float          // 0.0+, 1.0 neutral
    hue: float                 // radians, 0.0 neutral
    gamma: float               // 0.0+, 1.0 neutral
    temperature: float         // -1.0 to 1.0 (3000K to 10000K)
    upscaler: string           // "bilinear", "bicubic", "lanczos"
    downscaler: string         // same set
  }
}
```

### libplacebo mapping

Validated against libplacebo v7.351.0 source:

| Slot property | libplacebo field | Notes |
|---|---|---|
| `crop` | `src_frame.crop` (`pl_rect2df`) | Source-side zoom/pan. Normalized by source dims. |
| `corner_rounding` | `params.corner_rounding` | Applies to `dst_frame.crop` bounds, not full canvas. |
| `background_color` | `params.background_color[3]` | Fills letterbox bars and `PL_CLEAR_COLOR` border. |
| `brightness` | `params.color_adjustment->brightness` | Folded into YCbCr decode matrix. Zero extra GPU cost. |
| `contrast` | `params.color_adjustment->contrast` | Same — pre-scaling, matrix-folded. |
| `saturation` | `params.color_adjustment->saturation` | Operates in YCbCr domain (perceptually correct). |
| `hue` | `params.color_adjustment->hue` | Rotation around [U,V] subvector. |
| `gamma` | `params.color_adjustment->gamma` | Applied in non-linear space (intentional for aesthetics). |
| `temperature` | `params.color_adjustment->temperature` | Bradford chromatic adaptation, matrix-folded. |
| `upscaler` | `params.upscaler` | Filter kernel pointer. LUT re-upload on switch, not shader recompile. |
| `downscaler` | `params.downscaler` | Same. |

### Per-slot `pl_render_params` is safe

`pl_render_params` can be varied freely on every call to
`pl_render_image` using the same `pl_renderer`. The renderer is
"conceptually (almost) stateless" per the docs. The only caveat is HDR
peak detection state, which is not relevant for SDR.

For slots with different `upscaler`/`downscaler` settings: switching
scalers causes a filter kernel LUT re-upload (not shader recompile). For
2-3 slots this is negligible. If it ever matters, allocate one
`pl_renderer` per slot to keep LUT caches stable.

### `corner_rounding` caveats

`corner_rounding` applies an alpha mask to the slot's `dst_frame.crop`
rectangle. This means:

- The canvas texture must have an alpha channel (ARGB8888, which it
  already is).
- Blending must be enabled for slots after the first, so the rounded
  corners compose correctly over the background/prior slots.
- The current pipeline clears the canvas to black before compositing.
  For rounded corners to show the black canvas through the transparent
  corners, the second+ slot renders need `background = PL_CLEAR_SKIP`
  and `blend_params` set to Porter-Duff "over".

### Blending for compositing

Currently each slot overwrites its canvas region. To support
`corner_rounding` and future opacity/alpha:

- First slot: `background = PL_CLEAR_COLOR`, `blend_params = NULL`
- Subsequent slots: `background = PL_CLEAR_SKIP`, `blend_params` with
  standard premultiplied-alpha "over" blend factors:
  ```
  src_rgb   = PL_BLEND_ONE
  src_alpha = PL_BLEND_ONE
  dst_rgb   = PL_BLEND_ONE_MINUS_SRC_ALPHA
  dst_alpha = PL_BLEND_ONE_MINUS_SRC_ALPHA
  ```
- Requires `PL_FMT_CAP_BLENDABLE` on the canvas texture format.

### Warp + style interaction

For slots with both perspective warp and style properties:

- `crop` and `color_adjustment` can be folded into the warp shader (crop
  adjusts the UV range before the homography, color adjustment is a
  matrix multiply after NV12→RGB decode).
- `corner_rounding` must be applied as a post-warp alpha mask — either
  in the same shader or as a separate `pl_render_image` pass after the
  warp dispatch.
- For v1: apply style properties only to non-warped slots via
  `pl_render_params`. Warped slots get the warp shader only. Extend
  later if needed.

## Part 3: API surface

### Proto messages

```protobuf
message SourceCrop {
  float x0 = 1;  // normalized 0.0-1.0
  float y0 = 2;
  float x1 = 3;
  float y1 = 4;
}

message SlotStyle {
  float corner_rounding = 1;
  float bg_r = 2;
  float bg_g = 3;
  float bg_b = 4;
  float brightness = 5;
  float contrast = 6;
  float saturation = 7;
  float hue = 8;
  float gamma = 9;
  float temperature = 10;
  string upscaler = 11;
  string downscaler = 12;
}

message LayoutSlot {
  string slot = 1;
  int32 x = 2;
  int32 y = 3;
  int32 w = 4;
  int32 h = 5;
  int32 rotation = 6;        // already added
  SourceCrop crop = 7;       // optional
  SlotStyle style = 8;       // optional
}
```

All fields have proto3 zero-value defaults that mean "no override"
(0.0 corner rounding = square, 1.0 contrast = neutral, empty string
upscaler = use default). Backward-compatible: old binaries ignore
fields 7-8, new binaries reading old messages get zero defaults.

### Go types

```go
type SourceCrop struct {
    X0 float32 `toml:"x0,omitempty" json:"x0,omitempty"`
    Y0 float32 `toml:"y0,omitempty" json:"y0,omitempty"`
    X1 float32 `toml:"x1,omitempty" json:"x1,omitempty"`
    Y1 float32 `toml:"y1,omitempty" json:"y1,omitempty"`
}

type SlotStyle struct {
    CornerRounding float32 `toml:"corner_rounding,omitempty" json:"corner_rounding,omitempty"`
    BackgroundColor [3]float32 `toml:"background_color,omitempty" json:"background_color,omitempty"`
    Brightness     float32 `toml:"brightness,omitempty" json:"brightness,omitempty"`
    Contrast       float32 `toml:"contrast,omitempty" json:"contrast,omitempty"`
    Saturation     float32 `toml:"saturation,omitempty" json:"saturation,omitempty"`
    Hue            float32 `toml:"hue,omitempty" json:"hue,omitempty"`
    Gamma          float32 `toml:"gamma,omitempty" json:"gamma,omitempty"`
    Temperature    float32 `toml:"temperature,omitempty" json:"temperature,omitempty"`
    Upscaler       string  `toml:"upscaler,omitempty" json:"upscaler,omitempty"`
    Downscaler     string  `toml:"downscaler,omitempty" json:"downscaler,omitempty"`
}

type LayoutSlot struct {
    Input    string      `toml:"input" json:"input"`
    X        int         `toml:"x" json:"x"`
    Y        int         `toml:"y" json:"y"`
    W        int         `toml:"w" json:"w"`
    H        int         `toml:"h" json:"h"`
    Rotation int         `toml:"rotation,omitempty" json:"rotation,omitempty"`
    Crop     *SourceCrop `toml:"crop,omitempty" json:"crop,omitempty"`
    Style    *SlotStyle  `toml:"style,omitempty" json:"style,omitempty"`
}
```

Pointer fields with `omitempty` — nil means no override. Existing TOML
configs without `crop`/`style` deserialize with nil (no migration
needed). Threaded through the same layers as rotation:
`models → service → pipeline → pipelinectl → manager → proto → C++`.

### UI exposure

Thread everything through the API and gRPC for programmatic consumers.
The frontend exposes a subset:

**Exposed in UI:**
- `rotation` (already done — Select dropdown in LayoutSlotInspector)
- `crop` (source zoom/pan — interactive drag on source preview)
- `corner_rounding` (slider)
- `brightness`, `contrast`, `saturation` (sliders)

**API/gRPC only (not in UI initially):**
- `hue`, `gamma`, `temperature`
- `upscaler`, `downscaler`
- `background_color`

## Implementation order

1. **Perspective warp shader** — wire up the TODO in `pl_compose.cpp`.
   This is the highest-value change: makes the existing perspective UI
   actually produce visible results.
2. **Slot style threading** — add `crop` and `style` to the proto/Go/C++
   data model. Start with `corner_rounding` and `color_adjustment` in
   the renderer since they're trivial to map.
3. **Mipmap support** — add per-frame mipmap generation for warped
   source textures to eliminate aliasing in foreshortened regions.
4. **UI controls** — crop editor, rounding slider, color sliders.
