//go:build linux && (amd64 || arm64)

package v4l2

import "syscall"

func makeTimeval(timeoutMs int) *syscall.Timeval {
	return &syscall.Timeval{
		Sec:  int64(timeoutMs / 1000),
		Usec: int64((timeoutMs % 1000) * 1000),
	}
}
