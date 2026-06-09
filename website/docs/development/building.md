# Building from source

This page covers building all three components of VideoNode from source: the Go daemon, the React UI, and the native C++ binaries (`videonode-source`, `videonode-sink`, `videonode-composer`).

## Prerequisites

Install the following before building:

- **Go** 1.25 or later
- **Node.js** 22.15 or later, **pnpm** 10
- **CMake** 3.25 or later for a bare configure (the presets used below require 3.30), **Ninja**, a C++20-capable compiler (GCC 10+ or Clang 10+)

## Building the Go daemon

To produce the `videonode` binary in the repo root:

```bash
go build -o videonode .
```

To verify the build and run tests:

```bash
go test ./...
golangci-lint run ./...
```

Integration tests need real hardware and longer timeouts, so they are gated behind a build tag and excluded from the default run:

```bash
go test -tags=integration ./...
```

## Building the UI

To compile the React frontend into `ui/dist/` (embedded into the daemon at build time):

```bash
cd ui && pnpm install && pnpm build
```

To check types and lint without building:

```bash
cd ui && pnpm typecheck
cd ui && pnpm lint
```

## Building the native binaries

Use the `relwithdebinfo` preset for daily installs: it produces optimized binaries with readable stack traces. The `dev` preset (Debug, unoptimized) is only necessary when stepping through code with a debugger.

To configure, build, and install to `~/.local/bin/`:

```bash
cmake --preset relwithdebinfo
cmake --build --preset relwithdebinfo
cmake --install composer/build/relwithdebinfo
```

The Go daemon looks for `videonode-source`, `videonode-sink`, and `videonode-composer` in `~/.local/bin/` by default. The install step is required for the daemon to pick up your changes.

To run tests and linters against the `dev` preset:

```bash
cmake --preset dev
cmake --build --preset dev
ctest --preset dev --output-on-failure
cmake --build build/dev --target lint
```
