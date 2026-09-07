package main

import (
	"sync"
	"time"
)

// terminalRedrawInterval bounds how often a burst of PTY output can wake the
// UI renderer. PTY data is still parsed immediately; only redundant frame
// requests are coalesced while the renderer is catching up.
const terminalRedrawInterval = 16 * time.Millisecond

type terminalRedrawScheduler struct {
	mu      sync.Mutex
	pending bool
	stopped bool
	redraw  func()
}

func newTerminalRedrawScheduler(redraw func()) *terminalRedrawScheduler {
	return &terminalRedrawScheduler{redraw: redraw}
}

func (s *terminalRedrawScheduler) request() {
	if s == nil {
		return
	}

	s.mu.Lock()
	if s.stopped || s.pending {
		s.mu.Unlock()
		return
	}
	s.pending = true
	redraw := s.redraw
	s.mu.Unlock()

	// Keep the first frame of a burst responsive, then suppress further
	// requests until the interval expires. FrameManager.Redraw itself is
	// asynchronous and non-blocking, so this is safe from the PTY reader.
	if redraw != nil {
		redraw()
	}
	time.AfterFunc(terminalRedrawInterval, func() {
		s.mu.Lock()
		s.pending = false
		s.mu.Unlock()
	})
}

func (s *terminalRedrawScheduler) stop() {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.stopped = true
	s.pending = false
	s.mu.Unlock()
}
