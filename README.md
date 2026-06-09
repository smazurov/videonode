# VideoNode

A self-hosted video streaming server for Linux that turns V4L2 capture devices (webcams, HDMI capture cards, etc.) into RTSP, SRT, and WebRTC streams. It runs headless on any Linux host with a GPU; hardware acceleration is tuned for Rockchip RK3588 boards (reference board: the FriendlyElec NanoPC-T6, including the T6 LTS) and likely works on any RK35xx.

VideoNode automatically detects connected capture devices and builds hardware-accelerated FFmpeg pipelines for low-latency streaming.

**Go to <https://mazurov.dev/videonode/> for full documentation.**

## Install

APT (Debian/Ubuntu arm64, recommended for RK3588 SBCs):

```bash
curl -fsSL https://mazurov.dev/videonode/public.key \
  | sudo gpg --dearmor -o /usr/share/keyrings/videonode-archive-keyring.gpg
echo "deb [arch=arm64 signed-by=/usr/share/keyrings/videonode-archive-keyring.gpg] https://mazurov.dev/videonode stable main" \
  | sudo tee /etc/apt/sources.list.d/videonode.list
sudo apt update
sudo apt install videonode
```

The web UI and HTTP API are served at http://localhost:8090 (interactive API docs at `/docs`).

## Features

- V4L2 device detection and hotplug monitoring
- Hardware encoder validation (Rockchip MPP, VAAPI, software libx264/libx265)
- RTSP, SRT, and WebRTC streaming
- GPU compositing of multiple sources onto a shared canvas
- Prometheus metrics and SSE events
