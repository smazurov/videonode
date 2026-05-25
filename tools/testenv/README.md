# testenv

Coordinator for parallel videonode test environments. Hands out
predictable port slots, manages exclusive device leases, and spawns
isolated instances so multiple worktrees can test concurrently.

## Install

```bash
cd tools/testenv
go install -ldflags \
  "-X github.com/smazurov/videonode/tools/testenv/cmd.Version=$(git rev-parse --short HEAD)" .

testenv install --project-dir ~/dev/videonode   # wire skills + hooks + MCP
```

## `.testenv.toml` configuration reference

### `version`

Required. Currently `1`.

### `max_slots`

Maximum number of concurrent environments. Default `9`.
Slots are numbered 1..max_slots; slot 0 is reserved for the
canonical dev daemon on default ports.

### `[ports.<name>]`

Each section defines a named port family with slot-based allocation.

| Field     | Type | Required | Description |
|-----------|------|----------|-------------|
| `base`    | int  | yes      | Port number for slot 0 (the default/dev slot) |
| `step`    | int  | yes      | Increment per slot: slot _i_ gets `base + step * i` |
| `primary` | bool | no       | If `true`, this port's URL is reported by `testenv up` and `testenv list` as the main entry point. At most one port may be primary. If none is marked, the first port in alphabetical order is used. |

Example:

```toml
[ports.http]
base = 8090
step = 10

[ports.rtsp]
base = 8554
step = 10

[ports.srt]
base = 6001
step = 10

[ports.vite]
base = 5173
step = 10
primary = true
```

With this config, slot 1 gets http=8100, rtsp=8564, srt=6011,
vite=5183. The reported URL is `http://localhost:5183` (the
primary port).

Port values are exposed as template variables `${TESTENV_PORT_<NAME>}`
(uppercased) for use in `spawn.*` fields.

### `[spawn]`

Controls how the daemon is built and started.

| Field           | Type   | Required | Description |
|-----------------|--------|----------|-------------|
| `build`         | string | no       | Shell command run before starting. Runs in the worktree with `sh -c`. |
| `command`       | string | yes      | Shell command to start the daemon. Must stay in the foreground. |
| `health_url`    | string | no       | URL to poll until 200 OK. Supports `${TESTENV_*}` expansion. |
| `health_timeout`| string | no       | Duration string (e.g. `"30s"`). Default `"15s"`. |
| `health_auth`   | string | no       | `user:password` for Basic Auth on the health check. |

### `[spawn.env]`

Key-value pairs added to the daemon's environment. Values support
`${TESTENV_*}` template expansion.

```toml
[spawn.env]
VIDEONODE_SERVER_PORT = ":${TESTENV_PORT_HTTP}"
VITE_DEV_PORT = "${TESTENV_PORT_VITE}"
```

### `[[spawn.files]]`

Files created before the build step. Each entry has `path` and
`content`, both supporting `${TESTENV_*}` expansion.

```toml
[[spawn.files]]
path = "${TESTENV_DIR}/streams.toml"
content = """
version = 2
[[sources]]
id = "${TESTENV_ENV_ID}-src"
test_mode = true
"""
```

### `[[hooks.block]]` / `[[hooks.warn]]`

Regex patterns matched against shell commands. Block hooks prevent
execution; warn hooks print a warning.

| Field      | Type   | Description |
|------------|--------|-------------|
| `match`    | string | Regex matched against the command |
| `cwd_match`| string | Optional regex matched against cwd |
| `message`  | string | Shown when the hook fires |

### Template variables

Available in `spawn.build`, `spawn.command`, `spawn.health_url`,
`spawn.env` values, and `spawn.files` paths/content:

| Variable | Description |
|----------|-------------|
| `${TESTENV_SLOT}` | Slot number (1-9) |
| `${TESTENV_ENV_ID}` | Unique env identifier (e.g. `env-a1b2`) |
| `${TESTENV_DIR}` | Per-env data directory |
| `${TESTENV_WORKTREE}` | Absolute path to the worktree |
| `${TESTENV_LOCKS}` | Comma-separated acquired lock IDs |
| `${TESTENV_PORT_<NAME>}` | Port for the named family at this slot |
