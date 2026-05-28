# Selecting a hardware encoder

This page shows how to discover which encoders work on your system and how to control which one VideoNode uses per stream.

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

Hardware backends:
  rkmpp:
    decoders working: h264, hevc, mjpeg
    filters working:  (none)
```

Results are saved to `streams.toml` and consulted at runtime whenever a stream starts. Re-run after a driver update or after installing a new ffmpeg build.

## How VideoNode picks an encoder

When a stream starts, VideoNode selects the concrete ffmpeg encoder from the saved validation results. If no validation data exists, it falls back to `AutodetectEncoder`, which probes `ffmpeg -encoders` and applies this precedence:

| Priority | H.264 encoder | H.265 encoder |
|---|---|---|
| 1 | `h264_rkmpp` | `hevc_rkmpp` |
| 2 | `libx264` | `libx265` |
| 3 | `h264_vaapi` | `hevc_vaapi` |

The first compiled encoder in the list wins. On a Rockchip board, `h264_rkmpp` is typically the only option because Rockchip's ffmpeg builds omit `libx264`.

## Set the codec for a stream

In `streams.toml`, set `encoder.codec` to choose the logical codec. VideoNode resolves it to the best available concrete encoder from the validation results:

```toml
[[streams]]
id = "main"
upstream = "source:cam-lobby"

[streams.encoder]
codec = "h264"
bitrate = "6M"
```

Valid values for `codec` are `h264` and `h265`. See the [REST API reference](../reference/rest-api) for the full encoder schema, including the live `/openapi.json` and Swagger UI at `/docs`.

## Pin a specific encoder

To bypass the resolver entirely and hand-write the ffmpeg encoding arguments, use `custom_encoder_args`. VideoNode splices this string directly after the ffmpeg input flags:

```toml
[[streams]]
id = "main"
upstream = "source:cam-lobby"
custom_encoder_args = "-c:v h264_vaapi -profile:v high -b:v 8M -g 60 -f rtsp rtsp://localhost:8554/main"
```

Use this only when the auto-selected encoder does not meet your requirements. The full output URL must be included because `custom_encoder_args` replaces everything from `-c:v` onward.
