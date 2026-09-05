package wincon

import "testing"

// Issue #805. The overlay is a child of a window in another process, so every
// call that moves or reshapes it is a synchronous call into another thread
// whose queue is attached to conhost's. The state machine exists so that the
// thread drawing a frame makes none of them: it writes here and posts one
// message. These tests are the contract that split relies on.

// show does what a frame does: it asks for the rectangle and hands over the
// pixels for it. The pump thread holds a move back until both have arrived,
// so a test that only places never gets a window on the screen.
func show(s *overlayState, r Rect) overlayOps {
	s.place(r)
	s.touchPixels()
	return s.take()
}

// One wake-up at a time. A frame is composed out of several calls — place,
// region, pixels — and a hundred frames in a row must not leave a hundred
// messages on a queue nobody has got to yet.
func TestOneWakeUpUntilItIsTaken(t *testing.T) {
	var s overlayState

	post, ok := s.place(Rect{W: 100, H: 50})
	if !ok || !post {
		t.Fatalf("the first change has to be posted: post=%v ok=%v", post, ok)
	}
	if s.setRegion([]Rect{{W: 10, H: 10}}) {
		t.Error("a second change while one is outstanding needs no message")
	}
	if s.touchPixels() {
		t.Error("a third change while one is outstanding needs no message")
	}

	ops := s.take()
	if !ops.SetRegion || !ops.Move || !ops.Invalidate {
		t.Fatalf("one wake-up has to carry all of it: %+v", ops)
	}
	if !s.touchPixels() {
		t.Error("a change after the take has to be posted again")
	}
}

// A change that arrives while the pump thread is inside the system calls is
// recorded as outstanding again, because take() has already cleared the flag.
func TestAChangeDuringApplyGetsItsOwnWakeUp(t *testing.T) {
	var s overlayState
	show(&s, Rect{W: 8, H: 8})
	if post, _ := s.place(Rect{W: 9, H: 9}); !post {
		t.Fatal("the move that arrived during apply would be lost")
	}
	s.touchPixels()
	if ops := s.take(); !ops.Move || ops.Rect.W != 9 {
		t.Errorf("got %+v, want the second rectangle", ops)
	}
}

// Asking for what is already on the screen is no work at all. This is what
// keeps a still picture from moving its window sixty times a second.
func TestPlacingTwiceInTheSameSpotDoesNothing(t *testing.T) {
	var s overlayState
	show(&s, Rect{X: 4, Y: 4, W: 20, H: 10})
	s.place(Rect{X: 4, Y: 4, W: 20, H: 10})
	if ops := s.take(); !ops.Empty() {
		t.Errorf("got %+v, want nothing to do", ops)
	}
}

// Hiding beats everything else in the same wake-up: there is no point moving a
// window that is about to go, and the pixels it would have painted are stale.
func TestHidingIsTheWholeOfTheWork(t *testing.T) {
	var s overlayState
	show(&s, Rect{W: 30, H: 30})

	if !s.hide() {
		t.Fatal("hiding a shown window has to be posted")
	}
	s.touchPixels()
	ops := s.take()
	if !ops.Hide || ops.Move || ops.Invalidate {
		t.Fatalf("got %+v, want a hide and nothing else", ops)
	}
	if s.visible() {
		t.Error("the overlay says it is still on the screen")
	}
	if s.hide() {
		t.Error("hiding what is already hidden needs no message")
	}
}

// Showing it again after a hide has to move it, even to the same rectangle:
// the window is hidden, so the position it remembers is not on the screen.
func TestShowingAgainMovesEvenToTheSameSpot(t *testing.T) {
	var s overlayState
	show(&s, Rect{X: 1, Y: 2, W: 3, H: 4})
	s.hide()
	s.take()

	if ops := show(&s, Rect{X: 1, Y: 2, W: 3, H: 4}); !ops.Move || !ops.Invalidate {
		t.Errorf("got %+v, want the window put back", ops)
	}
}

// A nil region means the whole window, and it has to reach the pump thread as
// such rather than being mistaken for no request at all.
func TestClearingTheRegionIsARequest(t *testing.T) {
	var s overlayState
	s.place(Rect{W: 40, H: 40})
	s.setRegion([]Rect{{W: 10, H: 10}})
	s.take()

	if !s.setRegion(nil) {
		t.Fatal("clearing the region has to be posted")
	}
	ops := s.take()
	if !ops.SetRegion || len(ops.Region) != 0 {
		t.Errorf("got %+v, want an empty region request", ops)
	}
}

