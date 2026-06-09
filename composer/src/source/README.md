# source/

`videonode-source` orchestration extracted from `bin/videonode_source_main.cpp`.
The binary entry point is ~40 lines of signal wiring; everything else
(capture lifecycle, V4L2 reinit on cable events, format-change handling,
control-plane dispatch, status heartbeats, SCM_RIGHTS broadcast) lives
here.

TUs in one `orchestrator` library:
- `orchestrator.cpp` — `Run()` main loop, format-change dispatch
- `orchestrator_flags.cpp` — ABSL_FLAG argv definitions and parsing into `Args`
- `capture_session.cpp` — `CaptureSession` + `try_open_capture` + V4L2 setup/teardown
- `broadcast.cpp` — `broadcast_nv12`, `broadcast_buffer`, `build_status_proto`, `now_ms`

ctest label: none — this library is exercised end-to-end via `videonode-source`,
not unit-tested in isolation. Unit-test surfaces live in the leaf libraries
(`ipc`, `capture`, `render`).

Invariant: do not pull `bin/` includes here. `source/` is the orchestrator
library; `bin/` is the thin binary entry. Reverse direction only.
