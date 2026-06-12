# AGENTS.md

This file provides guidance for agentic coding agents working with this Go-based video streaming service.

## Build/Test Commands

### Backend (Go)

- **Build**: `go build -o videonode .`
- **Test all**: `go test ./...`
- **Test single package**: `go test ./internal/ffmpeg`
- **Test with verbose**: `go test -v ./internal/ffmpeg`
- **Lint**: `./bin/custom-gcl run ./...` (if results look stale, clear the cache first: `./bin/custom-gcl cache clean`)
- **Lint & fix**: `./bin/custom-gcl run --fix ./...`
- **Build the linter** (first run, or after `.custom-gcl.yml` changes): `golangci-lint custom` — plain `golangci-lint run` fails with `plugin "commentsize" not found` because `.golangci.yml` requires the commentsize plugin; `golangci-lint custom` bakes it into `./bin/custom-gcl` (gitignored). Your golangci-lint version must match `version:` in `.custom-gcl.yml`.
- **Install deps**: `go mod tidy`
- **Validate encoders**: `./videonode validate-encoders`

### Frontend (React/TypeScript)

- **Install deps**: `cd ui && pnpm install`
- **Dev server**: **!ASSUME ITS RUNNING!** (unless you're in a worktree — worktrees don't share the host's dev server; run `pnpm dev` yourself if you need it)
- **Build**: `cd ui && pnpm build`
- **Lint & fix**: `cd ui && pnpm lint:fix`
- **Type check**: `cd ui && pnpm typecheck`

### Native binaries (C++, host)

The Go daemon spawns `videonode-source`, `videonode-sink`, and
`videonode-composer` from `~/.local/bin/` on the dev box, and `/usr/bin/` on
the SBC/rig.
`process-compose` / `air` won't see your C++ changes until you install
them there. **Any time you build C++ in `composer/`, also install:**

```bash
# Daily install — RelWithDebInfo. Realistic CPU + readable stack traces.
cmake --preset relwithdebinfo                            # if not configured
cmake --build --preset relwithdebinfo
cmake --install composer/build/relwithdebinfo            # writes to ~/.local/bin
```

Use the `dev` preset (Debug) only when you're actively stepping with
lldb; running daemons against it has ~2× the per-frame CPU because
`-O0` defeats inlining. Sanitizer presets
(`dev-asan`, `dev-tsan`) build into separate dirs; install from those
only when deliberately running the daemon against an instrumented
binary. Verify with `ls -l ~/.local/bin/videonode-{source,sink,composer}`
— mtimes should match the build.


## Code Style Guidelines

### Go Backend

- **Imports**: Standard library first, then third-party, then local packages with blank lines between groups
- **API Models**: Use snake_case for JSON field tags (e.g., `json:"device_path"`)
- **Error Handling**: Return structured errors, use fmt.Errorf for wrapping
- **Naming**: Use Go conventions - PascalCase for exported, camelCase for unexported
- **Types**: Define constants for enums (e.g., `VideoFormat` type with const values)
- **Interfaces**: Keep interfaces small and focused (e.g., `StreamService`)
- **Comments**: Document all exported symbols following Go conventions

### React Frontend

- **TypeScript**: Strict mode enabled, use proper typing
- **Imports**: Use path aliases (@components/_, @routes/_, @/\*)
- **Components**: Functional components with TypeScript interfaces
- **Styling**: Tailwind CSS with cva for component variants
- **State**: Zustand for global state management
- **Unused vars**: Prefix with underscore for ignored parameters

## Testing Guidelines

### Go Testing Idioms

- **Table-driven tests**: Prefer table-driven tests for multiple similar cases
  ```go
  tests := []struct {
      name string
      input string
      want string
  }{
      {"case1", "input1", "output1"},
      {"case2", "input2", "output2"},
  }
  for _, tt := range tests {
      t.Run(tt.name, func(t *testing.T) {
          // test logic
      })
  }
  ```

- **Manual mocks**: Use simple manual mocks over complex frameworks
  - Define small mock structs that implement interfaces
  - Embed interfaces to satisfy contracts, override only needed methods
  - No external mocking libraries (gomock, testify) unless absolutely necessary

- **Interface-based testing**: Design for testability
  - Accept interfaces, return concrete types
  - Define interfaces at point of use, not implementation
  - Keep interfaces small and focused (1-3 methods ideal)

- **Test structure**: Follow standard Go conventions
  - Test files: `*_test.go` in same package
  - Test functions: `Test*` with `*testing.T` parameter
  - Subtests: Use `t.Run()` for logical grouping
  - Helpers: Accept `*testing.T` as first parameter

- **Test naming**: Be descriptive
  - Function: `TestComponentName_Scenario`
  - Table cases: Use descriptive `name` field
  - Example: `TestSysfsController_Set_InvalidType`

- **Assertions**: Use simple comparisons
  - Prefer `if got != want` over assertion libraries
  - Use `t.Errorf()` for failures with clear messages
  - Use `t.Fatal()` only when test cannot continue

- **Keep tests simple**: Test behavior, not implementation
  - Focus on inputs and outputs
  - Avoid brittle tests that break on refactoring
  - Each test should verify one behavior

### Integration Tests

Integration tests require real hardware or long timeouts and are excluded from normal test runs via build tags.

- **Run unit tests only**: `go test ./...`
- **Run with integration tests**: `go test -tags=integration ./...`

Available integration tests:
- `pkg/linuxav/hotplug` - Tests real udev hotplug events (30s timeout, plug/unplug USB device)

## Architecture

### Pipeline model (post pipeline-rip)

Sources, composers, and streams are three independent top-level entities. Each has its own identity, CRUD surface, and lifecycle policy. Streams reference upstream by explicit string ref (`source:<id>` or `composer:<id>`) — there is no monolithic `[[streams]]` table carrying inputs/layout/effects, and there is no implicit "canvas" entity.

- **Source** (`videonode-source`, one per source-id) — captures V4L2 frames (or runs an RPC-driven test pattern when `test_mode = true`), broadcasts NV12 dma-bufs via SCM_RIGHTS to N consumers. **Lifecycle: pipeline-gated.** Sources run when the pipeline master switch is on and stop when it's off. While on, sources stay up until deleted; composers and streams attach/detach without restarting them. CRUD persists config regardless of switch state; on start, everything rehydrates.
- **Composer** (`videonode-composer`, one per composer-id) — reads N source SCM sockets, GLES-composites onto a BGRA canvas, broadcasts the canvas dma-buf via SCM_RIGHTS. **Lifecycle: pipeline-gated.** Layout and per-input effects are live-editable via unary RPCs without restarting the process.
- **Stream** (encoder = `vn-sink | ffmpeg`, one per stream-id) — vn-sink dials the upstream SCM (NV12 from a source or BGRA from a composer) and pipes to ffmpeg. Stream-id is encoder identity end-to-end (RTSP/SRT path, WebRTC peer key, metrics label). **Lifecycle: pipeline-gated, lazy-encoder-on-reader.** The stream's pipeline plan is resident, but the encoder process only spawns when the pipeline switch is on AND a reader (WebRTC/SRT/RTSP) connects, and stops after the last reader disconnects (debounced). The encoder publishes to the daemon's local RTSP relay (`rtsp://localhost:<rtsp_port>/<stream-id>`), hardcoded at the pipeline-build boundary (`buildEncoder` in `pipeline.go`); SRT and WebRTC fan out from there. There is no user-configurable publish list.

User-facing config: three top-level tables — `[[sources]]`, `[[composers]]`, `[[streams]]` — with explicit `upstream = "source:<id>"` or `upstream = "composer:<id>"` references. Streams carry only encoder + audio config; the publish destination is the hardcoded local RTSP relay, not a config field.

### Application Structure
- **CLI Framework**: Uses Huma v2 with humacli for command-line interface and API server
- **API Server**: Huma v2 API with native Go 1.22+ routing, serves RESTful endpoints with OpenAPI documentation at `/docs`
- **Video Capture**: FFmpeg integration for screenshot capture from V4L2 devices with configurable delay
- **Device Detection**: Pure Go V4L2 device detection via `pkg/linuxav/v4l2`
- **Stream Management**: embedded gortsplib RTSP server with native pion/webrtc signaling, fed by per-stream `vn-sink | ffmpeg` pipelines
- **Native Control Plane**: gRPC over per-instance Unix sockets to the C++ binaries
  (`videonode-source`, `videonode-composer`). Daemon dials each spawned binary,
  calls `Describe()`, then issues unary RPCs (SetFormat / SetCanvas / SetSource /
  SetLayout / SetEffects / SetSourceState / Snapshot / Shutdown) and subscribes
  to `Source.StreamStatus` for the status push. Schemas in `proto/control/*.proto`;
  generated stubs in `internal/streams/pipelinectl/pb/`.
- **Observability**: Built-in metrics collection with Prometheus export and SSE real-time updates

### Key Packages

Use `go doc` or the `mcp__godoc__get_doc` tool to read package documentation:

```bash
# Internal packages
go doc ./internal/api                            # API server: streams.go + sources.go + composers.go handlers
go doc ./internal/streams                        # SourceService / ComposerService / StreamService split
go doc ./internal/streams/store                  # TOML persistence (v2 only; non-v2 rejected on load)
go doc ./internal/streams/pipelinectl            # gRPC client manager for native binaries
go doc ./internal/streams/pipelinectl/pb         # Generated control-plane proto stubs
go doc ./internal/encoders                       # Hardware encoder detection
go doc ./internal/capture                        # Screenshot capture
go doc ./internal/config                         # Configuration loading
go doc ./internal/metrics                        # Metrics collection
go doc ./internal/ffmpeg                         # FFmpeg command building
go doc ./internal/logging                        # Structured logging
go doc ./pkg/linuxav/v4l2                        # V4L2 device detection
go doc ./pkg/linuxav/hotplug                     # USB hotplug monitoring

# External modules (use full import path)
go doc github.com/danielgtaylor/huma/v2          # Huma API framework
go doc github.com/danielgtaylor/huma/v2.Register # Specific symbol
go doc github.com/pelletier/go-toml/v2           # TOML parsing
```

Service-layer split (all live under `internal/streams`):
- `SourceService` — CRUD + lifecycle for `videonode-source` instances (pipeline-gated).
- `ComposerService` — CRUD + live layout/effect edits for `videonode-composer` instances (pipeline-gated).
- `StreamService` — CRUD for encoder/audio config; owns the lazy-encoder-on-reader lifecycle and the pipeline master switch. The publish destination is the hardcoded local RTSP relay, set when the encoder stage is built.

The HTTP surface is implemented in `internal/api/sources.go` and `internal/api/composers.go` (new alongside `streams.go`), with their request/response models in `internal/api/models/`. TOML persistence lives in `internal/streams/store` (see `toml.go`); only `version = 2` is supported and a non-v2 file is rejected on load.

Use `go doc -all <path>` for complete documentation including unexported symbols.

### API Design
- **OpenAPI Documentation**: Automatically generated at `/docs` endpoint
- **Authentication**: All endpoints except `/api/health` require auth — Linux (PAM via the setuid `videonode-session` helper) by default, Basic Auth fallback when the helper is not installed
- **RESTful Design**: Standard HTTP methods and status codes
- **Error Handling**: Structured error responses with Huma v2 error format
- **SSE Support**: Real-time updates via Server-Sent Events at `/api/events/*`

#### Entity endpoints

Three parallel CRUD surfaces, one per top-level entity:

- `/api/sources` — list + create
  - `/api/sources/{source_id}` — get / patch / delete
  - `/api/sources/{source_id}/snapshot.jpg` — latest cached JPEG snapshot from the producer
  - `/api/sources/{source_id}/preview.mjpg` — live multipart MJPEG preview
- `/api/composers` — list + create
  - `/api/composers/{composer_id}` — get / patch / delete
  - `/api/composers/{composer_id}/layout` — PATCH the layout (live edit, no restart)
  - `/api/composers/{composer_id}/inputs/{ref}/effect` — PATCH per-input effect (e.g. perspective)
  - `/api/composers/{composer_id}/snapshot.jpg` — latest cached JPEG snapshot of the canvas
  - `/api/composers/{composer_id}/preview.mjpg` — live multipart MJPEG preview
- `/api/streams` — list + create
  - `/api/streams/{stream_id}` — get / patch / delete

The legacy `/api/streams/canvas/layout` endpoint is gone; canvas layout has moved to `PATCH /api/composers/{composer_id}/layout`. Sources cannot be deleted while a composer or stream references them; composers cannot be deleted while a stream references them — the API returns a structured error listing the blockers.

### Configuration
- **Main Config**: `config.toml` with sections for server, obs, capture, auth, features, and logging
- **Entity Definitions**: `streams.toml` carries the three top-level tables — `[[sources]]`, `[[composers]]`, `[[streams]]` — with `version = 2` at the top.
- **Config version**: only `version = 2` is supported. A non-v2 file is rejected on load with a clear error; the v1→v2 auto-migration and the `migrate-config` command were removed. There is no `publish` config field — the publish destination is hardcoded (any stray entries are ignored on decode).
- **Environment Variables**: All config values can be overridden via env vars (e.g., `VIDEONODE_SERVER_PORT`)

### Debugging & Logging

#### Daemon Logs
- The daemon logs to the system journal via a journal handler (direct `exec.Command`, no `systemd-run`).
- **View logs**: `journalctl --since "1 hour ago" | grep ffmpeg`

#### slog Attributes in journald
slog attributes map to uppercase journal fields (e.g., `STREAM_ID`, `MODULE`).

```bash
journalctl -t videonode -o verbose      # show all fields
journalctl -t videonode -o json | jq '{MESSAGE, STREAM_ID, MODULE}'
journalctl -t videonode STREAM_ID=test  # filter by attribute
journalctl -F STREAM_ID                 # list all values for a field
```

### Device Monitoring
- **Hotplug Support**: udev-based monitoring for USB device insertion/removal via `pkg/linuxav/hotplug`
- **SSE Updates**: Real-time notifications when devices are added/removed
- **V4L2 Integration**: Pure Go V4L2 device detection via `pkg/linuxav/v4l2`

### API Documentation

Full API documentation is available via:
- **Interactive Docs**: http://localhost:8090/docs (Swagger UI)
- **OpenAPI Spec**: http://localhost:8090/openapi.json

The API includes endpoints for:
- Device management and capture
- Hardware encoder detection and validation
- Stream lifecycle management
- Real-time Server-Sent Events

## Development Notes

- **Server is always running via air** on port 8090 with Basic Auth credentials: `videonode:videonode` (unless you're in a worktree — air watches the host checkout, not the worktree; build and run the binary directly there)
- **Health check**: `curl http://localhost:8090/api/health`
- **When writing API models, make sure every field is in snake_case**
- **Run all python commands through uv**
- **Don't be helpful** - do exactly what's asked, nothing more
- **No attribution in commits** - never add Co-Authored-By, Signed-off-by, or similar trailers
- **After making changes**, always run the relevant quality checks:
  - Go (any `.go` change):
    1. `go build ./...`
    2. `go test ./...`
    3. `./bin/custom-gcl run ./...` (build it first with `golangci-lint custom` if missing — see Lint above)
  - TypeScript (any `ui/` change):
    1. `cd ui && pnpm typecheck`
    2. `cd ui && pnpm lint`
  - C++ (any `composer/` change, from `composer/`):
    1. `cmake --build --preset dev`
    2. `ctest --preset dev --output-on-failure`
    3. `cmake --build build/dev --target lint`       # clang-format dry-run
    4. `cmake --build build/dev --target tidy-diff`  # clang-tidy on changed lines vs origin/main
    5. If you added/moved a lib or changed `DEPS`: `cmake -B build/strict -G Ninja -DCMAKE_BUILD_TYPE=Debug -DVN_STRICT_DEPS=ON -DBUILD_TESTS=OFF && cmake --build build/strict`  # under-declared lib deps fail the link
