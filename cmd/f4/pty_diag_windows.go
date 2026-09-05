//go:build windows

package main

import (
	"os"
	"runtime"

	"github.com/unxed/vtui"
)

// logPTYDiagnostics has nothing device-shaped to inspect on Windows, where
// the console pseudoterminal comes from ConPTY rather than from a device in
// the filesystem, so it records only the identity of the process.
func logPTYDiagnostics() {
	vtui.DebugLog("PTY_DIAG: os=%s arch=%s pid=%d", runtime.GOOS, runtime.GOARCH, os.Getpid())
}
