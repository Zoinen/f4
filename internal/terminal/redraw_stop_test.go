package terminal

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// Stop has to join a redraw that is already running. The panels frame's redraw
// reads the global vtui.FrameManager, and tests replace that global right after
// closing the frame; a callback still on its way through the timer was reported
// by the race detector as a data race against that replacement.
func TestTerminalRedrawSchedulerStopWaitsForRunningRedraw(t *testing.T) {
	var once sync.Once
	started := make(chan struct{})
	release := make(chan struct{})
	var finished atomic.Bool

	s := NewTerminalRedrawScheduler(func() {
		once.Do(func() { close(started) })
		<-release
		finished.Store(true)
	})

	requested := make(chan struct{})
	go func() {
		s.Request()
		close(requested)
	}()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("redraw never started")
	}

	stopped := make(chan struct{})
	go func() {
		s.Stop()
		close(stopped)
	}()
	select {
	case <-stopped:
		t.Fatal("Stop returned while a redraw was still running")
	case <-time.After(100 * time.Millisecond):
	}

	close(release)
	select {
	case <-stopped:
	case <-time.After(5 * time.Second):
		t.Fatal("Stop did not return after the redraw finished")
	}
	if !finished.Load() {
		t.Fatal("Stop returned before the redraw finished")
	}
	<-requested
}

func TestTerminalRedrawSchedulerStopWithNothingRunningReturns(t *testing.T) {
	s := NewTerminalRedrawScheduler(func() {})
	done := make(chan struct{})
	go func() {
		s.Stop()
		s.Stop() // a second Stop is harmless
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Stop blocked with no redraw running")
	}
}

// A request after Stop must not reach the callback, and the timer of a window
// opened before Stop must not either.
func TestTerminalRedrawSchedulerNoRedrawAfterStop(t *testing.T) {
	var calls atomic.Int32
	s := NewTerminalRedrawScheduler(func() { calls.Add(1) })
	s.Request()
	s.Request() // opens a trailing frame: missed is set
	s.Stop()
	before := calls.Load()
	time.Sleep(3 * terminalRedrawInterval)
	s.Request()
	if got := calls.Load(); got != before {
		t.Fatalf("redraw ran %d time(s) after Stop", got-before)
	}
}
