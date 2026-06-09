# Source inputs and formats

`videonode-source` captures from one V4L2 device and broadcasts NV12 dma-bufs. This page explains the inputs it accepts, how it detects the pixel format, and why every input is normalized to NV12 before broadcast. For the process model and zero-copy transport, see [architecture](./architecture).

## The NV12-only wire contract

Every consumer (`videonode-sink`, `videonode-composer`, and the snapshot endpoint) receives NV12 and nothing else, because a single semi-planar format keeps consumers trivial: one Y plane, one interleaved UV plane, a fixed header shape. `videonode-source` therefore converts whatever the device produces into NV12 before it reaches the SCM_RIGHTS broadcast.

## Input classes

Two device classes reach the capture loop:

- **HDMI-RX bridge** (for example `rk_hdmirx`). Exposes raw frames and supports DV timings plus `V4L2_EVENT_SOURCE_CHANGE`, so resolution and signal changes arrive in-band. Cable and signal truth come from `VIDIOC_QUERY_DV_TIMINGS`, not `V4L2_CID_DV_RX_POWER_PRESENT` (which reports present even with the cable unplugged).
- **UVC** webcams and HDMI-to-USB dongles. Deliver raw frames (YUYV and friends) or MJPEG. They reject the `SOURCE_CHANGE` subscribe by design, and their resolution is fixed for the streaming session, so a change goes through a full device reopen.

