//go:build !windows && !freebsd

package main

import (
	"os/exec"
	"syscall"
)

// configureExternalEditorProcess makes the terminal passed through the Unix
// session daemon the editor's controlling terminal. The daemon itself is
// deliberately detached from the user's session, so merely inheriting the
// attached stdin/stdout is not enough for editors such as micro that open
// /dev/tty during startup.
func configureExternalEditorProcess(cmd *exec.Cmd) {
	if runningGUI {
		// GUI hosts normally have no terminal at all. In particular, forcing
		// Setctty on /dev/null would make even a GUI editor fail before it
		// starts.
		return
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setsid:  true,
		Setctty: true,
	}
}
