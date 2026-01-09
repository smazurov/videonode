//go:build linux

package alsa

import (
	"bytes"
	"syscall"
	"unsafe"
)

func ioctl(fd uintptr, req uintptr, arg unsafe.Pointer) error {
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, fd, req, uintptr(arg))
	if errno != 0 {
		return errno
	}
	return nil
}

func cstr(b []byte) string {
	before, _, _ := bytes.Cut(b, []byte{0})
	return string(before)
}
