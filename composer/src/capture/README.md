# capture/

V4L2 capture + MJPEG decode + source-health probe. Sits between the
kernel driver and the CSC stage.

ctest label: `capture`

Invariant: V4L2 fds use `O_CLOEXEC`; every mmap region is wrapped in
`std::span<uint8_t>` before crossing back out of capture/. Format
negotiation is fallible — never assume a `set_format` succeeded without
a follow-up `get_format`.
