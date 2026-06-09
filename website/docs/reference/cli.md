# CLI

The `videonode` binary runs the daemon with no subcommand, and exposes a small set of subcommands for validation, schema export, and entity CRUD. This page lists those subcommands and the flags they share.

## Subcommands

| Command | Purpose | Notable flags |
|---|---|---|
| (none) | Start the daemon (HTTP API, RTSP/SRT/WebRTC, pipeline supervisor) | `-c, --config`, plus every `[section]` override flag |
| `validate-encoders` | Probe H.264 and H.265 encoders and write the results to `streams.toml` | `-q, --quiet`, `--streams-config` |
| `validate-config` | Check `streams.toml` for dangling upstream refs, layout entries pointing at unknown inputs, ID collisions, and source `test_mode`/`device` conflicts | `--streams-config` |
| `openapi` | Dump the OpenAPI spec to stdout | |
| `version` | Print build version and metadata | `--json` |
| `source list\|get\|create\|delete` | CRUD over `/api/sources` | `--api-url`, `--username`, `--password`, `-f, --file` (create) |
| `composer list\|get\|create\|delete` | CRUD over `/api/composers` | `--api-url`, `--username`, `--password`, `-f, --file` (create) |
| `stream list\|get\|create\|delete` | CRUD over `/api/streams` | `--api-url`, `--username`, `--password`, `-f, --file` (create) |

The `source`, `composer`, and `stream` groups are thin REST clients (`cmd/httpclient.go`): each subcommand calls the matching endpoint on a running daemon, so it mirrors the documented [REST API](./rest-api) rather than adding behavior. `create` reads its JSON body from `--file` or stdin; `get` and `delete` take the entity id as a positional argument.

## API-client flags

The CRUD subcommands resolve their target with the same precedence as the rest of the CLI: flag, then environment variable, then default.

| Flag | Environment variable | Default |
|---|---|---|
| `--api-url` | `VIDEONODE_API_URL` | `http://localhost:8090` |
| `--username` | `VIDEONODE_USERNAME` | `videonode` |
| `--password` | `VIDEONODE_PASSWORD` | `videonode` |

For daemon configuration flags and their env overrides, see the [config.toml reference](./config-toml).
