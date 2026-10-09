package ttyx

// Two more branches left untested by every other file in this package: onKey
// never had its translated-event path exercised at all -- every existing
// test drives it only through the "no translator configured" early return --
// and SetBounds's own rectangle validation was only ever reached through the
// "not shaped" guard, never through a shaped overlay whose rectangles are
// themselves rejected. Both cases here are chosen, like coverage_no_display_
// test.go, so that the code returns before it would ever touch a real
// connection.

import (
	"math"
	"testing"

	"github.com/jezek/xgb"
	"github.com/jezek/xgb/xproto"
	"github.com/unxed/vtinput"
	"github.com/unxed/winkeys"
)

// fakeTranslator stands in for the real keytrans backend, which needs a live
// X connection to build at all. onKey only ever calls TranslateX11 on it; the
// other methods exist only to satisfy keytrans.Translator.
type fakeTranslator struct {
	next winkeys.InputEvent
}

func (f fakeTranslator) Name() string { return "fake" }

func (f fakeTranslator) TranslateX11(detail uint8, state uint16, isDown bool) winkeys.InputEvent {
	return f.next
}

func (f fakeTranslator) TranslateWayland(keycode uint32, isDown bool) winkeys.InputEvent {
	return winkeys.InputEvent{}
}

func (f fakeTranslator) UpdateWaylandModifiers(modsDepressed, modsLatched, modsLocked, group uint32) {
}

func (f fakeTranslator) Close() {}

// A dead key or a bare modifier translates to a zero VirtualKeyCode and a
// zero Char; onKey must swallow that rather than forward a phantom keypress.
func TestOnKeyDropsAnEmptyTranslation(t *testing.T) {
	events := make(chan *vtinput.InputEvent, 1)
	s := &Session{keys: &keyState{events: events, tr: fakeTranslator{}}}

	s.onKey(38, 0, true)

	select {
	case ev := <-events:
		t.Fatalf("an empty translation must not reach the event channel, got %+v", ev)
	default:
	}
}

// A real translation must reach the channel with the translator's own key
// fields copied through and the modifier state computed from the X state
// mask by modsFromState -- that mask, not anything the translator returns, is
// what the rest of f4 reads for Ctrl/Alt/Shift.
func TestOnKeyDeliversATranslatedEvent(t *testing.T) {
	events := make(chan *vtinput.InputEvent, 1)
	tr := fakeTranslator{next: winkeys.InputEvent{
		VirtualKeyCode: 0x41,
		Char:           'a',
		KeyDown:        true,
		InputSource:    "x11",
	}}
	s := &Session{keys: &keyState{events: events, tr: tr}}

	s.onKey(38, xproto.ModMaskShift|xproto.ModMaskControl, true)

	select {
	case ev := <-events:
		if ev.Type != vtinput.KeyEventType {
			t.Fatalf("Type = %v, want KeyEventType", ev.Type)
		}
		if !ev.KeyDown || ev.VirtualKeyCode != 0x41 || ev.Char != 'a' || ev.InputSource != "x11" {
			t.Fatalf("onKey did not copy the translation through: %+v", ev)
		}
		if want := vtinput.ShiftPressed | vtinput.LeftCtrlPressed; ev.ControlKeyState != want {
			t.Fatalf("ControlKeyState = %v, want %v", ev.ControlKeyState, want)
		}
	default:
		t.Fatal("a real translation must reach the event channel")
	}
}

// The event loop must never block on a reader that fell behind: a full
// channel is dropped and counted, not delivered nor blocked on.
func TestOnKeyCountsADropWhenTheChannelIsFull(t *testing.T) {
	events := make(chan *vtinput.InputEvent, 1)
	events <- &vtinput.InputEvent{} // fill the only slot.
	tr := fakeTranslator{next: winkeys.InputEvent{VirtualKeyCode: 0x41}}
	s := &Session{keys: &keyState{events: events, tr: tr}}

	s.onKey(38, 0, true)

	if got := s.Dropped(); got != 1 {
		t.Fatalf("Dropped() = %d, want 1", got)
	}
}

// SetBounds rejects a rectangle outside the X11 wire range before it ever
// builds a request, so this must be refused even on a shaped overlay whose
// connection would otherwise block on any real request.
func TestOverlaySetBoundsRejectsOutOfRangeRectangle(t *testing.T) {
	s := &Session{conn: &xgb.Conn{}}
	o := &Overlay{s: s, shaped: true}

	if o.SetBounds([]Rect{{X: math.MaxInt16 + 1, Y: 0, W: 1, H: 1}}) {
		t.Fatal("a rectangle outside the X11 wire range must be rejected")
	}
}

// A set made entirely of empty rectangles is not the same request as an empty
// set: MaskChecked(None) means "restore the whole window", so SetBounds must
// refuse instead of silently doing that when every rectangle was filtered out.
func TestOverlaySetBoundsRejectsAllDegenerateRectangles(t *testing.T) {
	s := &Session{conn: &xgb.Conn{}}
	o := &Overlay{s: s, shaped: true}

	if o.SetBounds([]Rect{{W: 0, H: 5}, {W: 5, H: 0}}) {
		t.Fatal("a set of only degenerate rectangles must be rejected")
	}
}
