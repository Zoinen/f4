package wincon

// The overlay's state machine, minus the system calls.
//
// Every window call on a child of the console window is a call into a window
// owned by another thread, whose input queue is attached to conhost's. Making
// one of those from the thread that is drawing a frame is what issue #805 is:
// SetWindowPos, ShowWindow and SetWindowRgn are synchronous across threads,
// so the frame ends up waiting on conhost, and conhost is what draws f4's own
// text and delivers its keys.
//
// So the caller does not call them. It records what it wants here, posts one
// thread message, and returns; the pump thread reads this and does the work.
// The split lives in its own file for the same reason geometry.go does: it is
// arithmetic and judgement, it has no system calls in it, and it is therefore
// testable on a machine with no Windows console anywhere near it.

import "sync"

// overlayOps is what the pump thread has to do to make the window agree with
// what was asked for. Everything zero means there is nothing to do.
type overlayOps struct {
	Hide       bool
	SetRegion  bool
	Region     []Rect
	Move       bool
	Rect       Rect
	Invalidate bool
}

// Empty says the wake-up found nothing left to do, which happens whenever two
// changes arrive before the pump thread gets to the first one.
func (o overlayOps) Empty() bool {
	return !o.Hide && !o.SetRegion && !o.Move && !o.Invalidate
}

// overlayState is what was asked for, what is already on the screen, and the
// one flag that keeps the wake-ups from piling up.
type overlayState struct {
	mu sync.Mutex

	// asked for
	wantShow   bool
	wantRect   Rect
	wantRegion []Rect
	regionSet  bool
	pixDirty   bool

	// on the screen; the pump thread's business only
	shown  bool
	placed Rect

	woken  bool
	closed bool
}

// note records that something changed and says whether a wake-up has to be
// posted. At most one is ever outstanding, so a hundred frames in a row cost
// one message rather than a hundred. Called with the lock held.
func (s *overlayState) note() bool {
	if s.closed || s.woken {
		return false
	}
	s.woken = true
	return true
}

// wakeFailed puts the flag back when the message could not be posted, so the
// next change tries again instead of waiting for a wake-up that never came.
func (s *overlayState) wakeFailed() {
	s.mu.Lock()
	s.woken = false
	s.mu.Unlock()
}

// place asks for the window at a rectangle of the console's client area.
func (s *overlayState) place(r Rect) (post, ok bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return false, false
	}
	s.wantShow = true
	s.wantRect = r
	return s.note(), true
}

// hide asks for the window off the screen, keeping the window itself.
func (s *overlayState) hide() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.wantShow && !s.shown {
		return false
	}
	s.wantShow = false
	s.wantRect = Rect{}
	return s.note()
}

// setRegion asks for the parts of the window that are the picture. A nil list
// means the whole of it.
func (s *overlayState) setRegion(rects []Rect) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return false
	}
	s.wantRegion = append([]Rect(nil), rects...)
	s.regionSet = true
	return s.note()
}

// touchPixels says the frame buffer has been replaced and wants repainting.
// It is also what releases a move the pump thread is holding back, so the
// window is shown or resized in the same wake-up that paints it.
func (s *overlayState) touchPixels() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return false
	}
	s.pixDirty = true
	return s.note()
}

// visible reports what was asked for rather than what has been applied: the
// caller uses it to decide whether to redraw, and the pump thread may not have
// caught up yet.
func (s *overlayState) visible() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.wantShow
}

// close marks the overlay finished and says whether this call was the one that
// did it, so the window is destroyed exactly once.
func (s *overlayState) close() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return false
	}
	s.closed = true
	return true
}

// take is the pump thread's half: it clears the wake-up flag and returns the
// work, having already recorded it as done. Recording it here rather than
// after the calls is deliberate — a change that arrives while the calls are
// running sets the flag again and gets its own wake-up.
//
// Second rule of this file, after the one about waiting: the window is never
// shown or moved except in the wake-up that also repaints it. See the comment
// where the move is held back.
func (s *overlayState) take() overlayOps {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.woken = false

	var ops overlayOps
	if s.closed {
		s.wantShow = false
		s.wantRect = Rect{}
		s.wantRegion = nil
		s.regionSet = false
		s.pixDirty = false
		return ops
	}
	if !s.wantShow {
		if s.shown {
			ops.Hide = true
			s.shown = false
			s.placed = Rect{}
		}
		s.pixDirty = false
		return ops
	}
	if s.regionSet {
		ops.SetRegion = true
		ops.Region = append([]Rect(nil), s.wantRegion...)
		s.regionSet = false
	}
	if !s.shown || s.wantRect != s.placed {
		if !s.pixDirty {
			// The pixels for this rectangle have not arrived yet.
			// Showing the window now would put an *unpainted* window
			// over the console: WM_ERASEBKGND is refused, so what is
			// on the screen until the next paint is whatever the
			// window last held — black on the first picture, which is
			// the other half of what issue #805 looked like. Resizing
			// it early is the same fault with the old picture still
			// in it, because paint blits at the frame buffer's size
			// and leaves the rest of a larger window alone.
			//
			// A frame always draws after it places (see
			// consoleImageOverlay.RenderExternal), so that Draw is at
			// most one wake-up away. Wait for it and do both at once.
			return ops
		}
		ops.Move = true
		ops.Rect = s.wantRect
		// Moving or showing the window invalidates it anyway, and the
		// frame buffer is the only thing it knows how to paint.
		ops.Invalidate = true
		s.shown = true
		s.placed = s.wantRect
	}
	if s.pixDirty {
		ops.Invalidate = true
		s.pixDirty = false
	}
	return ops
}
