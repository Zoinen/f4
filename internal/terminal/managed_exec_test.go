//go:build !windows && !solaris && !illumos

package terminal

// solaris/illumos: NewPTY returns *SolarisPTY there (pty_solaris.go), the PTY
// type these helpers take does not exist on those targets.

import (
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// waitForPTYCondition drains whatever the PTY prints into out while polling
// cond, until cond is true or timeout elapses. It returns cond's final
// value, so callers can tell a satisfied wait from a timed-out one.
func waitForPTYCondition(p *PTY, out *strings.Builder, timeout time.Duration, cond func() bool) bool {
	deadline := time.Now().Add(timeout)
	buf := make([]byte, 4096)
	for {
		if cond() {
			return true
		}
		_ = p.Master.SetReadDeadline(time.Now().Add(20 * time.Millisecond))
		if n, err := p.Master.Read(buf); n > 0 {
			out.Write(buf[:n])
		} else if err != nil && !errors.Is(err, os.ErrDeadlineExceeded) {
			return cond()
		}
		if time.Now().After(deadline) {
			return cond()
		}
	}
}

// TestManagedForegroundCommand_JobControlStopReclaimsTerminal pins down,
// against a real PTY and a real interactive shell (not a mock), the
// mechanism behind f4 #1603: a foreground command run from f4's own command
// line is wrapped by ManagedForegroundCommand in OSC 133 C/D markers so f4
// knows when it is done (internal/panel/frame.go's BeginManagedExecution /
// endExecution, gated on the D marker via shellBusyChanged).
//
// An earlier version of this test additionally asserted that the D marker
// is *never* printed once the wrapped command is job-control *stopped*
// (Ctrl+Z) instead of finishing, on the theory that bash abandons the rest
// of the `;`-separated compound command once the foreground job stops. That
// theory does not hold: real CI runs of this exact test (linux/amd64,
// linux/arm64, darwin/arm64 -- see the run cited in the #1603 comment
// thread) show bash reliably continuing past the stopped "sleep" and
// printing the D marker within a few dozen milliseconds of the "Stopped"
// job-control notice, on every one of those platforms alike. That makes
// sense once you look at what actually stops: SIGTSTP only suspends the
// process group of the forked "sleep" job; the interactive shell's own
// execution of the compound list is not itself paused, so it sails through
// the remaining `;`-separated statements (FARVTRESULT=$?; printf D; ...)
// immediately, leaving "sleep" parked as a stopped background job. So the
// presence or absence of a belated D marker here is not a meaningful,
// platform-independent signal either way, and this test does not assert
// on it in either direction.
//
// What *is* meaningful, and what this test does assert, is that
// PTY.IsBusy() -- the TIOCGPGRP check pty_linux.go/pty_darwin.go/
// pty_bsd.go already use, independent of any marker -- correctly and
// stably notices the shell reclaiming the terminal's foreground process
// group after the stop, and does not flap back to true afterwards. That is
// the empirical groundwork f4 #1603's real fix needs: today
// PanelsFrame.IsPtyBusy() ORs this same IsBusy() with pf.Executing, so a
// stuck-true pf.Executing (were the D marker ever to not arrive) wins
// regardless. The fix notices IsBusy() going false while pf.Executing is
// still true and treats that as "the job stopped", debounced against the
// real, separate window right after BeginManagedExecution() where
// IsBusy() also reads false before the wrapped command has even forked
// yet (see internal/panel/frame.go's pollManagedExecutionDebounce).
func TestManagedForegroundCommand_JobControlStopReclaimsTerminal(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash not available")
	}

	p, err := NewPTY()
	if err != nil {
		t.Fatalf("NewPTY: %v", err)
	}
	t.Cleanup(func() { _ = p.Close() })

	// No args: this is exactly how GetSystemShell()'s result gets started in
	// internal/panel/frame.go (InitPTY -> p.Run(shell)) -- bash detects it is
	// interactive itself (tty on stdin/stdout, no -c) and turns job control
	// (monitor mode) on, which is the whole mechanism under test.
	if err := p.Run("bash"); err != nil {
		t.Fatalf("Run(bash): %v", err)
	}

	// "sleep 30" stands in for the reported python3 REPL: a foreground
	// program that blocks instead of returning, the shape #1603 is about.
	wire := " " + ManagedForegroundCommand("'sleep 30'") + "\r"
	if _, err := p.Write([]byte(wire)); err != nil {
		t.Fatalf("writing the managed command: %v", err)
	}

	// The waits below return the moment their condition holds; the generous
	// ptyStartupTimeout only matters on an overloaded runner, where bash
	// starting up and forking sleep can take well over the 5 s this used to
	// allow.
	const ptyStartupTimeout = 20 * time.Second
	var out strings.Builder
	if !waitForPTYCondition(p, &out, ptyStartupTimeout, func() bool {
		return strings.Contains(out.String(), "\x1b]133;C\x07")
	}) {
		t.Fatalf("never saw the C marker; PTY output so far: %q", out.String())
	}
	if !waitForPTYCondition(p, &out, ptyStartupTimeout, p.IsBusy) {
		t.Fatalf("sleep never became the foreground job (IsBusy never went true); PTY output so far: %q", out.String())
	}

	// Ctrl+Z: the exact control byte f4's own key dispatch already writes to
	// the master for this key (confirmed reaching the PTY in the CI run
	// cited in the #1603 comment thread).
	//
	// IsBusy goes true as soon as bash hands the terminal to the child's
	// process group, which can be before that child has reset SIGTSTP from
	// the interactive shell's "ignore" back to default; a Ctrl+Z landing in
	// that window is silently dropped, and under load the window is wide
	// enough to hit. So keep re-sending Ctrl+Z until the shell reclaims the
	// terminal (the panel-side test does the same): once sleep is stopped,
	// nothing more is sent, and a stray one at bash's prompt is ignored.
	var lastCtrlZ time.Time
	if !waitForPTYCondition(p, &out, ptyStartupTimeout, func() bool {
		if !p.IsBusy() {
			return true
		}
		if time.Since(lastCtrlZ) >= 250*time.Millisecond {
			if _, err := p.Write([]byte{0x1a}); err != nil {
				t.Errorf("sending Ctrl+Z: %v", err)
				return true
			}
			lastCtrlZ = time.Now()
		}
		return false
	}) {
		t.Fatalf("shell never reclaimed the terminal after Ctrl+Z (IsBusy stayed true); PTY output so far: %q", out.String())
	}

	// The reclaim above is not the end of the wrapper yet: the shell carries
	// on through the rest of the compound list (see the doc comment above),
	// and its very last statement, `(exit $FARVTRESULT)`, is a *subshell*.
	// An interactive job-control shell forks every subshell as a foreground
	// job of its own -- new process group, handed the terminal via
	// tcsetpgrp -- so IsBusy() legitimately reads true again while that
	// subshell lives, on every managed command, stopped or not. A tight
	// TIOCGPGRP sampler on macos-latest measured it at about 2-4 ms, with
	// the foreground group being a fresh one (not sleep's), right after the
	// D marker and before the prompt. The loop below lands in that window
	// now and then (PR #1629's CI: D marker printed, no prompt yet), so asserting
	// "never true again" straight after the reclaim tested the wrapper's
	// own tail, not the stopped job.
	//
	// So first let the shell finish that list: it reads the next input line
	// only once the current one is fully done, and the quotes split the
	// sentinel so its echoed input never matches its output.
	if _, err := p.Write([]byte(" echo F4SET''TLED\r")); err != nil {
		t.Fatalf("writing the settle sentinel: %v", err)
	}
	if !waitForPTYCondition(p, &out, ptyStartupTimeout, func() bool {
		return strings.Contains(out.String(), "F4SETTLED")
	}) {
		t.Fatalf("the shell never got back to reading input after the stop; PTY output so far: %q", out.String())
	}

	// The shell may still have printed a belated D marker for the abandoned
	// "sleep" job -- that is not a bug and this test does not check for it
	// either way. What matters for #1603's fix is that IsBusy() has
	// genuinely, stably reclaimed "false": drain a further window and
	// confirm it never flaps back to true, the way it would if the shell
	// handed the terminal's foreground process group back to "sleep" again
	// (e.g. a shell that resumed the stopped job on its own instead of
	// leaving it stopped).
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		waitForPTYCondition(p, &out, 20*time.Millisecond, func() bool { return false })
		if p.IsBusy() {
			t.Fatalf("IsBusy() flipped back to true after the shell reclaimed the terminal; PTY output so far: %q", out.String())
		}
	}
}
