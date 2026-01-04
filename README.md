# VideoNode

Video capture and streaming service for V4L2 devices with hardware encoder support.

## Quick Start

```bash
# Build the V4L2 detector
cd v4l2_detector && ./build.sh && cd ..

# Build and run
go build -o videonode .
./videonode
```

The server runs on port 8090. API documentation is available at http://localhost:8090/docs

## Configuration

Configure via `config.toml` or environment variables. See `config.toml` for all options.

## Features

- V4L2 device detection and capture
- Hardware encoder validation (NVENC, VAAPI, QSV, AMF)
- RTSP/WebRTC streaming via go2rtc
- Real-time device monitoring
- Prometheus metrics export

## Commands

```bash
# Validate hardware encoders
./videonode validate-encoders
```

## API

Full API documentation: http://localhost:8090/docs

Basic auth required for all endpoints except `/api/health`.
