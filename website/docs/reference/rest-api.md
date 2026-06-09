# REST API

VideoNode exposes a Huma-generated REST API. The authoritative schema is served by the running daemon. This page is a pointer, not a copy.

| Where | What |
|---|---|
| `http://<host>:8090/docs` | Interactive API reference rendered by [Stoplight Elements](https://stoplight.io/open-source/elements) |
| `http://<host>:8090/openapi.json` | Machine-readable OpenAPI 3 description (JSON) |
| `http://<host>:8090/openapi.yaml` | Same description in YAML |

Use those for endpoint shapes, request/response schemas, and authentication details. The schema is always in sync with the daemon you're hitting; this static page would not be.

## Snapshot and preview endpoints

Each source and composer exposes two image endpoints outside the Huma schema:

| Endpoint | Returns |
|---|---|
| `GET /api/sources/{id}/snapshot.jpg` | Latest cached JPEG frame from the producer (sets an `ETag`) |
| `GET /api/sources/{id}/preview.mjpg` | Live multipart MJPEG stream |
| `GET /api/composers/{id}/snapshot.jpg` | Latest cached JPEG of the canvas |
| `GET /api/composers/{id}/preview.mjpg` | Live multipart MJPEG stream |

## Operational endpoints

The daemon also serves `/api/health`, `/metrics`, `/api/options`, `/api/devices`, `/api/encoders`, `/api/pipeline`, `/api/leds`, `/api/processes`, and `/api/metrics`. Their request and response shapes are described in `/openapi.json`; this page does not duplicate them.

## Authentication

All endpoints except `/api/health` and `/metrics` require either Linux account auth (default) or basic auth, depending on the daemon's `auth.type` setting. See [Installation – Web UI authentication](../getting-started/installation#web-ui-authentication) for the user/group setup, or [config.toml reference](./config-toml#auth) to switch to basic credentials.

## Server-Sent Events

Live state updates are pushed on the `/api/events` SSE channel. See [Events and SSE](../development/events-and-sse) for the event model, or [Observability](../operating/observability#consuming-sse-events) to consume the stream.
