package panel

import (
	"strings"
	"sync"
	"time"

	"github.com/unxed/f4/internal/terminal"
	"github.com/unxed/vtui"
)

// cmdShellSession decides when the local cmd.exe has finished the line f4
// typed into it.
//
// Why a prompt mark alone cannot decide it (issue #409): f4 injects PROMPT so
// that cmd prints an OSC 133 mark with every prompt. But cmd also prints the
// prompt in front of every line of a batch file that runs with ECHO on — so
// the first line of foo.bat looks exactly like "command finished", the panels
// come back and the batch keeps running behind them. cmd interprets batch
// files in-process, so a child-process check is blind to them too.
//
// What tells a real prompt apart is that nothing follows it: after an echoed
// batch line the command text and a line break follow within the same frame,
// while a prompt that waits for input leaves the cursor resting right after
// the prompt text. The session therefore ends an execution only when a prompt
// mark (B, printed after $P$G) has arrived since the command was sent, the
// screen in front of the cursor looks like cmd's prompt and has not changed
// for cmdPromptSettleDelay, and no console child process that would own the
// terminal is running.
//
// What was learned from the field (docs/TERMINAL_WINDOWS.md §3.1–3.3):
//
//   - The screen is examined at settle time, never from a snapshot taken when
//     the mark arrived. ConPTY passes the mark through before it has
//     necessarily rendered the prompt text that precedes it, so a snapshot
//     taken at the mark can be empty; comparing against it failed every plain
//     `dir` and left the panels to a five-second fallback.
//   - The console title is unreadable behind a pseudoconsole, so there is no
//     title veto: cmd's "<title> - <command>" form never reaches us.
//   - A child process vetoes completion only if it is a console program that
//     is not itself cmd. A nested cmd prints the same prompt and is the shell
//     now; a GUI program (notepad) is not waited for by cmd, so cmd is already
//     at its prompt. The veto for a running console child is not bounded:
//     `ping -t` keeps the terminal busy for as long as it runs.
//
// The session is only created for the local Windows shell; the remote FISH+
// peer and Unix shells keep their own completion signals.
type cmdShellSession struct {
	mu sync.Mutex
	Pf *PanelsFrame

	// promptSeq counts every prompt-end mark; sentSeq is its value when the
	// first of the lines now outstanding was typed. A prompt can only answer
	// one of them if it was printed after that — a startup prompt still
	// crossing ConPTY does not count.
	promptSeq uint64
	sentSeq   uint64

	// pendingLines counts typed lines (a directory sync, a command, or
	// both queued back to back) whose own completion prompt has not been
	// seen yet. f4 does not wait for one typed line to settle before typing
	// the next: syncPTYDirectory's cwd ping runs at panel-path-change time,
	// unconditionally, and a command the user runs can follow it within the
	// same frame. On an ordinary shell the ping's prompt arrives long before
	// anything else is typed, so this never exceeds 1. But when the shell is
	// slow to print even its first prompt (a cold cmd.exe start took ~4s in
	// the field, #1376), both lines can already be queued when it finally
	// answers -- and a single "the most recent line" slot cannot tell the
	// sync's own, perfectly ordinary completion prompt apart from the
	// command's. Treated as the answer to whichever was tracked, it ended
	// the command (dropped f4 back to its panels, and in ShellModeHost off
	// the primary screen) while Far Manager had not even started. Each real
	// settle now retires one outstanding line for every mark it turns out to
	// answer (see skippedMarks below), and pendingLines only reaches zero
	// once the last one still owed a prompt has had its turn.
	//
	// notedAtSeq is promptSeq's value the last time a line was typed. It is
	// what tells the #1376 startup race (both lines queued while promptSeq
	// is still the same, unanswered value) apart from a plain sequence of
	// separate commands, each typed only after the previous one already got
	// its own prompt mark -- a nested shell (#1376's own regression test,
	// TestCmdSessionNestedCmdHoldsTerminal) held by a console child settles
	// into "held", never released, but its prompt mark still lands and moves
	// promptSeq. Without this, a command typed into that nested shell piled
	// its line onto the still-outstanding, merely-held one instead of
	// starting fresh, and the extra pendingLines it left behind was never
	// anyone's to retire: nothing on the wire answers a line nobody
	// remembers sending. pendingLines then never reached zero and the wait
	// for the outer shell's own prompt, after the nested shell was long
	// gone, never let go of the panels.
	pendingLines int
	notedAtSeq   uint64

	// skippedMarks counts prompt marks that arrived while an earlier mark's
	// settle() was still scheduled but had not run yet, so handleMark
	// replaced it (#1376, the Host-mode report that survived the fix above):
	// on a cold shell that queues both the sync ping and the command before
	// printing anything, it can then answer both within a few milliseconds
	// of each other -- well inside cmdPromptSettleDelay -- so the sync's
	// mark's own timer never fires; the command's mark's timer, scheduled
	// right after, cancels it first. settle() only ever runs for the mark
	// whose timer actually fires, so retiring one line per settle() call
	// left the sync's line stuck in pendingLines forever: nothing was ever
	// going to answer it individually, and a stuck pendingLines wedges f4 in
	// "Terminal (executing)" for good, since release() is the only thing
	// that ever clears it and it is only reached at zero. skippedMarks is
	// what tells this apart from the ordinary, well-separated case
	// (TestCmdSessionTwoQueuedLinesEachNeedTheirOwnPrompt): a prompt whose
	// own timer fires without being pre-empted answers exactly the one line
	// it was scheduled for, but a mark that pre-empted N earlier ones
	// answered N+1 lines that cmd, being a single serial shell, must have
	// finished in order to have gotten this far -- so settle() retires
	// 1+skippedMarks lines (capped at pendingLines) instead of exactly one,
	// and resets the counter once spent.
	skippedMarks int

	inBatch  bool // a .bat/.cmd file is being executed: nested cmd is not the shell
	observed terminal.PromptSnapshot
	timer    *time.Timer
	attempts int
	Closed   bool
}

