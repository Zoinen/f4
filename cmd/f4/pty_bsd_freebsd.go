//go:build freebsd

package main

import (
	"fmt"
	"syscall"

	"golang.org/x/sys/unix"
)

// openPTYMaster uses FreeBSD's native posix_openpt(2) system call.  Opening
// /dev/ptmx normally reaches the same kernel allocator, but the device node can
// be hidden by a jail's devfs ruleset even though PTY allocation itself is
// allowed.  SSH and tmux already have their PTYs by then; an f4 started inside
// that session used to be the first program that visibly failed.
func openPTYMaster() (int, error) {
	fd, _, errno := syscall.Syscall(
		syscall.SYS_POSIX_OPENPT,
		uintptr(unix.O_RDWR|unix.O_NOCTTY|unix.O_CLOEXEC),
		0,
		0,
	)
	if errno != 0 {
		return -1, errno
	}
	return int(fd), nil
}

// ptySlaveName asks the master which pts it is paired with. FreeBSD numbers
// its slaves and exposes them as /dev/pts/N, so the number is the whole
// answer; TIOCGPTN is _IOR('t', 15, int) in sys/sys/ttycom.h.
//
// This used to issue TIOCPTYGNAME instead, which is a macOS ioctl
// (_IOC(IOC_OUT, 't', 83, 128)) and does not exist on FreeBSD at all. The
// value that was hardcoded for it, 0x40807448, decodes to 't'/72, a command
// number FreeBSD does not define either, so the call always failed with
// ENOTTY and the terminal never started. See issue #444.
func ptySlaveName(masterFd int) (string, error) {
	n, err := unix.IoctlGetInt(masterFd, unix.TIOCGPTN)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("/dev/pts/%d", n), nil
}
