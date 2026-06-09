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

- **Board:** any Rockchip RK3588 board. The reference and recommended board is the [FriendlyElec NanoPC-T6](https://www.friendlyelec.com/index.php?route=product/product&product_id=292) (including the T6 LTS), which the encoder paths are tuned against. Other RK35xx SoCs likely work but are not validated.
- **OS:** Debian 13 (trixie) or newer. The packaged native binaries link trixie-era shared libraries, so `apt` refuses to install on bookworm or older.
- **Architecture:** the APT package is `arm64` only. Other architectures require [building from source](../development/building).

## Minimum requirements

- Linux with a DRM render node (`/dev/dri/renderD*`). The composer needs a GPU that libplacebo or RGA can drive.
- A V4L2 capture device (`/dev/video*`) for live capture: USB UVC cameras, MIPI-CSI cameras, or HDMI capture cards. Verify with `v4l2-ctl --list-devices`. A test-mode source needs no device, so the pipeline can run hardware-free for dev or CI.
- An `ffmpeg` on `PATH` with a working H.264 or H.265 encoder. Run `videonode validate-encoders` to confirm; see [Encoders](../operating/encoders) for the precedence rules.

## LED status indicators

VideoNode can drive board status LEDs over sysfs when `led_control_enabled = true` (see [config.toml reference](./config-toml#features)). With the gate off, the LED routes are not registered and the controller is a no-op.

| Capability | Boards with a built-in driver | LED types exposed |
|---|---|---|
| sysfs LED control | FriendlyElec NanoPC-T6 | `user`, `system` |
| sysfs LED control | Orange Pi | `blue`, `green` |
| sysfs LED control | Raspberry Pi | `act` |

The board is detected from `/proc/device-tree/model`. Any other board falls back to the no-op controller, so the API accepts requests but no LED changes.

When a supported board is present, the API exposes:

- `POST /api/leds`: set a LED's `type`, `enabled` state, and an optional `pattern`.
- `GET /api/leds/capabilities`: list the LED types and patterns this board supports.

The supported patterns are `solid`, `blink`, and `heartbeat`.

To install on the validated platform, see [Installing VideoNode](../getting-started/installation).
