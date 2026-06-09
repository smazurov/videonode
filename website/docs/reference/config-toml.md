# config.toml

```toml
[server]
port = ":8090"

[streams]
config_file = "streams.toml"

[streaming]
rtsp_port = ":8554"

[srt]
enabled = true
addr    = ":6001"
latency = 20

[metrics]
sse_enabled = true

[auth]
type     = "linux"
username = "videonode"
password = "videonode"

[features]
led_control_enabled = false
pprof_enabled       = false
pprof_addr          = "127.0.0.1:6060"

[preview]
max_fps = 10

[native_pipeline]
source   = "~/.local/bin/videonode-source"
sink     = "~/.local/bin/videonode-sink"
composer = "~/.local/bin/videonode-composer"

[logging]
level  = "info"
format = "text"
# Per-module overrides: any key other than level/format is treated as a module name.
streams    = "info"
devices    = "info"
encoder    = "info"
api        = "info"
pipeline   = "info"
webrtc     = "debug"
srt        = "debug"
```

## Environment variable overrides

Every field can be overridden at runtime without editing the file. Prefix the env key from the `env` tag with `VIDEONODE_`:

```bash
VIDEONODE_SERVER_PORT=:8080 videonode
VIDEONODE_AUTH_TYPE=basic videonode
```

The mapping is `VIDEONODE_<ENV_TAG>`, where the env tag corresponds to the section and key (`SERVER_PORT` → `server.port`, `AUTH_TYPE` → `auth.type`). Precedence order: CLI flag > environment variable > config file.

## `[server]`

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `port` | string | `":8090"` | TCP address the HTTP server binds to. |

## `[streams]`

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `config_file` | string | `"streams.toml"` | Path to the entity definitions file containing `[[sources]]`, `[[composers]]`, and `[[streams]]` tables. |

## `[streaming]`

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `rtsp_port` | string | `":8554"` | TCP address the embedded RTSP server binds to. |

## `[srt]`

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `enabled` | bool | `true` | Enable the SRT server. |
| `addr` | string | `":6001"` | UDP address the SRT server binds to. |
| `latency` | int | `20` | SRT latency in milliseconds. |

## `[metrics]`

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `sse_enabled` | bool | `true` | Enable periodic metrics publishing over SSE. The `/api/events` endpoint is always registered; this flag only gates the metrics exporter that pushes to it. |

## `[auth]`

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `type` | string | `"linux"` | Authentication type. `"linux"` verifies credentials against `/etc/shadow` in pure Go (yescrypt, SHA-256/512, MD5-APR1) and requires the user to be in the `videonode` group; `"basic"` uses the username and password fields below. If `/etc/shadow` is unreadable, the daemon silently falls back to basic auth. |
| `username` | string | `"videonode"` | Basic auth username. Used when `type = "basic"` or when Linux auth is unavailable. |
| `password` | string | `"videonode"` | Basic auth password. Used when `type = "basic"` or when Linux auth is unavailable. |

## `[features]`

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `led_control_enabled` | bool | `false` | Enable LED control. Requires appropriate hardware. |
| `pprof_enabled` | bool | `false` | Enable the `net/http/pprof` debug server. The listener only binds when this is `true`. |
| `pprof_addr` | string | `"127.0.0.1:6060"` | Address the pprof debug server binds to. |

## `[preview]`

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `max_fps` | int | `10` | Upper bound on the frame rate served by `preview.mjpg` snapshot streams. Per-request `?fps` is clamped to `[1, max_fps]`. |

## `[native_pipeline]`

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `source` | string | `"~/.local/bin/videonode-source"` | Path to the `videonode-source` binary. An empty or missing path makes source creation fail; there is no ffmpeg fallback. |
| `sink` | string | `"~/.local/bin/videonode-sink"` | Path to the `videonode-sink` binary. |
| `composer` | string | `"~/.local/bin/videonode-composer"` | Path to the `videonode-composer` binary. |

## `[logging]`

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `level` | string | `"info"` | Global log level. Accepted values: `"debug"`, `"info"`, `"warn"` (or `"warning"`), `"error"`. |
| `format` | string | `"text"` | Log output format. `"text"` for human-readable; `"json"` for structured output. |
| `<module>` | string | inherits `level` | Any other key is treated as a module-specific level override. Module loggers are created on first use, so the set is not fixed; common names include `api`, `auth`, `devices`, `pipeline`, `pipelinectl`, `producer`, `composer`, `encoder`, `snapshots`, `srt`, `streams`, and `webrtc`. |
