package cmd

import "golang.org/x/sys/unix"

// signalDaemon best-effort SIGTERMs a daemon PID and its process group.
func signalDaemon(pid int) {
	if pid <= 0 {
		return
	}
	_ = unix.Kill(-pid, unix.SIGTERM)
	_ = unix.Kill(pid, unix.SIGTERM)
}
