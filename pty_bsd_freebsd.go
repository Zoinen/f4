//go:build freebsd

package main

import (
	"fmt"

	"golang.org/x/sys/unix"
)

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
