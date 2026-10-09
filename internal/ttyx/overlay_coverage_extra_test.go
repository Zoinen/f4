package ttyx

import (
	"errors"
	"testing"
)

func TestNewOverlayWithoutDisplay(t *testing.T) {
	var s Session
	if _, err := s.NewOverlay(); !errors.Is(err, ErrNoDisplay) {
		t.Fatalf("NewOverlay error = %v, want ErrNoDisplay", err)
	}
}

func TestOverlayPlaceWithoutDisplay(t *testing.T) {
	var s Session
	o := &Overlay{s: &s}
	if err := o.Place(Rect{X: 1, Y: 2, W: 3, H: 4}); !errors.Is(err, ErrNoDisplay) {
		t.Fatalf("Place error = %v, want ErrNoDisplay", err)
	}
}

func TestOverlaySetBoundsWithoutDisplay(t *testing.T) {
	var s Session
	o := &Overlay{s: &s}
	if o.SetBounds([]Rect{{W: 1, H: 1}}) {
		t.Fatal("SetBounds must fail without a display")
	}
}

func TestOverlayDrawWithoutDisplay(t *testing.T) {
	var s Session
	o := &Overlay{s: &s}
	if err := o.Draw(nil, 1, 1, 4); !errors.Is(err, ErrNoDisplay) {
		t.Fatalf("Draw error = %v, want ErrNoDisplay", err)
	}
}

func TestOverlaySuspendWithoutDisplay(t *testing.T) {
	var s Session
	o := &Overlay{s: &s, Mapped: true}
	o.Suspend()
	if !o.Mapped {
		t.Fatal("Suspend must not change mapping without a display")
	}
}

func TestOverlayFollowedParentUpdatesPlacedRectangle(t *testing.T) {
	var s Session
	o := &Overlay{s: &s, rect: Rect{X: 10, Y: 20, W: 30, H: 40}}
	o.followedParent(5, -7)
	if got := o.Rect(); got != (Rect{X: 15, Y: 13, W: 30, H: 40}) {
		t.Fatalf("Rect after parent move = %+v", got)
	}
}

func TestOverlayFollowedParentLeavesEmptyRectangleEmpty(t *testing.T) {
	var s Session
	o := &Overlay{s: &s}
	o.followedParent(5, 7)
	if got := o.Rect(); got != (Rect{}) {
		t.Fatalf("empty Rect after parent move = %+v", got)
	}
}

func TestOverlayHideWithoutDisplayClearsWanted(t *testing.T) {
	var s Session
	o := &Overlay{s: &s, wanted: true}
	o.Hide()
	if o.wanted {
		t.Fatal("Hide must clear wanted state")
	}
}

func TestOverlayReportsDefaultState(t *testing.T) {
	var s Session
	o := &Overlay{s: &s, win: 42}
	if o.PassesInput() {
		t.Fatal("an unshaped overlay must not pass input")
	}
	if o.Visible() {
		t.Fatal("an unmapped overlay must not be visible")
	}
	if o.Window() != 42 {
		t.Fatalf("Window = %d, want 42", o.Window())
	}
}

func TestOverlayCloseWithoutDisplayUnregisters(t *testing.T) {
	var s Session
	o := &Overlay{s: &s}
	s.overlays = []*Overlay{o}
	o.Close()
	if len(s.overlays) != 0 {
		t.Fatalf("overlays after Close = %d, want 0", len(s.overlays))
	}
}
