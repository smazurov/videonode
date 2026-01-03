# NATS IPC Research: VideoNode Hub-Spoke Architecture

## Overview

Research findings for implementing inter-process communication between VideoNode (hub) and FFmpeg containers (spokes), with UI streaming capability.

## Why NATS

| Requirement | NATS Solution |
|-------------|---------------|
| Hub ↔ FFmpeg IPC | Embedded server + client in containers |
| UI streaming | WebSocket gateway via nats.ws |
| Bidirectional | Request/reply + pub/sub patterns |
| Same-host deployment | Unix socket or in-process transport |
| 1-5 containers | Lightweight, minimal overhead |

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                     VideoNode (Hub)                         │
│  ┌──────────────┐    ┌──────────────┐    ┌──────────────┐  │
│  │  Event Bus   │───▶│ NATS Server  │◀───│  API Server  │  │
│  │ (kelindar)   │    │  (embedded)  │    │   (Huma)     │  │
│  └──────────────┘    └──────┬───────┘    └──────────────┘  │
│                             │                               │
│                    ┌────────┴────────┐                     │
│                    │   :4222 (TCP)   │                     │
│                    │   :8222 (WS)    │                     │
│                    └────────┬────────┘                     │
└─────────────────────────────┼───────────────────────────────┘
                              │
        ┌─────────────────────┼─────────────────────┐
        │                     │                     │
        ▼                     ▼                     ▼
  ┌───────────┐         ┌───────────┐         ┌───────────┐
  │ FFmpeg 1  │         │ FFmpeg 2  │         │    UI     │
  │(container)│         │(container)│         │ (nats.ws) │
  │   NATS    │         │   NATS    │         │  browser  │
  │  client   │         │  client   │         │  client   │
  └───────────┘         └───────────┘         └───────────┘
```

## Subject Namespace Design

```
videonode.
├── stream.{stream_id}.
│   ├── command        # Hub → Spoke: start, stop, configure
│   ├── status         # Spoke → Hub: state changes (starting, running, error)
│   ├── progress       # Spoke → Hub: frame count, fps, bitrate (1Hz)
│   └── metrics        # Spoke → Hub: detailed metrics (5Hz)
│
├── device.
│   ├── discovered     # Hub broadcasts: new device found
│   └── removed        # Hub broadcasts: device disconnected
│
├── alert.
│   ├── error          # Any: error conditions
│   └── warning        # Any: warning conditions
│
└── system.
    ├── health         # Hub: periodic health status
    └── config         # Hub: configuration changes
```

## Implementation Details

### Embedded NATS Server in VideoNode

```go
import (
    "github.com/nats-io/nats-server/v2/server"
    "github.com/nats-io/nats.go"
)

type NATSHub struct {
    server *server.Server
    conn   *nats.Conn
}

func NewNATSHub(opts *HubOptions) (*NATSHub, error) {
    // Server options
    sopts := &server.Options{
        Host:           "0.0.0.0",
        Port:           4222,
        NoLog:          false,
        NoSigs:         true,
        MaxControlLine: 4096,
        // WebSocket for UI
        Websocket: server.WebsocketOpts{
            Host:  "0.0.0.0",
            Port:  8222,
            NoTLS: true, // TLS in production
        },
    }

    ns, err := server.NewServer(sopts)
    if err != nil {
        return nil, err
    }

    go ns.Start()
    if !ns.ReadyForConnections(5 * time.Second) {
        return nil, errors.New("nats server failed to start")
    }

    // In-process connection (no network hop)
    nc, err := nats.Connect("", nats.InProcessServer(ns))
    if err != nil {
        return nil, err
    }

    return &NATSHub{server: ns, conn: nc}, nil
}
```

### FFmpeg Container Client

```go
import "github.com/nats-io/nats.go"

type FFmpegSpoke struct {
    conn     *nats.Conn
    streamID string
}

