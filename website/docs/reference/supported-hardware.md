# Supported hardware

VideoNode runs on any Linux host with a GPU and a V4L2 capture device. Hardware acceleration is currently tuned to Rockchip rk35xx; everywhere else it uses portable software backends. This page lists what each pipeline stage needs and what is validated today.

## Support matrix

| Stage | Hardware-accelerated (Rockchip rk35xx today) | Portable software (any Linux GPU host) |
| --- | --- | --- |
| MJPEG decode | MPP (`librockchip-mpp`) | TurboJPEG |
| Color conversion (CSC) | RGA (`librga`) | libplacebo (Vulkan/GL) |
| H.264 / H.265 encode | `h264_rkmpp` / `hevc_rkmpp` | `libx264` / `libx265`, or `*_vaapi` |
| GPU compositing | Mali via the `panthor` DRM render node | any DRM render node libplacebo can open |

The accelerated column is what the packaged build targets today. The software column runs anywhere VideoNode is built from source against a working GPU.

## Validated platform

- **Board:** [FriendlyElec NanoPC-T6](https://www.friendlyelec.com/index.php?route=product/product&product_id=292) (RK3588, 8GB / 64GB eMMC or higher). Other RK3588 boards work, but the encoder paths are tuned and validated against the T6.
- **OS:** Debian 13 (trixie) or newer. The packaged native binaries link trixie-era shared libraries, so `apt` refuses to install on bookworm or older.
- **Architecture:** the APT package is `arm64` only. Other architectures require [building from source](../development/building).

## Minimum requirements

- Linux with a DRM render node (`/dev/dri/renderD*`). The composer needs a GPU that libplacebo or RGA can drive.
- A V4L2 capture device (`/dev/video*`): USB UVC cameras, MIPI-CSI cameras, or HDMI capture cards. Verify with `v4l2-ctl --list-devices`.
- An `ffmpeg` on `PATH` with a working H.264 or H.265 encoder. Run `videonode validate-encoders` to confirm; see [Encoders](../operating/encoders) for the precedence rules.

To install on the validated platform, see [Installing VideoNode](../getting-started/installation).
