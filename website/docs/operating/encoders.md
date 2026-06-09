# Selecting a hardware encoder

This page covers what encoders are available on your system and how to control which one VideoNode uses per stream. All choices are made via the web UI or REST API. For which encoders each platform provides, see [supported hardware](../reference/supported-hardware).

## Run the validator

Before starting the pipeline, run the built-in encoder probe. It encodes a 2-second 640×480 test clip with every compiled encoder and records what works:

```bash
videonode validate-encoders
```

To suppress the per-probe progress and print only the summary, add `--quiet` (`-q`):

```bash
videonode validate-encoders --quiet
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

## What the validator checks

For each compiled encoder the validator runs a real ffmpeg encode of the 2-second clip and records `WORKING` only if ffmpeg exits 0 and the output file exceeds 1 KB. It applies that encoder's production rate-control settings during the probe (rkmpp uses `rc_mode=VBR`, vaapi uses `qp=20`, software uses `crf` with `preset=ultrafast`), so a backend that compiles but can't actually run on this host fails here instead of at stream start.

Hardware backends (rkmpp, vaapi) add two more probe sweeps, each gated on the device node being present: rkmpp needs `/proc/mpp_service/load`, vaapi needs `/dev/dri/renderD128`. If the node is absent, that backend's probes are skipped entirely.

- **Decoder probes** feed a synthetic encoded stream through hardware decode for `h264`, `hevc`, and `mjpeg`. The mjpeg probe uses 4:2:0 on rkmpp (its decoder rejects 4:2:2) and 4:2:2 on vaapi (to catch the radeonsi VPP gap).
- **Filter probes** exercise the exact filters the canvas builder emits: scale (NV12 and BGRA output), transpose, and BGRA-on-YUV overlay. The vaapi sweep adds an alignment probe that encodes red at a height where `h % 16 = 8` and checks the decoded bottom row, catching the AMD VCN green-row defect.

Each backend's working and failed decoders and filters land in the summary under `Hardware backends`.

The rate-control matrix the running encoders accept differs per backend:

| Mode | rkmpp | vaapi | software (libx264/libx265) |
| --- | --- | --- | --- |
| CBR | `rc_mode=CBR`, `b:v` | `rc_mode=CBR`, `b:v`, `bufsize` | `b:v`=`minrate`=`maxrate`, `bufsize` |
| VBR | `rc_mode=VBR`, `b:v`, `minrate`, `maxrate` | `rc_mode=VBR`, `b:v`, `maxrate`, `bufsize` | `b:v`, `maxrate`, `bufsize` |
| CRF | mapped to CQP (`qp_init`) | rejected (use CQP) | `crf` |
| CQP | `rc_mode=CQP`, `qp_init` | `rc_mode=CQP`, `qp` | `qp` |

rkmpp has no CRF, so a CRF request is carried as CQP through `qp_init`. vaapi rejects CRF outright and returns an error telling you to use CQP.

## How VideoNode picks an encoder

When a stream starts, VideoNode selects the concrete ffmpeg encoder from the saved validation results. If no validation data exists, it falls back to software: `libx264` for H.264 and `libx265` for H.265. This fallback does not probe `ffmpeg -encoders` and never selects a hardware encoder, so run the validator and restart the daemon before you expect a hardware encoder to be chosen automatically.

## List available encoders over the API

`GET /api/encoders` returns the validated encoders the UI offers, split into `video_encoders` and `audio_encoders` with a `count`. It needs validation data: if you haven't run the validator, it returns `400` telling you to validate first.

The video list is exactly the encoders that passed validation. Each entry carries an `hwaccel` flag set by a name heuristic: an encoder name containing `lib` is treated as software, anything else as hardware-accelerated. The audio list is hardcoded, not validated: `aac`, `libopus`, `libmp3lame`, and `ac3`, in that order, all reported as software.

```bash
curl -u videonode:videonode http://localhost:8090/api/encoders
```

## Set the codec for a stream

In the UI, open the stream you want to edit (or create a new one) and set **Codec** to `H.264` or `H.265`. VideoNode resolves the logical codec to the best available concrete encoder from the validation results above.

To do it via the API, see the `POST /api/streams` and `PATCH /api/streams/{stream_id}` operations in [REST API](../reference/rest-api).

## Pin a specific encoder

The stream form has a **Custom encoder args** field, an escape hatch that's spliced verbatim into the ffmpeg command and overrides codec/bitrate when set. Use it only when the auto-selected encoder doesn't meet your requirements. Example value:

```
-c:v h264_vaapi -profile:v high -b:v 8M -g 60 -f rtsp rtsp://localhost:8554/cam-lobby
```

Custom args replace the entire encoder side of the command. VideoNode supplies only the ffmpeg input flags; everything from `-c:v` through the output target is yours, including the RTSP publish. To keep the stream reachable over RTSP/SRT/WebRTC, end your args with `-f rtsp rtsp://localhost:8554/<stream_id>`. Without that, the encoder runs but publishes nowhere.

In a config file, this is the `custom_encoder_args` field under `[streams.<stream_id>.encoder]` in `streams.toml`.
