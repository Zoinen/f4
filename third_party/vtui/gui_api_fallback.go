//go:build !linux && !openbsd && !netbsd && !dragonfly && !darwin && !freebsd

package vtui

import (
	"fmt"
	"os"
	"runtime"
)

// RunInGUIWindow launches the TUI within a native graphical window.
// On platforms without X11/Wayland (like Windows), it defaults to gogpu.
func RunInGUIWindow(cols, rows int, backend string, fontName string, fontSize float64, setupApp func()) error {
	if backend == "x11" {
		return runInX11Window(cols, rows, fontName, fontSize, setupApp)
	}
	if backend == "gogpu" || backend == "" {
		if os.Getenv("DISPLAY") != "" && backend == "" {
			return runInX11Window(cols, rows, fontName, fontSize, setupApp)
		}
		return runInGogpuWindow(cols, rows, fontName, fontSize, setupApp)
	}
	return fmt.Errorf("GUI backend %q is not supported on %s", backend, runtime.GOOS)
}

func runInGogpuWindow(cols, rows int, fontName string, fontSize float64, setupApp func()) error {
	return RunGogpuHost(cols, rows, fontName, fontSize, setupApp)
}
