# ipc/

Unix-domain fd passing. dma-buf allocator + SCM_RIGHTS socket transport
for fanning frame fds out from a producer to ≤16 consumers.

ctest label: `ipc`

Invariant: dma-buf fds are passed by SCM_RIGHTS; never `sendmsg` from
outside `ipc/`. Always `dup()` before storing and set `FD_CLOEXEC` on
every export.
