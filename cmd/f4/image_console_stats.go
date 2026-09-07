package main

// What the console overlay is costing, in one line a second.
//
// The three reports on issue #805 disagree — one machine hangs, one is only
// black, one runs hot after a few pictures — and none of them can be told
// apart from a screenshot. A line per frame would be a flood, and on this
// path a flood is also a lie: composing a frame here scales the picture on
// the thread that holds the screen lock, so the log would be measuring the
// logging. So it counts, and prints a summary of the period: how many frames
// arrived, how many of them were new work, where the milliseconds went, and
// what the pump thread did with them.
//
// Everything here is touched from the frame-composing thread only, except the
// pump counters, which are atomic in wincon.

import (
	"fmt"
	"strings"
	"time"

	"github.com/unxed/f4/internal/wincon"
	"github.com/unxed/vtui"
)

// consoleStatsPeriod is how often a line may be printed at the very most.
const consoleStatsPeriod = time.Second

type consoleOverlayStats struct {
	since time.Time

	frames  int // RenderExternal calls
	changes int // frames that were not the one before
	hides   int
	reason  string // why the last frame gave up, empty if it did not

	scale time.Duration // scaling the picture into the frame buffer
	worst time.Duration // the slowest single scale of the period
	place time.Duration // Place, SetBounds and Draw together
	pump  wincon.Stats  // the reading at the end of the last period
}

func (s *consoleOverlayStats) frame() { s.frames++ }

func (s *consoleOverlayStats) change() { s.changes++ }

func (s *consoleOverlayStats) gaveUp(reason string) {
	s.hides++
	s.reason = reason
}

func (s *consoleOverlayStats) scaled(d time.Duration) {
	s.scale += d
	if d > s.worst {
		s.worst = d
	}
}

func (s *consoleOverlayStats) placed(d time.Duration) { s.place += d }

// report prints the period and starts a new one, if the period is over and
// anything happened in it. now is a parameter so the test does not sleep.
func (s *consoleOverlayStats) report(now time.Time, pump wincon.Stats) string {
	if s.since.IsZero() {
		s.since = now
		s.pump = pump
		return ""
	}
	elapsed := now.Sub(s.since)
	if elapsed < consoleStatsPeriod {
		return ""
	}
	delta := pump.Sub(s.pump)
	if s.frames == 0 && delta.Empty() {
		s.since = now
		return ""
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "WINCON: %.1fs frames=%d new=%d", elapsed.Seconds(), s.frames, s.changes)
	if s.scale > 0 {
		fmt.Fprintf(&sb, " scale=%dms/%dms", s.scale.Milliseconds(), s.worst.Milliseconds())
	}
	if s.place > 0 {
		fmt.Fprintf(&sb, " window=%dms", s.place.Milliseconds())
	}
	fmt.Fprintf(&sb, " pump=%d move=%d rgn=%d inval=%d paint=%d blank=%d",
		delta.Applies, delta.Moves, delta.Regions, delta.Invalidates, delta.Paints, delta.Blank)
	if s.hides > 0 {
		fmt.Fprintf(&sb, " gaveup=%d(%s)", s.hides, s.reason)
	}

	*s = consoleOverlayStats{since: now, pump: pump}
	return sb.String()
}

// flush prints the period if it is over. Called at the end of every frame,
// because there is no other thread here that could do it on a timer.
func (s *consoleOverlayStats) flush(pump wincon.Stats) {
	if line := s.report(time.Now(), pump); line != "" {
		vtui.DebugLog("%s", line)
	}
}