A device-less mode (`test_mode`) bypasses capture entirely for hardware-free bring-up. See [test mode](#test-mode-hardware-free-sources).

::: info
HDMI input and MJPEG never coincide. HDMI bridges deliver raw frames (the RGA path); MJPEG comes only from UVC devices. That is why a `SOURCE_CHANGE` event never reaches the MJPEG decode path.
:::

## Format detection

At open, `negotiate_format_` reads `VIDIOC_G_FMT` and picks a decode mode:

- `pixel_format == MJPEG` selects `DecodeMode::Mjpeg`.
- Anything else selects `DecodeMode::Rga`.

## Discovering a device's formats

`GET /api/devices/{device_id}/formats` enumerates what a device advertises, so an operator can pick a capture format before creating a source. The endpoint reports only formats VideoNode recognizes; an advertised format that maps to no known pixel code is skipped rather than failing the listing.

The recognized set is the V4L2 fourcc codes VideoNode maps to lowercase format names: `yuyv422` (YUYV), `nv12`, `h264` (H264), `mjpeg` (MJPG), `yu12`/I420 (YU12), `yv12`, `bgr24` (BGR3), `rgb24` (RGB3), `nv24` (Y/UV 4:4:4), and `nv16` (Y/UV 4:2:2). The lowercase name is what `POST /api/sources` and the source's `SetFormat` RPC expect; the API converts it back to the 4-char fourcc on dispatch. The set a given device returns is a subset: an HDMI bridge advertises raw formats and a webcam advertises `mjpeg` plus its raw modes, never the full list.

## Color matrix

Both decode modes trust the matrix V4L2 reports through `VIDIOC_G_FMT`, because the UVC color-matching descriptor and the `rk_hdmirx` colorspace are more reliable than a per-format assumption. `resolve_matrix` prefers an explicit `ycbcr_enc`, then maps the known `colorspace` values (`REC709`, `SMPTE170M`, the two `470_SYSTEM` variants), and falls back to the SD/HD convention (height >= 720 maps to BT.709) for any other colorspace, `DEFAULT` included. MJPEG used to be hardcoded BT.601, which mislabeled HD webcams that are really BT.709. The opposite case also occurs: a camera that reports explicit ITU-R 601 even at 1080p resolves to BT.601, where the old hardcode happened to agree.

Hardware JPEG decoders can also report the JPEG's own matrix coefficients; MPP exposes this through `mpp_frame_get_colorspace`. When that signal is definite and disagrees with the V4L2 choice, the source adopts it once and logs the change. JFIF rarely carries it, so the V4L2 value usually stands. The software decode path exposes no matrix tag, so it always keeps the V4L2 value. The resolved matrix rides the broadcast header to the composer and the gRPC status to the daemon, where it sets the encoder's color flags and the read-only `color_matrix` field on `/api/sources`.

The pipeline models the matrix but not the quantization range. Every frame is tagged limited range on the wire and in the encoder flags. JPEG is conventionally full range, and V4L2 confirms it through the `Quantization` field, but no consumer reads the range today, so the source discards it. Carrying full range would need matching work in the composer shader and the ffmpeg flags.

## Decode and conversion

videonode runs on any Linux host with a GPU. It uses hardware blocks where the platform provides them (Rockchip's MPP and RGA today) and portable software backends ([libplacebo](https://libplacebo.org/), [TurboJPEG](https://libjpeg-turbo.org/)) everywhere else.

Raw frames take the `DecodeMode::Rga` path: non-NV12 input is color-converted to NV12 by the color-space-conversion (CSC) backend, which is Rockchip's [RGA 2D accelerator](https://github.com/tsukumijima/librga-rockchip) where a hardware accelerator is available, and a libplacebo software backend otherwise.

MJPEG frames take the `DecodeMode::Mjpeg` path and are decoded by one of two backends:

- **[MPP](https://github.com/tsukumijima/mpp-rockchip)** (hardware decode) passes the JPEG's native chroma subsampling straight through, so a 4:2:0 source decodes to NV12, 4:2:2 to NV16, and 4:4:4 to NV24.
- **[TurboJPEG](https://libjpeg-turbo.org/)** (portable software, or when hardware decode init fails) always downconverts to NV12.

Because MPP passes subsampling through, non-4:2:0 sources arrive as NV16 or NV24 and need a normalizing pass.

## Subsampling normalization

| Decoded format | Path | Output | Notes |
| --- | --- | --- | --- |
| NV12 (4:2:0) | passthrough | NV12 | broadcast straight from the MPP pool, zero-copy |
| NV16 (4:2:2) | RGA CSC | NV12 | the MACROSILICON VC-002 dongle; the tested path |
| NV24 (4:4:4) | RGA CSC | NV12 | best-effort chroma; emits a one-shot warning |
| unknown | rejected | none | frame dropped, error logged |

The CSC ring is allocated on the first non-NV12 MJPEG frame, so 4:2:0 cameras stay zero-copy and pay nothing. When conversion cannot run (the backend lacks the subsampling, or the ring fails to allocate), the frame is dropped and the cause is logged once. The source goes black with a log line rather than stalling silently, and nothing non-NV12 ever reaches the wire.

## HDMI signal state

An HDMI bridge has no boolean "connected" flag worth trusting, so VideoNode reads signal truth from how `VIDIOC_QUERY_DV_TIMINGS` (and the configured-timings sibling `VIDIOC_G_DV_TIMINGS`) fails. A success with non-zero width, height, and pixel clock means the signal is locked; otherwise the errno carries the reason. The internal signal state maps as:

| ioctl result | Signal state | Meaning |
| --- | --- | --- |
| success, valid timings | Locked | signal present and stable |
| success, zero timings | NoSignal | cable connected, no usable signal |
| `ENOLINK` | NoLink | no cable |
| `ENOLCK` | Unstable | signal present but unstable |
| `ERANGE` | OutOfRange | signal outside the supported range |
| `ENOTTY` | NotSupported | device exposes no DV timings |
| any other errno | NoSignal | fallback |

That signal state feeds the source's `liveness` field on `/api/sources` (for example `no_cable`, `no_signal`), so a missing or unstable HDMI signal is visible without inspecting the device directly.

A locked result also reports the BT timing fields: `width`, `height`, an `interlaced` flag, and a frame rate computed from the timings rather than read from a register. The FPS formula divides the pixel clock by the total blanked frame size:

```text
fps = pixelclock / (htotal * vtotal)
htotal = width + hfrontporch + hsync + hbackporch
vtotal = height + vfrontporch + vsync + vbackporch   # halved when interlaced
```

This is why an HDMI source can report a precise non-integer rate (for example 59.94) that no fixed-rate assumption would produce.

## Test mode (hardware-free sources)

A source with `test_mode = true` carries no device and runs the `videonode-source` binary without a `--device` argument, so it produces frames with no V4L2 hardware attached. This is the bring-up path on a host with no capture device. The API enforces exact-one semantics: `device` and `test_mode` are mutually exclusive, and the daemon rejects a source that sets both or neither. To switch an existing source, `PATCH /api/sources/{source_id}` toggles `test_mode` and clears `device` by sending it as an empty string. Everything downstream (the NV12 wire contract, SCM_RIGHTS broadcast, snapshot and preview endpoints) is identical to a hardware source.

## Handoff

The resulting NV12 dma-buf broadcasts to consumers over SCM_RIGHTS. See [architecture](./architecture) for the wire format and [pipeline model](../reference/pipeline-model) for source, composer, and stream lifecycles.
