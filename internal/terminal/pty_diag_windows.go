//go:build windows

package terminal

import (
	"github.com/unxed/vtui"
	"os"
	"runtime"
)

// LogPTYDiagnostics has nothing device-shaped to inspect on Windows, where
// the console pseudoterminal comes from ConPTY rather than from a device in
// the filesystem, so it records only the identity of the process.
func LogPTYDiagnostics() {
	vtui.DebugLog("PTY_DIAG: os=%s arch=%s pid=%d", runtime.GOOS, runtime.GOARCH, os.Getpid())
}
