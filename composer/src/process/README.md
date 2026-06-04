# process/

Child-process spawn/lifecycle + ffmpeg pipe-source wrapper used by the
composer when a `videonode-source` socket isn't available.

ctest label: `process`

Invariant: every spawned child gets `PR_SET_PDEATHSIG`; orphan pids
from crashed parents are unacceptable. Stdout/stderr fds set `FD_CLOEXEC`
before fork.
