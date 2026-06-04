# Selecting a hardware encoder

This page covers what encoders are available on your system and how to control which one VideoNode uses per stream. All choices are made via the web UI or REST API. For which encoders each platform provides, see [supported hardware](../reference/supported-hardware).

## Run the validator

Before starting the pipeline, run the built-in encoder probe. It encodes a 2-second 640×480 test clip with every compiled encoder and records what works:

```bash
videonode validate-encoders
```

Example output on a Rockchip RK3588 board:

```text
=== RKMPP (Rockchip Media Process Platform) - Hardware acceleration for Rockchip SoCs ===
Testing h264_rkmpp...
h264_rkmpp: ✓ WORKING
Testing hevc_rkmpp...
hevc_rkmpp: ✓ WORKING
# ...

=== VALIDATION SUMMARY ===
H.264 encoders working: 1
  Working: h264_rkmpp
H.265 encoders working: 1
  Working: hevc_rkmpp
```

Results are written to the daemon's state and consulted whenever a stream starts. Re-run after a driver update or after installing a new ffmpeg build.

## How VideoNode picks an encoder

When a stream starts, VideoNode selects the concrete ffmpeg encoder from the saved validation results. If no validation data exists, it falls back to `AutodetectEncoder`, which probes `ffmpeg -encoders` and applies this precedence:

| Priority | H.264 encoder | H.265 encoder |
|---|---|---|
| 1 | `h264_rkmpp` | `hevc_rkmpp` |
| 2 | `libx264` | `libx265` |
| 3 | `h264_vaapi` | `hevc_vaapi` |

The first compiled encoder in the list wins. On a Rockchip board, `h264_rkmpp` is typically the only option because Rockchip's ffmpeg builds omit `libx264`.

## Set the codec for a stream

In the UI, open the stream you want to edit (or create a new one) and set **Codec** to `H.264` or `H.265`. VideoNode resolves the logical codec to the best available concrete encoder from the validation results above.

To do it via the API, see the `POST /api/streams` and `PATCH /api/streams/{id}` operations in [REST API](../reference/rest-api).

## Pin a specific encoder

The stream form has a **Custom encoder args** field, an escape hatch that's spliced verbatim into the ffmpeg command and overrides codec/bitrate when set. Use it only when the auto-selected encoder doesn't meet your requirements. Example value:

```
-c:v h264_vaapi -profile:v high -b:v 8M -g 60
```

The publish target (RTSP path) is still managed by the daemon; you supply only the codec-side flags.
