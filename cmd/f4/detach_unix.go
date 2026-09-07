//go:build !windows

package main

import (
	"os"
	"syscall"

	"golang.org/x/sys/unix"
)

func checkAndDetach(attached bool) {
	if attached || os.Getenv("F4_DETACHED") == "1" {
		return
	}

	exe, err := f4Executable()
	if err != nil {
		return
	}

	cmd := selfCommand(exe, os.Args[1:]...)
	cmd.Env = append(cmd.Env, "F4_DETACHED=1")
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setsid: true,
	}

	// The copy is on its own: stdin is closed off, and its stdout and
	// stderr start at /dev/null so it never holds the terminal it was
	// launched from. Once it has its crash log it points both at that
	// (see redirectDetachedStdout), so that whatever a library prints on
	// its way out is recorded rather than dropped.
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

// redirectDetachedStdout sends the standard output of a detached copy to
// wherever its standard error already goes -- the crash log, once
// vtui.SetupStderrLog has pointed fd 2 at it. Call it right after that.
//
// A detached GUI process has no terminal, so its stdout is /dev/null, and
// a library that reports its exit reason there -- neurlang/wayland prints
// the display loop's error with fmt.Println -- leaves an empty crash log
// and a window that simply vanishes. Only the detached copy is touched:
// in the terminal backends stdout is the screen and must stay so.
func redirectDetachedStdout() {
	if os.Getenv("F4_DETACHED") != "1" {
		return
	}
	_ = unix.Dup2(2, 1)
}
