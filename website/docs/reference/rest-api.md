# REST API

The machine-readable schema lives at [`/openapi.json`](http://localhost:8090/openapi.json); the interactive Swagger UI is at [`/docs`](http://localhost:8090/docs). This page is a navigational index — it does not reproduce field-level schemas.

## Authentication

All endpoints except `/api/health` require HTTP Basic Auth. Default credentials: `videonode` / `videonode`.

## Endpoints

### Health

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/health` | Return `{"status":"ok"}` — no auth required |

### Pipeline

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/pipeline` | Get the pipeline master switch state |
| `POST` | `/api/pipeline/start` | Start all configured sources, composers, and streams |
| `POST` | `/api/pipeline/stop` | Stop all supervised processes |

### Sources

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/sources` | List all sources |
| `POST` | `/api/sources` | Create a source (V4L2 device or test-pattern) |
| `GET` | `/api/sources/{source_id}` | Fetch one source |
| `PATCH` | `/api/sources/{source_id}` | Partially update a source |
| `DELETE` | `/api/sources/{source_id}` | Delete a source (refused with 409 if referenced) |
| `GET` | `/api/sources/{source_id}/snapshot.jpg` | Latest JPEG frame from the source |
| `GET` | `/api/sources/{source_id}/preview.mjpg` | MJPEG live preview stream (`?fps=N` optional) |

### Composers

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/composers` | List all composers |
| `POST` | `/api/composers` | Create a composer |
| `GET` | `/api/composers/{id}` | Fetch one composer |
| `PATCH` | `/api/composers/{id}` | Partially update a composer |
| `DELETE` | `/api/composers/{id}` | Delete a composer (refused with 409 if referenced) |
| `PATCH` | `/api/composers/{id}/layout` | Replace the full layout array (live, no restart) |
| `PATCH` | `/api/composers/{id}/inputs/{ref}/effect` | Set or clear a per-input effect |
| `GET` | `/api/composers/{id}/snapshot.jpg` | Latest JPEG frame from the composer canvas |
| `GET` | `/api/composers/{id}/preview.mjpg` | MJPEG live preview stream (`?fps=N` optional) |

### Streams

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/streams` | List all streams |
| `POST` | `/api/streams` | Create a stream referencing a source or composer |
| `GET` | `/api/streams/{stream_id}` | Fetch one stream |
| `PATCH` | `/api/streams/{stream_id}` | Partially update a stream |
| `DELETE` | `/api/streams/{stream_id}` | Delete a stream |
| `POST` | `/api/streams/{stream_id}/restart` | Stop and restart the encoder process |

### Devices

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/devices` | List detected V4L2 devices |
| `GET` | `/api/devices/{device_id}/formats` | List pixel formats for a device |
| `GET` | `/api/devices/{device_id}/resolutions` | List resolutions for a format |
| `GET` | `/api/devices/{device_id}/framerates` | List frame rates for a resolution |
| `POST` | `/api/devices/{device_id}/format` | Set the V4L2 capture format |

### Events (SSE)

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/events` | Server-Sent Events stream — device, source, composer, and stream lifecycle events; pipeline state; heartbeat every 15 s |

Event types on `/api/events`: `device-discovery`, `source-created`, `source-updated`, `source-deleted`, `source-status`, `composer-created`, `composer-updated`, `composer-deleted`, `composer-layout-changed`, `stream-created`, `stream-updated`, `stream-deleted`, `stream-state-changed`, `pipeline-state-changed`, `entity`, `heartbeat`.

## Examples

Check health (no auth):

```bash
curl http://localhost:8090/api/health
```

Create a source capturing `/dev/video0`:

```bash
curl -u videonode:videonode \
  -X POST http://localhost:8090/api/sources \
  -H 'Content-Type: application/json' \
  -d '{"source_id":"cam-lobby","device":"/dev/video0"}'
```
