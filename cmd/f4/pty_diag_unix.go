//go:build !windows

package main

import (
	"os"
	"runtime"

	"github.com/unxed/vtui"
	"golang.org/x/sys/unix"
)

// logPTYDiagnostics records the state of everything PTY allocation depends on.
// It runs only after an allocation has already failed, so the cost does not
// matter, and it exists so that a single trace log from a reporter is enough
// to work out why: the errno alone cannot distinguish a device that is
// missing from one that is present but refuses the caller, nor a per-process
// descriptor limit from a system that never had the device at all. See
// issue #444, where the same symptom had a different cause on FreeBSD,
// illumos and DragonFly.
func logPTYDiagnostics() {
	vtui.DebugLog("PTY_DIAG: os=%s arch=%s uid=%d euid=%d gid=%d pid=%d",
		runtime.GOOS, runtime.GOARCH, os.Getuid(), os.Geteuid(), os.Getgid(), os.Getpid())

	for _, path := range []string{"/dev/ptmx", "/dev/pts", "/dev/ptm", "/dev/ptc"} {
		fi, err := os.Stat(path)
		if err != nil {
			vtui.DebugLog("PTY_DIAG: %s: %v", path, err)
			continue
		}
		vtui.DebugLog("PTY_DIAG: %s: mode=%v", path, fi.Mode())
	}

	// A controlling terminal is worth reporting because the PTY is allocated
	// by the session daemon, which may not have one, and a backend that
	// depends on it would fail there and nowhere else.
	if tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0); err != nil {
		vtui.DebugLog("PTY_DIAG: controlling tty: none (%v)", err)
	} else {
		tty.Close()
		vtui.DebugLog("PTY_DIAG: controlling tty: present")
	}

	// Printed with %d rather than compared against RLIM_INFINITY: the field
	// is unsigned on Linux and signed on the BSDs, so any arithmetic here
	// would have to be written per platform for no diagnostic gain.
	var lim unix.Rlimit
	if err := unix.Getrlimit(unix.RLIMIT_NOFILE, &lim); err == nil {
		vtui.DebugLog("PTY_DIAG: RLIMIT_NOFILE cur=%d max=%d", lim.Cur, lim.Max)
	}
}
