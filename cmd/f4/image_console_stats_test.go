package main

import (
	"strings"
	"testing"
	"time"

	"github.com/unxed/f4/internal/wincon"
)

// The report exists to be pasted into an issue, so the two things it has to
// get right are that it is bounded — one line a second at the very most — and
// that the line carries the numbers that tell the three reports on #805 apart:
// how much of the second went into scaling, and what the pump thread did.

func TestNothingIsPrintedBeforeThePeriodIsOver(t *testing.T) {
	var s consoleOverlayStats
	t0 := time.Unix(1000, 0)

	if line := s.report(t0, wincon.Stats{}); line != "" {
		t.Fatalf("the first call primes the period: %q", line)
	}
	s.frame()
	s.change()
	if line := s.report(t0.Add(999*time.Millisecond), wincon.Stats{Paints: 1}); line != "" {
		t.Errorf("a line arrived before the second was up: %q", line)
	}
}

func TestThePeriodCarriesTheNumbersThatMatter(t *testing.T) {
	var s consoleOverlayStats
	t0 := time.Unix(2000, 0)
	s.report(t0, wincon.Stats{})

	for i := 0; i < 60; i++ {
		s.frame()
	}
	s.change()
	s.scaled(400 * time.Millisecond)
	s.scaled(120 * time.Millisecond)
	s.placed(3 * time.Millisecond)

	line := s.report(t0.Add(time.Second), wincon.Stats{Applies: 3, Moves: 1, Paints: 2, Blank: 1})
	for _, want := range []string{"frames=60", "new=1", "scale=520ms/400ms", "paint=2", "blank=1"} {
		if !strings.Contains(line, want) {
			t.Errorf("%q is missing from %q", want, line)
		}
	}
}

// A second in which nothing happened is not worth a line. This is what keeps a
// log small enough to read while f4 sits on a picture doing nothing.
func TestAQuietPeriodSaysNothing(t *testing.T) {
	var s consoleOverlayStats
	t0 := time.Unix(3000, 0)
	pump := wincon.Stats{Applies: 5}
	s.report(t0, pump)

	if line := s.report(t0.Add(2*time.Second), pump); line != "" {
		t.Errorf("got %q, want silence", line)
	}
}

// The counters are the period's, not the run's, so a report starts a fresh
// one. A total would hide a machine that goes slow only after a few pictures,
// which is exactly what one of the reports says happens.
func TestAReportStartsAFreshPeriod(t *testing.T) {
	var s consoleOverlayStats
	t0 := time.Unix(4000, 0)
	s.report(t0, wincon.Stats{})

	s.frame()
	s.scaled(50 * time.Millisecond)
	if line := s.report(t0.Add(time.Second), wincon.Stats{Paints: 1}); line == "" {
		t.Fatal("the first period had work in it and said nothing")
	}

	s.frame()
	line := s.report(t0.Add(2*time.Second), wincon.Stats{Paints: 1})
	if !strings.Contains(line, "frames=1") {
		t.Errorf("got %q, want the second period alone", line)
	}
	if strings.Contains(line, "scale=") {
		t.Errorf("got %q, want no scaling carried over", line)
	}
	if !strings.Contains(line, "paint=0") {
		t.Errorf("got %q, want the pump counters as a difference", line)
	}
}

// Giving up is the interesting case: it is the one that leaves the reader
// looking at a black rectangle, and the reason is the whole of the diagnosis.
func TestGivingUpIsNamed(t *testing.T) {
	var s consoleOverlayStats
	t0 := time.Unix(5000, 0)
	s.report(t0, wincon.Stats{})

	s.frame()
	s.gaveUp("no client size")
	line := s.report(t0.Add(time.Second), wincon.Stats{})
	if !strings.Contains(line, "gaveup=1(no client size)") {
		t.Errorf("got %q, want the reason", line)
	}
}
