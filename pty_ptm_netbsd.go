//go:build netbsd

package main

import (
	"os"

	"golang.org/x/sys/unix"
)

// NetBSD's struct ptmget has 1024-byte name fields and a different ioctl
// request number (TIOCPTMGET = 0x40287446) than OpenBSD's PTMGET
// (0x40287401, see pty_ptm_openbsd.go) — the two are not interchangeable.
// x/sys/unix already ships the matching Ptmget type and IoctlGetPtmget
// helper for NetBSD, so use those instead of a manual ioctl.
func NewPTY() (*PTY, error) {
	ptmFd, err := unix.Open("/dev/ptm", unix.O_RDWR|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	defer unix.Close(ptmFd)

	pg, err := unix.IoctlGetPtmget(ptmFd, unix.TIOCPTMGET)
	if err != nil {
		return nil, err
	}

	master := os.NewFile(uintptr(pg.Cfd), "/dev/ptmx")
	slave := os.NewFile(uintptr(pg.Sfd), "slave")

	return &PTY{
		Master: master,
		Slave:  slave,
	}, nil
}
