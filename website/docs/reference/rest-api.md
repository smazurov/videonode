# REST API

VideoNode exposes a Huma-generated REST API. The authoritative schema is served by the running daemon. This page is a pointer, not a copy.

| Where | What |
|---|---|
| `http://<host>:8090/docs` | Interactive API reference rendered by [Stoplight Elements](https://stoplight.io/open-source/elements) |
| `http://<host>:8090/openapi.json` | Machine-readable OpenAPI 3 description (JSON) |
| `http://<host>:8090/openapi.yaml` | Same description in YAML |

Use those for endpoint shapes, request/response schemas, and authentication details. The schema is always in sync with the daemon you're hitting; this static page would not be.

## Authentication

All endpoints except `/api/health` require either Linux account auth (default) or basic auth, depending on the daemon's `auth.type` setting. See [Installation – Web UI authentication](../getting-started/installation#web-ui-authentication) for the user/group setup, or [config.toml reference](./config-toml#auth) to switch to basic credentials.

## Server-Sent Events

Live state updates are pushed on the `/api/events` SSE channel. See [Observability](../operating/observability#consuming-sse-events) for the channel and event types.
