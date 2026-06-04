# VideoNode

A self-hosted video streaming server for Linux that turns V4L2 capture devices (webcams, HDMI capture cards, etc.) into RTSP and WebRTC streams. Designed for headless operation on single-board computers like Orange Pi and Raspberry Pi.

VideoNode automatically detects connected capture devices, validates available hardware encoders, and generates optimized FFmpeg pipelines for low-latency streaming.

## Installation

### APT (Debian/Ubuntu arm64 — recommended for RK3588 SBCs)

```bash
# Add the repository
curl -fsSL https://mazurov.dev/videonode/gpg.key \
  | sudo gpg --dearmor -o /usr/share/keyrings/videonode.gpg
echo "deb [arch=arm64 signed-by=/usr/share/keyrings/videonode.gpg] https://mazurov.dev/videonode stable main" \
  | sudo tee /etc/apt/sources.list.d/videonode.list

# Install
sudo apt update
sudo apt install videonode
```

The `.deb` package includes the Go daemon, native C++ pipeline binaries
(`videonode-source`, `videonode-sink`, `videonode-composer`), and a
systemd service. It expects the Rockchip hardware stack (ffmpeg with
rkmpp, librga, librockchip-mpp) to be installed separately via
[videonode-sbc-config](https://github.com/smazurov/videonode-sbc-config) —
the installer warns if these are missing.

### Uninstall (legacy script install)

If migrating from a script install to the `.deb` package, first remove the
per-user installation:

```bash
curl -fsSL https://raw.githubusercontent.com/smazurov/videonode/main/uninstall.sh | bash
```

This stops the user service, removes binaries from `~/.local/bin`, and
cleans up the systemd unit. Config files in `~/.config/videonode/` are
preserved — migrate them to `/etc/videonode/` after installing the `.deb`.

## Quick Start (from source)

```bash
go build -o videonode .
./videonode
```

## Servers

- **HTTP API**: http://localhost:8090 (configurable)
- **RTSP**: rtsp://localhost:8554 (configurable)
- **SRT**: srt://localhost:6001 (configurable)
- **API Docs**: http://localhost:8090/docs

## Configuration

- `config.toml` - Main configuration (server, logging, auth, features)
- `streams.toml` - Stream definitions and encoder results
- Environment variables with `VIDEONODE_` prefix override config.toml (e.g., `VIDEONODE_SERVER_PORT=:8080`)

### Example config.toml

```toml
[server]
port = ":8090"

[streams]
config_file = "streams.toml"

[streaming]
rtsp_port = ":8554"

[metrics]
sse_enabled = true

[update]
enabled = true
prerelease = false

[capture]
default_delay_ms = 3000

[auth]
username = "admin"
password = "password"

[features]
led_control_enabled = false

[logging]
level = "info"
format = "text"
# Module-specific levels: streams, streaming, devices, encoders, capture, api, webrtc
```

### Streams (v2 config shape)

VideoNode v2 splits the config into three top-level entities. Each one
is independent and references the others by id:

- **`[[sources]]`** — a frame producer. Either a V4L2 device
  (`device = "usb-1-2"`) or a device-less test pattern (`test_mode = true`).
- **`[[composers]]`** — an optional GLES compositor that reads N sources
  and writes a single BGRA canvas. Multiple streams can share one composer:
  the GPU work runs once, each stream pays only its own encoder cost.
- **`[[streams]]`** — an encoder + audio + publish targets. Each stream
  points at a single `upstream`, either a source (`"source:<id>"`) or a
  composer (`"composer:<id>"`).

Pipeline shape per stream: `source -> [composer] -> encoder -> publish`.
A composer is engaged only when explicitly defined; a stream that points
directly at a source skips the GLES stage.

Minimal example:

```toml
version = 2

[[sources]]
id = "cam-host"
device = "usb-1-2"

[[streams]]
id = "host-solo"
upstream = "source:cam-host"
  [streams.host-solo.encoder]
  codec = "h264"
  bitrate = "4M"
  [[streams.host-solo.publish]]
  type = "rtsp"
  url = "rtsp://localhost:8554/host-solo"
```

For a realistic multi-entity layout (4 sources, 2 composers, 4 streams
including multi-encode of one shared scene), see
[`examples/sources-composers-streams.toml`](examples/sources-composers-streams.toml).

**Upgrading from v1?** v1 configs are migrated automatically on daemon
load (or explicitly via `videonode migrate-config <path>`). See
[`examples/MIGRATION.md`](examples/MIGRATION.md) for what gets converted
and how to revert.

One field-level note:

- `custom_encoder_args` — when non-empty, replaces the daemon-generated encoder argv from `-c:v` onward. The daemon always prepends the input fragment (`vn-sink --socket X | ffmpeg -f yuv4mpegpipe -i pipe:0` for NV12, `-f rawvideo -pix_fmt bgra -s WxH -framerate N -i pipe:0` for BGRA composer output) so user-supplied args can't break the plumbing.

## Playback

### WebRTC

Open `http://localhost:8090` and click on a live stream.

### SRT (lowest latency)

```bash
ffplay -fflags nobuffer -flags low_delay -framedrop -fast \
  -analyzeduration 0 -probesize 32768 -sync ext \
  -i "srt://localhost:6001?streamid=<stream-id>&latency=20000"
```

### RTSP

```bash
ffplay -fflags nobuffer -flags low_delay -framedrop \
  -rtsp_transport tcp rtsp://localhost:8554/<stream-id>
```

## Features

- V4L2 device detection and real-time monitoring (hotplug)
- Hardware encoder validation (NVENC, VAAPI, QSV, AMF)
- RTSP and WebRTC streaming
- Prometheus metrics at `/metrics`
- SSE events for device discovery

## Commands

```bash
# Start server (default)
./videonode

# Validate hardware encoders and save to streams.toml
./videonode validate-encoders

# Run a specific stream process with hot-reload
./videonode stream <stream-id>
```

## Testing

```bash
# Run unit tests
go test ./...

# Run with integration tests (requires hardware, longer timeouts)
go test -tags=integration ./...
```

## API

Full documentation at http://localhost:8090/docs

Basic auth required for all endpoints except `/api/health`, `/api/version`, and `/metrics`.
