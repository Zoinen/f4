package app

import (
	"github.com/unxed/f4/internal/panel"
	"github.com/unxed/f4/internal/paneltest"
	"github.com/unxed/f4/internal/testutil"
	"github.com/unxed/vtui"
	"os/exec"
	"runtime"
	"strings"
	"testing"
)

// testutil.DrainUITasks runs everything the frame manager has queued ahead of this
// point, the way FrameManager.Run would.
//
// The queue is process-wide and outlives FrameManager.Init, so it also carries
// whatever earlier tests left behind. That backlog is why this cannot be a
// wall-clock wait: draining for a fixed span races it, and on a loaded runner
// the leftovers can outlast the window and leave this test's own task still
// pending when the assertion runs.
//
// The queue is FIFO, so a sentinel posted behind the work cannot be reached
// until the work has run. Draining until the sentinel arrives is therefore
// exact no matter how long the backlog is, and needs no guess about timing.
// runBusyChange delivers an OSC 133 C/D transition and settles the UI tasks
// it posts.
func runBusyChange(pf *panel.PanelsFrame, busy bool) {
	pf.TermView.OnBusyChange(busy)
	testutil.DrainUITasks()
}

func newExecutionTestFrame(t *testing.T) *panel.PanelsFrame {
	t.Helper()
	t.Cleanup(paneltest.SwapFrameManager(t))
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	pf := panel.NewPanelsFrame()
	t.Cleanup(pf.Close)
	pf.ShowPanels = false
	return pf
}

// A command f4 wrapped in its own OSC 133 C/D pair ends the moment its D
// marker arrives, even though no prompt marker was ever seen before it.
// Shells that do not mark their prompts (every Unix shell: f4 injects PROMPT
// on Windows only) never set shellPromptReady on their own, so treating that
// first D as a stale startup prompt left pf.executing stuck on forever after
// the first command of the session. With a busy terminal the command line and
// the keybar stay hidden, TerminalQuiet stays false so F3/F4 no longer open
// the terminal log, and every keystroke is forwarded raw to the term.PTY.
func TestWrappedCommandCompletionEndsExecution(t *testing.T) {
	pf := newExecutionTestFrame(t)

	pf.BeginManagedExecution()
	runBusyChange(pf, true)
	runBusyChange(pf, false)

	if pf.Executing {
		t.Fatal("executing is still set after the command's own OSC 133 D marker")
	}
}

// The startup prompt of a prompt-marking shell can still be crossing ConPTY
// when the command is sent. Consuming it would end the execution while the
// command is only just starting, so the first marker is discarded and the
// real prompt that follows the command ends it.
func TestPromptDrivenCommandIgnoresStartupPrompt(t *testing.T) {
	pf := newExecutionTestFrame(t)

	pf.BeginPromptDrivenExecution()
	runBusyChange(pf, false) // stale prompt from shell startup
	if !pf.Executing {
		t.Fatal("a stale startup prompt ended the execution")
	}

	runBusyChange(pf, false) // prompt printed after the command finished
	if pf.Executing {
		t.Fatal("executing is still set after the command's prompt marker")
	}
}

// Once a prompt has been seen, no marker is stale any more: the next
// prompt-driven command ends on its very first marker.
func TestPromptDrivenCommandAfterPromptSeen(t *testing.T) {
	pf := newExecutionTestFrame(t)

	runBusyChange(pf, false) // shell startup prompt, no command running
	pf.BeginPromptDrivenExecution()
	runBusyChange(pf, false)

	if pf.Executing {
		t.Fatal("executing is still set after the command's prompt marker")
	}
}

// A command whose text is a syntax error must still let the wrapper report
// completion. The shell parses the whole line before running any of it, so
// without eval a bare ">" makes it reject the group -- OSC 133 markers and
// all -- and f4 waits for a completion that will never be printed.
func TestShellSingleQuote(t *testing.T) {
	cases := map[string]string{
		">":              "'>'",
		"echo hi":        "'echo hi'",
		"echo 'a'":       `'echo '\''a'\'''`,
		`echo "unclosed`: `'echo "unclosed'`,
	}
	for in, want := range cases {
		if got := panel.ShellSingleQuote(in); got != want {
			t.Errorf("shellSingleQuote(%q) = %s, want %s", in, got, want)
		}
	}
}

// The quoting has to survive an actual shell, not just look right.
func TestShellSingleQuoteRoundTripsThroughSh(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX shell quoting")
	}
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("no sh available")
	}
	for _, payload := range []string{">", "echo 'quoted'", `echo "x"`, "a'b", "$HOME"} {
		script := "printf '%s' " + panel.ShellSingleQuote(payload)
		out, err := exec.Command("sh", "-c", script).Output()
		if err != nil {
			t.Fatalf("sh rejected the quoting of %q: %v", payload, err)
		}
		if string(out) != payload {
			t.Errorf("sh saw %q, want %q", out, payload)
		}
	}
}

// End to end: the wrapper f4 sends must print its completion marker even when
// the user's command cannot be parsed.
func TestManagedWrapperReportsCompletionOnSyntaxError(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix managed execution")
	}
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("no bash available")
	}
	for _, payload := range []string{">", `echo "unterminated`, "echo ok"} {
		line := `{ printf "[C]"; eval ` + panel.ShellSingleQuote(payload) +
			` ; R=$?; printf "[D]"; }`
		out, _ := exec.Command("bash", "-c", line).CombinedOutput()
		if !strings.Contains(string(out), "[D]") {
			t.Errorf("no completion marker for %q; shell said: %s", payload, out)
		}
	}
}
