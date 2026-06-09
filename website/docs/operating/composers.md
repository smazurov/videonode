# Compositing sources onto a canvas

A composer reads one or more sources, composites them onto a fixed BGRA canvas, and republishes the canvas as a single upstream that streams can encode. This page covers creating a composer, editing its layout live, applying a per-input perspective effect, and previewing the result. For how composers fit between sources and streams, see [the pipeline model](../reference/pipeline-model). For the composer binary path and other settings, see [config.toml](../reference/config-toml).

All endpoints require Basic Auth. The examples use the default `videonode:videonode` credentials and port `8090`.

## Create a composer

To create a composer with a canvas, inputs, and an initial layout, POST to `/api/composers`:

```bash
curl -u videonode:videonode -X POST http://localhost:8090/api/composers \
  -H 'Content-Type: application/json' \
  -d '{
    "id": "main-scene",
    "canvas": {"w": 1920, "h": 1080, "fps": 60, "background": "#1a1a2e"},
    "inputs": [
      {"ref": "source:hdmi-slides"},
      {"ref": "source:cam-host"}
    ],
    "layout": [
      {"input": "source:hdmi-slides", "x": 0,    "y": 0,   "w": 1920, "h": 1080},
      {"input": "source:cam-host",    "x": 1440, "y": 720, "w": 480,  "h": 360}
    ]
  }'
```

The `canvas.background` field accepts `#RRGGBB` or `#RRGGBBAA` hex; an empty value renders opaque black. Setting `fps` to `0` uses the daemon default of 60. Each `layout[].input` must match an `inputs[].ref` by name. Compositing only runs while the pipeline master switch is on.

## Edit the layout live

To replace the full layout without restarting the composer, PATCH `/api/composers/{id}/layout`. The body is the complete replacement array, validated against the composer's inputs:

```bash
curl -u videonode:videonode -X PATCH \
  http://localhost:8090/api/composers/main-scene/layout \
  -H 'Content-Type: application/json' \
  -d '{
    "layout": [
      {"input": "source:hdmi-slides", "x": 0, "y": 0, "w": 1920, "h": 1080,
       "aspect_ratio_mode": "fit"},
      {"input": "source:cam-host", "x": 1440, "y": 720, "w": 480, "h": 360,
       "rotation": 0}
    ]
  }'
```

Each slot accepts `rotation` (clockwise degrees: `0`, `90`, `180`, or `270`) and `aspect_ratio_mode`:

- `stretch` (default): fills the slot, ignoring the source aspect ratio.
- `fit`: letterboxes or pillarboxes to preserve aspect ratio.
- `crop`: fills the slot and clips the excess.

Slot `x` and `y` may be negative; the off-canvas region is clipped.

### Position a cropped source

When `aspect_ratio_mode` is `crop`, add a `crop` object to choose which part of the source fills the slot:

```bash
{"input": "source:cam-host", "x": 1440, "y": 720, "w": 480, "h": 360,
 "aspect_ratio_mode": "crop",
 "crop": {"x": 0.5, "y": 0.5, "scale": 1.0}}
```

`crop.x` and `crop.y` are normalized offsets (0 to 1, `0.5` is centered). `crop.scale` is an overfill factor (`>= 1.0`, where `1.0` is the minimum fill). `crop` is ignored unless the mode is `crop`.

## Apply a perspective effect

To warp one input into a four-corner quad, PATCH `/api/composers/{id}/inputs/{ref}/effect`. The `ref` path segment matches an `inputs[].ref`:

```bash
curl -u videonode:videonode -X PATCH \
  http://localhost:8090/api/composers/main-scene/inputs/source:hdmi-slides/effect \
  -H 'Content-Type: application/json' \
  -d '{
    "effect": {
      "type": "perspective",
      "corners": [[40, 30], [1880, 60], [1840, 1040], [80, 1010]],
      "snapshot_w": 1920,
      "snapshot_h": 1080
    }
  }'
```

`corners` lists the four points in `[top-left, top-right, bottom-right, bottom-left]` order, expressed in source pixel space. `snapshot_w` and `snapshot_h` declare that pixel space (typically the source's native resolution) so the composer normalizes the corners. `perspective` is the only effect type.

### Clear an effect

To remove the effect from an input, send an explicit `null`:

```bash
curl -u videonode:videonode -X PATCH \
  http://localhost:8090/api/composers/main-scene/inputs/source:hdmi-slides/effect \
  -H 'Content-Type: application/json' \
  -d '{"effect": null}'
```

The `effect` field is required: omitting it returns `400`.

## Preview the canvas

To fetch the latest rendered canvas frame as a JPEG, GET `/api/composers/{id}/snapshot.jpg`:

```bash
curl -u videonode:videonode \
  http://localhost:8090/api/composers/main-scene/snapshot.jpg -o canvas.jpg
```

The response carries an `ETag` of the form `"frame-<n>"`, so a conditional request with `If-None-Match` returns `304` when the frame has not advanced.

To watch a live multipart MJPEG stream of the canvas, GET `/api/composers/{id}/preview.mjpg`. Add `?fps=<n>` to cap the frame rate:

```bash
ffplay "http://videonode:videonode@localhost:8090/api/composers/main-scene/preview.mjpg?fps=10"
```

## Delete a composer

To delete a composer, DELETE `/api/composers/{id}`:

```bash
curl -u videonode:videonode -X DELETE \
  http://localhost:8090/api/composers/main-scene
```

The request returns `409` if any stream still references the composer; the error body lists the blocking stream IDs. Delete or repoint those streams first.
