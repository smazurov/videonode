# AGENTS.md

This file provides guidance for agentic coding agents working with this Go-based video streaming service.

## Build/Test Commands

### Backend (Go)

- **Build**: `go build -o videonode .`
- **Test all**: `go test ./...`
- **Test single package**: `go test ./internal/ffmpeg`
- **Test with verbose**: `go test -v ./internal/ffmpeg`
- **Lint**: `golangci-lint run ./...`
- **Lint & fix**: `golangci-lint run --fix ./...`
- **Clear lint cache**: `golangci-lint cache clean` (run if you see intermittent false positives - golangci-lint has cache bugs)
- **Install deps**: `go mod tidy`
- **Validate encoders**: `./videonode validate-encoders`

### Frontend (React/TypeScript)

- **Install deps**: `cd ui && pnpm install`
- **Dev server**: **!ASSUME ITS RUNNING!**
- **Build**: `cd ui && pnpm build`
- **Lint & fix**: `cd ui && pnpm lint:fix`
- **Type check**: `cd ui && pnpm typecheck`


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

### Application Structure
- **CLI Framework**: Uses Huma v2 with humacli for command-line interface and API server
- **API Server**: Huma v2 API with native Go 1.22+ routing, serves RESTful endpoints with OpenAPI documentation at `/docs`
- **Video Capture**: FFmpeg integration for screenshot capture from V4L2 devices with configurable delay
- **Device Detection**: Pure Go V4L2 device detection via `pkg/linuxav/v4l2`
- **Stream Management**: go2rtc integration for RTSP/WebRTC streaming with TOML-based configuration
- **Observability**: Built-in metrics collection with Prometheus export and SSE real-time updates

### Key Packages

Use `go doc` or the `mcp__godoc__get_doc` tool to read package documentation:

```bash
go doc ./internal/api          # API server and endpoints
go doc ./internal/streams      # Stream lifecycle management
go doc ./internal/encoders     # Hardware encoder detection
go doc ./internal/capture      # Screenshot capture
go doc ./internal/config       # Configuration loading
go doc ./internal/metrics      # Metrics collection
go doc ./internal/ffmpeg       # FFmpeg command building
go doc ./internal/logging      # Structured logging
go doc ./pkg/linuxav/v4l2      # V4L2 device detection
go doc ./pkg/linuxav/hotplug   # USB hotplug monitoring
```

### API Design
- **OpenAPI Documentation**: Automatically generated at `/docs` endpoint
- **Basic Authentication**: All endpoints except `/api/health` require Basic Auth
- **RESTful Design**: Standard HTTP methods and status codes
- **Error Handling**: Structured error responses with Huma v2 error format
- **SSE Support**: Real-time updates via Server-Sent Events at `/api/events/*`

### Configuration
- **Main Config**: `config.toml` with sections for server, streams, obs, capture, auth, features, and logging
- **Stream Definitions**: `streams.toml` for individual stream configurations
- **Environment Variables**: All config values can be overridden via env vars (e.g., `VIDEONODE_SERVER_PORT`)

### go2rtc Integration
- **Control API**: go2rtc provides a RESTful API on port 1984 for dynamic stream management
- **API Documentation**: https://github.com/AlexxIT/go2rtc/wiki/REST-API
- **Key Endpoints**:
  - `/api/streams`: List and manage streams
  - `/api/ws`: WebSocket for WebRTC signaling
- **Stream Source Options**: `rtsp://`, `rtmp://`, `http://`, `ffmpeg:`, WebRTC, file playback

### Debugging & Logging

#### systemd-run Logs
- **Critical Finding**: `systemd-run --user` logs appear in the **system journal**, NOT the user journal
- Even though the command runs in user systemd, stdout/stderr goes to system journal
- Logs are tagged with parent process (e.g., `go2rtc[PID]`) when go2rtc captures FFmpeg output
- **View logs**: `journalctl --since "1 hour ago" | grep -E "ffmpeg|go2rtc"`
- **NOT**: `journalctl --user` (returns empty/minimal results)
- The `--collect` flag removes the unit after completion, but **logs persist in journald**
- Per systemd docs: "after unloading the unit it cannot be inspected using systemctl status, but its logs are still in journal"

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

- **Server is always running via air** on port 8090 with Basic Auth credentials: `pinball:ilovepinball`
- **Health check**: `curl http://localhost:8090/api/health`
- **When writing API models, make sure every field is in snake_case**
- **Run all python commands through uv**
- **Don't be helpful** - do exactly what's asked, nothing more
- **After making changes**, always run all three checks:
  1. `go build ./...`
  2. `go test ./...`
  3. `golangci-lint run ./...`
