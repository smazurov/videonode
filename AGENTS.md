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
- **Build V4L2 detector**: `cd v4l2_detector && ./build.sh`
- **Run V4L2 detector**: `cd v4l2_detector/build && ./v4l2_detector`
- **Install deps**: `go mod tidy`
- **Validate encoders**: `./videonode validate-encoders`

### Frontend (React/TypeScript)

- **Install deps**: `cd ui && pnpm install`
- **Dev server**: **!ASSUME ITS RUNNING!**
- **Build**: `cd ui && pnpm build`
- **Lint & fix**: `cd ui && pnpm lint:fix`
- **Type check**: `cd ui && pnpm typecheck`


### SBC Scripts (pyinfra)

- **Install deps**: `cd sbc_scripts && uv sync`
- **Run deployment**: `cd sbc_scripts && uv run pyinfra inventory.py deploys/<script>.py`
- **List hosts**: `cd sbc_scripts && uv run pyinfra inventory.py --list-hosts`

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

### SBC Scripts

- **Python 3.12**: Use modern typed Python with type hints
- **Pyinfra**: Infrastructure automation tool, simpler than Ansible
- **Inventory**: Defined in `inventory.py` with host credentials
- **Deploys**: Individual deployment scripts in `deploys/` directory

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

## Architecture

### Application Structure
- **CLI Framework**: Uses Huma v2 with humacli for command-line interface and API server
- **API Server**: Huma v2 API with native Go 1.22+ routing, serves RESTful endpoints with OpenAPI documentation at `/docs`
- **Video Capture**: FFmpeg integration for screenshot capture from V4L2 devices with configurable delay
- **Device Detection**: Custom v4l2_detector Go package wrapping C library for V4L2 device enumeration
- **Stream Management**: MediaMTX integration for RTSP/WebRTC streaming with TOML-based configuration
- **Observability**: Built-in metrics collection with Prometheus export and SSE real-time updates

### Key Components

#### API Server (`internal/api/`)
- **server.go**: Huma v2 API server with Basic Auth middleware and udev monitoring integration
- **devices.go**: Device listing and capture endpoints with SSE support
- **encoders.go**: Hardware encoder detection and validation endpoints
- **streams.go**: Stream management endpoints (create, update, delete, status)
- **events.go**: SSE endpoints for real-time updates (device hotplug, capture events)
- **models/**: API request/response models with snake_case field naming

#### Legacy Server (`internal/server/`) - DEPRECATED
- **DO NOT MODIFY**: This package is deprecated, reference only for understanding old implementation

#### Core Components

##### Capture (`internal/capture/`)
- **capture.go**: Screenshot capture using FFmpeg with delay support for devices like Elgato
- Supports both immediate capture and delayed capture (for "no signal" detection)
- Returns bytes or saves to file

##### Encoders (`internal/encoders/`)
- **encoders.go**: Core encoder detection and management
- **validation.go**: Hardware encoder validation logic
- **registry.go**: Encoder registry and priority system
- **validation/**: Platform-specific validation implementations (nvenc, vaapi, qsv, amf, etc.)

##### Streams (`internal/streams/`)
- **service.go**: Stream lifecycle management
- **domain.go**: Stream domain models and business logic
- **errors.go**: Stream-specific error types
- Integration with MediaMTX for RTSP/WebRTC streaming

##### Observability (`internal/obs/`)
- **manager.go**: Central observability coordination
- **store.go**: Time-series metrics storage
- **collectors/**: System, FFmpeg, and custom metric collectors
- **exporters/**: Prometheus and SSE exporters for metrics

##### Monitoring (`internal/monitoring/`)
- **udev.go**: USB device hotplug detection via udev
- **socket_listener.go**: Unix socket monitoring for IPC

##### Config (`internal/config/`)
- **config.go**: TOML configuration loading with environment variable support
- **streams.go**: Stream definitions management

#### V4L2 Detector (`v4l2_detector/`)
- **C library**: CMake-based build system for V4L2 device detection
- **Go bindings**: Integration layer between Go and C components
- **Build script**: `build.sh` for compiling the C component

### API Design
- **OpenAPI Documentation**: Automatically generated at `/docs` endpoint
- **Basic Authentication**: All endpoints except `/api/health` require Basic Auth
- **RESTful Design**: Standard HTTP methods and status codes
- **Error Handling**: Structured error responses with Huma v2 error format
- **SSE Support**: Real-time updates via Server-Sent Events at `/api/events/*`

### Configuration
- **Main Config**: `config.toml` with sections for server, streams, obs, encoders, capture, and auth
- **Stream Definitions**: `streams.toml` for individual stream configurations
- **MediaMTX Config**: `mediamtx.yml` for RTSP/WebRTC server settings
- **Environment Variables**: All config values can be overridden via env vars (e.g., `SERVER_PORT`, `AUTH_USERNAME`)

### MediaMTX Integration
- **Control API**: MediaMTX provides a RESTful API on port 9997 for dynamic stream management
- **API Documentation**: https://mediamtx.org/docs/references/control-api
- **OpenAPI Spec**: https://raw.githubusercontent.com/bluenviron/mediamtx/v1.15.5/api/openapi.yaml
- **Key Endpoints**:
  - `/v3/config/paths/add/{name}`: Add new stream path dynamically
  - `/v3/config/paths/patch/{name}`: Update existing stream configuration
  - `/v3/config/paths/delete/{name}`: Remove stream path
  - `/v3/paths/list`: List all active paths and their status
- **Stream Source Options**: `rtsp://`, `rtmp://`, `http://`, `udp://`, `srt://`, WebRTC, file playback
- **Authentication**: Supports internal users, HTTP callbacks, and JWT tokens

### Debugging & Logging

#### systemd-run Logs
- **Critical Finding**: `systemd-run --user` logs appear in the **system journal**, NOT the user journal
- Even though the command runs in user systemd, stdout/stderr goes to system journal
- Logs are tagged with parent process (e.g., `mediamtx[PID]`) when MediaMTX captures FFmpeg output
- **View logs**: `journalctl --since "1 hour ago" | grep -E "ffmpeg|mediamtx"`
- **NOT**: `journalctl --user` (returns empty/minimal results)
- The `--collect` flag removes the unit after completion, but **logs persist in journald**
- Per systemd docs: "after unloading the unit it cannot be inspected using systemctl status, but its logs are still in journal"

### Device Monitoring
- **Hotplug Support**: udev-based monitoring for USB device insertion/removal
- **SSE Updates**: Real-time notifications when devices are added/removed
- **V4L2 Integration**: Direct V4L2 API access via C library for device capabilities

### API Documentation

Full API documentation is available via:
- **Interactive Docs**: http://localhost:8090/docs (Swagger UI)
- **OpenAPI Spec**: http://localhost:8090/openapi.json

The API includes endpoints for:
- Device management and capture
- Hardware encoder detection and validation
- Stream lifecycle management
- Real-time Server-Sent Events

### SBC Scripts (`sbc_scripts/`)
- **Purpose**: Pyinfra deployments for Orange Pi 5 Ultra management
- **Commands**: `cd sbc_scripts && uv run pyinfra inventory.py deploys/<script>.py`
- **Dependencies**: Managed via `uv` with pyproject.toml

## Development Notes

- **Never modify internal/server** - it's deprecated, reference only
- **Server is always running via air** on port 8090 with Basic Auth credentials: `pinball:ilovepinball`
- **Health check**: `curl http://localhost:8090/api/health`
- **When writing API models, make sure every field is in snake_case**
- **Run all python commands through uv**
- **Don't be helpful** - do exactly what's asked, nothing more
