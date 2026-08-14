//go:build linux

package main

import (
	"fmt"
	"os"
	"syscall"
	"unsafe"

	"golang.org/x/sys/unix"
)

// newMockTTYSlave opens a real Linux pty pair through /dev/ptmx and hands
// back only the slave. The mock's own "master" (from MockSolarisStreams.Open
// on "/dev/ptmx") stays a plain temp file — nothing here reads or writes
// through this pty, it only has to be a genuine tty device so that ioctls
// issued against it, such as the TIOCSCTTY that Setctty triggers in
// SolarisPTY.Run(), succeed the way they would on real illumos. The real
// master side is closed immediately once the slave is unlocked; nothing in
// these tests needs it kept open.
func newMockTTYSlave() (*os.File, error) {
	masterFd, err := unix.Open("/dev/ptmx", unix.O_RDWR|unix.O_NOCTTY, 0)
	if err != nil {
		return nil, err
	}
	master := os.NewFile(uintptr(masterFd), "/dev/ptmx")

	var n uint32
	if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, uintptr(masterFd), unix.TIOCGPTN, uintptr(unsafe.Pointer(&n))); errno != 0 {
		master.Close()
		return nil, errno
	}

	var unlock int
	if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, uintptr(masterFd), unix.TIOCSPTLCK, uintptr(unsafe.Pointer(&unlock))); errno != 0 {
		master.Close()
		return nil, errno
	}

	slaveFd, err := unix.Open(fmt.Sprintf("/dev/pts/%d", n), unix.O_RDWR|unix.O_NOCTTY, 0)
	if err != nil {
		master.Close()
		return nil, err
	}

	// The master has to stay open for the process's lifetime: once every
	// master-side fd of a pty closes, the pty hangs up and the slave turns
	// into a lame duck that answers most operations with EIO — including
	// the very ioctl this helper exists to make succeed. Nothing in these
	// tests reads or writes through it, so it is parked here and never
	// explicitly closed; the OS reclaims it when the test binary exits.
	leakedMockPtyMasters = append(leakedMockPtyMasters, master)

	return os.NewFile(uintptr(slaveFd), fmt.Sprintf("/dev/pts/%d", n)), nil
}

// leakedMockPtyMasters keeps the master side of every pty newMockTTYSlave
// opens reachable, so the Go runtime's file finalizer never closes one out
// from under a still-running test.
var leakedMockPtyMasters []*os.File
