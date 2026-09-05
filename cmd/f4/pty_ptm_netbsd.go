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

	// The O_CLOEXEC above covers only the /dev/ptm handle, which is closed
	// on the way out anyway. TIOCPTMGET hands back two freshly opened
	// descriptors that carry no FD_CLOEXEC of their own, so they must be
	// flagged here: otherwise the shell inherits a copy of its own master,
	// the master never drops to zero references when f4 dies, no SIGHUP is
	// delivered, and the shell survives as an orphan pinning its pty.
	setCloseOnExec([]int{int(pg.Cfd), int(pg.Sfd)})
	// TIOCPTMGET's master descriptor needs the same non-blocking flag as
	// the Linux/BSD paths (see pty_unix.go): otherwise a blocked
	// Master.Read() cannot be interrupted by Close() from another
	// goroutine, and the reader goroutine leaks along with the pty.
	if err := unix.SetNonblock(int(pg.Cfd), true); err != nil {
		unix.Close(int(pg.Cfd))
		unix.Close(int(pg.Sfd))
		return nil, err
	}

	master := os.NewFile(uintptr(pg.Cfd), "/dev/ptmx")
	slave := os.NewFile(uintptr(pg.Sfd), "slave")

	p := &PTY{
		Master: master,
		Slave:  slave,
	}
	registerPTYOpened()
	return p, nil
}
