//go:build linux

package editor

import (
	"io"
	"os"

	"golang.org/x/sys/unix"
)

// inheritsControllingTTY reports whether stdin is a terminal that is already
// the controlling terminal of our own session. Such a terminal must be
// inherited as is: a child that starts a new session and then tries to take
// it with TIOCSCTTY gets EPERM from the kernel, because the terminal still
// belongs to our session (f4#1721, "fork/exec /usr/bin/nano: operation not
// permitted").
func inheritsControllingTTY(stdin io.Reader) bool {
	f, ok := stdin.(*os.File)
	if !ok {
		return false
	}
	rc, err := f.SyscallConn()
	if err != nil {
		return false
	}
	inherits := false
	// Control keeps the descriptor's blocking mode intact, unlike File.Fd.
	_ = rc.Control(func(fd uintptr) {
		sid, err := unix.IoctlGetInt(int(fd), unix.TIOCGSID)
		if err != nil {
			return // not a terminal, or one without a session
		}
		own, err := unix.Getsid(0)
		inherits = err == nil && sid == own
	})
	return inherits
}
