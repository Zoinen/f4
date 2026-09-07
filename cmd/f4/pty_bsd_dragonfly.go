//go:build dragonfly

package main

import (
	"fmt"

	"github.com/unxed/vtui"
	"golang.org/x/sys/unix"
)

func openPTYMaster() (int, error) {
	return unix.Open("/dev/ptmx", unix.O_RDWR|unix.O_NOCTTY|unix.O_CLOEXEC, 0)
}

// ptySlaveName names the pts paired with a master opened on /dev/ptmx.
//
// This used to issue TIOCGPTN with a hardcoded 0x4004740f, copied from the
// FreeBSD backend on the assumption that DragonFly spells the ioctl the
// same way. DragonFly does not define it at all: _IOR('t', 15, int) is a
// gap between TIOCNXCL ('t'/14) and TIOCFLUSH ('t'/16), and the only pty
// ioctl it does define is TIOCISPTMASTER. See issue #444, and the same
// mistake fixed earlier for FreeBSD in pty_bsd_freebsd.go.
//
// DragonFly names the master ptm/N and its slave /dev/pts/N, which is how
// its own libc derives one from the other, via fdevname rather than an
// ioctl, because a master there does not even satisfy isatty. The unit
// number is the minor of the master device, so a stat of the master is
// enough and no ioctl is needed.
func ptySlaveName(masterFd int) (string, error) {
	// Reported rather than enforced: a master that fails this check has
	// still given us a device number to work from, and refusing here would
	// turn a bad guess about one ioctl into another dead terminal.
	if err := unix.IoctlSetInt(masterFd, unix.TIOCISPTMASTER, 0); err != nil {
		vtui.DebugLog("PTY: TIOCISPTMASTER on master rejected: %v", err)
	}

	var st unix.Stat_t
	if err := unix.Fstat(masterFd, &st); err != nil {
		return "", err
	}

	unit := unix.Minor(uint64(st.Rdev))
	name := fmt.Sprintf("/dev/pts/%d", unit)
	vtui.DebugLog("PTY: master rdev=%d unit=%d slave=%s", st.Rdev, unit, name)
	return name, nil
}
