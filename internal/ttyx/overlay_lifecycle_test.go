package ttyx

import "testing"

func TestOverlayLifecycleWithoutDisplay(t *testing.T) {
	s := &Session{}
	if _, err := s.NewOverlay(); err != ErrNoDisplay {
		t.Fatalf("NewOverlay without a display: got %v, want %v", err, ErrNoDisplay)
	}

	ov := &Overlay{
		s:    s,
		win:  17,
		rect: Rect{X: 10, Y: 20, W: 30, H: 40},
	}

	if got := ov.Window(); got != 17 {
		t.Fatalf("Window: got %d, want 17", got)
	}
	if got := ov.Rect(); got != (Rect{X: 10, Y: 20, W: 30, H: 40}) {
		t.Fatalf("Rect: got %+v", got)
	}
	if ov.PassesInput() {
		t.Fatal("an uninitialised overlay must not claim to pass input")
	}
	if ov.Visible() {
		t.Fatal("an uninitialised overlay must not be visible")
	}
	if err := ov.Place(Rect{X: 1, Y: 2, W: 3, H: 4}); err != ErrNoDisplay {
		t.Fatalf("Place without a display: got %v, want %v", err, ErrNoDisplay)
	}
	if err := ov.Draw([]byte{0, 0, 0, 0}, 1, 1, 4); err != ErrNoDisplay {
		t.Fatalf("Draw without a display: got %v, want %v", err, ErrNoDisplay)
	}
	if ov.SetBounds(nil) {
		t.Fatal("SetBounds without a display must fail")
	}

	ov.Suspend()
	ov.Hide()
	ov.followedParent(5, 7)
	if got := ov.Rect(); got != (Rect{X: 15, Y: 27, W: 30, H: 40}) {
		t.Fatalf("followedParent: got %+v", got)
	}

	ov.Close()
}
