package main

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/unxed/vtui"
)

// waitForAnyKey reads a single keystroke immediately using _getch on Windows/Wine or stdin read on Unix.
var waitForAnyKey = func() {
	if runtime.GOOS == "windows" {
		mod := os.Getenv("COMSPEC")
		_ = mod
		if proc := modMsvcrtProc(); proc != nil {
			proc.Call()
			return
		}
	}
	var buf [1]byte
	_, _ = os.Stdin.Read(buf[:])
}

func modMsvcrtProc() interface {
	Call(...uintptr) (uintptr, uintptr, error)
} {
	return modMsvcrtProcImpl()
}

// runSimpleInlineCommand executes a command directly in the host console without a PTY
// by suspending vtui, running the command with inherited stdio, waiting for a keypress,
// and restoring vtui.
func (pf *PanelsFrame) runSimpleInlineCommand(dir, command string) {
	shell := GetSystemShell()
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.Command(shell, "/c", command)
	} else {
		cmd = exec.Command(shell, "-c", command)
	}
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if dir != "" {
		cmd.Dir = dir
	}

	// Always clear the overlay before running the command so output
	// does not scroll trailing keybar or command-line cells into history.
	pf.clearConsoleOverlay()

	// clearConsoleOverlay() just restored the cursor to overlaySavedCursor
	// -- the console's real cursor position from before f4 ever drew an
	// overlay over it, e.g. wherever an earlier shell prompt happened to
	// end. That column is almost never 0. The row it's on was just blanked
	// by clearConsoleOverlay() (winClearConsoleOverlay), so a bare "\r"
	// (no newline, no scroll, nothing consumed) is enough to put the
	// child's own first output character at the start of that already-
	// blank row instead of wherever a previous, unrelated line of text
	// used to end.
	os.Stdout.WriteString("\r")

	inConsoleView := !pf.showPanels && pf.shellMode == ShellModeSimpleInline &&
		pf.consoleStyle() == ConsoleViewFar

	vtui.Suspend()
	_ = cmd.Run()

	if inConsoleView {
		// The child just wrote its own output starting wherever the cursor
		// happened to be: clearConsoleOverlay() above parks it at
		// overlaySavedCursor, the console's real cursor position from
		// *before* f4 ever drew an overlay there -- not necessarily column
		// 0 of a fresh line. A real interactive shell only looks safe to
		// redraw at a fixed bottom row because it always prints a fresh
		// "\r\n" before the next prompt, so command output never lands on
		// the same row the prompt is about to reclaim. Nothing here was
		// doing that: the child's entire output (all of it, for a
		// single-line command like "echo 123") could end up sitting on
		// exactly the rows drawConsoleOverlay() is about to overwrite
		// below, and get silently erased regardless of platform -- this
		// isn't a Wine timing issue, it's the same outcome a real Windows
		// console would produce with this same sequence.
		//
		// Force those rows clear first: n newlines guarantee at least n
		// scroll events by the time the cursor (wherever it started) is
		// done, which is exactly enough to push anything that was sitting
		// in the overlay's n reserved rows up and out of them.
		if n := pf.overlayLines(); n > 0 {
			os.Stdout.WriteString(strings.Repeat("\r\n", n))
		}

		// Snapshot the console now, while the command's output is still the
		// visible content of hStdOut. Without this, clearConsoleViewBackground()
		// finds no saved buffer on the next Ctrl+O round-trip and blanks the
		// whole window instead of restoring it (the exact bug this comment
		// used to sit next to, minus the missing capture).
		captureHostConsoleBuffer(pf.lastW, pf.lastH)

		// Re-enable input without the AltScreen round trip Resume() would do
		// (host buffer -> f4's own buffer -> host buffer again, all inside a
		// few milliseconds): the host buffer is already the active one, set
		// by Suspend() before cmd.Run() and never touched since. Under Wine
		// that rapid double SetConsoleActiveScreenBuffer is exactly the kind
		// of call vtui's own WINE.md documents as unreliable -- see the
		// "single-line command output vanishes after Ctrl+O" report this
		// call replaced Resume()+SetAltScreen(false) for.
		vtui.ResumeWithoutAltScreen()
		pf.SetBusy(true)
		pf.drawConsoleOverlay()
		return
	}

	fmt.Print("\r\nPress any key to return to f4...")
	waitForAnyKey()

	captureHostConsoleBuffer(pf.lastW, pf.lastH)

	vtui.Resume()
	if vtui.FrameManager != nil {
		vtui.FrameManager.HardRefresh()
	}
	pf.RefreshAll()
}

func captureHostConsoleBuffer(w, h int) {
	captureHostConsoleBufferImpl(w, h)
}

// runSimpleCapturedCommand executes a command via LocalCommandRunner and displays
// the streaming output in a scrollable f4 window.
func (pf *PanelsFrame) runSimpleCapturedCommand(dir, command string) {
	runner := NewLocalCommandRunner()
	showRemoteCommandOutput(pf, runner, dir, command)
}