func NewFFmpegSpoke(natsURL, streamID string) (*FFmpegSpoke, error) {
    nc, err := nats.Connect(natsURL,
        nats.Name("ffmpeg-"+streamID),
        nats.ReconnectWait(time.Second),
        nats.MaxReconnects(-1), // infinite
    )
    if err != nil {
        return nil, err
    }

    spoke := &FFmpegSpoke{conn: nc, streamID: streamID}

    // Subscribe to commands
    nc.Subscribe(fmt.Sprintf("videonode.stream.%s.command", streamID),
        spoke.handleCommand)

    return spoke, nil
}

func (s *FFmpegSpoke) PublishProgress(p *Progress) error {
    data, _ := json.Marshal(p)
    return s.conn.Publish(
        fmt.Sprintf("videonode.stream.%s.progress", s.streamID),
        data,
    )
}
```

### UI Client (Browser)

```javascript
import { connect } from 'nats.ws';

const nc = await connect({ servers: 'ws://videonode:8222' });

// Subscribe to all stream progress
const sub = nc.subscribe('videonode.stream.*.progress');
for await (const msg of sub) {
    const progress = JSON.parse(new TextDecoder().decode(msg.data));
    updateUI(progress);
}
```

## Message Schemas

### Command (Hub → Spoke)

```go
type StreamCommand struct {
    Action    string          `json:"action"`    // "start", "stop", "configure"
    StreamID  string          `json:"stream_id"`
    Config    *StreamConfig   `json:"config,omitempty"`
    Timestamp time.Time       `json:"timestamp"`
}

type StreamConfig struct {
    Device     string `json:"device"`
    Resolution string `json:"resolution"`
    Framerate  int    `json:"framerate"`
    Codec      string `json:"codec"`
    Bitrate    int    `json:"bitrate"`
    OutputURL  string `json:"output_url"`
}
```

### Status (Spoke → Hub)

```go
type StreamStatus struct {
    StreamID  string       `json:"stream_id"`
    State     StreamState  `json:"state"`  // "starting", "running", "stopped", "error"
    Error     string       `json:"error,omitempty"`
    PID       int          `json:"pid,omitempty"`
    StartedAt *time.Time   `json:"started_at,omitempty"`
    Timestamp time.Time    `json:"timestamp"`
}

type StreamState string

const (
    StateStarting StreamState = "starting"
    StateRunning  StreamState = "running"
    StateStopped  StreamState = "stopped"
    StateError    StreamState = "error"
)
```

### Progress (Spoke → Hub, 1Hz)

```go
type StreamProgress struct {
    StreamID      string    `json:"stream_id"`
    Frame         int64     `json:"frame"`
    FPS           float64   `json:"fps"`
    Bitrate       float64   `json:"bitrate_kbps"`
    TotalSize     int64     `json:"total_size_bytes"`
    DroppedFrames int64     `json:"dropped_frames"`
    Speed         float64   `json:"speed"`
    Timestamp     time.Time `json:"timestamp"`
}
```

## Request/Reply Pattern

For synchronous operations (e.g., get current status):

```go
// Hub requests status
msg, err := nc.Request(
    fmt.Sprintf("videonode.stream.%s.status.get", streamID),
    nil,
    2*time.Second,
)

// Spoke responds
nc.Subscribe(fmt.Sprintf("videonode.stream.%s.status.get", streamID),
    func(msg *nats.Msg) {
        status := getCurrentStatus()
        data, _ := json.Marshal(status)
        msg.Respond(data)
    })
```

## JetStream for Persistence (Optional)

For message persistence and replay:

```go
js, _ := nc.JetStream()

// Create stream for metrics history
js.AddStream(&nats.StreamConfig{
    Name:     "METRICS",
    Subjects: []string{"videonode.stream.*.metrics"},
    Storage:  nats.FileStorage,
    MaxAge:   24 * time.Hour,
})