// The region handed to the pump thread is a copy: the caller reuses its slice
// for the next frame, and a grid of thumbnails would otherwise be reshaped to
// whatever the next frame happened to be.
func TestTheRegionIsCopied(t *testing.T) {
	var s overlayState
	s.place(Rect{W: 40, H: 40})
	rects := []Rect{{W: 10, H: 10}}
	s.setRegion(rects)
	rects[0] = Rect{W: 999, H: 999}
	if ops := s.take(); ops.Region[0].W != 10 {
		t.Errorf("got %+v, want the rectangle as it was when it was asked for", ops.Region)
	}
}

// Once it is closed nothing is recorded and nothing is posted, so no message
// goes to a window that is on its way out.
func TestAClosedOverlayRecordsNothing(t *testing.T) {
	var s overlayState
	if !s.close() {
		t.Fatal("the first close is the one that counts")
	}
	if s.close() {
		t.Error("the second close must not act twice")
	}
	if post, ok := s.place(Rect{W: 5, H: 5}); ok || post {
		t.Errorf("a closed overlay took a placement: post=%v ok=%v", post, ok)
	}
	if s.setRegion(nil) || s.touchPixels() {
		t.Error("a closed overlay posted a wake-up")
	}
}

// Closing also discards a wake-up that was already queued. The pump may see
// that stale message before the quit message, but it must not touch a window
// that Close has already declared dead.
func TestAClosedOverlayDropsQueuedWork(t *testing.T) {
	var s overlayState
	s.place(Rect{W: 5, H: 5})
	s.touchPixels()
	if !s.close() {
		t.Fatal("the first close is the one that counts")
	}
	if ops := s.take(); !ops.Empty() {
		t.Errorf("got %+v, want no work after close", ops)
	}
}

// A wake-up that could not be posted leaves the flag down, so the next change
// tries again instead of waiting forever for a message that was never sent.
func TestAFailedWakeUpIsTriedAgain(t *testing.T) {
	var s overlayState
	if post, _ := s.place(Rect{W: 5, H: 5}); !post {
		t.Fatal("the first change has to be posted")
	}
	s.wakeFailed()
	if !s.touchPixels() {
		t.Error("the next change has to post again")
	}
}

// The other half of issue #805: the window must not be shown before there is
// anything to paint into it. WM_ERASEBKGND is refused, so a window shown early
// holds whatever it held before — black, the first time — for as long as it
// takes to scale a photograph, which is exactly the black rectangle that was
// reported.
func TestTheWindowIsNotShownBeforeItHasPixels(t *testing.T) {
	var s overlayState

	s.place(Rect{W: 640, H: 480})
	if ops := s.take(); !ops.Empty() {
		t.Fatalf("got %+v, want nothing until the pixels are here", ops)
	}
	if !s.touchPixels() {
		t.Fatal("the pixels have to post their own wake-up")
	}
	ops := s.take()
	if !ops.Move || !ops.Invalidate || ops.Rect.W != 640 {
		t.Fatalf("got %+v, want the window shown and painted at once", ops)
	}
}

// The same for a resize. paint blits the frame buffer at its own size and
// leaves the rest of the window alone, so a window made larger before its new
// pixels arrive shows the old picture with a black margin around it.
func TestAResizeWaitsForItsPixelsToo(t *testing.T) {
	var s overlayState
	show(&s, Rect{W: 100, H: 100})

	s.place(Rect{W: 200, H: 200})
	if ops := s.take(); ops.Move {
		t.Fatalf("got %+v, want the resize held back", ops)
	}
	s.touchPixels()
	if ops := s.take(); !ops.Move || ops.Rect.W != 200 {
		t.Errorf("got %+v, want the resize once the pixels are here", ops)
	}
}

// Nothing else is held back with it. The region is the shape of the window and
// costs nothing on a window that is not on the screen yet, and repainting a
// window that is already in the right place never needed a wait.
func TestOnlyTheMoveWaits(t *testing.T) {
	var s overlayState
	s.place(Rect{W: 40, H: 40})
	s.setRegion([]Rect{{W: 10, H: 10}})
	if ops := s.take(); !ops.SetRegion || ops.Move {
		t.Fatalf("got %+v, want the region and no move", ops)
	}

	s.touchPixels()
	s.take()
	s.touchPixels()
	if ops := s.take(); !ops.Invalidate || ops.Move {
		t.Errorf("got %+v, want a repaint where it stands", ops)
	}
}
