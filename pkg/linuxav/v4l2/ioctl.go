//go:build linux

package v4l2

import (
	"syscall"
	"unsafe"
)

func ioctl(fd int, req uint, arg unsafe.Pointer) error {
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd), uintptr(req), uintptr(arg))
	if errno != 0 {
		return errno
	}
	return nil
}

func open(path string) (int, error) {
	return syscall.Open(path, syscall.O_RDWR|syscall.O_NONBLOCK, 0)
}

func close(fd int) error {
	return syscall.Close(fd)
}
