package main

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/unxed/f4/piecetable"
	"github.com/unxed/vtui"
)

// mockSlowHighlighter spends a fixed amount of time per line, standing in for
// a parser whose per-line cost is high enough that a fixed batch size turns
// into a visible freeze (Colorer through wasm, in practice).
type mockSlowHighlighter struct {
	perLine time.Duration
	calls   int
}

func (m *mockSlowHighlighter) Highlight(line string, prev any, base uint64) ([]uint64, any) {
	m.calls++
	if m.perLine > 0 {
		time.Sleep(m.perLine)
	}
	depth := 0
	if prev != nil {
		depth = prev.(int)
	}
	return make([]uint64, len(line)), depth + 1
}

func buildLinesPT(n int) *piecetable.PieceTable {
	var sb strings.Builder
	for i := 0; i < n; i++ {
		fmt.Fprintf(&sb, "line %d\n", i)
	}
	return piecetable.New([]byte(sb.String()))
}

func TestHighlightIdleGap(t *testing.T) {
	work := 4 * time.Millisecond

	if got := highlightIdleGap(work, 50); got != work {
		t.Errorf("50%% duty should idle for as long as it worked, got %v want %v", got, work)
	}
	if got := highlightIdleGap(work, 25); got != 3*work {
		t.Errorf("25%% duty should idle for three times the work, got %v want %v", got, 3*work)
	}
	if got := highlightIdleGap(work, 10); got != 9*work {
		t.Errorf("10%% duty should idle for nine times the work, got %v want %v", got, 9*work)
	}
	if got := highlightIdleGap(work, 100); got != 0 {
		t.Errorf("100%% duty should not idle at all, got %v", got)
	}
	if got := highlightIdleGap(work, 0); got != hlIdleMax {
		t.Errorf("a zero duty should idle for the maximum, got %v want %v", got, hlIdleMax)
	}
	if got := highlightIdleGap(time.Nanosecond, 50); got != hlIdleMin {
		t.Errorf("idle gap must not fall below hlIdleMin, got %v", got)
	}
	if got := highlightIdleGap(time.Second, 10); got != hlIdleMax {
		t.Errorf("idle gap must be capped at hlIdleMax, got %v", got)
	}
}

func TestHighlightDuty(t *testing.T) {
	if highlightDuty(true, true) != hlDutyIndexing {
		t.Error("indexing must win over a waiting viewport: the index is what blocks the user")
	}
	if highlightDuty(false, true) != hlDutyIndexing {
		t.Error("indexing must throttle the walker")
	}
	if highlightDuty(true, false) != hlDutyVisible {
		t.Error("a viewport still without colours deserves the larger share")
	}
	if highlightDuty(false, false) != hlDutyAhead {
		t.Error("walking ahead of the viewport should take the smaller share")
	}
	if hlDutyIndexing >= hlDutyAhead || hlDutyAhead >= hlDutyVisible {
		t.Error("duty levels must stay ordered: indexing < ahead < visible")
	}
}

func TestNextIndexPoll(t *testing.T) {
	if got := nextIndexPoll(indexPollMin); got != 2*indexPollMin {
		t.Errorf("poll should double, got %v", got)
	}
	if got := nextIndexPoll(indexPollMax); got != indexPollMax {
		t.Errorf("poll should saturate at indexPollMax, got %v", got)
	}
	if got := nextIndexPoll(0); got != indexPollMin {
		t.Errorf("poll should never drop below indexPollMin, got %v", got)
	}
	if indexPollMax >= 20*time.Millisecond {
		t.Error("the poll cap must stay well under the old fixed 20ms sleep")
	}
}

// The regression this whole change is about: one slice must cost a bounded
// amount of UI time no matter how slow the highlighter is. The previous
// implementation walked a fixed 200 or 2500 lines per slice, which is a
// multi-second freeze with a parser this slow.
func TestEditor_HighlightSlice_StopsOnBudget(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())

	ev := NewEditorView(buildLinesPT(5000), nil, "test.txt")
	defer ev.Close()
	ev.SetPosition(0, 0, 80, 10)
	ev.highlighter = &mockSlowHighlighter{perLine: time.Millisecond}

	plan := ev.highlightSlice(0)

	if plan.lines == 0 {
		t.Fatal("slice highlighted nothing")
	}
	if plan.done {
		t.Error("a budgeted slice cannot have finished 5000 slow lines")
	}
	if plan.lines > 8*hlClockStride {
		t.Errorf("slice ran for %d lines, budget should have stopped it near %d", plan.lines, hlClockStride)
	}
	if plan.work > 20*hlSliceBudget {
		t.Errorf("slice occupied the UI thread for %v, budget is %v", plan.work, hlSliceBudget)
	}
	if len(ev.lineStates) != plan.lines {
		t.Errorf("state chain grew by %d, slice reported %d", len(ev.lineStates), plan.lines)
	}
}

