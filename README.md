# VideoNode

A self-hosted video streaming server for Linux that turns V4L2 capture devices (webcams, HDMI capture cards, etc.) into RTSP and WebRTC streams. Designed for headless operation on single-board computers like Orange Pi and Raspberry Pi.

VideoNode automatically detects connected capture devices, validates available hardware encoders, and generates optimized FFmpeg pipelines for low-latency streaming.

## Quick Start

```bash
go build -o videonode .
./videonode
```

## Servers

- **HTTP API**: http://localhost:8090 (configurable)
- **RTSP**: rtsp://localhost:8554 (configurable)
- **API Docs**: http://localhost:8090/docs

## Configuration

- `config.toml` - Main configuration (server, logging, auth, features)
- `streams.toml` - Stream definitions and encoder results
- Environment variables with `VIDEONODE_` prefix override config.toml

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
