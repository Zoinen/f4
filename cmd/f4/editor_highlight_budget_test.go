package main

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/unxed/f4/piecetable"
	"github.com/unxed/vtui"
)

// fakeHLClock replaces the slice budget clock, so a "slow" highlighter costs
// exact fake time instead of real sleeps. Budget tests then assert equalities
// instead of wall-clock bounds that a descheduled CI runner blows through.
type fakeHLClock struct{ now time.Time }

func (c *fakeHLClock) get() time.Time { return c.now }

func installFakeHLClock(t *testing.T) *fakeHLClock {
	t.Helper()
	c := &fakeHLClock{now: time.Unix(0, 0)}
	prev := hlNow
	hlNow = c.get
	t.Cleanup(func() { hlNow = prev })
	return c
}

// hlSliceLines is how many lines one slice processes before the budget stops
// it, for a highlighter costing perLine of fake time: the first clock-stride
// multiple at which the elapsed fake time reaches the budget.
func hlSliceLines(perLine time.Duration) int {
	lines := 0
	for {
		lines += hlClockStride
		if time.Duration(lines)*perLine >= hlSliceBudget {
			return lines
		}
	}
}

// mockSlowHighlighter charges a fixed amount of fake time per line, standing
// in for a parser whose per-line cost is high enough that a fixed batch size
// turns into a visible freeze (Colorer through wasm, in practice).
type mockSlowHighlighter struct {
	perLine time.Duration
	clock   *fakeHLClock
	calls   int
}

func (m *mockSlowHighlighter) Highlight(line string, prev any, base uint64) ([]uint64, any) {
	m.calls++
	if m.clock != nil {
		m.clock.now = m.clock.now.Add(m.perLine)
	}
	depth := 0
	if prev != nil {
		depth = prev.(int)
	}
	return make([]uint64, len(line)), depth + 1
}

func buildLinesPT(t *testing.T, n int) *piecetable.PieceTable {
	t.Helper()
	var sb strings.Builder
	for i := 0; i < n; i++ {
		if _, err := fmt.Fprintf(&sb, "line %d\n", i); err != nil {
			t.Fatal(err)
		}
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
	clock := installFakeHLClock(t)

	const perLine = time.Millisecond

	ev := NewEditorView(buildLinesPT(t, 5000), nil, "test.txt")
	defer ev.Close()
	ev.SetPosition(0, 0, 80, 10)
	ev.highlighter = &mockSlowHighlighter{perLine: perLine, clock: clock}

	plan := ev.highlightSlice(0)

	if plan.done {
		t.Error("a budgeted slice cannot have finished 5000 slow lines")
	}
	if want := hlSliceLines(perLine); plan.lines != want {
		t.Errorf("slice ran for %d lines, budget stops it at exactly %d", plan.lines, want)
	}
	if want := time.Duration(plan.lines) * perLine; plan.work != want {
		t.Errorf("slice reported %v of work, %d lines cost exactly %v", plan.work, plan.lines, want)
	}
	if len(ev.lineStates) != plan.lines {
		t.Errorf("state chain grew by %d, slice reported %d", len(ev.lineStates), plan.lines)
	}
}

// While the line index is still being built the walker has to get out of its
// way, because the index is what the scroll bar and every jump wait for.
func TestEditor_HighlightSlice_YieldsWhileIndexing(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	clock := installFakeHLClock(t)

	ev := NewEditorView(buildLinesPT(t, 5000), nil, "test.txt")
	defer ev.Close()
	ev.SetPosition(0, 0, 80, 10)
	ev.highlighter = &mockSlowHighlighter{perLine: 200 * time.Microsecond, clock: clock}

	ev.indexing = false
	normal := ev.highlightSlice(0)

	ev.indexing = true
	throttled := ev.highlightSlice(0)

	if want := highlightIdleGap(normal.work, hlDutyAhead); normal.idle != want {
		t.Errorf("idle gap outside indexing is %v, want %v", normal.idle, want)
	}
	if want := highlightIdleGap(throttled.work, hlDutyIndexing); throttled.idle != want {
		t.Errorf("idle gap while indexing is %v, want %v", throttled.idle, want)
	}
	if normal.idle >= hlIdleMax || throttled.idle >= hlIdleMax {
		t.Fatalf("fake-clock slices must not clamp at hlIdleMax (normal %v, throttled %v)",
			normal.idle, throttled.idle)
	}
	if throttled.idle <= normal.idle {
		t.Errorf("walker must back off harder while indexing: idle %v is not above %v",
			throttled.idle, normal.idle)
	}
}

// End to end through the task queue: the chain still gets built, and no single
// task consumes more than one slice budget's worth of highlighting. The stall
// is asserted in the fake clock's currency — lines processed per task — not in
// real elapsed time, which on a shared CI runner measures the machine's load,
// not this code.
func TestEditor_BackgroundWalker_NoLongUIStalls(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	clock := installFakeHLClock(t)

	const total = 3000
	const perLine = 50 * time.Microsecond
	maxLines := hlSliceLines(perLine)

	ev := NewEditorView(buildLinesPT(t, total), nil, "test.txt")
	defer ev.Close()
	ev.SetPosition(0, 0, 80, 10)
	ev.highlighter = &mockSlowHighlighter{perLine: perLine, clock: clock}

	ev.startHighlighting()

	timeout := time.After(20 * time.Second)
	for len(ev.lineStates) < total {
		select {
		case task := <-vtui.FrameManager.TaskChan:
			before := len(ev.lineStates)
			task()
			if grew := len(ev.lineStates) - before; grew > maxLines {
				t.Fatalf("a single UI task highlighted %d lines, the budget stops a slice at %d", grew, maxLines)
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

	ev := NewEditorView(buildLinesPT(t, 5000), nil, "test.txt")
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
