//go:build !solaris && !illumos

package panel

// solaris/illumos: terminal.PTY does not exist there (NewPTY returns
// *terminal.SolarisPTY), so this real-PTY test is not built for them.

import (
	"os/exec"
	"runtime"
	"testing"
	"time"

	"github.com/unxed/f4/internal/terminal"
	"github.com/unxed/vtui"
)

// This file is part 2 of #1603's fix: part 1 (internal/terminal/
// managed_exec_test.go) confirmed against a real PTY that a job-control stop
// (Ctrl+Z / SIGTSTP) of a command run from f4's own command line never
// prints the OSC 133 D marker ManagedForegroundCommand wraps it in, leaving
// pf.Executing (and IsPtyBusy/TerminalOwnsKeyboard) stuck true forever --
// #1603 itself. The fix is pollManagedExecutionDebounce in frame.go: it
// reuses PTY.IsBusy()'s TIOCGPGRP check, debounced against the real race
// right after a command starts (see managedExecStartGuard's doc comment).
//
// Both tests below drive that fix the same way production does -- through
// repeated PanelsFrame.Show calls against a real bash PTY with real job
// control, no mocks -- rather than calling the unexported poll method
// directly, so the wiring in Show() is exercised too, not just the method
// in isolation.

// newManagedExecDebounceFrame wires a real bash PTY into a full PanelsFrame
// exactly the way InitPTY wires a fresh local shell in production: Parser,
// TermView.OnBusyChange (set by NewPanelsFrame), and GetActivePTY's fallback
// to pf.Pty (confirmed by TestPanelsFrame_GetActivePTY). newLocalPTY is left
// alone -- SpawnLocalShellPTY only gates whether InitPTY spawns a *fresh*
// shell when pf.Pty is nil; since pf.Pty is already our real PTY when
// InitPTY runs, it skips straight to the read loop.
func newManagedExecDebounceFrame(t *testing.T) (*PanelsFrame, *terminal.PTY, *vtui.ScreenBuf) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("PTY job control is a Unix concept; not meaningful on Windows")
	}
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash not available")
	}

	t.Cleanup(swapFrameManager(t))
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())

	// Constructed while SpawnLocalShellPTY is still the TestMain default
	// (false): its own internal InitPTY() call is a no-op, so it never
	// forks a shell of its own before we hand it the real PTY below.
	pf := NewPanelsFrame()
	t.Cleanup(pf.Close)

	p, err := terminal.NewPTY()
	if err != nil {
		t.Fatalf("NewPTY: %v", err)
	}
	// No args: exactly how GetSystemShell()'s result gets started in
	// production (InitPTY -> p.Run(shell)) -- bash detects an interactive
	// tty on stdin/stdout and turns on job control (monitor mode) itself.
	if err := p.Run("bash"); err != nil {
		t.Fatalf("Run(bash): %v", err)
	}

	oldSpawn := SpawnLocalShellPTY
	SpawnLocalShellPTY = true
	t.Cleanup(func() { SpawnLocalShellPTY = oldSpawn })

	pf.Pty = p
	pf.InitPTY()
	// Also creates the default OSVFS panels lazily (frame.go), which the
	// job-control-stop test needs: it ends with pf.ShowPanels going back to
	// true (endExecution's ReturnToPanels), and Show() dereferences
	// pf.Panels[0]/[1] unconditionally whenever panels are visible.
	pf.ResizeConsole(80, 25)

	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(80, 25)

	return pf, p, scr
}

// drivePanelsFrameShowUntil renders pf on scr, exactly like production's
// render loop, until cond is true or timeout elapses -- this is what drives
// pollManagedExecutionDebounce (called from inside Show) forward.
func drivePanelsFrameShowUntil(t *testing.T, pf *PanelsFrame, scr *vtui.ScreenBuf, timeout time.Duration, cond func() bool) bool {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		pf.Show(scr)
		if cond() {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(2 * time.Millisecond)
	}
}

