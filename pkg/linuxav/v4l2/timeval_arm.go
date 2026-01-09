//go:build linux && arm && !arm64

package v4l2

import "syscall"

func makeTimeval(timeoutMs int) *syscall.Timeval {
	return &syscall.Timeval{
		Sec:  int32(timeoutMs / 1000),
		Usec: int32((timeoutMs % 1000) * 1000),
	}
}
