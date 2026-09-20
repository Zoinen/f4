//go:build windows

package app

import (
	"os"
	"os/exec"
	"syscall"

	winescape "github.com/unxed/libwinescape/go"

	"github.com/unxed/f4/vfs/hostmode"
)

func checkAndDetach(attached bool) {
	if attached || os.Getenv("F4_DETACHED") == "1" {
		return
	}

	exe, err := os.Executable()
	if err != nil {
		return
	}

	cmd := exec.Command(exe, os.Args[1:]...)
	cmd.Env = append(os.Environ(), "F4_DETACHED=1")
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: 0x08000000, // CREATE_NO_WINDOW
	}

	null, _ := os.Open(os.DevNull)
	if null != nil {
		cmd.Stdin = null
		cmd.Stdout = null
		cmd.Stderr = null
	}

	if err := cmd.Start(); err == nil {
		os.Exit(0)
	}
}

// redirectDetachedStdout closes the one way a detached copy running under
// Wine still reaches the terminal it was launched from. Call it right after
// vtui.SetupStderrLog, as on Unix. On native Windows it does nothing: the
// copy has no console, and its crash log already takes everything that
// matters.
//
// checkAndDetach hands the copy NUL for stdin, stdout and stderr. Wine turns
// the first two into the copy's Unix descriptors 0 and 1, but leaves Unix
// descriptor 2 as it was in the launching process: spawn_process in
// dlls/ntdll/unix/process.c only ever sets 0 and 1. Wine prints its own
// diagnostics -- fixme and err lines, and the MESSAGE lines WINEDEBUG does
// not silence -- to that descriptor, not to the Windows handle, so they kept
// going to the launcher's terminal. When nobody reads that terminal, as far2l
// does not once the command it ran has returned, its buffer fills and the
// thread that wrote next blocks for good: the window stopped answering after
// a few dozen keystrokes (issue #474). Pointing descriptor 2 at /dev/null
// gives Wine the NUL the copy was asked to have.
func redirectDetachedStdout() {
	detached := os.Getenv("F4_DETACHED") == "1"
	// hostmode.Allowed() is the UseWinescape setting: with it off, f4 uses
	// libwinescape nowhere, this fix included. A copy started under Wine by a
	// user who turned it off keeps Wine's descriptor 2, which is the pre-#474
	// behaviour and their choice to make.
	if !detached || !hostmode.Allowed() || !winescape.Available() {
		return
	}
	var tio winescape.Termios
	stdoutIsTerminal := winescape.Tcgetattr(1, &tio) == nil
	if !detachedWineStderrGoesToNull(detached, stdoutIsTerminal) {
		return
	}
	null, err := winescape.Open("/dev/null", winescape.O_WRONLY|winescape.O_CLOEXEC, 0)
	if err != nil {
		return
	}
	if null != 2 {
		_ = winescape.Dup3(null, 2, 0)
		_ = winescape.Close(null)
	}
}

// detachedWineStderrGoesToNull decides whether this process is the copy
// checkAndDetach started, from the two facts that say so. The flag alone is
// not proof: it travels in the environment, and a process that inherited it
// has a terminal on its standard output (the same trap as issue #1151 on
// Unix), where Wine's messages belong.
func detachedWineStderrGoesToNull(detached, stdoutIsTerminal bool) bool {
	return detached && !stdoutIsTerminal
}