// TestPanelsFrame_ManagedExecutionDebounce_JobControlStopFreesKeyboard is
// the positive case: a real Ctrl+Z into a real bash PTY, with a real
// long-running foreground command, must eventually clear pf.Executing and
// hand the keyboard back to f4 -- entirely through the debounce in
// pollManagedExecutionDebounce, since (as part 1 already proved) no D marker
// is ever coming.
func TestPanelsFrame_ManagedExecutionDebounce_JobControlStopFreesKeyboard(t *testing.T) {
	pf, p, scr := newManagedExecDebounceFrame(t)

	// Every wait here returns the moment its condition holds; the generous
	// bound only matters on an overloaded runner (bash starting up and
	// forking sleep took longer than the 5 s this used to allow).
	const jobControlWaitTimeout = 20 * time.Second

	// "sleep 30" stands in for the reported python3 REPL: a foreground
	// program that blocks instead of returning, the shape #1603 is about.
	wire := " " + terminal.ManagedForegroundCommand(ShellSingleQuote("sleep 30")) + "\r"
	pf.BeginManagedExecution()
	pf.ReturnToPanels = pf.ShowPanels
	pf.ShowPanels = false
	if _, err := p.Write([]byte(wire)); err != nil {
		t.Fatalf("writing the managed command: %v", err)
	}

	if !drivePanelsFrameShowUntil(t, pf, scr, jobControlWaitTimeout, func() bool { return p.IsBusy() }) {
		t.Fatal("sleep never became the foreground job (IsBusy never went true)")
	}
	if !pf.TerminalOwnsKeyboard() {
		t.Fatal("terminal should own the keyboard while sleep runs in the foreground")
	}

	// Real Ctrl+Z (SIGTSTP): the exact control byte f4's own key dispatch
	// already writes to the master for this key (part 1's comment thread).
	// IsBusy goes true as soon as bash hands the terminal to the child's
	// process group, which can be before that child has reset SIGTSTP from
	// the interactive shell's "ignore" back to default; a Ctrl+Z landing in
	// that window is silently dropped. Under load (full `go test ./...` on a
	// macOS runner) that window is wide enough to hit, so keep re-sending
	// Ctrl+Z until the shell reclaims the terminal -- once sleep is stopped,
	// further Ctrl+Z at bash's prompt are ignored by bash itself.
	lastCtrlZ := time.Time{}
	if !drivePanelsFrameShowUntil(t, pf, scr, jobControlWaitTimeout, func() bool {
		if !p.IsBusy() {
			return true
		}
		if time.Since(lastCtrlZ) >= 250*time.Millisecond {
			if _, err := p.Write([]byte{0x1a}); err != nil {
				t.Fatalf("sending Ctrl+Z: %v", err)
			}
			lastCtrlZ = time.Now()
		}
		return false
	}) {
		t.Fatal("shell never reclaimed the terminal after Ctrl+Z (IsBusy stayed true)")
	}

	// The debounce (managedExecStartGuard + managedExecIdleDebounceStreak,
	// frame.go) is deliberately not instant; give it a generous window of
	// further Show() polls before concluding it never fires -- that would
	// be #1603 itself, regressed.
	if !drivePanelsFrameShowUntil(t, pf, scr, jobControlWaitTimeout, func() bool { return !pf.Executing }) {
		t.Fatal("pf.Executing never cleared after the job-control stop")
	}

	// The D marker can be what cleared pf.Executing (it is printed just
	// before the wrapper's last statement, `(exit $FARVTRESULT)`). That
	// statement is a subshell, which an interactive job-control shell runs as
	// a foreground job of its own, so IsBusy() legitimately reads true for a
	// few milliseconds after the marker (see the terminal package's
	// JobControlStopReclaimsTerminal test). Let the shell get past it before
	// asserting the terminal is idle, then check it stays that way.
	if !drivePanelsFrameShowUntil(t, pf, scr, jobControlWaitTimeout, func() bool { return !pf.IsPtyBusy() }) {
		t.Fatal("IsPtyBusy still true long after the job-control stop was debounced")
	}
	settled := time.Now().Add(300 * time.Millisecond)
	for time.Now().Before(settled) {
		pf.Show(scr)
		if pf.IsPtyBusy() {
			t.Fatal("IsPtyBusy flipped back to true after the job-control stop was debounced")
		}
		time.Sleep(2 * time.Millisecond)
	}
	if pf.TerminalOwnsKeyboard() {
		t.Fatal("keyboard was not handed back to f4 after the job-control stop")
	}
	if !pf.ShowPanels {
		t.Fatal("panels did not come back after the job-control stop (ReturnToPanels)")
	}
}

// TestPanelsFrame_ManagedExecutionDebounce_FreshStartDoesNotTripImmediately
// is the negative case the follow-up on part 1 explicitly called out: right
// after BeginManagedExecution, the wrapped command has not necessarily
// forked and claimed the terminal's foreground process group yet, so
// PTY.IsBusy() reads false for a real, brief window -- indistinguishable
// from a job-control stop by IsBusy() alone. A naive "IsBusy()==false &&
// Executing" check would clear pf.Executing immediately on every single
// managed command, never mind Ctrl+Z. This never sends Ctrl+Z: with the
// command genuinely still running throughout, pf.Executing must stay true
// through the whole guard window and well past it.
func TestPanelsFrame_ManagedExecutionDebounce_FreshStartDoesNotTripImmediately(t *testing.T) {
	pf, p, scr := newManagedExecDebounceFrame(t)

	wire := " " + terminal.ManagedForegroundCommand(ShellSingleQuote("sleep 30")) + "\r"
	pf.BeginManagedExecution()
	pf.ReturnToPanels = pf.ShowPanels
	pf.ShowPanels = false
	if _, err := p.Write([]byte(wire)); err != nil {
		t.Fatalf("writing the managed command: %v", err)
	}

	// Hammer Show() through the entire just-launched race window at a much
	// tighter interval than production's own render cadence, and confirm
	// managedExecStartGuard holds pf.Executing true throughout.
	guardDeadline := time.Now().Add(managedExecStartGuard + 100*time.Millisecond)
	for time.Now().Before(guardDeadline) {
		pf.Show(scr)
		if !pf.Executing {
			t.Fatal("pf.Executing was cleared during the just-launched race window, before the command could possibly have been stopped")
		}
		time.Sleep(time.Millisecond)
	}

	// Now let the command actually become the foreground job, and keep
	// polling well past the debounce streak threshold: with no Ctrl+Z ever
	// sent, IsBusy() reads true and pf.Executing must stay set for as long
	// as the command genuinely keeps running in the foreground.
	if !drivePanelsFrameShowUntil(t, pf, scr, 5*time.Second, func() bool { return p.IsBusy() }) {
		t.Fatal("sleep never became the foreground job (IsBusy never went true)")
	}
	for i := 0; i < managedExecIdleDebounceStreak+2; i++ {
		pf.Show(scr)
		if !pf.Executing {
			t.Fatalf("pf.Executing was cleared while the command was still genuinely running in the foreground (poll %d)", i)
		}
		if !p.IsBusy() {
			t.Fatal("sleep unexpectedly stopped being the foreground job on its own")
		}
		time.Sleep(20 * time.Millisecond)
	}
}
