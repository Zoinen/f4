//go:build !windows

package main

import (
	"strings"
	"testing"

	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

func issue863ScreenRows(scr *vtui.ScreenBuf) []string {
	rows := make([]string, scr.Height())
	for y := 0; y < scr.Height(); y++ {
		var row strings.Builder
		for x := 0; x < scr.Width(); x++ {
			row.WriteString(vtui.CellString(scr.GetCell(x, y).Char))
		}
		rows[y] = strings.TrimSpace(row.String())
	}
	return rows
}

func issue863CountRow(rows []string, want string) int {
	count := 0
	for _, row := range rows {
		if row == want {
			count++
		}
	}
	return count
}

// issue863RunCommand drives the Unix own-terminal command path for a command
// whose output arrives as the given chunks, then renders the frame. It returns
// the trimmed screen rows and the prompt text f4 paints itself, which is also
// the text the mock shell writes into the PTY as its native prompt: a native
// prompt that stays visible therefore shows up as a second matching row.
func issue863RunCommand(t *testing.T, chunks ...string) (*PanelsFrame, []string, string) {
	t.Helper()
	vtui.SetDefaultPalette()
	t.Cleanup(swapFrameManager(t))
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())

	pf := NewPanelsFrame()
	t.Cleanup(pf.Close)
	pf.shellMode = ShellModeOwn
	pf.showPanels = false
	pf.showKeyBar = true
	pty := &mockPty{}
	pf.pty = pty
	pf.termView.pty = pty
	pf.parser = NewAnsiParser(pf.termView, pty)
	pf.ResizeConsole(80, 25)

	prompt := cellsText(pf.buildPrompt())
	pf.consumeLocalOutput(pty, []byte(prompt))

	pf.cmdLine.Edit.SetText("cat cat_tst")
	if !pf.ProcessKey(&vtinput.InputEvent{
		Type:           vtinput.KeyEventType,
		KeyDown:        true,
		VirtualKeyCode: vtinput.VK_RETURN,
	}) {
		t.Fatal("f4 command line did not handle Enter")
	}

	pf.consumeLocalOutput(pty, []byte("\x1b]133;C\x07"))
	for _, chunk := range chunks {
		pf.consumeLocalOutput(pty, []byte(chunk))
	}
	pf.consumeLocalOutput(pty, []byte("\x1b]133;D\x07"+prompt))
	drainUITasks()

	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(80, 25)
	pf.Show(scr)
	pf.keyBar.Show(scr)
	return pf, issue863ScreenRows(scr), prompt
}

// TestIssue863OwnTerminalKeepsLineWithoutTrailingNewline reproduces #863: a
// file whose last line has no trailing newline leaves the shell cursor in the
// middle of a row, the shell appends its prompt to that same row, and f4's
// command line covers it -- taking the last line of the file with it. `cat
// file > out` writes both lines, so the loss is purely on the display side.
func TestIssue863OwnTerminalKeepsLineWithoutTrailingNewline(t *testing.T) {
	_, rows, _ := issue863RunCommand(t, "1\r\n", "2")

	if got := issue863CountRow(rows, "1"); got != 1 {
		t.Errorf("visible row 1 count = %d, want 1; screen:\n%s", got, strings.Join(rows, "\n"))
	}
	if got := issue863CountRow(rows, "2"); got != 1 {
		t.Errorf("last output line without a trailing newline is not visible: row 2 count = %d, want 1; screen:\n%s",
			got, strings.Join(rows, "\n"))
	}
}

// TestIssue863OwnTerminalNoDuplicatePromptWithoutTrailingNewline guards the
// obvious wrong fix for the test above: freeing the covered row by shrinking
// the terminal, or by pushing the grid up one row too far, makes the plain
// native prompt reappear above f4's own coloured one. Exactly one prompt row
// may be visible, and it has to be the row f4 draws its command line on.
func TestIssue863OwnTerminalNoDuplicatePromptWithoutTrailingNewline(t *testing.T) {
	pf, rows, prompt := issue863RunCommand(t, "1\r\n", "2")

	if got := issue863CountRow(rows, strings.TrimSpace(prompt)); got != 1 {
		t.Errorf("visible prompt row count = %d, want 1; screen:\n%s", got, strings.Join(rows, "\n"))
	}
	if pf.termView.Y2 != pf.cmdLine.Y1 {
		t.Errorf("native and f4 prompts use different rows: terminal Y2=%d command line Y1=%d", pf.termView.Y2, pf.cmdLine.Y1)
	}
}

// TestIssue863OwnTerminalFinalNewlineKeepsSinglePrompt follows the Unix own-
// terminal command path for a file containing "1\n2\n". The PTY must not be
// resized when the command line and keybar temporarily disappear, both output
// rows must remain visible, and the native prompt must occupy the same screen
// row as f4's editable prompt rather than appearing as a duplicate above it.
func TestIssue863OwnTerminalFinalNewlineKeepsSinglePrompt(t *testing.T) {
	vtui.SetDefaultPalette()
	t.Cleanup(swapFrameManager(t))
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())

	pf := NewPanelsFrame()
	t.Cleanup(pf.Close)
	pf.shellMode = ShellModeOwn
	pf.showPanels = false
	pf.showKeyBar = true
	pty := &mockPty{}
	pf.pty = pty
	pf.termView.pty = pty
	pf.parser = NewAnsiParser(pf.termView, pty)
	pf.ResizeConsole(80, 25)

	idleHeight := pf.termView.Height
	prompt := cellsText(pf.buildPrompt())
	pf.consumeLocalOutput(pty, []byte(prompt))

	pf.cmdLine.Edit.SetText("cat cat_tst")
	if !pf.ProcessKey(&vtinput.InputEvent{
		Type:           vtinput.KeyEventType,
		KeyDown:        true,
		VirtualKeyCode: vtinput.VK_RETURN,
	}) {
		t.Fatal("f4 command line did not handle Enter")
	}
	if wire := pty.String(); !strings.Contains(wire, "eval 'cat cat_tst'") {
		t.Fatalf("f4 did not send the managed cat command to its PTY: %q", wire)
	}
	pf.ResizeConsole(80, 25)
	busyHeight := pf.termView.Height

	pf.consumeLocalOutput(pty, []byte("\x1b]133;C\x07"))
	pf.consumeLocalOutput(pty, []byte("1\r\n"))
	pf.consumeLocalOutput(pty, []byte("2\r\n"))
	pf.consumeLocalOutput(pty, []byte("\x1b]133;D\x07"+prompt))
	drainUITasks()

	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(80, 25)
	pf.Show(scr)
	pf.keyBar.Show(scr)
	rows := issue863ScreenRows(scr)

	if busyHeight != idleHeight {
		t.Errorf("PTY height changed while cat ran: idle=%d busy=%d", idleHeight, busyHeight)
	}
	if got := issue863CountRow(rows, "1"); got != 1 {
		t.Errorf("visible row 1 count = %d, want 1; screen:\n%s", got, strings.Join(rows, "\n"))
	}
	if got := issue863CountRow(rows, "2"); got != 1 {
		t.Errorf("visible row 2 count = %d, want 1; screen:\n%s", got, strings.Join(rows, "\n"))
	}
	if got := issue863CountRow(rows, strings.TrimSpace(prompt)); got != 1 {
		t.Errorf("active prompt row count = %d, want 1; screen:\n%s", got, strings.Join(rows, "\n"))
	}
	if pf.termView.Y2 != pf.cmdLine.Y1 {
		t.Errorf("native and f4 prompts use different rows: terminal Y2=%d command line Y1=%d", pf.termView.Y2, pf.cmdLine.Y1)
	}
}