// UI can replay last N messages on connect
sub, _ := js.Subscribe("videonode.stream.*.metrics",
    handler,
    nats.DeliverLast(), // or DeliverAll(), DeliverNew()
)
```

## Integration with Existing Event Bus

Bridge `kelindar/event` to NATS:

```go
// In internal/events/nats_bridge.go
type NATSBridge struct {
    nc  *nats.Conn
    bus *event.Bus
}

func (b *NATSBridge) Setup() {
    // Forward internal events to NATS
    event.Subscribe[StreamStateChangedEvent](b.bus, func(e StreamStateChangedEvent) {
        data, _ := json.Marshal(e)
        b.nc.Publish(fmt.Sprintf("videonode.stream.%s.status", e.StreamID), data)
    })

    event.Subscribe[StreamMetricsEvent](b.bus, func(e StreamMetricsEvent) {
        data, _ := json.Marshal(e)
        b.nc.Publish(fmt.Sprintf("videonode.stream.%s.metrics", e.StreamID), data)
    })
}
```

## Container Deployment

### Docker Compose

```yaml
services:
  videonode:
    build: .
    ports:
      - "8080:8080"   # API
      - "4222:4222"   # NATS
      - "8222:8222"   # NATS WebSocket
    environment:
      - VIDEONODE_NATS_ENABLED=true

  ffmpeg-stream1:
    build: ./containers/ffmpeg
    environment:
      - NATS_URL=nats://videonode:4222
      - STREAM_ID=stream1
    depends_on:
      - videonode
    devices:
      - /dev/video0:/dev/video0
```

### FFmpeg Container Dockerfile

```dockerfile
FROM golang:1.22-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -o /ffmpeg-spoke ./cmd/ffmpeg-spoke

FROM alpine:3.19
RUN apk add --no-cache ffmpeg
COPY --from=builder /ffmpeg-spoke /usr/local/bin/
ENTRYPOINT ["/usr/local/bin/ffmpeg-spoke"]
```

## Performance Considerations

| Metric | Value | Notes |
|--------|-------|-------|
| Latency (in-process) | ~50µs | Hub to embedded server |
| Latency (same-host TCP) | ~200µs | Container to hub |
| Latency (WebSocket) | ~1-5ms | Browser to hub |
| Throughput | 10M+ msg/sec | Single server |
| Memory (embedded) | ~20-50MB | Depends on connections |
| Binary size impact | ~5MB | Added to videonode |

## Dependencies

```go
// go.mod additions
require (
    github.com/nats-io/nats-server/v2 v2.10.x
    github.com/nats-io/nats.go v1.37.x
)
```

## Security Considerations

1. **Authentication**: NATS supports user/password, token, and NKey authentication
2. **Authorization**: Subject-based permissions (e.g., FFmpeg can only publish to its stream)
3. **TLS**: Enable for WebSocket in production
4. **Network isolation**: Containers on dedicated Docker network

```go
// Example: per-client permissions
sopts := &server.Options{
    Users: []*server.User{
        {
            Username: "ffmpeg-stream1",
            Password: "secret",
            Permissions: &server.Permissions{
                Publish: &server.SubjectPermission{
                    Allow: []string{"videonode.stream.stream1.>"},
                },
                Subscribe: &server.SubjectPermission{
                    Allow: []string{"videonode.stream.stream1.command"},
                },
            },
        },
    },
}
```

## Migration Path

1. **Phase 1**: Add embedded NATS server to VideoNode
2. **Phase 2**: Bridge existing event bus to NATS
3. **Phase 3**: Create FFmpeg spoke container with NATS client
4. **Phase 4**: Add WebSocket endpoint for UI
5. **Phase 5**: Migrate FFmpeg process management to container orchestration

## References

- [NATS Documentation](https://docs.nats.io/)
- [Embedding NATS in Go](https://dev.to/karanpratapsingh/embedding-nats-in-go-19o)
- [nats.ws - Browser Client](https://github.com/nats-io/nats.ws)
- [NATS by Example](https://natsbyexample.com/)
- [Synadia: Embed NATS Server](https://www.synadia.com/screencasts/how-to-embed-nats-server-in-your-app)