// While the line index is still being built the walker has to get out of its
// way, because the index is what the scroll bar and every jump wait for.
func TestEditor_HighlightSlice_YieldsWhileIndexing(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())

	ev := NewEditorView(buildLinesPT(5000), nil, "test.txt")
	defer ev.Close()
	ev.SetPosition(0, 0, 80, 10)
	ev.highlighter = &mockSlowHighlighter{perLine: 200 * time.Microsecond}

	ev.indexing = false
	normal := ev.highlightSlice(0)

	ev.indexing = true
	throttled := ev.highlightSlice(0)

	// Compared against the plan's own measured work, so the assertion holds
	// on a loaded CI box as well as on a fast desktop.
	if want := highlightIdleGap(normal.work, hlDutyAhead); normal.idle != want {
		t.Errorf("idle gap outside indexing is %v, want %v", normal.idle, want)
	}
	if want := highlightIdleGap(throttled.work, hlDutyIndexing); throttled.idle != want {
		t.Errorf("idle gap while indexing is %v, want %v", throttled.idle, want)
	}
	if normal.idle >= hlIdleMax || throttled.idle >= hlIdleMax {
		return // both clamped, the ratio below would prove nothing
	}
	if throttled.idle <= normal.idle {
		t.Errorf("walker must back off harder while indexing: idle %v is not above %v",
			throttled.idle, normal.idle)
	}
}

// End to end through the task queue: the chain still gets built, and no single
// task blocks the UI thread for long enough to be felt.
func TestEditor_BackgroundWalker_NoLongUIStalls(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())

	const total = 3000
	const maxStall = 150 * time.Millisecond

	ev := NewEditorView(buildLinesPT(total), nil, "test.txt")
	defer ev.Close()
	ev.SetPosition(0, 0, 80, 10)
	ev.highlighter = &mockSlowHighlighter{perLine: 50 * time.Microsecond}

	ev.startHighlighting()

	timeout := time.After(20 * time.Second)
	for len(ev.lineStates) < total {
		select {
		case task := <-vtui.FrameManager.TaskChan:
			began := time.Now()
			task()
			if stall := time.Since(began); stall > maxStall {
				t.Fatalf("a single UI task ran for %v, slice budget is %v", stall, hlSliceBudget)
			}
		case <-timeout:
			t.Fatalf("walker stalled at %d of %d lines", len(ev.lineStates), total)
		}
	}

	// Drain the walker's own shutdown tasks so it does not outlive the test.
	drain := time.After(200 * time.Millisecond)
	for {
		select {
		case task := <-vtui.FrameManager.TaskChan:
			task()
		case <-drain:
			return
		}
	}
}

// Colorer must stay out of the walker. Its "state" is a line number, the real
// one lives in a forward-only wasm session, and dragging that session through
// the file is what left a viewport opened in the middle of a large file
// without colours: the walk fills the session with lines nobody is looking at
// and parks it ahead of the viewport, so every frame has to rewind it.
func TestEditor_HighlightWalker_SkipsColorer(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())

	if usesStateChain(&ColorerHighlighter{}) {
		t.Error("Colorer's state cannot be carried in the chain")
	}
	if !usesStateChain(&mockSlowHighlighter{}) {
		t.Error("a highlighter that returns a real state must keep the walker")
	}
	if usesStateChain(nil) {
		t.Error("no highlighter, nothing to walk")
	}

	ev := NewEditorView(buildLinesPT(5000), nil, "test.txt")
	defer ev.Close()
	ev.SetPosition(0, 0, 80, 10)
	ev.highlighter = &ColorerHighlighter{}

	plan := ev.highlightSlice(0)
	if plan.lines != 0 {
		t.Errorf("slice walked %d lines of a Colorer session", plan.lines)
	}
	if !plan.done {
		t.Error("a slice that must not run has to report itself finished, or the walker loops on it")
	}
	if len(ev.lineStates) != 0 {
		t.Errorf("state chain grew to %d for a highlighter that has no state", len(ev.lineStates))
	}

	ev.startHighlighting()
	if ev.highlighting {
		t.Error("walker started for Colorer")
	}
}
