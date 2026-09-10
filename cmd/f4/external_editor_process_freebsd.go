//go:build freebsd

package main

import "os/exec"

// FreeBSD's terminal is already the controlling terminal of the shell
// session. The TTY session stays attached on this platform, so the editor
// must inherit that session instead of trying to acquire the terminal again.
func configureExternalEditorProcess(cmd *exec.Cmd) {}
