package main

import (
	"fmt"
	"os"
	"runtime"
	"strings"

	"github.com/unxed/vtui"
	"golang.org/x/term"
)

// consoleProbe is what the platform can tell us about the console the user is
// actually looking at. On non-Windows every field stays zero and OK is false.
type consoleProbe struct {
	OK        bool // GetConsoleScreenBufferInfo succeeded
	BufW      int  // dwSize
	BufH      int
	WinLeft   int // srWindow
	WinTop    int
	WinRight  int
	WinBottom int
	CursorX   int
	CursorY   int
}

// WinRows is the height of the visible window, which is what the overlay must
// use: a console buffer is routinely much taller than the window showing it.
func (p consoleProbe) WinRows() int { return p.WinBottom - p.WinTop + 1 }

// WinCols is the width of the visible window.
func (p consoleProbe) WinCols() int { return p.WinRight - p.WinLeft + 1 }

// wineProbeReport renders the environment facts stage A0 of WINE.md asks for.
// Everything here is read-only: no renderer is created, no screen is touched,
// so the report is safe to print from a plain (non-TTY) shell too.
func wineProbeReport() string {
	var sb strings.Builder
	line := func(k string, v any) { fmt.Fprintf(&sb, "%-22s %v\n", k+":", v) }

	line("f4 version", getFormattedVersionInfo())
	line("GOOS/GOARCH", runtime.GOOS+"/"+runtime.GOARCH)
	line("vtui.IsWine", vtui.IsWine())
	line("DefaultConsoleBackend", vtui.DefaultConsoleBackend())
	line("SelectedTTYBackend", SelectedTTYBackend)
	line("stdout is terminal", term.IsTerminal(int(os.Stdout.Fd())))
	line("stdin is terminal", term.IsTerminal(int(os.Stdin.Fd())))

	if w, h, err := term.GetSize(int(os.Stdout.Fd())); err == nil {
		line("term.GetSize(stdout)", fmt.Sprintf("%dx%d", w, h))
	} else {
		line("term.GetSize(stdout)", fmt.Sprintf("error: %v", err))
	}
	if w, h, err := term.GetSize(int(os.Stdin.Fd())); err == nil {
		line("term.GetSize(stdin)", fmt.Sprintf("%dx%d", w, h))
	} else {
		line("term.GetSize(stdin)", fmt.Sprintf("error: %v", err))
	}
	w, h, err := vtui.GetTerminalSize()
	line("vtui.GetTerminalSize", fmt.Sprintf("%dx%d (err: %v)", w, h, err))

	for _, name := range []string{"TERM", "COLUMNS", "LINES", "WINEDEBUG", "SHELL", "COMSPEC"} {
		line("env "+name, orEmptyMarker(os.Getenv(name)))
	}

	p := probeConsole()
	line("console buffer info", p.OK)
	if p.OK {
		line("  dwSize", fmt.Sprintf("%dx%d", p.BufW, p.BufH))
		line("  srWindow", fmt.Sprintf("L%d T%d R%d B%d", p.WinLeft, p.WinTop, p.WinRight, p.WinBottom))
		line("  window cells", fmt.Sprintf("%dx%d", p.WinCols(), p.WinRows()))
		line("  cursor", fmt.Sprintf("%d,%d", p.CursorX, p.CursorY))
	}

	// Shell mode is a pure function of these probes, so the report can show what
	// each ConsoleMode setting would resolve to without starting the UI.
	line("probePTYUsable", probePTYUsable())
	line("probeHostTTY", probeHostTTY())
	line("probeGUIBackend", orEmptyMarker(probeGUIBackend()))
	for _, mode := range []string{ConsoleViewOwn, ConsoleViewFar, ConsoleViewMc} {
		resolved := resolveShellMode(ShellModeConfig{ConsoleMode: mode})
		line("ConsoleMode="+mode, fmt.Sprintf("%s (view: %s)", resolved, consoleViewStyleOf(ShellModeConfig{ConsoleMode: mode})))
	}
	return sb.String()
}

// orEmptyMarker renders empty strings visibly so an unset variable is not confused
// with a variable set to nothing.
func orEmptyMarker(s string) string {
	if s == "" {
		return "(empty)"
	}
	return s
}

// runWineProbe prints the report and is expected to be followed by a return
// from main: the probe never starts the UI.
func runWineProbe() {
	fmt.Print(wineProbeReport())
}
