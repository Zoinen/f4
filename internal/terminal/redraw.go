package terminal

import (
	"sync"
	"time"
)

// terminalRedrawInterval bounds how often a burst of PTY output can wake the
// UI renderer. PTY data is still parsed immediately; only redundant frame
// requests are coalesced while the renderer is catching up.
const terminalRedrawInterval = 16 * time.Millisecond

type TerminalRedrawScheduler struct {
	mu      sync.Mutex
	pending bool
	missed  bool
	stopped bool
	redraw  func()

	// inflight counts redraw calls that have left the lock and not yet
	// returned; idle is signalled when it reaches zero. Stop waits on it, so
	// that once Stop returns no redraw is running and none will start.
	inflight int
	idle     *sync.Cond
}

func NewTerminalRedrawScheduler(redraw func()) *TerminalRedrawScheduler {
	return &TerminalRedrawScheduler{redraw: redraw}
}

func (s *TerminalRedrawScheduler) Request() {
	if s == nil {
		return
	}

	s.mu.Lock()
	if s.stopped {
		s.mu.Unlock()
		return
	}
	if s.pending {
		// Remember that newer output arrived while the window is open; the
		// timer below turns it into exactly one trailing frame. Dropping
		// these outright is what made a program that paints once and then
		// falls silent invisible: mc writes its whole screen in a handful
		// of reads a millisecond apart, so every chunk after the first
		// landed inside the window and nothing ever woke the renderer
		// again (#249).
		s.missed = true
		s.mu.Unlock()
		return
	}
	s.pending = true
	redraw := s.redraw
	if redraw != nil {
		s.inflight++
	}
	s.mu.Unlock()

	// Keep the first frame of a burst responsive, then suppress further
	// requests until the interval expires. FrameManager.Redraw itself is
	// asynchronous and non-blocking, so this is safe from the PTY reader.
	if redraw != nil {
		redraw()
		s.finished()
	}
	s.arm()
}

// finished records that a redraw call returned and wakes Stop if it was the
// last one.
func (s *TerminalRedrawScheduler) finished() {
	s.mu.Lock()
	s.inflight--
	if s.inflight == 0 && s.idle != nil {
		s.idle.Broadcast()
	}
	s.mu.Unlock()
}

// arm closes the coalescing window after terminalRedrawInterval. If
// requests arrived while it was open, one trailing frame is drawn and a new
// window is opened for whatever arrives during that frame, so a sustained
// stream still costs at most one frame per interval while the last chunk of
// a burst is never left unseen.
func (s *TerminalRedrawScheduler) arm() {
	time.AfterFunc(terminalRedrawInterval, func() {
		s.mu.Lock()
		if s.stopped {
			s.pending = false
			s.missed = false
			s.mu.Unlock()
			return
		}
		if !s.missed {
			s.pending = false
			s.mu.Unlock()
			return
		}
		s.missed = false
		redraw := s.redraw
		if redraw != nil {
			s.inflight++
		}
		s.mu.Unlock()

		if redraw != nil {
			redraw()
			s.finished()
		}
		s.arm()
	})
}

// Stop ends the scheduler and waits for a redraw that is already running. A
// timer callback that had passed the stopped check before Stop took the lock
// would otherwise go on to call redraw after Stop returned; for the panels
// frame that call reads the global vtui.FrameManager, which a test's cleanup
// replaces right after the frame is closed, and the race detector reported it
// as a data race. After Stop returns no redraw is running and none will start.
//
// It must not be called from inside the redraw callback itself.
func (s *TerminalRedrawScheduler) Stop() {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.stopped = true
	s.pending = false
	s.missed = false
	if s.idle == nil {
		s.idle = sync.NewCond(&s.mu)
	}
	for s.inflight > 0 {
		s.idle.Wait()
	}
	s.mu.Unlock()
}
