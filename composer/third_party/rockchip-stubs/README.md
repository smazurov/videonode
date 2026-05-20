# Rockchip stubs (host-side compile only)

These headers and `stubs.c` exist so that composer-spike can **compile** on
a generic Linux dev machine without the real `librga` and `librockchip_mpp`
binaries (which are ARM-only Rockchip libraries with no x86_64 build).

What this gets you:

- The full `composer-spike` binary and all probes build on the dev machine.
- Unit tests covering anything that doesn't actually call into RGA / MPP
  pass on the dev machine.
- IDE indexing, clangd, etc. resolve `#include <rga/im2d.h>` and
  `#include <rockchip/rk_mpi.h>` so we get autocompletion and Go-To-Def.

What this does NOT do:

- Run anything that actually uses RGA or MPP. The stub functions print
  `stub: <name> called on host build (no real Rockchip libs)` to stderr
  and return failure codes. Run those code paths on the rig.

The stubs are gated by CMake: when CMake fails to find the real `librga`
or `librockchip_mpp` in the system, it adds this directory to the
include path and links a tiny `rockchip_stubs` object library in their
place. On the rig the real libs are found and these stubs are never
touched.

If the real headers grow new symbols our code starts using, add them to
the stub headers + `stubs.c` in lock-step. The stubs intentionally cover
only the surface area `composer-spike` actually calls.