// cmdPromptSettleDelay is how long after a prompt mark the screen is first
// examined. ConPTY renders the text that follows an echoed batch prompt within
// a frame or two; a prompt still alone after this is a prompt.
//
// Lowered from 150ms to keep the return-to-panels snappy: the settle check
// still requires two unchanged screen looks, so a shorter window only means we
// confirm prompt stability a few frames sooner.
var cmdPromptSettleDelay = 50 * time.Millisecond

// cmdPromptRecheckDelay is the poll interval while the prompt has not settled
// yet, or while a console child holds the terminal.
//
// Lowered from 250ms: this was the dominant ~250ms lag felt after external
// commands and batches finished. The prompt must be unchanged across two looks
// to release, so a tighter interval just polls faster without false positives.
var cmdPromptRecheckDelay = 100 * time.Millisecond

// cmdPromptMaxAttempts bounds how long a prompt-shaped screen that will not
// hold still is waited on before the wait is released (roughly five seconds).
// It bounds only that flickering-prompt case: a busy shell is waited on
// without limit (rescheduleWhileBusy), and a console child holds the terminal
// for as long as it runs. A batch step longer than five seconds is therefore
// no longer cut off -- that was the "batch runs in the background" bug.
var cmdPromptMaxAttempts = 20

// windowsShellPrompt marks the prompt start and end. The prompt end mark is
// printed after $P$G, so the cursor position at its arrival is the position
// the shell reads input from.
const windowsShellPrompt = `$E]133;A$E\$P$G$E]133;B$E\`

// childInspector is implemented by terminal.PTY backends that can list the shell's
// direct children. Backends that cannot are treated as having none.
type childInspector interface {
	ChildProcesses() []terminal.ChildProcess
}

// nestedShellImages are children that print a cmd-style prompt and accept
// the lines f4 types (cd /d "..." & command). Their prompt ends the outer
// command's wait, and the panels come back with the nested shell as the shell.
// cmd.exe is deliberately absent: it keeps the terminal in raw mode until
// the user leaves it (exit), like ssh or python. PowerShell is also absent
// for the same reason.
var nestedShellImages = map[string]bool{}

// childHoldsTerminal reports whether one of the shell's children is a console
// program f4 has to wait for.
func childHoldsTerminal(children []terminal.ChildProcess) bool {
	for _, c := range children {
		if c.GUI {
			continue
		}
		if nestedShellImages[strings.ToLower(c.Name)] {
			continue
		}
		return true
	}
	return false
}

// heldByChild reports whether one of the shell's children still holds the
// terminal, folding in the same batch exception the caller needs: in batch
// mode a nested cmd.exe child is not "the shell now" -- the batch file will
// continue after it exits, so the terminal is still held.
func heldByChild(children []terminal.ChildProcess, inBatch bool) bool {
	if childHoldsTerminal(children) {
		return true
	}
	if !inBatch {
		return false
	}
	for _, c := range children {
		if !c.GUI && strings.ToLower(c.Name) == "cmd.exe" {
			return true
		}
	}
	return false
}

// promptShaped reports whether text is what cmd's $P$G leaves in front of the
// cursor: a path with a drive, then ">". A batch line echo has the command
// text after the ">", a `set /p` prompt has no drive, and program output that
// happens to end in ">" has no path.
func promptShaped(text string) bool {
	text = strings.TrimRight(text, " ")
	return strings.HasSuffix(text, ">") && strings.Contains(text, `:\`)
}

func newCmdShellSession(pf *PanelsFrame) *cmdShellSession {
	return &cmdShellSession{Pf: pf}
}

// noteSent records that one more line was typed into the shell. It only
// piles onto whatever is already outstanding when no prompt mark has
// arrived since the previous line was typed -- the #1376 startup race,
// where syncPTYDirectory's ping and the command right after it can both be
// queued before the shell has printed a single byte. Once a mark has been
// seen since, whatever was outstanding has already had the state machine's
// attention (settled, held by a console child, still flickering -- settle
// decides which), even if it did not release: this new line starts its own
// fresh count rather than adding a line nobody will ever retire on its
// behalf, which left pendingLines never reaching zero. Only the first line
// of a fresh count sets sentSeq: it marks the threshold no prompt already
// in flight can cross, and it must not move while later lines queue up
// behind the first, or a prompt that only answers an earlier one would
// start looking like it predates the newest line too.
func (s *cmdShellSession) noteSent() {
	if s == nil {
		return
	}
	s.mu.Lock()
	if s.pendingLines == 0 || s.promptSeq != s.notedAtSeq {
		s.sentSeq = s.promptSeq
		if s.promptSeq == 0 {
			// No prompt has been seen yet, so the shell's startup prompt is
			// still on its way and will arrive after the line: it is not the
			// answer to it.
			s.sentSeq = 1
		}
		s.pendingLines = 0
		s.skippedMarks = 0
	}
	s.notedAtSeq = s.promptSeq
	s.pendingLines++
	s.mu.Unlock()
	s.Pf.noteLocalShellBusy(true)
}

// noteBatchExecution marks the current command as a .bat/.cmd file execution.
// In batch mode a nested cmd.exe child is not treated as "the shell now"
// because the batch file will continue after it exits.
func (s *cmdShellSession) NoteBatchExecution() {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.inBatch = true
	s.mu.Unlock()
}

// idle reports whether every typed line has been answered by a settled
// prompt, i.e. whether the shell is known to be reading input.
func (s *cmdShellSession) idle() bool {
	if s == nil {
		return true
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.pendingLines == 0
}

func (s *cmdShellSession) close() {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.Closed = true
	if s.timer != nil {
		s.timer.Stop()
	}
	s.mu.Unlock()
}

// childOwnsCommandMarks reports whether an OSC 133 C or D that just crossed
// the local shell's output belongs to a console program running inside the
// shell rather than to the shell. cmd.exe under the PROMPT f4 gives it prints
// A and B only, and completion of a typed line is this session's to decide
// from B. A console child that speaks shell integration itself prints its own
// C and D, though: Far Manager 3 wraps every command run from its command
// line in D, A, B, C ... D (#1376). Taken for cmd's, Far's first D ended the
// line that started Far, and f4's panels and hotkeys came back over a Far
// that was still running.
//
// It runs on the goroutine that parses the terminal output.
func (s *cmdShellSession) childOwnsCommandMarks() bool {
	if s == nil {
		return false
	}
	s.mu.Lock()
	closed, inBatch := s.Closed, s.inBatch
	s.mu.Unlock()
	if closed {
		return false
	}
	inspector, ok := s.Pf.localPTY().(childInspector)
	if !ok {
		return false
	}
	children := inspector.ChildProcesses()
	if !heldByChild(children, inBatch) {
		return false
	}
	vtui.DebugLog("CMD_SESSION: OSC 133 C/D printed by child %v, not by the shell; ignored", children)
	return true
}

// handleMark runs on the terminal.PTY goroutine for every OSC 133 mark of the local
// shell.
func (s *cmdShellSession) handleMark(mark string, snap terminal.PromptSnapshot) {
	if s == nil || mark != "B" {
		return
	}
	s.mu.Lock()
	if s.Closed {
		s.mu.Unlock()
		return
	}
	s.promptSeq++
	s.observed = snap
	s.attempts = 0
	seq := s.promptSeq
	if s.timer != nil {
		// Stop reports whether the previous mark's settle was still
		// scheduled and got cancelled here, rather than having already run
		// (or already fired and be running right now): only then did this
		// new mark rob it of its own look at the screen, so only then does
		// it owe skippedMarks a line (see the skippedMarks field doc above
		// and settle()'s retire step below).
		if s.timer.Stop() {
			s.skippedMarks++
		}
	}
	s.timer = time.AfterFunc(cmdPromptSettleDelay, func() { s.settle(seq) })
	s.mu.Unlock()
}

// settle examines, on the UI goroutine, whether prompt seq is the shell
// waiting for input, and if so ends whatever f4 was waiting for.
func (s *cmdShellSession) settle(seq uint64) {
	// Close can race the timer callback while test cleanup replaces the global
	// frame manager. Check liveness while holding the session lock, before
	// touching that global, so a stopped callback cannot read a manager that
	// cleanup is restoring.
	s.mu.Lock()
	if s.Closed || seq != s.promptSeq {
		s.mu.Unlock()
		return
	}
	manager := vtui.FrameManager
	s.mu.Unlock()
	if manager == nil {
		return
	}
	manager.PostTask(func() {
		s.mu.Lock()
		if s.Closed || seq != s.promptSeq {
			s.mu.Unlock()
			return
		}
		previous, sentSeq, pendingLines := s.observed, s.sentSeq, s.pendingLines
		inBatch := s.inBatch
		s.mu.Unlock()

		pf := s.Pf
		if pf.TermView == nil {
			return
		}

		// What is on screen now, not what was there when the mark arrived.
		current := pf.TermView.PromptSnapshot()

		if pf.TermView.UseAltScreen || !promptShaped(current.Text) {
			// The screen is not a prompt: an interactive full-screen program,
			// a batch line still echoing, program output, or a batch step
			// that is simply taking its time (`timeout /t 10`, a `pause` the
			// user is reading). f4 waits, with no bound on how long -- a
			// batch step is as long as it is, and returning the panels in the
			// middle of one is the "batch runs in the background" bug over
			// again. The escape hatch, if the shell truly wedged, is Ctrl+O,
			// which is not gated on the terminal being busy. A later prompt
			// mark reschedules a fresh look; failing that, keep polling.
			s.rescheduleWhileBusy(seq)
			return
		}

		if current != previous {
			// The screen is prompt-shaped but changed since the last look:
			// the prompt is still being drawn. ConPTY can pass the prompt
			// mark through before the text in front of it, so the first look
			// often lands here. This settles within a frame or two, so a
			// short bounded retry is right -- and bounded so a prompt that
			// flickers forever cannot strand the wait.
			s.mu.Lock()
			if !s.Closed && seq == s.promptSeq {
				s.observed = current
			}
			s.mu.Unlock()
			s.retryOrRelease(seq)
			return
		}

		if pendingLines > 0 && seq <= sentSeq {
			// This prompt was printed before the line we typed; the shell has
			// not even started on it. The prompt for our line will come.
			vtui.DebugLog("CMD_SESSION: prompt %d predates the typed line (sent=%d), ignoring", seq, sentSeq)
			return
		}

		// children is captured here so it can be logged below whichever way
		// this comes out (held, or falling through to settled/release):
		// f4#1376's residual "cls inside Far still drops to f4" report went
		// through four earlier rounds, PRs #1451/#1495/#1498/#1608, each
		// fixing a real but distinct bug in this file; a fifth confirmed
		// cause -- the mark-coalescing bug retired at the bottom of this
		// function, see skippedMarks' field doc -- reproduced in every one of
		// the field's Host-mode debug logs, with no Far involved at all, so
		// it is unlikely to be the whole story behind every report filed
		// against this ticket. One candidate this scan still cannot rule out
		// from the code alone is that ChildProcesses -- a live, uncached,
		// direct-children-only Toolhelp32 scan of the *outer* shell --
		// momentarily does not list the console child (Far) at exactly the
		// moment Far spawns its own grandchild (a console command or program
		// run from inside it). Logging what this scan actually returned
		// right at the settle/release decision, instead of only when it
		// holds, is what the next --debug capture of the report needs: if
		// `children` reads empty on the very call that releases the wait
		// while Far is still visibly running on screen, that confirms this
		// function as the source; if it is never empty, the drop is not
		// coming from here at all.
		var children []terminal.ChildProcess
		if inspector, ok := pf.localPTY().(childInspector); ok {
			children = inspector.ChildProcesses()
			if heldByChild(children, inBatch) {
				vtui.DebugLog("CMD_SESSION: prompt %d held by child %v, rechecking", seq, children)
				s.mu.Lock()
				if !s.Closed && seq == s.promptSeq {
					// Reset the attempt counter for the same reason
					// rescheduleWhileBusy and retryOrRelease's own veto do:
					// a console child genuinely holding the terminal is not
					// "stuck settling", so whatever budget an earlier,
					// unrelated bit of flicker already spent must not carry
					// into whatever flicker comes next. Without this, Far
					// Manager (#1376) sitting at its own idle, prompt-shaped
					// command line -- unchanged and held for as long as the
					// user takes before typing anything -- silently freezes
					// the counter wherever settling into that steady state
					// left it, so the very next flicker (`cls` redrawing
					// that same line) can reach retryOrRelease's
					// bound-exceeded fallback, and therefore have to trust a
					// single, uncached child-process scan, after only one or
					// two looks instead of the intended ~cmdPromptMaxAttempts
					// of them.
					s.attempts = 0
					s.timer = time.AfterFunc(cmdPromptRecheckDelay, func() { s.settle(seq) })
				}
				s.mu.Unlock()
				return
			}
		}

		// This settle answers one outstanding line for its own mark, plus
		// one more for every earlier mark that never got its own settle
		// call because this one's timer pre-empted it (skippedMarks,
		// handleMark). cmd is a single serial shell: it cannot have printed
		// this many real prompts since sentSeq without having finished that
		// many typed lines in order, even though this session only watched
		// the last one settle (#1376 -- see skippedMarks' field doc for the
		// Host-mode report this fixes). The count is capped at pendingLines
		// so a skip left over from a mark that turned out to predate the
		// typed lines (sentSeq's own veto above) cannot retire a line that
		// was never sent. The execution ends once this reaches zero; if it
		// does not, the next real prompt mark schedules another settle via
		// handleMark, which will retire what is left the same way.
		s.mu.Lock()
		retired := 0
		if !s.Closed && seq == s.promptSeq && s.pendingLines > 0 {
			retired = 1 + s.skippedMarks
			if retired > s.pendingLines {
				retired = s.pendingLines
			}
			s.pendingLines -= retired
			s.skippedMarks = 0
		}
		remaining := s.pendingLines
		s.mu.Unlock()

		if remaining > 0 {
			vtui.DebugLog("CMD_SESSION: prompt %d settled %d of the outstanding lines (sent=%d children=%v), %d left", seq, retired, sentSeq, children, remaining)
			return
		}

		vtui.DebugLog("CMD_SESSION: prompt %d settled (sent=%d children=%v)", seq, sentSeq, children)
		s.release()
	})
}

// rescheduleWhileBusy looks again after cmdPromptRecheckDelay, with no
// attempt limit: the shell is busy and f4 waits for as long as that lasts.
// The attempt counter is reset so that the bounded settle-retry, once the
// screen does become a prompt, gets its full budget of looks.
func (s *cmdShellSession) rescheduleWhileBusy(seq uint64) {
	s.mu.Lock()
	if !s.Closed && seq == s.promptSeq {
		s.attempts = 0
		s.timer = time.AfterFunc(cmdPromptRecheckDelay, func() { s.settle(seq) })
	}
	s.mu.Unlock()
}

// retryOrRelease looks at prompt seq again shortly while a prompt-shaped
// screen is still settling, and after cmdPromptMaxAttempts releases the wait.
// This bound applies only to a prompt that will not hold still -- never to a
// busy shell, which rescheduleWhileBusy waits on without limit. Releasing
// rather than holding matters because a stuck wait leaves pf.executing set,
// and isPtyBusy reports that as busy, disabling every hotkey gated on a quiet
// terminal (Esc) while leaving the ungated ones (Ctrl+O) alive.
//
// But a console child that is still running is never "stuck settling" --
// it is simply busy, exactly like the held branch in settle() below, and
// must get the same veto before the bound gives up on it. Without this, a
// full-screen program with no alternate screen of its own (Far Manager,
// #1376) whose own command line happens to look prompt-shaped (a bare
// "C:\path>") can flicker across a few looks while it repaints -- `cls`
// clears and redraws that exact line -- and exhausting the retry budget
// then released the wait and handed the terminal back to f4 while Far was
// still very much running.
func (s *cmdShellSession) retryOrRelease(seq uint64) {
	s.mu.Lock()
	if s.Closed || seq != s.promptSeq {
		s.mu.Unlock()
		return
	}
	s.attempts++
	if s.attempts < cmdPromptMaxAttempts {
		s.timer = time.AfterFunc(cmdPromptRecheckDelay, func() { s.settle(seq) })
		s.mu.Unlock()
		return
	}
	inBatch := s.inBatch
	s.mu.Unlock()

	// children is logged below either way, for the same #1376 diagnostic
	// reason settle() now logs it at its own settled/release call: the open
	// question after four earlier fix rounds (#1451/#1495/#1498/#1608) is
	// whether this live, uncached, direct-children-only scan of the outer
	// shell ever reads empty here while Far is still genuinely running --
	// which is what the next --debug capture of the report needs to show.
	var children []terminal.ChildProcess
	if inspector, ok := s.Pf.localPTY().(childInspector); ok {
		children = inspector.ChildProcesses()
	}
	if heldByChild(children, inBatch) {
		vtui.DebugLog("CMD_SESSION: prompt %d flickering but held by a child, rechecking", seq)
		s.mu.Lock()
		if !s.Closed && seq == s.promptSeq {
			s.attempts = 0
			s.timer = time.AfterFunc(cmdPromptRecheckDelay, func() { s.settle(seq) })
		}
		s.mu.Unlock()
		return
	}

	vtui.DebugLog("CMD_SESSION: prompt %d never settled, releasing the wait (children=%v)", seq, children)
	s.release()
}

// release ends the wait: the shell is taken to be at its prompt.
func (s *cmdShellSession) release() {
	pf := s.Pf
	s.mu.Lock()
	s.pendingLines = 0
	s.skippedMarks = 0
	s.inBatch = false
	s.mu.Unlock()
	pf.ShellPromptReady = true
	pf.ignoreNextPrompt = false
	if pf.Executing {
		pf.endExecution()
	}
	pf.noteLocalShellBusy(false)
	pf.catchUpProcessEnvironment(true)
	// The shell is idle at a prompt with no console child: the one moment
	// a resize reaches nothing that would react to it.
}
