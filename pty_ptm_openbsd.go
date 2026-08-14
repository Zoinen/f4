//go:build openbsd

package main

import (
	"os"
	"syscall"
	"unsafe"

	"golang.org/x/sys/unix"
)

// OpenBSD's struct ptmget (see /usr/include/sys/ioctl.h) has 16-byte name
// fields, giving _IOR('t', 1, sizeof(ptmget)) = 0x40287401. This differs
// from NetBSD's TIOCPTMGET (0x40287446), whose struct ptmget has 1024-byte
// name fields — using either constant with the other OS's struct layout
// reads/writes past the end of pg and misinterprets the reply. See
// pty_ptm_netbsd.go, and PORTABILITY_BSD.md, 4.5.
//
// x/sys/unix has no Ptmget type or IoctlGetPtmget for OpenBSD (only for
// NetBSD), so this stays a manual ioctl.
func NewPTY() (*PTY, error) {
	ptmFd, err := unix.Open("/dev/ptm", unix.O_RDWR|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	defer unix.Close(ptmFd)

	type ptmget struct {
		Cfd int32
		Sfd int32
		Cn  [16]byte
		Sn  [16]byte
	}

	var pg ptmget
	const openbsdPTMGET = 0x40287401

	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, uintptr(ptmFd), openbsdPTMGET, uintptr(unsafe.Pointer(&pg)))
	if errno != 0 {
		return nil, errno
	}

	master := os.NewFile(uintptr(pg.Cfd), "/dev/ptmx")
	slave := os.NewFile(uintptr(pg.Sfd), "slave")

	return &PTY{
		Master: master,
		Slave:  slave,
	}, nil
}
