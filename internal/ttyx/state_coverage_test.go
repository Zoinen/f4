package ttyx

import (
	"github.com/jezek/xgb/xproto"
	"testing"
)

func TestFocusEventFilter(t *testing.T) {
	tests := []struct {
		name   string
		mode   byte
		detail byte
		want   bool
	}{
		{name: "ordinary focus change", mode: 0, detail: 0, want: true},
		{name: "focus moved to child", mode: 0, detail: xproto.NotifyDetailInferior, want: false},
		{name: "grab transition", mode: xproto.NotifyModeGrab, detail: 0, want: false},
		{name: "ungrab transition", mode: xproto.NotifyModeUngrab, detail: 0, want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := focusEventIsReal(test.mode, test.detail); got != test.want {
				t.Fatalf("focusEventIsReal(%d, %d) = %t, want %t", test.mode, test.detail, got, test.want)
			}
		})
	}
}

func TestSessionFocusTransitionsNotifyOnce(t *testing.T) {
	s := &Session{alive: true, changed: make(chan struct{}, 1)}
	if s.Focused() {
		t.Fatal("an empty session must not start focused")
	}

	s.setFocused(true)
	if !s.Focused() {
		t.Fatal("a focus transition to true must be visible to callers")
	}
	select {
	case <-s.Changed():
	default:
		t.Fatal("a focus transition must notify listeners")
	}

	s.setFocused(true)
	select {
	case <-s.Changed():
		t.Fatal("an unchanged focus state must not notify listeners")
	default:
	}

	s.setFocused(false)
	if s.Focused() {
		t.Fatal("a focus transition to false must be visible to callers")
	}
	select {
	case <-s.Changed():
	default:
		t.Fatal("losing focus must notify listeners")
	}
}

func TestSessionOperationsWithoutDisplay(t *testing.T) {
	s := &Session{}
	if _, err := s.Geometry(); err != ErrNoDisplay {
		t.Fatalf("Geometry without a display: got %v, want %v", err, ErrNoDisplay)
	}
	if err := s.GrabKeys(nil); err != ErrNoDisplay {
		t.Fatalf("GrabKeys without a display: got %v, want %v", err, ErrNoDisplay)
	}
	if s.focusedNow() {
		t.Fatal("focusedNow without a display must be false")
	}
	if _, ok := s.InnerWindow(Rect{W: 10, H: 10}, 1); ok {
		t.Fatal("InnerWindow without a display must fail")
	}

	s.UngrabKeys()
	s.Close()
}
